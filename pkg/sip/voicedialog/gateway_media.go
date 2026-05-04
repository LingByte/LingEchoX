package voicedialog

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/media"
	"github.com/LingByte/SoulNexus/pkg/media/encoder"
	"github.com/LingByte/SoulNexus/pkg/recognizer"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	sipdtmf "github.com/LingByte/SoulNexus/pkg/sip/dtmf"
	sipvad "github.com/LingByte/SoulNexus/pkg/sip/vad"
	"github.com/LingByte/SoulNexus/pkg/synthesizer"
	"github.com/LingByte/SoulNexus/pkg/utils"
	sipasr "github.com/LingByte/SoulNexus/pkg/voice/asr"
	siptts "github.com/LingByte/SoulNexus/pkg/voice/tts"
	"go.uber.org/zap"
)

func pcmBridgeHz(sess *dialogSession) int {
	if sess == nil || sess.cs == nil {
		return 16000
	}
	sr := sess.cs.PCMSampleRate()
	if sr <= 0 {
		return 16000
	}
	return sr
}

func gatewayTTSCloudSR(ttsSampleRate int, pcmBridge int) int {
	if ttsSampleRate > 0 {
		return ttsSampleRate
	}
	if pcmBridge > 0 {
		return pcmBridge
	}
	return 16000
}

// gatewayQcloudTTSStream adapts synthesizer.QCloudService to siptts.Service (streaming PCM chunks).
type gatewayQcloudTTSStream struct {
	svc *synthesizer.QCloudService
}

func (q *gatewayQcloudTTSStream) SynthesizeStream(ctx context.Context, text string, callback func(pcm []byte) error) error {
	if q == nil || q.svc == nil {
		return fmt.Errorf("voicedialog gateway: nil tts")
	}
	done := make(chan error, 1)
	go func() {
		h := &gatewayTTSStreamHandler{callback: callback, ctx: ctx}
		done <- q.svc.Synthesize(context.Background(), h, text)
	}()
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-done:
		return err
	}
}

type gatewayTTSStreamHandler struct {
	ctx        context.Context
	callback   func([]byte) error
	firstChunk bool
}

func (h *gatewayTTSStreamHandler) OnMessage(data []byte) {
	if h == nil || len(data) == 0 {
		return
	}
	if h.ctx != nil && h.ctx.Err() != nil {
		return
	}
	if !h.firstChunk {
		h.firstChunk = true
		data = encoder.StripWavHeader(data)
	}
	_ = h.callback(data)
}

func (h *gatewayTTSStreamHandler) OnTimestamp(_ synthesizer.SentenceTimestamp) {}

// attachGatewayMedia wires ASR→WebSocket events and WebSocket tts.speak→TTS→RTP on the SIP leg.
func attachGatewayMedia(sess *dialogSession) error {
	if sess == nil || sess.cs == nil {
		return errors.New("voicedialog gateway: nil session")
	}
	asrAppID := strings.TrimSpace(utils.GetEnv("ASR_APPID"))
	asrSecretID := strings.TrimSpace(utils.GetEnv("ASR_SECRET_ID"))
	asrSecretKey := strings.TrimSpace(utils.GetEnv("ASR_SECRET_KEY"))
	asrModelType := strings.TrimSpace(utils.GetEnv("ASR_MODEL_TYPE"))
	ttsAppID := strings.TrimSpace(utils.GetEnv("TTS_APPID"))
	ttsSecretID := strings.TrimSpace(utils.GetEnv("TTS_SECRET_ID"))
	ttsSecretKey := strings.TrimSpace(utils.GetEnv("TTS_SECRET_KEY"))
	ttsVoiceType, _ := strconv.ParseInt(strings.TrimSpace(utils.GetEnv("TTS_VOICE_TYPE")), 10, 64)
	ttsSpeed, _ := strconv.ParseInt(strings.TrimSpace(utils.GetEnv("TTS_SPEED")), 10, 64)
	ttsSampleRate, _ := strconv.Atoi(strings.TrimSpace(utils.GetEnv("TTS_SAMPLE_RATE")))
	if ttsSampleRate <= 0 {
		ttsSampleRate = 16000
	}
	if asrAppID == "" || asrSecretID == "" || asrSecretKey == "" {
		return fmt.Errorf("voicedialog gateway: missing ASR credentials (ASR_APPID, ASR_SECRET_ID, ASR_SECRET_KEY)")
	}
	if ttsAppID == "" || ttsSecretID == "" || ttsSecretKey == "" {
		return fmt.Errorf("voicedialog gateway: missing TTS credentials (TTS_APPID, TTS_SECRET_ID, TTS_SECRET_KEY)")
	}

	ms := sess.cs.MediaSession()
	if ms == nil {
		return errors.New("voicedialog gateway: nil media session")
	}

	zlg := zap.L()

	asrOpt := recognizer.NewQcloudASROption(asrAppID, asrSecretID, asrSecretKey)
	if asrModelType != "" {
		asrOpt.ModelType = asrModelType
	}
	asrSvc := recognizer.NewQcloudASR(asrOpt)

	asrProvLabel := "qcloud_asr"
	if strings.TrimSpace(asrOpt.ModelType) != "" {
		asrProvLabel = strings.TrimSpace(asrOpt.ModelType)
	}
	sess.voicedialogASRProv = asrProvLabel

	asrOutRate := 16000
	if strings.Contains(strings.ToLower(asrOpt.ModelType), "8k") {
		asrOutRate = 8000
	}
	asrInRate := pcmBridgeHz(sess)
	pcmBridgeSR := asrInRate
	ttsCloudSR := gatewayTTSCloudSR(ttsSampleRate, pcmBridgeSR)

	pipe, err := sipasr.New(sipasr.Options{
		ASR:        asrSvc,
		SampleRate: asrOutRate,
		Channels:   1,
		Logger:     zlg,
	})
	if err != nil {
		return fmt.Errorf("voicedialog gateway: asr pipeline: %w", err)
	}

	voiceType := ttsVoiceType
	if voiceType == 0 {
		voiceType = 101007
	}
	ttsCfg := synthesizer.NewQcloudTTSConfig(ttsAppID, ttsSecretID, ttsSecretKey, voiceType, "pcm", ttsCloudSR)
	ttsCfg.Speed = ttsSpeed
	qcTTS := synthesizer.NewQCloudService(ttsCfg)
	ttsStream := &gatewayQcloudTTSStream{svc: qcTTS}

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
			if ttsCloudSR != pcmBridgeSR && len(frame) >= 2 {
				if out, err := media.ResamplePCM(frame, ttsCloudSR, pcmBridgeSR); err == nil && len(out) > 0 {
					pcmOut = out
				}
			}
			ms.SendToOutput("voice-gateway-tts", &media.AudioPacket{
				Payload:       pcmOut,
				IsSynthesized: true,
			})
			return nil
		},
		Logger: zlg,
	})
	if err != nil {
		return fmt.Errorf("voicedialog gateway: tts pipeline: %w", err)
	}

	sess.gatewayMu.Lock()
	sess.ttsPipe = ttsPipe
	sess.gatewayMu.Unlock()

	var ttsPlaying atomic.Bool
	var ttsStartedAtNS atomic.Int64

	var vadDet *sipvad.Detector
	if config.GlobalConfig.SIP.SIPVADBargeIn {
		vadDet = sipvad.NewDetector()
		vadDet.SetLogger(zlg)
		vadDet.SetThreshold(config.GlobalConfig.SIP.SIPVADThreshold)
		vadDet.SetConsecutiveFrames(config.GlobalConfig.SIP.SIPVADConsecFrames)
		logger.Info("voicedialog gateway RMS barge-in enabled",
			zap.String(KeyCallID, sess.meta.CallID),
			zap.Float64("threshold", config.GlobalConfig.SIP.SIPVADThreshold),
			zap.Int("consecutive_frames", config.GlobalConfig.SIP.SIPVADConsecFrames),
		)
	}

	callID := sess.meta.CallID

	pipe.SetTextCallback(func(text string, isFinal bool) {
		if ms.GetContext().Err() != nil {
			return
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return
		}
		if isFinal {
			sess.setLastASRFinal(trimmed)
		}
		evType := EvASRPartial
		if isFinal {
			evType = EvASRFinal
		}
		sess.emitGateway(event(evType, callID, map[string]any{
			KeyText:  trimmed,
			KeyFinal: isFinal,
		}))
	})

	pipe.SetErrorCallback(func(err error, fatal bool) {
		logger.Warn("voicedialog gateway asr error",
			zap.String(KeyCallID, callID),
			zap.Error(err),
			zap.Bool(KeyFatal, fatal),
		)
		sess.emitGateway(event(EvASRError, callID, map[string]any{
			KeyMessage: err.Error(),
			KeyFatal:   fatal,
		}))
	})

	proc := media.NewPacketProcessor("voice-gateway-asr-feed", media.PriorityHigh,
		func(c context.Context, _ *media.MediaSession, packet media.MediaPacket) error {
			ap, ok := packet.(*media.AudioPacket)
			if !ok || ap == nil || len(ap.Payload) == 0 || ap.IsSynthesized {
				return nil
			}
			pcm16 := ap.Payload
			pcmASR := pcm16
			if asrOutRate != asrInRate {
				out, err := media.ResamplePCM(pcm16, asrInRate, asrOutRate)
				if err != nil {
					return nil
				}
				pcmASR = out
			}
			allowBargeIn := true
			if vadDet != nil && ttsPlaying.Load() {
				if started := ttsStartedAtNS.Load(); started > 0 && time.Since(time.Unix(0, started)) < 700*time.Millisecond {
					allowBargeIn = false
				}
			}
			if allowBargeIn && vadDet != nil && ttsPlaying.Load() && vadDet.CheckBargeIn(pcm16, true) {
				logger.Info("voicedialog gateway interrupt (VAD barge-in)", zap.String(KeyCallID, callID))
				sess.emitGateway(event(EvInterrupt, callID, map[string]any{
					KeyOrigin: OriginGateway,
					KeyCause:  CauseBargeIn,
				}))
				sess.gatewayMu.Lock()
				tp := sess.ttsPipe
				sess.gatewayMu.Unlock()
				if tp != nil {
					tp.Stop()
					tp.Start(ms.GetContext())
				}
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

	sipdtmf.AttachProcessor(ms, "voice-gateway-dtmf", func(_ context.Context, digit string) {
		if digit == "" {
			return
		}
		sess.emitGateway(event(EvDTMF, callID, map[string]any{
			KeyDigit: digit,
		}))
	})

	sess.attachTTSLifecycle(&ttsPlaying, &ttsStartedAtNS, ms)

	logger.Info("voicedialog gateway media attached",
		zap.String(KeyCallID, callID),
		zap.String("asr_model", asrOpt.ModelType),
		zap.Int("pcm_bridge_hz", pcmBridgeSR),
		zap.Int("tts_cloud_hz", ttsCloudSR),
	)
	sess.cs.StartOnACK()
	go sess.runDialogWelcome()
	return nil
}

func (sess *dialogSession) attachTTSLifecycle(playing *atomic.Bool, startedAt *atomic.Int64, ms *media.MediaSession) {
	sess.gatewayMu.Lock()
	defer sess.gatewayMu.Unlock()
	sess.ttsPlayingPtr = playing
	sess.ttsStartedNS = startedAt
	sess.mediaSession = ms
}

func (sess *dialogSession) gatewayShutdown() {
	sess.stopTransferLoadingPlayback()
	sess.gatewayMu.Lock()
	defer sess.gatewayMu.Unlock()
	if sess.ttsPipe != nil {
		sess.ttsPipe.Stop()
		sess.ttsPipe = nil
	}
	sess.ttsPlayingPtr = nil
	sess.ttsStartedNS = nil
	sess.mediaSession = nil
}

func (sess *dialogSession) stopGatewayTTSPlayback() {
	ms := sess.cs.MediaSession()
	if ms == nil {
		return
	}
	sess.gatewayMu.Lock()
	pipe := sess.ttsPipe
	sess.gatewayMu.Unlock()
	if pipe == nil {
		return
	}
	pipe.Stop()
	pipe.Start(ms.GetContext())
	if sess.ttsPlayingPtr != nil {
		sess.ttsPlayingPtr.Store(false)
	}
	if sess.ttsStartedNS != nil {
		sess.ttsStartedNS.Store(0)
	}
}

func (sess *dialogSession) setLastASRFinal(text string) {
	sess.dialogTurnMu.Lock()
	defer sess.dialogTurnMu.Unlock()
	sess.lastASRFinal = strings.TrimSpace(text)
}

func (sess *dialogSession) lastASRFinalSnapshot() string {
	sess.dialogTurnMu.Lock()
	defer sess.dialogTurnMu.Unlock()
	return sess.lastASRFinal
}

func (sess *dialogSession) setPendingLoopbackLLM(model string, llmWallMs int) {
	sess.pendingTurnMu.Lock()
	defer sess.pendingTurnMu.Unlock()
	sess.pendingLLMModel = strings.TrimSpace(model)
	sess.pendingLLMWallMs = llmWallMs
}

func (sess *dialogSession) takePendingLLMMeta() (model string, llmWallMs int) {
	sess.pendingTurnMu.Lock()
	defer sess.pendingTurnMu.Unlock()
	model = sess.pendingLLMModel
	llmWallMs = sess.pendingLLMWallMs
	sess.pendingLLMModel = ""
	sess.pendingLLMWallMs = 0
	return model, llmWallMs
}

func (sess *dialogSession) handleTTSSpeak(text, utteranceID string) {
	sess.ttsSpeakMu.Lock()
	defer sess.ttsSpeakMu.Unlock()

	pipelineT0 := time.Now()

	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	ms := sess.cs.MediaSession()
	if ms == nil {
		return
	}
	sess.gatewayMu.Lock()
	pipe := sess.ttsPipe
	sess.gatewayMu.Unlock()
	if pipe == nil {
		return
	}

	logger.Info("voicedialog gateway tts speak start",
		zap.String(KeyCallID, sess.meta.CallID),
		zap.String(KeyUtteranceID, utteranceID),
		zap.Int("text_len", len([]rune(text))),
		zap.String("text_preview", truncateRunes(text, 120)),
	)

	sess.emitGateway(event(EvTTSStarted, sess.meta.CallID, map[string]any{
		KeyUtteranceID: utteranceID,
		KeyTextPreview: truncateRunes(text, 160),
	}))

	if sess.ttsPlayingPtr != nil {
		sess.ttsPlayingPtr.Store(true)
	}
	if sess.ttsStartedNS != nil {
		sess.ttsStartedNS.Store(time.Now().UnixNano())
	}

	pipe.Start(ms.GetContext())
	ttsT0 := time.Now()
	err := pipe.Speak(text)
	ttsMs := int(time.Since(ttsT0).Milliseconds())
	pipe.Stop()

	if sess.ttsPlayingPtr != nil {
		sess.ttsPlayingPtr.Store(false)
	}
	if sess.ttsStartedNS != nil {
		sess.ttsStartedNS.Store(0)
	}

	sess.emitGateway(event(EvTTSEnded, sess.meta.CallID, map[string]any{
		KeyUtteranceID: utteranceID,
		KeyOK:          err == nil,
	}))

	if err != nil {
		logger.Warn("voicedialog gateway tts speak error",
			zap.String(KeyCallID, sess.meta.CallID),
			zap.String(KeyUtteranceID, utteranceID),
			zap.Error(err),
		)
	} else {
		logger.Info("voicedialog gateway tts speak done",
			zap.String(KeyCallID, sess.meta.CallID),
			zap.String(KeyUtteranceID, utteranceID),
		)
		asrSnap := sess.lastASRFinalSnapshot()
		asrProv := strings.TrimSpace(sess.voicedialogASRProv)
		if asrProv == "" {
			asrProv = "qcloud_asr"
		}
		llmModel, llmWall := sess.takePendingLLMMeta()
		go conversation.RecordDialogTurn(context.Background(), sess.meta.CallID, conversation.DialogTurn{
			ASRText:     asrSnap,
			LLMText:     text,
			ASRProvider: asrProv,
			TTSProvider: "qcloud_tts",
			LLMModel:    llmModel,
			Trigger:     "final",
			LLMWallMs:   llmWall,
			TTSMs:       ttsMs,
			PipelineMs:  int(time.Since(pipelineT0).Milliseconds()),
		})
	}
}

func (sess *dialogSession) handleTTSCancel() {
	sess.ttsSpeakMu.Lock()
	defer sess.ttsSpeakMu.Unlock()
	sess.stopGatewayTTSPlayback()
	sess.emitGateway(event(EvTTSCancelled, sess.meta.CallID, nil))
	logger.Info("voicedialog gateway tts.cancel handled",
		zap.String(KeyCallID, sess.meta.CallID),
	)
}

func (sess *dialogSession) handleInterruptFromWS(clientReason string) {
	sess.ttsSpeakMu.Lock()
	defer sess.ttsSpeakMu.Unlock()
	sess.stopGatewayTTSPlayback()
	clientReason = strings.TrimSpace(clientReason)
	sess.emitGateway(event(EvInterrupt, sess.meta.CallID, map[string]any{
		KeyOrigin: OriginGateway,
		KeyCause:  CauseApplied,
		KeyReason: clientReason,
	}))
	logger.Info("voicedialog gateway interrupt handled",
		zap.String(KeyCallID, sess.meta.CallID),
		zap.String(KeyReason, clientReason),
	)
}
