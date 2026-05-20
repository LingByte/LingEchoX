package voicedialog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/recognizer"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	sipdtmf "github.com/LinByte/VoiceServer/pkg/sip/dtmf"
	sipvad "github.com/LinByte/VoiceServer/pkg/sip/vad"
	"github.com/LinByte/VoiceServer/pkg/synthesizer"
	sipasr "github.com/LinByte/VoiceServer/pkg/voice/asr"
	voiceMetrics "github.com/LinByte/VoiceServer/pkg/voice/metrics"
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

// gatewayTTSCloudSR picks the cloud-side sample rate to request.
//
//   - When the tenant TTS JSON pins `sampleRate`, honor it (caller already
//     accepted the resample cost downstream).
//   - Otherwise prefer the synthesizer's native rate; for QCloud we hand back
//     the bridge rate so the SDK emits at the exact PCM rate the call leg
//     uses (avoids a 16→8 kHz decimation with audible aliasing on G.711).
//     For providers that produce a fixed rate (e.g. Aliyun Qwen-TTS @ 24
//     kHz) we trust the synthesizer's reported sample rate instead.
//
// `synthRate` is the rate the resolved synthesizer claims via Format(); 0
// means "no preference" and we fall through to the bridge.
func gatewayTTSCloudSR(ttsSampleRate, synthRate, pcmBridge int) int {
	if ttsSampleRate > 0 {
		return ttsSampleRate
	}
	if synthRate > 0 {
		return synthRate
	}
	if pcmBridge > 0 {
		return pcmBridge
	}
	return 16000
}

// attachGatewayMedia wires ASR→WebSocket events and WebSocket tts.speak→TTS→RTP on the SIP leg.
func attachGatewayMedia(sess *dialogSession) error {
	if sess == nil || sess.cs == nil {
		return errors.New("voicedialog gateway: nil session")
	}
	voiceEnv, vLoaded, vErr := conversation.ResolveTenantVoiceEnv(context.Background(), sess.cs)
	if vErr != nil {
		return fmt.Errorf("voicedialog gateway: tenant voice env: %w", vErr)
	}
	if !vLoaded {
		return fmt.Errorf("voicedialog gateway: tenant config not loaded")
	}
	// Voicedialog gateway protocol assumes the WS client drives turns
	// (tts.speak / asr.* events). Realtime providers (Qwen-Omni, GPT-4o
	// realtime) generate replies internally on a single full-duplex
	// session, so the per-segment tts.speak surface has no equivalent.
	// Refuse cleanly when the tenant is on realtime mode — either by
	// explicit voice_mode column or by inference (only realtime creds
	// populated). Same inference rule the embedded SIP path uses, so
	// behavior is consistent across both attach surfaces.
	realtimeIntent := strings.EqualFold(voiceEnv.VoiceMode, "realtime") ||
		(voiceEnv.VoiceMode == "" && conversation.TenantRealtimeReady(voiceEnv))
	if realtimeIntent {
		return fmt.Errorf("voicedialog gateway: tenant voiceMode=realtime is not yet supported by gateway protocol; switch tenant to pipeline mode or use embedded SIP voice attach")
	}
	if !conversation.TenantVoiceReady(voiceEnv) {
		return fmt.Errorf("voicedialog gateway: tenant ASR/TTS/LLM JSON missing or incomplete")
	}
	asrAppID := voiceEnv.ASRAppID
	asrSecretID := voiceEnv.ASRSecretID
	asrSecretKey := voiceEnv.ASRSecretKey
	asrModelType := voiceEnv.ASRModelType
	ttsSampleRate := voiceEnv.TTSSampleRate

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

	pipe, err := sipasr.New(sipasr.Options{
		ASR:        asrSvc,
		SampleRate: asrOutRate,
		Channels:   1,
		Logger:     zlg,
	})
	if err != nil {
		return fmt.Errorf("voicedialog gateway: asr pipeline: %w", err)
	}

	// Provider-agnostic TTS: drive every vendor in TENANT_TTS_PROVIDER_RULES
	// through the same StreamingSynthesizer contract. The credential JSON
	// (tenant ttsConfig) carries `provider + per-vendor fields`; the
	// factory wires the fastest streaming path it can (QCloud WS / Aliyun
	// DashScope realtime WS / etc.) and falls back to one-shot Synthesize
	// wrapping for providers without a native streaming entry yet.
	if voiceEnv.TTSConfigRaw == nil {
		return fmt.Errorf("voicedialog gateway: tenant TTS config missing")
	}
	handle, err := synthesizer.NewStreamingFromCredential(voiceEnv.TTSConfigRaw)
	if err != nil {
		return fmt.Errorf("voicedialog gateway: tts provider: %w", err)
	}
	ttsCloudSR := gatewayTTSCloudSR(ttsSampleRate, handle.SampleRate, pcmBridgeSR)
	ttsStream := handle.Stream

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

	// TTS prewarm: fire a tiny background synthesis so the FIRST real
	// tts.speak in this call doesn't pay full cold-start cost. Saves
	// roughly the DNS+TCP+TLS+WebSocket handshake budget (~50-150ms)
	// — server-side processing time can't be reduced from here. Runs
	// concurrent with welcome.wav playback so it's effectively free.
	go func(stream synthesizer.StreamingSynthesizer, provider string, cid string) {
		if stream == nil {
			return
		}
		// 1.5s ceiling: prewarm must NEVER block real traffic. Anything
		// slower than that is failing for reasons (auth / quota /
		// network) we can't fix from a warmup goroutine.
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		t0 := time.Now()
		// "嗯" — single rune, valid syllable, will produce a few ms of PCM
		// then the SDK hangs up. We only care that the WS handshake +
		// auth happened; output is intentionally discarded by no-op cb.
		err := stream.SynthesizeStream(ctx, "嗯", func(_ []byte) error {
			// First PCM frame received → handshake done. Cancel the
			// context to short-circuit the rest of the synthesis so
			// the SDK doesn't waste server-side time on audio we
			// won't use.
			cancel()
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.Warn("voicedialog gateway tts prewarm failed",
				zap.String(KeyCallID, cid),
				zap.String("provider", provider),
				zap.Duration("elapsed", time.Since(t0)),
				zap.Error(err),
			)
			return
		}
		logger.Info("voicedialog gateway tts prewarm done",
			zap.String(KeyCallID, cid),
			zap.String("provider", provider),
			zap.Duration("elapsed", time.Since(t0)),
		)
	}(ttsStream, string(handle.Service.Provider()), sess.meta.CallID)

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
		// ASR-driven barge-in. The RMS-VAD path above only fires for
		// loud speech (configurable threshold, 3200 by default) — soft
		// callers / phones with AGC normalisation can drop below that
		// while still producing a perfectly clear ASR transcript. If
		// the recogniser is emitting words while TTS is playing, the
		// AI is talking over the caller; cancel it immediately.
		//
		// Mirrors the VAD path's 700ms post-start grace so a final
		// transcript landing in the same tick as a tts.started doesn't
		// kill the reply we just queued.
		if ttsPlaying.Load() {
			pastGrace := true
			if started := ttsStartedAtNS.Load(); started > 0 && time.Since(time.Unix(0, started)) < 700*time.Millisecond {
				pastGrace = false
			}
			if pastGrace {
				voiceMetrics.BargeIn("sip")
				logger.Info("voicedialog gateway interrupt (ASR barge-in)",
					zap.String(KeyCallID, callID),
					zap.Bool("final", isFinal),
					zap.String("text_preview", truncateRunes(trimmed, 60)),
				)
				sess.invalidateQueuedTTS()
				sess.cancelCurrentTTSSegment()
				if cs := sess.cs; cs != nil {
					if m := cs.MediaSession(); m != nil {
						m.DrainOutputs()
					}
				}
				sess.emitGateway(event(EvInterrupt, callID, map[string]any{
					KeyOrigin: OriginGateway,
					KeyCause:  CauseBargeIn,
				}))
				ttsPlaying.Store(false)
				ttsStartedAtNS.Store(0)
			}
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
		voiceMetrics.ASRError("sip")
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
				voiceMetrics.BargeIn("sip")
				logger.Info("voicedialog gateway interrupt (VAD barge-in)", zap.String(KeyCallID, callID))
				// Invalidate every tts.speak that has been queued so far so the queued
				// chain of segments is dropped, not just the currently-playing one. The
				// player goroutine sees the new ttsGenInvalidBefore watermark on the next
				// frame check and exits the in-flight segment.
				sess.invalidateQueuedTTS()
				sess.cancelCurrentTTSSegment()
				// Drop PCM already sitting in the SIP RTP transport queue so
				// the caller stops hearing the AI within ~one packet (20ms).
				if m := ms; m != nil {
					m.DrainOutputs()
				}
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
