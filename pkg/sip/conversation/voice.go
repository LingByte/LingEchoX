// Package conversation wires SIP CallSession media to ASR → LLM → TTS using env-driven credentials.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/llm"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/recognizer"
	"github.com/LinByte/VoiceServer/pkg/scriptlisten"
	sipdtmf "github.com/LinByte/VoiceServer/pkg/sip/dtmf"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	sipvad "github.com/LinByte/VoiceServer/pkg/sip/vad"
	"github.com/LinByte/VoiceServer/pkg/synthesizer"
	"github.com/LinByte/VoiceServer/pkg/utils"
	sipasr "github.com/LinByte/VoiceServer/pkg/voice/asr"
	siprecorder "github.com/LinByte/VoiceServer/pkg/voice/recorder"
	siptts "github.com/LinByte/VoiceServer/pkg/voice/tts"
	"go.uber.org/zap"
)

var (
	sipSystemPromptMu          sync.Mutex
	sipSystemPromptByCallID    = map[string]string{}
	sipTransferPendingMu       sync.Mutex
	sipTransferPendingByCallID = map[string]bool{}
)

func popSIPCallSystemPrompt(callID string) string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return ""
	}
	sipSystemPromptMu.Lock()
	defer sipSystemPromptMu.Unlock()
	v := strings.TrimSpace(sipSystemPromptByCallID[callID])
	delete(sipSystemPromptByCallID, callID)
	return v
}

func markSIPTransferPending(callID string) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	sipTransferPendingMu.Lock()
	sipTransferPendingByCallID[callID] = true
	sipTransferPendingMu.Unlock()
}

func consumeSIPTransferPending(callID string) bool {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	sipTransferPendingMu.Lock()
	defer sipTransferPendingMu.Unlock()
	v := sipTransferPendingByCallID[callID]
	delete(sipTransferPendingByCallID, callID)
	return v
}

// VoiceEnv holds SIP voice pipeline settings read with utils.GetEnv (see SoulNexus .env).
type VoiceEnv struct {
	LLMProvider string
	LLMBaseURL  string
	LLMAppID    string
	LLMAPIKey   string
	LLMModel    string

	ASRAppID     string
	ASRSecretID  string
	ASRSecretKey string
	ASRModelType string

	TTSAppID      string
	TTSSecretID   string
	TTSSecretKey  string
	TTSVoiceType  int64
	TTSSpeed      int64
	TTSSampleRate int
}

// sipVoicePCMBridgeRate is the negotiated PCM rate between RTP decode and encode (must match MediaSession).
func sipVoicePCMBridgeRate(cs *sipSession.CallSession) int {
	if cs == nil {
		return 16000
	}
	sr := cs.PCMSampleRate()
	if sr <= 0 {
		return 16000
	}
	return sr
}

// sipVoiceTTSCloudSampleRate chooses QCloud output rate; when TTS_SAMPLE_RATE is unset, match the SIP PCM bridge.
func sipVoiceTTSCloudSampleRate(env VoiceEnv, pcmBridgeSR int) int {
	sr := env.TTSSampleRate
	if sr > 0 {
		return sr
	}
	if pcmBridgeSR > 0 {
		return pcmBridgeSR
	}
	return 16000
}

func VoiceEnvFromProcess() VoiceEnv {
	voiceType, _ := strconv.ParseInt(utils.GetEnv("TTS_VOICE_TYPE"), 10, 64)
	ttsSpeed, _ := strconv.ParseInt(utils.GetEnv("TTS_SPEED"), 10, 64)
	// TTS_SAMPLE_RATE: when unset / non-positive we leave it at 0 so that
	// sipVoiceTTSCloudSampleRate() falls through to the negotiated SIP PCM
	// bridge rate. For G.711 (PCMU/PCMA) this means TTS is requested at
	// 8 kHz directly — NO resample, NO interpolator chunk-boundary
	// artifacts (the source of the high-frequency "滋滋" hiss). Previously
	// this defaulted to 16000 which forced an unnecessary 16k→8k
	// downsample on every G.711 call and caused the audible hiss.
	sr, _ := strconv.Atoi(utils.GetEnv("TTS_SAMPLE_RATE"))
	if sr < 0 {
		sr = 0
	}
	provider := utils.GetEnv("LLM_PROVIDER")
	appID := utils.GetEnv("LLM_APP_ID")
	if appID == "" {
		appID = utils.GetEnv("ALIBABA_AI_APP_ID")
	}
	apiKey := utils.GetEnv("LLM_APIKEY")
	if apiKey == "" && strings.EqualFold(provider, "alibaba") {
		apiKey = utils.GetEnv("ALIBABA_AI_API_KEY")
	}
	return VoiceEnv{
		LLMProvider: provider,
		LLMBaseURL:  utils.GetEnv("LLM_BASEURL"),
		LLMAppID:    appID,
		LLMAPIKey:   apiKey,
		LLMModel:    utils.GetEnv("LLM_MODEL"),

		ASRAppID:     utils.GetEnv("ASR_APPID"),
		ASRSecretID:  utils.GetEnv("ASR_SECRET_ID"),
		ASRSecretKey: utils.GetEnv("ASR_SECRET_KEY"),
		ASRModelType: utils.GetEnv("ASR_MODEL_TYPE"),

		TTSAppID:      utils.GetEnv("TTS_APPID"),
		TTSSecretID:   utils.GetEnv("TTS_SECRET_ID"),
		TTSSecretKey:  utils.GetEnv("TTS_SECRET_KEY"),
		TTSVoiceType:  voiceType,
		TTSSpeed:      ttsSpeed,
		TTSSampleRate: sr,
	}
}

// llmAPIURLForProvider returns the apiUrl argument for llm.NewLLMProvider (OpenAI-compatible base URL or Alibaba App ID).
func llmAPIURLForProvider(env VoiceEnv) string {
	if strings.EqualFold(env.LLMProvider, "alibaba") {
		return env.LLMAppID
	}
	return env.LLMBaseURL
}

func (e VoiceEnv) readyForVoice() bool {
	llmReady := e.LLMAPIKey != "" && e.LLMBaseURL != ""
	if strings.EqualFold(e.LLMProvider, "alibaba") {
		// Alibaba provider in pkg/llm consumes AppID in apiUrl slot.
		llmReady = e.LLMAPIKey != "" && e.LLMAppID != ""
	}
	return e.ASRAppID != "" && e.ASRSecretID != "" && e.ASRSecretKey != "" &&
		llmReady &&
		e.TTSAppID != "" && e.TTSSecretID != "" && e.TTSSecretKey != ""
}

// AttachVoicePipeline registers a MediaSession processor that feeds decoded PCM into ASR,
// then on final transcripts runs the LLM and streams TTS back to the RTP output.
// Call once per call from the SIP ACK path before MediaSession.Serve() starts (before StartOnACK).
func AttachVoicePipeline(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	env := VoiceEnvFromProcess()
	if !env.readyForVoice() {
		if lg != nil {
			lg.Info("sip voice pipeline skipped (missing ASR/LLM/TTS env)")
		}
		return nil
	}
	if lg == nil {
		if logger.Lg != nil {
			lg = logger.Lg
		} else {
			lg, _ = zap.NewDevelopment()
		}
	}

	return cs.AttachVoiceConversation(func() error {
		return attachVoiceInner(ctx, cs, env, lg)
	})
}

func sipHangupPhrasesFromEnv() []string {
	s := utils.GetEnv("SIP_AI_HANGUP_PHRASES")
	if s == "" {
		return []string{"再见", "拜拜", "挂断", "先挂了", "挂了啊"}
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"再见", "拜拜"}
	}
	return out
}

// sipASRTriggerPartialEnabled controls whether non-final ASR hypotheses can trigger LLM.
// Default false to avoid premature responses ("抢话") that sound like stutter/choppy dialog.
func sipASRTriggerPartialEnabled() bool {
	v := strings.ToLower(utils.GetEnv("SIP_ASR_TRIGGER_PARTIAL"))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// sipASRPartialTimeoutMs is a fallback when providers keep emitting partial
// hypotheses but delay/omit final marks on noisy phone lines.
// <=0 disables timeout trigger.
func sipASRPartialTimeoutMs() int {
	s := utils.GetEnv("SIP_ASR_PARTIAL_TIMEOUT_MS")
	if s == "" {
		return 1200
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 1200
	}
	if n <= 0 {
		return 0
	}
	if n < 300 {
		return 300
	}
	return n
}

// welcomeWaitFirstRTPMs is how long we wait for the first inbound RTP datagram before playing the welcome clip.
// Default 2000 ms gives the RTP layer time to learn symmetric NAT (remote send target updates from first packet).
// Set SIP_WELCOME_WAIT_FIRST_RTP_MS=0 to skip waiting (e.g. same-LAN tests).
func welcomeWaitFirstRTPMs() int {
	s := utils.GetEnv("SIP_WELCOME_WAIT_FIRST_RTP_MS")
	if s == "" {
		return 2000
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 2000
	}
	if v < 0 {
		return 0
	}
	return v
}

func waitFirstRTPBeforeWelcome(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger, waitMs int) {
	if waitMs <= 0 || cs == nil {
		return
	}
	rtpSess := cs.RTPSession()
	if rtpSess == nil {
		return
	}
	t := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
	defer t.Stop()
	select {
	case <-rtpSess.FirstPacket():
		if lg != nil {
			lg.Info("sip voice: inbound RTP before welcome (symmetric NAT can update send target)",
				zap.String("call_id", cs.CallID))
		}
	case <-t.C:
		if lg != nil {
			lg.Warn("sip voice: no inbound RTP before welcome wait — check client/NAT/firewall or set SIP_WELCOME_WAIT_FIRST_RTP_MS=0 for lab",
				zap.String("call_id", cs.CallID),
				zap.Int("wait_ms", waitMs),
			)
		}
	case <-ctx.Done():
		return
	}
}

func attachVoiceInner(ctx context.Context, cs *sipSession.CallSession, env VoiceEnv, lg *zap.Logger) error {
	ms := cs.MediaSession()
	if ms == nil {
		return fmt.Errorf("sip conversation: nil media session")
	}
	asrOpt := recognizer.NewQcloudASROption(env.ASRAppID, env.ASRSecretID, env.ASRSecretKey)
	if env.ASRModelType != "" {
		asrOpt.ModelType = env.ASRModelType
	}
	asrSvc := recognizer.NewQcloudASR(asrOpt)

	asrOutRate := 16000
	if strings.Contains(strings.ToLower(asrOpt.ModelType), "8k") {
		asrOutRate = 8000
	}
	asrInRate := cs.PCMSampleRate()
	if asrInRate <= 0 {
		asrInRate = 16000
	}
	pcmBridgeSR := sipVoicePCMBridgeRate(cs)
	ttsCloudSR := sipVoiceTTSCloudSampleRate(env, pcmBridgeSR)

	pipe, err := sipasr.New(sipasr.Options{
		ASR:        asrSvc,
		SampleRate: asrOutRate,
		Channels:   1,
		Logger:     lg,
	})
	if err != nil {
		return fmt.Errorf("sip conversation: asr pipeline: %w", err)
	}

	llmModel := env.LLMModel
	if llmModel == "" {
		llmModel = "qwen-plus"
	}
	systemPrompt := popSIPCallSystemPrompt(cs.CallID)
	llmEndpointOrAppID := llmAPIURLForProvider(env)
	llmProvider, err := llm.NewLLMProvider(ctx, env.LLMProvider, env.LLMAPIKey, llmEndpointOrAppID, systemPrompt)
	if err != nil {
		return fmt.Errorf("sip conversation: llm provider init: %w", err)
	}
	registerSIPTransferTool(llmProvider, cs.CallID, lg)
	lg.Info("sip voice pipeline config",
		zap.String("llm_model", llmModel),
		zap.String("llm_provider", env.LLMProvider),
		zap.String("llm_endpoint_or_app_id", llmEndpointOrAppID),
		zap.String("asr_model", asrOpt.ModelType),
		zap.Int64("tts_speed", env.TTSSpeed),
		zap.Int("pcm_bridge_hz", pcmBridgeSR),
		zap.Int("tts_cloud_hz", ttsCloudSR),
		zap.Int("tts_sample_rate_env", env.TTSSampleRate),
	)
	if asrInRate != asrOutRate {
		lg.Info("sip voice: resampling decode PCM for ASR",
			zap.Int("pcm_decode_hz", asrInRate),
			zap.Int("asr_hz", asrOutRate),
			zap.String("asr_model", asrOpt.ModelType),
		)
	}

	// Stereo PCM recorder (replaces SN3 inline recording for new calls).
	// 录音始终启用 —— 通话录音是平台基线能力，不再提供开关。
	// SIP_RECORDER_CHUNK_SECS 调节滚动分片上传周期（0 = 通话结束一次性写）。
	// 存储桶由 pkg/stores 后端自己读取（COS_BUCKET_NAME / S3_BUCKET / …）。
	recCfg := siprecorder.Config{
		Logger: lg,
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SIP_RECORDER_CHUNK_SECS"))); err == nil && secs > 0 {
		recCfg.ChunkInterval = time.Duration(secs) * time.Second
	}
	if cs.EnableRecorder(recCfg) {
		lg.Info("sip voice: stereo PCM recorder enabled",
			zap.String("call_id", cs.CallID),
			zap.Int("sample_rate", pcmBridgeSR),
			zap.Duration("chunk_interval", recCfg.ChunkInterval),
		)
	}

	voiceType := env.TTSVoiceType
	if voiceType == 0 {
		voiceType = 101007 // 知性女声（智娜）
	}
	ttsCfg := synthesizer.NewQcloudTTSConfig(env.TTSAppID, env.TTSSecretID, env.TTSSecretKey, voiceType, "pcm", ttsCloudSR)
	ttsCfg.Speed = env.TTSSpeed
	qcTTS := synthesizer.NewQCloudService(ttsCfg)
	ttsStream := &qcloudTTSStream{svc: qcTTS}
	scriptMode := isSIPScriptMode(cs.CallID)
	if scriptMode {
		lg.Info("sip voice: script mode enabled (suppress built-in welcome and auto TTS reply)",
			zap.String("call_id", cs.CallID))
	}

	var turnMu sync.Mutex
	inFlight := false
	asrState := NewASRStateManager()
	hotwordCorrector := NewSIPHotwordCorrector(lg)
	partialTimeoutMs := sipASRPartialTimeoutMs()
	var partialMu sync.Mutex
	var partialTimer *time.Timer
	var pendingPartial string
	// When partial-timeout already ran LLM+TTS for text T, ASR often sends final with the same T;
	// skipping the second trigger avoids duplicate user turns in LLM history and extra latency.
	var partialTimeoutDoneMu sync.Mutex
	var partialTimeoutDoneText string
	var partialTimeoutDoneAt time.Time
	const partialTimeoutFinalDedupeWindow = 12 * time.Second
	ttsPipe, err := siptts.New(siptts.Config{
		Service:       ttsStream,
		SampleRate:    ttsCloudSR,
		Channels:      1,
		FrameDuration: 20 * time.Millisecond,
		// Match RTP real-time pacing so the far end does not receive whole replies in a few ms bursts.
		PaceRealtime: true,
		SendPCMFrame: func(frame []byte) error {
			if len(frame) == 0 {
				return nil
			}
			pcmOut := frame
			recordedAtBridgeRate := ttsCloudSR == pcmBridgeSR
			if !recordedAtBridgeRate && len(frame) >= 2 {
				if out, err := media.ResamplePCM(frame, ttsCloudSR, pcmBridgeSR); err == nil && len(out) > 0 {
					pcmOut = out
					recordedAtBridgeRate = true
				}
			}
			// Stereo recorder: only capture AI when the buffer is at the
			// bridge rate the recorder was configured with — otherwise we
			// would silently mix in pitch-shifted PCM at the wrong rate
			// and the playback would sound like high-pitched static
			// ("电流音"). Skipping the rare resample-failure path is
			// far better than producing a garbled WAV.
			if recordedAtBridgeRate {
				cs.WriteAIPCM(pcmOut)
			}
			pkt := &media.AudioPacket{
				Payload:       pcmOut,
				IsSynthesized: true,
			}
			ms.SendToOutput("sip-voice-tts", pkt)
			return nil
		},
		Logger: lg,
	})
	if err != nil {
		return fmt.Errorf("sip conversation: tts pipeline: %w", err)
	}

	var ttsPlaying atomic.Bool
	var ttsStartedAtNS atomic.Int64
	var welcomePlaying atomic.Bool
	var welcomeCancelMu sync.Mutex
	var welcomeCancel context.CancelFunc

	// cancelWelcomeIfPlaying 同时取消正在播放的欢迎语并清掉播放状态，
	// 无论 VAD barge-in 是否启用都能保证「welcome 永不与 TTS 同框」。
	// 任何会让 AI 开口（ASR 触发 / 转接 / script TTS）的路径都应当先调它，
	// 避免坐席 / 主叫听到 welcome 还没念完 TTS 已经压上去的"双语重叠"。
	cancelWelcomeIfPlaying := func(reason string) {
		if !welcomePlaying.Load() {
			return
		}
		welcomeCancelMu.Lock()
		cancel := welcomeCancel
		welcomeCancel = nil
		welcomeCancelMu.Unlock()
		if cancel != nil {
			cancel()
		}
		welcomePlaying.Store(false)
		if lg != nil {
			lg.Info("sip voice: welcome interrupted",
				zap.String("call_id", cs.CallID),
				zap.String("reason", reason))
		}
	}
	var vadDet *sipvad.Detector
	if config.GlobalConfig.SIP.SIPVADBargeIn {
		vadDet = sipvad.NewDetector()
		vadDet.SetLogger(lg)
		vadDet.SetThreshold(config.GlobalConfig.SIP.SIPVADThreshold)
		vadDet.SetConsecutiveFrames(config.GlobalConfig.SIP.SIPVADConsecFrames)
		lg.Info("sip voice: RMS VAD barge-in enabled (TTS playback only)",
			zap.Float64("threshold_effective", config.GlobalConfig.SIP.SIPVADThreshold),
			zap.Int("consecutive_frames", config.GlobalConfig.SIP.SIPVADConsecFrames),
		)
	} else {
		lg.Info("sip voice: RMS VAD barge-in disabled (SIP_VAD_BARGE_IN)")
	}

	// Same incremental strategy as pkg/hardware: ASRStateManager extracts new sentences from
	// cumulative QCloud text without restarting the recognizer.
	triggerTurn := func(userText string, asrIsFinal bool, trigger string) {
		userText = strings.TrimSpace(userText)
		if userText == "" {
			return
		}
		// 转人工已发起：从此刻起不再喂 ASR 文本进 LLM/TTS。
		// 否则主叫端正在听 hold 音乐 / 坐席声音时，AI 仍然会继续应答，导致"两边一起说话"。
		// 转接结束（坐席挂断回 AI 极少见，目前不支持回流）后由通话结束 CleanupCallState 统一清理。
		if IsTransferInProgress(cs.CallID) {
			lg.Debug("sip voice asr trigger suppressed (transfer in progress)",
				zap.String("call_id", cs.CallID),
				zap.String("trigger", trigger),
			)
			return
		}
		go func(userText string, asrIsFinal bool, trigger string) {
			if ms == nil || ms.GetContext().Err() != nil {
				return
			}
			// Re-check inside the goroutine: TriggerTransferToAgent may have fired
			// between the outer gate and the actual LLM call (the same LLM turn that
			// just decided "transfer_to_agent" will race us here).
			if IsTransferInProgress(cs.CallID) {
				return
			}
			turnMu.Lock()
			if inFlight {
				turnMu.Unlock()
				return
			}
			inFlight = true
			turnMu.Unlock()

			defer func() {
				turnMu.Lock()
				inFlight = false
				turnMu.Unlock()
			}()

			lg.Info("sip voice asr trigger",
				zap.String("call_id", cs.CallID),
				zap.String("user_text", userText),
				zap.Bool("asr_isFinal", asrIsFinal),
				zap.String("trigger", trigger),
			)

			var reply string
			var err error
			pipelineT0 := time.Now()
			var streamMeta StreamTurnTimings
			if scriptMode {
				// Persist only on ASR final to avoid duplicate sip_calls.turns rows when partial ASR is enabled.
				if asrIsFinal {
					asrProv := "qcloud_asr"
					if env.ASRModelType != "" {
						asrProv = env.ASRModelType
					}
					go RecordDialogTurn(context.Background(), cs.CallID, DialogTurn{
						ASRText: userText, ASRProvider: asrProv, Trigger: trigger,
						PipelineMs: int(time.Since(pipelineT0).Milliseconds()),
					})
				}
			} else {
				// Welcome and TTS must not speak together. Even on
				// SIP_VAD_BARGE_IN=off paths (or before VAD has accumulated
				// enough consecutive frames to fire), once ASR resolves to
				// an LLM turn we are about to send synthesized PCM into the
				// SAME output bus that's playing welcome.wav — cancel it
				// here so the two audio streams never overlap on-wire.
				cancelWelcomeIfPlaying("tts_start")
				ttsPipe.Start(ms.GetContext())
				defer func() {
					ttsPlaying.Store(false)
					ttsStartedAtNS.Store(0)
					ttsPipe.Stop()
				}()
				ttsPlaying.Store(true)
				ttsStartedAtNS.Store(time.Now().UnixNano())
				reply, streamMeta, err = streamLLMToTTS(ms.GetContext(), llmProvider, llmModel, userText, ttsPipe, lg)
			}
			if err != nil {
				lg.Warn("sip voice llm/tts", zap.Error(err))
				return
			}
			if !scriptMode && trigger == "partial-timeout" {
				partialTimeoutDoneMu.Lock()
				partialTimeoutDoneText = userText
				partialTimeoutDoneAt = time.Now()
				partialTimeoutDoneMu.Unlock()
			}
			reply = normalizeTTSText(reply)
			if scriptMode {
				// In script mode, output is controlled by script steps (say/llm_reply). Do not auto-play here.
				return
			}
			transferNow := false
			if ap, ok := llmProvider.(*llm.AlibabaProvider); ok {
				if action := ap.ConsumePendingAction(); action == "transfer_to_agent" {
					transferNow = true
				}
			} else if consumeSIPTransferPending(cs.CallID) {
				transferNow = true
			}
			if transferNow && ms != nil && ms.GetContext().Err() == nil {
				lg.Info("sip voice: transfer after ai tts confirmation",
					zap.String("call_id", cs.CallID),
				)
				TriggerTransferToAgent(context.Background(), cs.CallID, lg)
			}
			if scriptMode {
				lg.Info("sip voice llm reply (script no-autoplay)", zap.Int("reply_chars", len(reply)))
			} else {
				lg.Info("sip voice assistant reply", zap.Int("reply_chars", len(reply)))
			}
			asrProv := "qcloud_asr"
			if env.ASRModelType != "" {
				asrProv = env.ASRModelType
			}
			dt := DialogTurn{
				ASRText: userText, LLMText: reply, ASRProvider: asrProv,
				LLMModel: llmModel, TTSProvider: "qcloud_tts", Trigger: trigger,
				RouteIntent: "",
				LLMFirstMs:  streamMeta.LLMFirstMs, LLMWallMs: streamMeta.LLMWallMs,
				TTSMs: streamMeta.TTSMs, PipelineMs: int(time.Since(pipelineT0).Milliseconds()),
			}
			go RecordDialogTurn(context.Background(), cs.CallID, dt)
		}(userText, asrIsFinal, trigger)
	}

	pipe.SetTextCallback(func(text string, isFinal bool) {
		if ms == nil || ms.GetContext().Err() != nil {
			return
		}
		// 转人工后 ASR 文本一律丢弃：不更新 asrState、不重置 partial timer，
		// 避免转接期间累积的句子在通话结束前突然把一段话喂给 LLM。
		if IsTransferInProgress(cs.CallID) {
			partialMu.Lock()
			pendingPartial = ""
			if partialTimer != nil {
				partialTimer.Stop()
			}
			partialMu.Unlock()
			return
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return
		}
		incremental := asrState.UpdateASRText(trimmed, isFinal)
		if incremental == "" {
			return
		}
		corrected := incremental
		if hotwordCorrector != nil {
			corrected = strings.TrimSpace(hotwordCorrector.Correct(incremental))
			if corrected == "" {
				corrected = incremental
			}
		}
		if corrected != incremental {
			lg.Info("sip voice asr corrected",
				zap.String("call_id", cs.CallID),
				zap.String("raw_text", incremental),
				zap.String("corrected_text", corrected),
			)
		}
		if isFillerOnlyUtterance(incremental) {
			lg.Debug("sip voice asr skip filler-only", zap.String("text", incremental))
			return
		}
		if isFinal {
			partialMu.Lock()
			pendingPartial = ""
			if partialTimer != nil {
				partialTimer.Stop()
			}
			partialMu.Unlock()
			partialTimeoutDoneMu.Lock()
			prevT := partialTimeoutDoneText
			prevAt := partialTimeoutDoneAt
			partialTimeoutDoneMu.Unlock()
			if prevT != "" && corrected == prevT && time.Since(prevAt) < partialTimeoutFinalDedupeWindow {
				lg.Info("sip voice asr skip duplicate final (same text as recent partial-timeout turn)",
					zap.String("call_id", cs.CallID),
					zap.Int("text_runes", len([]rune(corrected))),
				)
				return
			}
			triggerTurn(corrected, true, "final")
			return
		}
		if sipASRTriggerPartialEnabled() {
			triggerTurn(corrected, false, "partial")
			return
		}
		if partialTimeoutMs <= 0 {
			return
		}
		partialMu.Lock()
		pendingPartial = corrected
		if partialTimer == nil {
			partialTimer = time.AfterFunc(time.Duration(partialTimeoutMs)*time.Millisecond, func() {
				partialMu.Lock()
				text := strings.TrimSpace(pendingPartial)
				pendingPartial = ""
				partialMu.Unlock()
				if text == "" {
					return
				}
				triggerTurn(text, false, "partial-timeout")
			})
		} else {
			partialTimer.Reset(time.Duration(partialTimeoutMs) * time.Millisecond)
		}
		partialMu.Unlock()
	})

	pipe.SetErrorCallback(func(err error, fatal bool) {
		lg.Warn("sip voice asr", zap.Error(err), zap.Bool("fatal", fatal))
	})

	go func() {
		<-ms.GetContext().Done()
		partialMu.Lock()
		pendingPartial = ""
		if partialTimer != nil {
			partialTimer.Stop()
		}
		partialMu.Unlock()
	}()

	proc := media.NewPacketProcessor("sip-voice-asr-feed", media.PriorityHigh,
		func(c context.Context, _ *media.MediaSession, packet media.MediaPacket) error {
			ap, ok := packet.(*media.AudioPacket)
			if !ok || ap == nil || len(ap.Payload) == 0 {
				return nil
			}
			if ap.IsSynthesized {
				return nil
			}
			pcm16 := ap.Payload
			// Stereo recorder: capture caller side at bridge rate. We tap
			// before the optional resample-to-ASR-rate step so the
			// recorder always sees frames at the bridge sample rate set
			// by EnableRecorder. Welcome / barge-in branches below may
			// short-circuit ASR processing but recording continues.
			cs.WriteCallerPCM(pcm16)
			if welcomePlaying.Load() {
				// Same RMS VAD path as TTS barge-in (requires SIP_VAD_BARGE_IN enabled).
				if vadDet != nil && vadDet.CheckBargeIn(pcm16, true) {
					cancelWelcomeIfPlaying("vad_user_speech")
				} else {
					return nil
				}
			}
			pcmASR := pcm16
			if asrOutRate != asrInRate {
				out, err := media.ResamplePCM(pcm16, asrInRate, asrOutRate)
				if err != nil {
					lg.Debug("sip voice resample", zap.Error(err))
					return nil
				}
				pcmASR = out
			}
			// RMS VAD on 16 kHz PCM (same as media decode path); only while TTS is playing.
			// Do not return early here: skipping ProcessPCM would drop user audio to ASR for the whole window.
			allowBargeIn := true
			if vadDet != nil && ttsPlaying.Load() {
				if started := ttsStartedAtNS.Load(); started > 0 && time.Since(time.Unix(0, started)) < 700*time.Millisecond {
					// Ignore barge-in only; still feed ASR below.
					allowBargeIn = false
				}
			}
			if allowBargeIn && vadDet != nil && ttsPlaying.Load() && vadDet.CheckBargeIn(pcm16, true) {
				lg.Info("sip voice: RMS barge-in, stopping TTS", zap.String("call_id", cs.CallID))
				ttsPipe.Stop()
				// Stop() cancels the pipeline ctx used by Speak(); re-Start so a later Speak in the same turn
				// is not stuck on a dead context.
				ttsPipe.Start(ms.GetContext())
				ttsPlaying.Store(false)
				ttsStartedAtNS.Store(0)
			}
			err := pipe.ProcessPCM(c, pcmASR)
			if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				return nil
			}
			return err
		})

	ms.RegisterProcessor(proc)

	sipdtmf.AttachProcessor(ms, "sip-dtmf", func(_ context.Context, digit string) {
		lg.Info("sip dtmf", zap.String("digit", digit), zap.String("call_id", cs.CallID))
		if isSIPScriptMode(cs.CallID) {
			scriptlisten.PublishDTMF(cs.CallID, digit)
		}
	})

	// Start RTP read/write before welcome so the first inbound packet can update symmetric RTP
	// (see pkg/sip/rtp/session.go) before we send audio to the SDP-private address.
	cs.StartOnACK()
	if w := welcomeWaitFirstRTPMs(); w > 0 {
		waitFirstRTPBeforeWelcome(ms.GetContext(), cs, lg, w)
	}

	if !scriptMode {
		// Resolve + decode the welcome WAV up front so we can short-
		// circuit (skip goroutine + welcomePlaying flag) when there is
		// no configured source. Two paths feed loadWelcomePCM:
		//   1) TrunkNumber.WelcomeAudioURL (per-DID, set by admin)
		//   2) scripts/welcome.wav (legacy / global default)
		// Both yield decoded PCM at the bridge sample rate; the
		// playback goroutine below is identical regardless of source.
		welcomePCM, src, werr := loadWelcomePCM(ms.GetContext(), cs.CallID, pcmBridgeSR, lg)
		switch {
		case werr != nil:
			// Configured source failed (URL unreachable / decode error).
			// We already logged inside loadWelcomePCM; skip playback
			// rather than silently falling back, so the operator sees
			// the misconfiguration in observability.
			lg.Warn("sip voice: welcome audio load failed, skipping welcome phase",
				zap.String("call_id", cs.CallID),
				zap.String("source", string(src)),
				zap.Error(werr))
		case len(welcomePCM) == 0 || src == welcomeSourceSkip:
			// No URL on trunk AND no local file → legitimate skip.
			// loadWelcomePCM already emitted an Info log.
		default:
			welcomePlaying.Store(true)
			welcomeCtx, cancelWelcome := context.WithCancel(ms.GetContext())
			welcomeCancelMu.Lock()
			welcomeCancel = cancelWelcome
			welcomeCancelMu.Unlock()
			go func() {
				defer welcomePlaying.Store(false)
				defer func() {
					welcomeCancelMu.Lock()
					welcomeCancel = nil
					welcomeCancelMu.Unlock()
				}()
				if err := playWelcomePCM(welcomeCtx, welcomePCM, ms, lg, pcmBridgeSR, cs.WriteAIPCM); err != nil {
					// Context cancellation = legitimate barge-in / TTS-start
					// pre-emption (cancelWelcomeIfPlaying), not an error.
					if errors.Is(err, context.Canceled) {
						return
					}
					lg.Warn("sip voice welcome playback failed",
						zap.String("call_id", cs.CallID),
						zap.String("source", string(src)),
						zap.Error(err))
					return
				}
				lg.Info("sip voice welcome playback finished",
					zap.String("call_id", cs.CallID),
					zap.String("source", string(src)),
					zap.Int("bytes", len(welcomePCM)))
			}()
		}
	}

	lg.Info("sip voice pipeline attached", zap.String("call_id", cs.CallID))
	return nil
}

// resolveWelcomeWavPath returns the cleaned filesystem path that the
// welcome WAV would be loaded from. Resolution order:
//
//  1. SIP_WELCOME_WAV_PATH env (highest, ops override).
//  2. scripts/welcome.wav (project default).
//
// Relative paths are cleaned but NOT made absolute — `LoadWAVAsPCM16Mono`
// honours the process working directory the same way.
func resolveWelcomeWavPath() string {
	path := utils.GetEnv("SIP_WELCOME_WAV_PATH")
	if path == "" {
		path = "scripts/welcome.wav"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Clean(path)
	}
	return path
}

// welcomeWavExists reports whether the welcome WAV exists at path AND is
// a regular file (not a directory). Errors other than IsNotExist surface
// to the caller so the operator can see permission / I-O issues; for
// "skip this phase" semantics callers should treat (false, _) the same.
func welcomeWavExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

// playWelcomeWav resolves the welcome WAV path itself and plays it.
// Retained as a thin shim so any external callers (cmd/sip standalone
// binaries, tests) keep working.
func playWelcomeWav(ctx context.Context, ms *media.MediaSession, lg *zap.Logger, sampleRate int, recordTap func(pcm []byte)) error {
	return playResolvedWelcomeWav(ctx, resolveWelcomeWavPath(), ms, lg, sampleRate, recordTap)
}

// playResolvedWelcomeWav is a backwards-compatible wrapper that loads
// the WAV from a filesystem path then delegates to playWelcomePCM. New
// call sites (AttachVoicePipeline) prefer loadWelcomePCM → playWelcomePCM
// so they can also serve per-DID remote URLs from TrunkNumber.WelcomeAudioURL.
func playResolvedWelcomeWav(ctx context.Context, path string, ms *media.MediaSession, lg *zap.Logger, sampleRate int, recordTap func(pcm []byte)) error {
	pcm, err := LoadWAVAsPCM16Mono(path, sampleRate)
	if err != nil {
		return fmt.Errorf("load welcome wav: %w", err)
	}
	return playWelcomePCM(ctx, pcm, ms, lg, sampleRate, recordTap)
}

// playWelcomePCM pumps already-decoded s16le mono PCM into the media
// session at 20 ms cadence, taking the recorder tap so the stereo
// recording's AI channel includes the welcome utterance. Pure
// playback: no file I/O, no URL fetches. Returns context.Canceled
// when the caller cancels (legitimate barge-in / TTS pre-emption).
func playWelcomePCM(ctx context.Context, pcm []byte, ms *media.MediaSession, lg *zap.Logger, sampleRate int, recordTap func(pcm []byte)) error {
	if ms == nil {
		return fmt.Errorf("media session is nil")
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	bytesPerFrame := sampleRate * 2 * 20 / 1000 // 16-bit mono, 20ms
	if bytesPerFrame <= 0 {
		bytesPerFrame = 640
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	if lg != nil {
		lg.Info("sip voice welcome playback started", zap.Int("bytes", len(pcm)))
	}

	for off := 0; off < len(pcm); off += bytesPerFrame {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		end := off + bytesPerFrame
		if end > len(pcm) {
			end = len(pcm)
		}
		frame := pcm[off:end]
		if len(frame) == 0 {
			continue
		}
		// Stereo recorder: capture welcome playback at bridge rate so
		// the AI track is not silent for the welcome window. Without
		// this tap, listeners of the recorded WAV would hear "TTS
		// starts mid-conversation" with the welcome utterance missing.
		if recordTap != nil {
			recordTap(frame)
		}
		ms.SendToOutput("sip-voice-welcome", &media.AudioPacket{
			Payload:       frame,
			IsSynthesized: true,
		})
	}
	return nil
}

// streamPlainTextToTTS speaks fixed text without calling an LLM (intent / script short-circuit).
func streamPlainTextToTTS(ctx context.Context, text string, ttsPipe *siptts.Pipeline, lg *zap.Logger) (string, StreamTurnTimings, error) {
	var meta StreamTurnTimings
	if ttsPipe == nil {
		return "", meta, fmt.Errorf("nil tts pipe")
	}
	text = normalizeTTSText(strings.TrimSpace(text))
	if text == "" {
		return "", meta, nil
	}
	t0 := time.Now()
	if err := ttsPipe.Speak(text); err != nil {
		meta.TTSMs = int(time.Since(t0).Milliseconds())
		if errors.Is(err, context.Canceled) {
			return text, meta, nil
		}
		return "", meta, err
	}
	// End-of-turn residual drain (intent / script single-shot path).
	_ = ttsPipe.Finalize()
	meta.TTSMs = int(time.Since(t0).Milliseconds())
	return text, meta, nil
}

func streamLLMToTTS(ctx context.Context, llmProvider llm.LLMProvider, model, userText string, ttsPipe *siptts.Pipeline, lg *zap.Logger) (string, StreamTurnTimings, error) {
	var meta StreamTurnTimings
	if llmProvider == nil {
		return "", meta, fmt.Errorf("nil llm provider")
	}
	if ttsPipe == nil {
		return "", meta, fmt.Errorf("nil tts pipe")
	}
	ttsMs := 0
	speak := func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		t0 := time.Now()
		err := ttsPipe.Speak(s)
		ttsMs += int(time.Since(t0).Milliseconds())
		return err
	}
	var full strings.Builder
	var seg strings.Builder
	flush := func(force bool) error {
		s := strings.TrimSpace(seg.String())
		if s == "" {
			return nil
		}
		if !force {
			runes := []rune(s)
			last := runes[len(runes)-1]
			if !strings.ContainsRune("。！？.!?,，；;:", last) && len(runes) < 18 {
				return nil
			}
		}
		seg.Reset()
		s = normalizeTTSText(s)
		if s == "" {
			return nil
		}
		return speak(s)
	}
	// Alibaba App API usually returns one JSON message per turn; non-streaming avoids long
	// silent waits when SSE chunks are sparse on some networks.
	if _, isAlibaba := llmProvider.(*llm.AlibabaProvider); isAlibaba {
		t0 := time.Now()
		reply, err := llmProvider.Query(userText, model)
		meta.LLMWallMs = int(time.Since(t0).Milliseconds())
		meta.LLMFirstMs = meta.LLMWallMs
		if err != nil {
			return "", meta, err
		}
		reply = normalizeTTSText(reply)
		if reply == "" {
			return "", meta, nil
		}
		if err := speak(reply); err != nil {
			meta.TTSMs = ttsMs
			if errors.Is(err, context.Canceled) {
				return reply, meta, nil
			}
			return "", meta, err
		}
		// End-of-turn residual drain (Alibaba single-shot path).
		_ = ttsPipe.Finalize()
		meta.TTSMs = ttsMs
		return strings.TrimSpace(reply), meta, nil
	}
	streamStart := time.Now()
	gotFirst := false
	options := llm.QueryOptions{Model: model, Stream: true}
	reply, err := llmProvider.QueryStream(userText, options, func(piece string, _ bool) error {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			return nil
		}
		if !gotFirst {
			meta.LLMFirstMs = int(time.Since(streamStart).Milliseconds())
			gotFirst = true
		}
		full.WriteString(piece)
		seg.WriteString(piece)
		return flush(false)
	})
	meta.LLMWallMs = int(time.Since(streamStart).Milliseconds())
	if err != nil {
		// fallback to non-streaming so behavior stays stable even if provider stream fails.
		ttsMs = 0
		t0 := time.Now()
		reply, err = llmProvider.Query(userText, model)
		meta.LLMWallMs = int(time.Since(t0).Milliseconds())
		meta.LLMFirstMs = meta.LLMWallMs
		if err != nil {
			return "", meta, err
		}
		reply = normalizeTTSText(reply)
		if reply == "" {
			if lg != nil {
				lg.Warn("sip voice tts skip empty/invalid reply after sanitize")
			}
			return "", meta, nil
		}
		if err := speak(reply); err != nil {
			meta.TTSMs = ttsMs
			if errors.Is(err, context.Canceled) {
				if lg != nil {
					lg.Info("sip voice tts stopped (barge-in or cancel)")
				}
				return reply, meta, nil
			}
			return "", meta, err
		}
		// End-of-turn residual drain (streaming fallback path).
		_ = ttsPipe.Finalize()
		meta.TTSMs = ttsMs
		return strings.TrimSpace(reply), meta, nil
	}
	if strings.TrimSpace(reply) == "" {
		reply = full.String()
	}
	if err := flush(true); err != nil {
		meta.TTSMs = ttsMs
		if errors.Is(err, context.Canceled) {
			return strings.TrimSpace(reply), meta, nil
		}
		return "", meta, err
	}
	// Drain the per-segment residual at end-of-turn so the very last
	// sub-frame of audio still gets emitted (with a single zero-pad
	// cliff at the absolute end, which is far less audible than a
	// cliff after every sentence). Best-effort: a Finalize failure
	// here does not invalidate the spoken reply.
	if err := ttsPipe.Finalize(); err != nil && lg != nil {
		lg.Debug("sip voice tts finalize residual", zap.Error(err))
	}
	meta.TTSMs = ttsMs
	return strings.TrimSpace(reply), meta, nil
}

// SpeakTextOnce sends one synthesized sentence to the current SIP media output.
// It is used by outbound script runtime for deterministic "say" steps.
func SpeakTextOnce(ctx context.Context, cs *sipSession.CallSession, text string, lg *zap.Logger) error {
	if cs == nil || strings.TrimSpace(cs.CallID) == "" {
		return fmt.Errorf("sip conversation: nil call session")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	ms := cs.MediaSession()
	if ms == nil {
		return fmt.Errorf("sip conversation: media session not ready")
	}
	if lg == nil {
		if logger.Lg != nil {
			lg = logger.Lg
		} else {
			lg = zap.NewNop()
		}
	}
	env := VoiceEnvFromProcess()
	if env.TTSAppID == "" || env.TTSSecretID == "" || env.TTSSecretKey == "" {
		return fmt.Errorf("sip conversation: missing TTS credentials")
	}
	pcmBridgeSR := sipVoicePCMBridgeRate(cs)
	ttsCloudSR := sipVoiceTTSCloudSampleRate(env, pcmBridgeSR)
	voiceType := env.TTSVoiceType
	if voiceType == 0 {
		voiceType = 101007 // 知性女声（智娜）
	}
	ttsCfg := synthesizer.NewQcloudTTSConfig(env.TTSAppID, env.TTSSecretID, env.TTSSecretKey, voiceType, "pcm", ttsCloudSR)
	ttsCfg.Speed = env.TTSSpeed
	qcTTS := synthesizer.NewQCloudService(ttsCfg)
	ttsStream := &qcloudTTSStream{svc: qcTTS}

	ttsPipe, err := siptts.New(siptts.Config{
		Service:       ttsStream,
		SampleRate:    ttsCloudSR,
		Channels:      1,
		FrameDuration: 20 * time.Millisecond,
		PaceRealtime:  true,
		SendPCMFrame: func(frame []byte) error {
			if len(frame) == 0 {
				return nil
			}
			pcmOut := frame
			recordedAtBridgeRate := ttsCloudSR == pcmBridgeSR
			if !recordedAtBridgeRate && len(frame) >= 2 {
				if out, err := media.ResamplePCM(frame, ttsCloudSR, pcmBridgeSR); err == nil && len(out) > 0 {
					pcmOut = out
					recordedAtBridgeRate = true
				}
			}
			// Stereo recorder: same constraint as the LLM TTS path —
			// only capture when the buffer is at the recorder's bridge
			// rate. See attachVoiceInner SendPCMFrame for rationale.
			if recordedAtBridgeRate {
				cs.WriteAIPCM(pcmOut)
			}
			ms.SendToOutput("sip-script-say", &media.AudioPacket{
				Payload:       pcmOut,
				IsSynthesized: true,
			})
			return nil
		},
		Logger: lg,
	})
	if err != nil {
		return fmt.Errorf("sip conversation: tts pipeline: %w", err)
	}
	runCtx := ctx
	if runCtx == nil {
		runCtx = ms.GetContext()
	}
	ttsPipe.Start(runCtx)
	defer ttsPipe.Stop()
	if err := ttsPipe.Speak(text); err != nil {
		return err
	}
	// SpeakTextOnce uses a fresh Pipeline created per call, so Finalize
	// here drains the only residual (sub-frame tail of `text`). Without
	// it the very last <20ms of the script's audio would be discarded.
	return ttsPipe.Finalize()
}

// qcloudTTSStream adapts synthesizer.QCloudService to siptts.Service (streaming PCM chunks).
type qcloudTTSStream struct {
	svc *synthesizer.QCloudService
}

func (q *qcloudTTSStream) SynthesizeStream(ctx context.Context, text string, callback func(pcm []byte) error) error {
	if q == nil || q.svc == nil {
		return fmt.Errorf("sip conversation: nil tts")
	}
	// 走 WS 路径，首字节比 HTTPS 低 50~150ms。barge-in 通过 ctx 取消触发 SDK CloseConn。
	return q.svc.SynthesizeStream(ctx, text, func(pcm []byte) error {
		if len(pcm) == 0 {
			return nil
		}
		return callback(pcm)
	})
}

// isFillerOnlyUtterance filters hesitation sounds that real-time ASR often emits on noisy or low-volume audio.
func isFillerOnlyUtterance(text string) bool {
	t := strings.TrimSpace(text)
	t = strings.Trim(t, "。！？.!?…~～、，,；;：:《》<>（）()[]【】“”\"'`")
	t = strings.TrimSpace(t)
	if t == "" {
		return true
	}
	for _, r := range t {
		switch r {
		case '嗯', '唔', '呃', '啊', '哦', '噢', '诶', '欸', '哈', '哼', '额',
			' ', '\t', '\n', '\r', '，', ',', '。', '.', '！', '!', '？', '?', '、', '…', '~', '～', '-', '—':
			continue
		default:
			return false
		}
	}
	return true
}

// normalizeTTSText removes segments that TTS providers commonly reject:
// empty/punctuation-only strings and strings without CJK/letters/digits after cleanup.
func normalizeTTSText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		// strip control chars
		if r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	s = strings.TrimSpace(b.String())
	if s == "" {
		return ""
	}
	onlyPunct := true
	for _, r := range s {
		if (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 0x4e00 && r <= 0x9fff) {
			onlyPunct = false
			break
		}
		switch r {
		case ' ', '\t', '\n', '\r', '，', ',', '。', '.', '！', '!', '？', '?',
			'；', ';', '：', ':', '、', '…', '-', '—', '~', '～', '“', '”', '"', '\'', '`', '(', ')', '（', '）', '[', ']', '【', '】', '<', '>', '《', '》':
			// punctuation/noise
		default:
			onlyPunct = false
			break
		}
	}
	if onlyPunct {
		return ""
	}
	return s
}
