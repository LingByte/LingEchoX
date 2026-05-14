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
	"github.com/LingByte/SoulNexus/pkg/recognizer"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	sipdtmf "github.com/LingByte/SoulNexus/pkg/sip/dtmf"
	sipvad "github.com/LingByte/SoulNexus/pkg/sip/vad"
	"github.com/LingByte/SoulNexus/pkg/synthesizer"
	"github.com/LingByte/SoulNexus/pkg/utils"
	sipasr "github.com/LingByte/SoulNexus/pkg/voice/asr"
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

// gatewayQcloudTTSStream adapts synthesizer.QCloudService to a streaming PCM callback
// signature consumed by tts_segmenter.go.
//
// 走 WebSocket 路径（QCloudService.SynthesizeStream），每段一条 WS 连接但首字节
// 比 HTTPS 路径低 50~150ms（PCM 直接 binary frame 推送，无 HTTP chunk 编码开销）。
// 段间的 WS 握手开销由 tts_segmenter.go 的 prefetch 并行化继续 hide。
type gatewayQcloudTTSStream struct {
	svc *synthesizer.QCloudService
}

func (q *gatewayQcloudTTSStream) SynthesizeStream(ctx context.Context, text string, callback func(pcm []byte) error) error {
	if q == nil || q.svc == nil {
		return fmt.Errorf("voicedialog gateway: nil tts")
	}
	// WS path: PCM frames arrive directly via callback; no WAV header to strip.
	return q.svc.SynthesizeStream(ctx, text, func(pcm []byte) error {
		if len(pcm) == 0 {
			return nil
		}
		return callback(pcm)
	})
}

// attachGatewayMedia wires ASR→WebSocket events and WebSocket tts.speak→TTS→RTP on the SIP leg.
func attachGatewayMedia(sess *dialogSession) error {
	if sess == nil || sess.cs == nil {
		return errors.New("voicedialog gateway: nil session")
	}
	asrAppID := utils.GetEnv("ASR_APPID")
	asrSecretID := utils.GetEnv("ASR_SECRET_ID")
	asrSecretKey := utils.GetEnv("ASR_SECRET_KEY")
	asrModelType := utils.GetEnv("ASR_MODEL_TYPE")
	ttsAppID := utils.GetEnv("TTS_APPID")
	ttsSecretID := utils.GetEnv("TTS_SECRET_ID")
	ttsSecretKey := utils.GetEnv("TTS_SECRET_KEY")
	ttsVoiceType, _ := strconv.ParseInt(utils.GetEnv("TTS_VOICE_TYPE"), 10, 64)
	ttsSpeed, _ := strconv.ParseInt(utils.GetEnv("TTS_SPEED"), 10, 64)
	// TTS_SAMPLE_RATE 显式指定时按用户配置走（一般 8000/16000/24000）。
	// **未设置时不再写死 16000**，而是 0 → 走 gatewayTTSCloudSR 的 fallback：
	// 跟 SIP 协商出的 pcmBridgeSR 对齐，避免下游 ResamplePCM 跨 chunk 重采样
	// 造成的相位累积错位（块边界周期性电流音）。
	ttsSampleRate, _ := strconv.Atoi(utils.GetEnv("TTS_SAMPLE_RATE"))
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

	// Wire the new pipelined TTS player. Each tts.speak segment kicks off its own SDK
	// call in parallel (prefetch); a single player goroutine drains segments serially
	// and frames PCM to the call leg, eliminating the per-segment WS handshake gap.
	sess.gatewayMu.Lock()
	sess.ttsService = ttsStream
	sess.ttsCloudSR = ttsCloudSR
	sess.ttsBridgeSR = pcmBridgeSR
	sess.gatewayMu.Unlock()
	sess.startTTSPlayer()
	_ = zlg // logger captured by player loop directly via the global logger.Lg

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
			// During transfer (ringing / loading / agent talking), drop caller PCM so the
			// ASR cloud session does not emit further partial / final events that would
			// (a) feed the LLM and (b) trigger redundant tts.speak segments.
			if conversation.IsTransferInProgress(callID) {
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
				// Invalidate every tts.speak that has been queued so far so the queued
				// chain of segments is dropped, not just the currently-playing one. The
				// player goroutine sees the new ttsGenInvalidBefore watermark on the next
				// frame check and exits the in-flight segment.
				sess.invalidateQueuedTTS()
				sess.cancelCurrentTTSSegment()
				sess.emitGateway(event(EvInterrupt, callID, map[string]any{
					KeyOrigin: OriginGateway,
					KeyCause:  CauseBargeIn,
				}))
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
	// Tear down the segment player goroutine and any in-flight prefetches before
	// we let the rest of the session GC.
	if sess.ttsSegmentCh != nil {
		sess.stopTTSPlayer()
	}
	sess.gatewayMu.Lock()
	defer sess.gatewayMu.Unlock()
	sess.ttsService = nil
	sess.ttsPlayingPtr = nil
	sess.ttsStartedNS = nil
	sess.mediaSession = nil
}

func (sess *dialogSession) stopGatewayTTSPlayback() {
	// Modern (segmenter-driven) shutdown: invalidate the queue + cancel the
	// in-flight segment ctx. The player will emit tts.ended ok=false for the
	// preempted segment and immediately move to the next (which is also stale).
	sess.invalidateQueuedTTS()
	sess.cancelCurrentTTSSegment()
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

func (sess *dialogSession) markTransferAfterNextTTS() {
	if sess != nil {
		sess.transferAfterNextTTS.Store(true)
	}
}

func (sess *dialogSession) consumeTransferAfterNextTTS() bool {
	if sess == nil {
		return false
	}
	return sess.transferAfterNextTTS.Swap(false)
}

// handleTTSSpeak is the entrypoint invoked by hub.go on every CmdTTSSpeak. With the
// pipelined player (tts_segmenter.go), this function only:
//  1. validates the segment (gen / transfer / empty text),
//  2. kicks off prefetch (parallel SDK call), and
//  3. enqueues the segment job onto the player channel.
//
// The actual synthesis & RTP framing happen on the player goroutine; counter
// decrement (endPendingTTS) is also done by the player so transfer-after-last-segment
// timing is correct even when the loopback queues many segments faster than they play.
func (sess *dialogSession) handleTTSSpeak(text, utteranceID string, gen uint64) {
	dropEnd := func(reason string) {
		logger.Info("voicedialog gateway tts speak dropped",
			zap.String(KeyCallID, sess.meta.CallID),
			zap.String(KeyUtteranceID, utteranceID),
			zap.Uint64("gen", gen),
			zap.String("reason", reason),
		)
		sess.emitGateway(event(EvTTSEnded, sess.meta.CallID, map[string]any{
			KeyUtteranceID: utteranceID,
			KeyOK:          false,
		}))
		sess.endPendingTTS()
	}

	if gen != 0 && gen <= sess.ttsGenInvalidBefore.Load() {
		dropEnd("stale_generation")
		return
	}
	if conversation.IsTransferInProgress(sess.meta.CallID) {
		dropEnd("transfer_in_progress")
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		dropEnd("empty_text")
		return
	}
	if sess.cs == nil || sess.cs.MediaSession() == nil {
		dropEnd("no_media_session")
		return
	}

	job := &ttsSegmentJob{
		text:        text,
		utteranceID: utteranceID,
		gen:         gen,
		enqueueT0:   time.Now(),
	}
	// Start prefetch BEFORE enqueueing so the SDK call begins while the player may still
	// be playing the previous segment. This is the core mechanism that hides the per-segment
	// SDK handshake (~200~400ms) behind the previous segment's playback wall-clock.
	sess.startSegmentPrefetch(job)
	if !sess.enqueueTTSSegment(job) {
		// queue full / closed → cancel prefetch, account counter, signal end.
		if job.prefetchCancel != nil {
			job.prefetchCancel()
		}
		dropEnd("queue_full")
		return
	}
}

// --- pending TTS counter (drives transfer-after-last-segment) ----------------

func (sess *dialogSession) beginPendingTTS() {
	if sess == nil {
		return
	}
	sess.pendingTTSCount.Add(1)
}

// endPendingTTS decrements counter; if counter reaches 0 AND transferAfterNextTTS flag
// is set, consume it and trigger transfer.
func (sess *dialogSession) endPendingTTS() {
	if sess == nil {
		return
	}
	n := sess.pendingTTSCount.Add(-1)
	if n < 0 {
		// underflow guard (e.g. tts.speak from an external client we never counted)
		sess.pendingTTSCount.Store(0)
		return
	}
	if n == 0 && sess.consumeTransferAfterNextTTS() {
		logger.Info("voicedialog: transfer after assistant TTS finished",
			zap.String(KeyCallID, sess.meta.CallID),
		)
		go conversation.TriggerTransferToAgent(context.Background(), sess.meta.CallID, logger.Lg)
	}
}

// endPendingTTSWithoutTransfer rolls back a beginPendingTTS that did not actually result
// in a queued tts.speak (e.g. write failed). Never fires transfer.
func (sess *dialogSession) endPendingTTSWithoutTransfer() {
	if sess == nil {
		return
	}
	if n := sess.pendingTTSCount.Add(-1); n < 0 {
		sess.pendingTTSCount.Store(0)
	}
}

func (sess *dialogSession) pendingTTSEmpty() bool {
	if sess == nil {
		return true
	}
	return sess.pendingTTSCount.Load() <= 0
}

// --- first-audio hook --------------------------------------------------------

func (sess *dialogSession) armFirstAudioHook(fn func()) {
	if sess == nil {
		return
	}
	sess.firstAudioMu.Lock()
	sess.firstAudioHook = fn
	sess.firstAudioMu.Unlock()
}

func (sess *dialogSession) fireAndClearFirstAudioHook() {
	if sess == nil {
		return
	}
	sess.firstAudioMu.Lock()
	fn := sess.firstAudioHook
	sess.firstAudioHook = nil
	sess.firstAudioMu.Unlock()
	if fn != nil {
		fn()
	}
}

// invalidateQueuedTTS bumps the invalid-before watermark to the latest issued generation,
// causing every queued segment job (sitting in ttsSegmentCh or partway through prefetch)
// to drop on its next gen check inside the player loop. The currently-playing segment
// is preempted separately by the caller via cancelCurrentTTSSegment() (or via
// stopGatewayTTSPlayback which calls both).
func (sess *dialogSession) invalidateQueuedTTS() {
	if sess == nil {
		return
	}
	cur := sess.ttsGenSeq.Load()
	for {
		prev := sess.ttsGenInvalidBefore.Load()
		if cur <= prev {
			return
		}
		if sess.ttsGenInvalidBefore.CompareAndSwap(prev, cur) {
			return
		}
	}
}

func (sess *dialogSession) handleTTSCancel() {
	// Publish the invalidation watermark FIRST (lock-free) so any queued segment in
	// ttsSegmentCh / mid-prefetch sees a stale generation when the player picks it
	// up. stopGatewayTTSPlayback also cancels the in-flight segment ctx.
	sess.invalidateQueuedTTS()
	sess.stopGatewayTTSPlayback()
	sess.emitGateway(event(EvTTSCancelled, sess.meta.CallID, nil))
	logger.Info("voicedialog gateway tts.cancel handled (queue invalidated)",
		zap.String(KeyCallID, sess.meta.CallID),
		zap.Uint64("invalid_before", sess.ttsGenInvalidBefore.Load()),
	)
}

func (sess *dialogSession) handleInterruptFromWS(clientReason string) {
	sess.invalidateQueuedTTS()
	sess.stopGatewayTTSPlayback()
	clientReason = strings.TrimSpace(clientReason)
	sess.emitGateway(event(EvInterrupt, sess.meta.CallID, map[string]any{
		KeyOrigin: OriginGateway,
		KeyCause:  CauseApplied,
		KeyReason: clientReason,
	}))
	logger.Info("voicedialog gateway interrupt handled (queue invalidated)",
		zap.String(KeyCallID, sess.meta.CallID),
		zap.String(KeyReason, clientReason),
		zap.Uint64("invalid_before", sess.ttsGenInvalidBefore.Load()),
	)
}
