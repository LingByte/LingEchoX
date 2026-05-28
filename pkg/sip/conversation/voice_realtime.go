package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// SIP voice attach — realtime multimodal mode.
//
// Sibling of attachVoiceInner (the 3-layer pipeline). Selected when
// env.VoiceMode == "realtime". Wires:
//
//   - One pkg/realtime.Agent (Qwen-Omni today; GPT-4o realtime / Doubao
//     planned) for the whole call.
//   - Caller PCM (decoded by RTP layer at bridge sample rate) → resample
//     to 16 kHz → agent.PushAudio.
//   - agent EventAssistantAudio (24 kHz) → resample to bridge → RTP +
//     stereo recorder.
//   - agent EventUserSpeechStarted → barge-in (cancels in-flight reply
//     server-side via response.cancel; SIP path drops any AI PCM still
//     in flight).
//   - Transfer-to-agent: Qwen3.5-Omni-Realtime uses WS Function Calling
//     (transfer_to_agent). The tool only marks pending; dial runs after the
//     assistant turn ends and local playback buffer drains (parity with
//     pipeline "transfer after TTS"). Legacy marker/keyword paths apply
//     only when FC tools are disabled.
//   - Once IsTransferInProgress fires (transfer ringing / loading music
//     started), both directions of audio between caller and agent are
//     hard-gated so the AI cannot talk over the hold music or the human
//     agent. We also call agent.Cancel() to free server-side budget.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/realtime"
	_ "github.com/LinByte/VoiceServer/pkg/realtime/aliyunomni"     // self-registers as aliyun_omni
	_ "github.com/LinByte/VoiceServer/pkg/realtime/volcdialogue" // self-registers as volcengine_dialogue
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	siprecorder "github.com/LinByte/VoiceServer/pkg/voice/recorder"
	"go.uber.org/zap"
)

// realtimeAgentInputRate is the sample rate the realtime providers expect
// caller PCM at. Qwen-Omni / GPT-4o both use 16 kHz mono PCM16LE on the
// input channel. Lift to a config knob if a future provider deviates.
const realtimeAgentInputRate = 16000

func attachRealtimeVoiceInner(ctx context.Context, cs *sipSession.CallSession, env VoiceEnv, lg *zap.Logger) error {
	ms := cs.MediaSession()
	if ms == nil {
		return fmt.Errorf("sip conversation: nil media session (realtime)")
	}
	if !TenantRealtimeReady(env) {
		return fmt.Errorf("sip conversation: realtime config incomplete (provider=%q)", env.RealtimeProvider)
	}

	pcmBridgeSR := sipVoicePCMBridgeRate(cs)

	// Stereo recorder — same baseline as the pipeline path. Recording is
	// not optional; tenants depend on call audit.
	recCfg := siprecorder.Config{Logger: lg}
	if secs, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SIP_RECORDER_CHUNK_SECS"))); err == nil && secs > 0 {
		recCfg.ChunkInterval = time.Duration(secs) * time.Second
	}
	if cs.EnableRecorder(recCfg) {
		lg.Info("sip voice (realtime): stereo PCM recorder enabled",
			zap.String("call_id", cs.CallID),
			zap.Int("sample_rate", pcmBridgeSR),
			zap.Duration("chunk_interval", recCfg.ChunkInterval),
		)
	}

	useTransferTool := realtimeSupportsTransferTools(env)
	transferConfirmRequired := TransferConfirmRequired(env)
	operatorCore := mergeRealtimeInstructions(
		realtimeInstructionsFromEnv(env),
		popSIPCallSystemPrompt(cs.CallID),
	)
	rulesBlock := realtimeAugmentSystemPrompt("", useTransferTool, transferConfirmRequired)
	baseSystemPrompt := mergeRealtimeInstructions(operatorCore, rulesBlock)

	hangupPhrases := sipHangupPhrasesFromEnv()

	// --- session-scoped state ---
	var (
		assistantBuf strings.Builder // accumulates EventAssistantText deltas for log/CDR
		assistantMu  sync.Mutex
		ttsPlaying   atomic.Bool
		turnT0       atomic.Int64
		userText     atomic.Value // string — last final user transcript
		closeOnce    sync.Once
		// Observability counters: realtime sessions are silent failure
		// modes if PCM never reaches the model. We log on the FIRST
		// successful PushAudio, then sample every N frames so a
		// half-broken session shows up in the log without flooding it.
		pushedFirst    atomic.Bool
		pushedFrames   atomic.Uint64
		audioRecvFirst atomic.Bool
		// Welcome-WAV playback gate (parity with pipeline mode).
		// While true, caller PCM is NOT forwarded to the agent so the
		// model's VAD doesn't trip on the WAV's echo and DashScope
		// audio budget is not consumed during the connect cue.
		welcomePlaying atomic.Bool
		// Barge-in mute deadline (unix nanos). Set far into the future
		// when EventUserSpeechStarted fires; pulled back to "now + grace"
		// when EventUserSpeechEnded fires. While `time.Now() < deadline`
		// every EventAssistantAudio is dropped — this drops in-flight
		// audio from the *cancelled* turn that DashScope emits before
		// our `response.cancel` round-trips. Pair with `DrainOutputs()`
		// to also clear what's already queued in the SIP RTP transport.
		aiMutedUntilNs atomic.Int64
		// AI PCM jitter buffer: Omni emits PCM in irregularly-sized
		// chunks (50–500 ms each), arriving in bursts. The SIP RTP
		// transport sends each AudioPacket as ONE RTP datagram with
		// the codec's 20ms timestamp increment — i.e. a 320ms chunk
		// gets sent as a single oversize packet with a 20ms-advanced
		// timestamp, which the SIP peer plays back as a burst with
		// scrambled timing. Symptom: "一下子蹦出来很多字". Fix: pump
		// PCM through a 20ms framer + ticker so RTP packets are
		// exactly one frame each, paced at realtime.
		aiBufMu    sync.Mutex
		aiBuf      []byte
		playerOnce sync.Once
		// While true, drop Omni assistant audio/text handling — fixed TTS
		// confirm reply is playing instead (avoids double Cancel + wrong wording).
		suppressOmniAssistant atomic.Bool
	)
	const (
		bargeInGrace    = 200 * time.Millisecond
		bargeInLongMute = 10 * time.Minute
		// 60 ms jitter prebuffer mirrors the value pipeline TTS uses
		// (pkg/sip/voicedialog/tts_segmenter.go::ttsJitterPrebufferMs).
		// Below 60 ms an early underrun is common when the first chunk
		// happens to be small + the next chunk is delayed.
		aiPrebufferBytesAt20msFrames = 3
	)

	// agent declaration is forward-referenced by the OnEvent closure
	// (Cancel on barge-in / hangup). We assign after construction.
	var agent realtime.Agent

	gateTransferOrShutdown := func() bool {
		// Returns true when *both* directions should be silent (transfer
		// in progress or media session torn down). Single source of
		// truth so callers don't race on multiple flags.
		if ms == nil || ms.GetContext().Err() != nil {
			return true
		}
		if IsTransferInProgress(cs.CallID) {
			return true
		}
		return false
	}

	// executeTransfer runs the dial leg after the caller has heard the AI
	// transfer acknowledgement (FC path) or immediately (legacy keyword/marker).
	transferStartedLocal := atomic.Bool{}
	confirmReplyBusy := atomic.Bool{}
	executeTransfer := func(reason string) {
		if !transferStartedLocal.CompareAndSwap(false, true) {
			return
		}
		if useTransferTool {
			if !consumeSIPTransferPending(cs.CallID) {
				transferStartedLocal.Store(false)
				return
			}
		} else {
			markSIPTransferPending(cs.CallID)
		}
		lg.Info("sip voice (realtime): transfer trigger",
			zap.String("call_id", cs.CallID),
			zap.String("reason", reason),
		)
		// Stop the model immediately; we don't want it to finish a
		// sentence over the agent's "loading…" music. We *don't* close
		// the agent here — leaving the WS open keeps the door open for
		// "no agent → return to AI" scenarios in the future without
		// re-paying handshake cost.
		if agent != nil {
			_ = agent.Cancel()
		}
		// Hard-mute any AI audio chunks still in flight from the
		// cancelled response (Cancel WS round-trip is ~50ms; chunks
		// already on the wire arrive after). Without this, the
		// caller hears "为您转接人工" prompt overlapping with the
		// tail of the model's previous reply. 60s window comfortably
		// covers prompt + transfer setup; the call handoff replaces
		// the agent-driven audio path entirely after that.
		aiMutedUntilNs.Store(time.Now().Add(60 * time.Second).UnixNano())
		aiBufMu.Lock()
		aiBuf = aiBuf[:0]
		aiBufMu.Unlock()
		if ms != nil {
			ms.DrainOutputs()
		}
		ttsPlaying.Store(false)
		// Dial in a goroutine: onEvent must NOT block (realtime WS read loop).
		// The transfer-confirm phrase is already spoken via tenant TTS
		// (PlayTransferConfirmReply) before this runs.
		go func() {
			TriggerTransferToAgent(context.Background(), cs.CallID, lg)
		}()
	}

	runForcedTransferConfirm := func(execute, triggerDial bool) {
		if !confirmReplyBusy.CompareAndSwap(false, true) {
			return
		}
		defer func() {
			confirmReplyBusy.Store(false)
			suppressOmniAssistant.Store(false)
		}()
		if ms == nil || ms.GetContext().Err() != nil {
			return
		}
		lg.Info("sip voice (realtime): playing fixed transfer-confirm reply",
			zap.String("call_id", cs.CallID),
			zap.Bool("execute", execute),
			zap.String("text", transferConfirmReplyText(execute)),
		)
		if err := PlayTransferConfirmReply(ms.GetContext(), cs, execute, lg); err != nil {
			lg.Warn("sip voice (realtime): fixed transfer-confirm reply failed",
				zap.String("call_id", cs.CallID),
				zap.Error(err),
			)
		}
		if triggerDial {
			if useTransferTool {
				markSIPTransferPending(cs.CallID)
			}
			executeTransfer("forced_confirm")
		}
	}

	waitRealtimePlaybackDrain := func(ctx context.Context, maxWait time.Duration) {
		deadline := time.Now().Add(maxWait)
		for time.Now().Before(deadline) {
			if ctx.Err() != nil {
				return
			}
			aiBufMu.Lock()
			empty := len(aiBuf) == 0
			aiBufMu.Unlock()
			if empty {
				time.Sleep(bargeInGrace)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	onEvent := func(ev realtime.Event) {
		switch ev.Type {
		case realtime.EventSessionOpen:
			lg.Info("sip voice (realtime): session open",
				zap.String("call_id", cs.CallID),
				zap.String("vendor", ev.Vendor),
			)
			syncRealtimeTransferInstructions(agent, baseSystemPrompt, cs.CallID, transferConfirmRequired)

		case realtime.EventSessionClose:
			lg.Info("sip voice (realtime): session closed",
				zap.String("call_id", cs.CallID),
			)

		case realtime.EventUserSpeechStarted:
			// Three-step barge-in:
			//
			//   1. Cancel — tell DashScope to stop generating audio
			//      for the current response. WS round-trip is ~50ms.
			//   2. Drain — drop every PCM packet already queued in
			//      the SIP RTP transport so the caller stops hearing
			//      the AI mid-sentence within ~one packet (20ms).
			//   3. Mute — drop EventAssistantAudio chunks that arrive
			//      while the user is still talking AND for a short
			//      grace period after they stop, so in-flight chunks
			//      from the cancelled response (still in transit
			//      between cancel and ack on the WS) don't leak out.
			//
			// Without steps 2 & 3 the caller hears AI finish a
			// sentence over their own voice, which is exactly the
			// "doesn't barge in properly" symptom the user reported.
			if agent != nil {
				_ = agent.Cancel()
			}
			// Flush in-process jitter buffer (player goroutine's
			// pending PCM) AND the SIP RTP transport queue.
			aiBufMu.Lock()
			bufBytes := len(aiBuf)
			aiBuf = aiBuf[:0]
			aiBufMu.Unlock()
			droppedPkts := 0
			if ms != nil {
				droppedPkts = ms.DrainOutputs()
			}
			aiMutedUntilNs.Store(time.Now().Add(bargeInLongMute).UnixNano())
			ttsPlaying.Store(false)
			lg.Info("sip voice (realtime): user speech started (server VAD) — barge-in",
				zap.String("call_id", cs.CallID),
				zap.Int("dropped_queued_packets", droppedPkts),
				zap.Int("dropped_buffered_bytes", bufBytes),
			)

		case realtime.EventUserSpeechEnded:
			// Pull the mute deadline in to "now + grace". The grace
			// covers the WS round-trip for `response.cancel` plus a
			// touch of slack — any AssistantAudio still arriving from
			// the cancelled response inside this window gets dropped.
			// After grace, the next response's audio flows freely.
			aiMutedUntilNs.Store(time.Now().Add(bargeInGrace).UnixNano())
			lg.Info("sip voice (realtime): user speech ended (server VAD)",
				zap.String("call_id", cs.CallID),
				zap.Duration("ai_mute_grace", bargeInGrace),
			)

		case realtime.EventUserTranscript:
			if !ev.Final {
				return
			}
			if gateTransferOrShutdown() {
				return
			}
			assistantMu.Lock()
			assistantBuf.Reset()
			assistantMu.Unlock()
			userText.Store(ev.Text)
			lg.Info("sip voice (realtime): user transcript",
				zap.String("call_id", cs.CallID),
				zap.String("text", ev.Text),
			)
			if c := recordSIPTransferIntent(cs.CallID, ev.Text); c > 0 && realtimeMatchTransferIntent("user", ev.Text, nil) {
				lg.Info("sip voice (realtime): transfer intent recorded",
					zap.String("call_id", cs.CallID),
					zap.Int("count", c),
					zap.Int("required", transferConfirmRequired),
				)
			}
			syncRealtimeTransferInstructions(agent, baseSystemPrompt, cs.CallID, transferConfirmRequired)
			if realtimeMatchTransferIntent("user", ev.Text, nil) {
				allowed, cnt := sipTransferMayExecute(cs.CallID, transferConfirmRequired)
				if transferConfirmRequired > 1 || allowed {
					suppressOmniAssistant.Store(true)
					sipRealtimeFlushAssistantOutput(ms, &aiBufMu, &aiBuf, &aiMutedUntilNs, &ttsPlaying, 60*time.Second)
					sipRealtimeSafeCancel(agent)
					go runForcedTransferConfirm(allowed, allowed)
					lg.Info("sip voice (realtime): transfer intent — fixed confirm reply scheduled",
						zap.String("call_id", cs.CallID),
						zap.Int("count", cnt),
						zap.Int("required", transferConfirmRequired),
						zap.Bool("execute", allowed),
					)
				} else if allowed {
					executeTransfer("user_keyword")
				}
			}

		case realtime.EventAssistantText:
			if ev.Final {
				assistantMu.Lock()
				full := assistantBuf.String()
				assistantBuf.Reset()
				assistantMu.Unlock()
				// Server may emit `done` with the full transcript and
				// without prior delta events — prefer the explicit
				// `done` payload when present.
				if strings.TrimSpace(ev.Text) != "" {
					full = ev.Text
				}
				clean := realtimeStripMarker(full)
				lg.Info("sip voice (realtime): assistant final",
					zap.String("call_id", cs.CallID),
					zap.String("text", clean),
				)
				// Transfer marker (legacy path when FC tools are off).
				if !useTransferTool && realtimeMatchTransferIntent("assistant", full, nil) {
					if allowed, _ := sipTransferMayExecute(cs.CallID, transferConfirmRequired); allowed {
						executeTransfer("assistant_marker")
					}
				}
				// FC path: dial when confirm gate satisfied. Multi-step confirm
				// uses forced TTS on user transcript — ignore late Omni finals here.
				if useTransferTool && !transferStartedLocal.Load() && transferConfirmRequired <= 1 {
					toolPending := isSIPTransferPending(cs.CallID)
					allowed, _ := sipTransferMayExecute(cs.CallID, transferConfirmRequired)
					if toolPending && allowed {
						go func() {
							waitRealtimePlaybackDrain(ms.GetContext(), 15*time.Second)
							if ms == nil || ms.GetContext().Err() != nil {
								return
							}
							executeTransfer("function_call")
						}()
					}
				}
				if suppressOmniAssistant.Load() || confirmReplyBusy.Load() {
					allowed, _ := sipTransferMayExecute(cs.CallID, transferConfirmRequired)
					if allowed {
						clean = transferConfirmExecuteReplyZH
					} else {
						clean = transferConfirmNormalReplyZH
					}
				} else if transferConfirmRequired > 1 && realtimeMatchTransferAckPhrase(clean) {
					allowed, cnt := sipTransferMayExecute(cs.CallID, transferConfirmRequired)
					if !allowed && cnt > 0 {
						lg.Debug("sip voice (realtime): ignored late model transfer wording (fixed TTS path)",
							zap.String("call_id", cs.CallID),
							zap.String("assistant_text", clean),
						)
						clean = transferConfirmNormalReplyZH
					}
				}
				// Hangup phrase: same semantics as pipeline mode —
				// only assistant farewells release.
				if !gateTransferOrShutdown() && realtimeMatchHangupIntent(clean, hangupPhrases) {
					lg.Info("sip voice (realtime): hangup phrase detected", zap.String("call_id", cs.CallID))
					// Same release path as pipeline mode (see
					// voice_tenant_loader.go AnnounceWAV); short-
					// circuits the call leg server-side.
					RequestSIPHangup(cs.CallID)
				}
				// CDR — one row per final assistant turn.
				go RecordDialogTurn(context.Background(), cs.CallID, DialogTurn{
					ASRText:     stringOrEmpty(userText.Load()),
					LLMText:     clean,
					ASRProvider: ev.Vendor,
					LLMModel:    ev.Vendor + "_realtime",
					TTSProvider: ev.Vendor,
					Trigger:     "realtime",
					PipelineMs:  int(time.Duration(time.Now().UnixNano() - turnT0.Load()).Milliseconds()),
				})
				turnT0.Store(0)
				return
			}
			// Delta: accumulate so the final-only branch above can log
			// a coherent reply even if the server skips `done.transcript`.
			assistantMu.Lock()
			assistantBuf.WriteString(ev.Text)
			assistantMu.Unlock()
			if turnT0.Load() == 0 {
				turnT0.Store(time.Now().UnixNano())
			}

		case realtime.EventAssistantAudio:
			// Hard gate: when the call is in transfer (or torn down),
			// drop AI audio so the caller hears the hold music / agent
			// only. This is the critical "AI 不抢话" guarantee the
			// pipeline path enforces via IsTransferInProgress at the
			// pipe.SetTextCallback level.
			if gateTransferOrShutdown() || suppressOmniAssistant.Load() {
				return
			}
			// Barge-in mute: drop in-flight chunks from the cancelled
			// response. Cleared automatically by the deadline set in
			// EventUserSpeechEnded (now + grace). Don't promote to a
			// `lg.Info` per chunk — we'd spam the log during a 5-second
			// utterance — but log the first dropped chunk per turn so
			// production can see barge-in is working.
			if dl := aiMutedUntilNs.Load(); dl > 0 && time.Now().UnixNano() < dl {
				return
			}
			pcm := ev.AudioPC
			if len(pcm) == 0 {
				return
			}
			if audioRecvFirst.CompareAndSwap(false, true) {
				lg.Info("sip voice (realtime): first AI audio chunk received",
					zap.String("call_id", cs.CallID),
					zap.Int("bytes", len(pcm)),
				)
			}
			outRate := agentOutputRateOrDefault(env)
			pcmOut := pcm
			atBridgeRate := outRate == pcmBridgeSR
			if !atBridgeRate && len(pcm) >= 2 {
				if out, err := media.ResamplePCM(pcm, outRate, pcmBridgeSR); err == nil && len(out) > 0 {
					pcmOut = out
					atBridgeRate = true
				}
			}
			if !atBridgeRate {
				return
			}
			// Hand the chunk off to the player goroutine. The player
			// owns: pacing, 20ms framing, RTP push, and recorder write
			// (recorder must track *playback* timing, not receipt
			// timing — otherwise a barge-in flush would leave the
			// recording with audio that never reached the caller).
			aiBufMu.Lock()
			aiBuf = append(aiBuf, pcmOut...)
			aiBufMu.Unlock()
			ttsPlaying.Store(true)

		case realtime.EventAssistantTurnEnd:
			ttsPlaying.Store(false)

		case realtime.EventError:
			if ev.Fatal {
				lg.Error("sip voice (realtime): fatal session error",
					zap.String("call_id", cs.CallID),
					zap.String("vendor", ev.Vendor),
					zap.Error(ev.Err),
				)
				closeOnce.Do(func() {
					if agent != nil {
						_ = agent.Close()
					}
				})
				return
			}
			lg.Warn("sip voice (realtime): non-fatal error",
				zap.String("call_id", cs.CallID),
				zap.Error(ev.Err),
			)
		}
	}

	// Build the agent. Construction is cheap (no network); Start opens
	// the WebSocket. We do both before registering the PCM processor so
	// any cred/handshake failure surfaces as a clean attach error and
	// the call leg falls back to config_error.wav.
	// SystemPrompt carries augment/rules; tenant instructions stay in
	// realtime_config.instructions and are merged inside aliyunomni.New.
	rtOpts := realtime.Options{
		SystemPrompt: mergeRealtimeInstructions(rulesBlock, transferConfirmSessionHint(cs.CallID, transferConfirmRequired)),
		Voice:            realtimeVoiceFromEnv(env),
		InputSampleRate:  realtimeAgentInputRate,
		OutputSampleRate: agentOutputRateOrDefault(env),
		// Default to the documented floor (0.6 for Qwen-Omni) so
		// replies are as deterministic / script-adherent as the
		// vendor permits. Telephony deployments value "say the same
		// thing the same way every time" over creative variation;
		// per-tenant override via realtime_config.temperature still
		// works (see realtimeTemperatureFromEnv).
		Temperature: realtimeTemperatureFromEnv(env),
		OnEvent:     onEvent,
	}
	if useTransferTool {
		rtOpts.Tools = SIPRealtimeTools()
		rtOpts.ToolHandler = newSIPRealtimeToolHandler(cs.CallID, transferConfirmRequired, lg, executeTransfer)
	}
	a, err := realtime.NewAgentFromCredential(env.RealtimeConfigRaw, rtOpts)
	if err != nil {
		return fmt.Errorf("sip conversation: realtime agent: %w", err)
	}
	agent = a
	if err := a.Start(ctx); err != nil {
		_ = a.Close()
		return fmt.Errorf("sip conversation: realtime start: %w", err)
	}
	lg.Info("sip voice (realtime) attached",
		zap.String("call_id", cs.CallID),
		zap.String("provider", env.RealtimeProvider),
		zap.Bool("transfer_tools", useTransferTool),
		zap.Int("transfer_confirm_required", transferConfirmRequired),
		zap.Int("operator_prompt_chars", len(operatorCore)),
		zap.Int("rules_prompt_chars", len(rulesBlock)),
		zap.Int("tools_count", len(rtOpts.Tools)),
		zap.Int("pcm_bridge_hz", pcmBridgeSR),
		zap.Int("agent_in_hz", realtimeAgentInputRate),
		zap.Int("agent_out_hz", agentOutputRateOrDefault(env)),
	)

	// Caller PCM → agent. Same registration shape as pipeline mode so
	// future media-stack work (e.g. echo cancellation) lands once and
	// covers both paths.
	proc := media.NewPacketProcessor("sip-voice-realtime-feed", media.PriorityHigh,
		func(_ context.Context, _ *media.MediaSession, packet media.MediaPacket) error {
			ap, ok := packet.(*media.AudioPacket)
			if !ok || ap == nil || len(ap.Payload) == 0 {
				return nil
			}
			if ap.IsSynthesized {
				return nil
			}
			pcm := ap.Payload
			cs.WriteCallerPCM(pcm)
			// Hard gate the upstream half too: once transfer started,
			// the caller's voice is meant for the human agent — the
			// realtime model must not "hear" it. This also ensures the
			// model doesn't queue a reply that would race with the
			// hold music when transfer fails over.
			if gateTransferOrShutdown() {
				return nil
			}
			feed := pcm
			if pcmBridgeSR != realtimeAgentInputRate {
				out, err := media.ResamplePCM(pcm, pcmBridgeSR, realtimeAgentInputRate)
				if err != nil || len(out) == 0 {
					return nil
				}
				feed = out
			}
			// Welcome-playback gate: while welcomePlaying is set, we
			// don't feed caller PCM to the agent so the model's VAD
			// doesn't trigger on welcome echo (caller's mic picks up
			// the WAV) and we don't burn DashScope budget on
			// welcome-period audio. Pipeline mode does the same.
			if welcomePlaying.Load() {
				return nil
			}
			if err := a.PushAudio(feed); err != nil {
				if errors.Is(err, realtime.ErrAgentClosed) {
					return nil
				}
				lg.Debug("sip voice (realtime): push audio",
					zap.String("call_id", cs.CallID),
					zap.Error(err),
				)
				return nil
			}
			// First-frame log + periodic sample. If pushedFirst never
			// fires, caller audio isn't reaching the model — most
			// often a media-stack registration issue (proc not wired)
			// or codec mismatch (caller PCM all zero / misaligned).
			if pushedFirst.CompareAndSwap(false, true) {
				lg.Info("sip voice (realtime): first caller PCM pushed to agent",
					zap.String("call_id", cs.CallID),
					zap.Int("bytes", len(feed)),
				)
			}
			if n := pushedFrames.Add(1); n%500 == 0 {
				// 500 frames @ 20 ms = 10 s sampling cadence.
				lg.Debug("sip voice (realtime): caller PCM pushed",
					zap.String("call_id", cs.CallID),
					zap.Uint64("total_frames", n),
				)
			}
			return nil
		})
	ms.RegisterProcessor(proc)

	// Tear the agent down with the media session so we don't leak WS
	// goroutines after the call ends.
	go func() {
		<-ms.GetContext().Done()
		closeOnce.Do(func() { _ = a.Close() })
	}()

	// Start the 20ms-paced AI PCM player. Runs until ms.GetContext is
	// cancelled (call ends) or the player is explicitly cancelled. One
	// frame per tick; underrun → skip (don't synthesize silence — see
	// the comment in voicedialog/tts_segmenter.go::ttsUnderrunSilenceMaxMs
	// for why zero-fill produces "电流音"). First frame waits for the
	// jitter prebuffer to avoid an early underrun.
	bytesPerFrame := pcmBridgeSR * 2 * 20 / 1000 // bridge-Hz * 16-bit * 20 ms
	if bytesPerFrame <= 0 {
		bytesPerFrame = 320 // 8kHz × 16-bit × 20ms fallback
	}
	prebufferBytes := bytesPerFrame * aiPrebufferBytesAt20msFrames
	playerCtx, playerCancelFn := context.WithCancel(ms.GetContext())
	playerOnce.Do(func() {
		go func() {
			defer playerCancelFn()
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			started := false
			frame := make([]byte, bytesPerFrame)
			for {
				select {
				case <-playerCtx.Done():
					return
				case <-ticker.C:
				}
				aiBufMu.Lock()
				avail := len(aiBuf)
				if !started {
					// Wait for jitter prebuffer before sending the
					// very first frame of the call. Once started,
					// underrun is a tolerated transient (skip the
					// tick, don't reset prebuffer).
					if avail < prebufferBytes {
						aiBufMu.Unlock()
						continue
					}
					started = true
				}
				if avail < bytesPerFrame {
					aiBufMu.Unlock()
					continue
				}
				copy(frame, aiBuf[:bytesPerFrame])
				// Pop: shift the slice down. For typical N=320 bytes
				// and buffers <100 KB this is cheap; growth is bounded
				// by Omni emitting ~600ms-worth of audio max before
				// pause.
				aiBuf = append(aiBuf[:0], aiBuf[bytesPerFrame:]...)
				aiBufMu.Unlock()
				cs.WriteAIPCM(frame)
				ms.SendToOutput("sip-voice-realtime", &media.AudioPacket{
					Payload:       append([]byte(nil), frame...),
					IsSynthesized: true,
				})
			}
		}()
	})

	cs.StartOnACK()
	if w := welcomeWaitFirstRTPMs(); w > 0 {
		waitFirstRTPBeforeWelcome(ms.GetContext(), cs, lg, w)
	}

	// Welcome-WAV: same source resolution as pipeline mode (per-DID URL on
	// TrunkNumber.WelcomeAudioURL → fallback to scripts/welcome.wav). Plays
	// on a goroutine so attach returns immediately. While playing,
	// `welcomePlaying` blocks caller PCM from reaching the agent — see the
	// proc gate above. After the WAV finishes (or fails), the flag clears
	// and the agent starts hearing the caller. This gives the user a clear
	// "call connected" cue and is mandatory for usability: Qwen-Omni waits
	// for user speech (server VAD) before greeting, so a silent connect
	// looks broken to the caller.
	welcomePCM, src, werr := loadWelcomePCM(ms.GetContext(), cs.CallID, pcmBridgeSR, lg)
	if werr != nil {
		lg.Warn("sip voice (realtime): welcome audio load failed, skipping welcome",
			zap.String("call_id", cs.CallID),
			zap.String("source", string(src)),
			zap.Error(werr),
		)
	} else if len(welcomePCM) > 0 && src != welcomeSourceSkip {
		welcomePlaying.Store(true)
		go func() {
			defer welcomePlaying.Store(false)
			if err := playWelcomePCM(ms.GetContext(), welcomePCM, ms, lg, pcmBridgeSR, cs.WriteAIPCM); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				lg.Warn("sip voice (realtime) welcome playback failed",
					zap.String("call_id", cs.CallID),
					zap.String("source", string(src)),
					zap.Error(err))
				return
			}
			lg.Info("sip voice (realtime) welcome playback finished — agent now listens",
				zap.String("call_id", cs.CallID),
				zap.String("source", string(src)),
				zap.Int("bytes", len(welcomePCM)),
			)
		}()
	}
	return nil
}

// agentOutputRateOrDefault picks the cloud-side audio rate for the
// realtime model. Tenant JSON may override via `outputSampleRate`;
// otherwise we use 24000 (Qwen-Omni / GPT-4o realtime native rate).
//
// Note on the recorder "sample-rate mismatch" warning: the recorder
// computes `implied_hz = observed_samples / wall_clock_seconds` over
// the whole call. The AI doesn't emit audio continuously (it pauses
// between turns and between chunks within a turn), so implied_hz is
// always lower than the configured rate for an AI leg. The warning is
// informational, not a real rate misconfiguration — do not "fix" it by
// changing the default rate.
func agentOutputRateOrDefault(env VoiceEnv) int {
	if env.RealtimeConfigRaw != nil {
		for _, k := range []string{"outputSampleRate", "output_sample_rate", "sampleRate", "sample_rate"} {
			if v, ok := env.RealtimeConfigRaw[k]; ok {
				switch t := v.(type) {
				case int:
					if t > 0 {
						return t
					}
				case int64:
					if t > 0 {
						return int(t)
					}
				case float64:
					if t > 0 {
						return int(t)
					}
				}
			}
		}
	}
	return 24000
}

// realtimeTemperatureFromEnv resolves the sampling temperature for the
// realtime model. Resolution order:
//
//  1. Per-tenant override in realtime_config: the single canonical key
//     is `temperature`. Numeric (or numeric string), ignored if invalid
//     or <= 0.
//  2. Provider floor — return 0.6 for Aliyun Qwen-Omni (the documented
//     minimum). Other providers fall back to 0 (= "use vendor default")
//     until we benchmark them.
//
// We default low because telephony deployments universally want
// script-adherent answers ("I asked the same question twice and got two
// completely different answers" is a top operator complaint). The
// in-client clamp in pkg/realtime/aliyunomni reasserts the range so an
// out-of-range tenant config can't slip through.
func realtimeTemperatureFromEnv(env VoiceEnv) float64 {
	if env.RealtimeConfigRaw != nil {
		if v, ok := env.RealtimeConfigRaw["temperature"]; ok {
			switch t := v.(type) {
			case float64:
				if t > 0 {
					return t
				}
			case float32:
				if t > 0 {
					return float64(t)
				}
			case int:
				if t > 0 {
					return float64(t)
				}
			case int64:
				if t > 0 {
					return float64(t)
				}
			case string:
				if s := strings.TrimSpace(t); s != "" {
					if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
						return f
					}
				}
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(env.RealtimeProvider)) {
	case "aliyun_omni", "aliyun-omni", "qwen_omni", "qwen-omni":
		return 0.6
	}
	return 0
}

// sipRealtimeSafeCancel sends response.cancel; Omni may return "none active
// response" if the turn already ended — treated as non-fatal in aliyunomni.
func sipRealtimeSafeCancel(agent realtime.Agent) {
	if agent != nil {
		_ = agent.Cancel()
	}
}

// sipRealtimeFlushAssistantOutput drops queued Omni PCM without Cancel.
func sipRealtimeFlushAssistantOutput(
	ms *media.MediaSession,
	aiBufMu *sync.Mutex,
	aiBuf *[]byte,
	aiMutedUntilNs *atomic.Int64,
	ttsPlaying *atomic.Bool,
	muteFor time.Duration,
) {
	if aiBufMu != nil && aiBuf != nil {
		aiBufMu.Lock()
		*aiBuf = (*aiBuf)[:0]
		aiBufMu.Unlock()
	}
	if ms != nil {
		_ = ms.DrainOutputs()
	}
	if aiMutedUntilNs != nil && muteFor > 0 {
		aiMutedUntilNs.Store(time.Now().Add(muteFor).UnixNano())
	}
	if ttsPlaying != nil {
		ttsPlaying.Store(false)
	}
}

// realtimeInstructionsFromEnv returns tenant realtime_config.instructions
// (primary persona). This must not be dropped when attaching the WS agent.
func realtimeInstructionsFromEnv(env VoiceEnv) string {
	if env.RealtimeConfigRaw == nil {
		return ""
	}
	for _, k := range []string{"instructions", "systemPrompt", "system_prompt"} {
		if v, ok := env.RealtimeConfigRaw[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// realtimeVoiceFromEnv extracts the AI voice name. Empty → provider default.
func realtimeVoiceFromEnv(env VoiceEnv) string {
	if env.RealtimeConfigRaw == nil {
		return ""
	}
	for _, k := range []string{"voice", "voiceId", "voice_id"} {
		if v, ok := env.RealtimeConfigRaw[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func stringOrEmpty(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
