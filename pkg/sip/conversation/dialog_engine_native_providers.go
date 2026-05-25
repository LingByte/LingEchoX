package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Phase 3 PR-9e — production-side provider adapters for the native
// cascaded.Engine.
//
// The cascaded engine speaks three slim interfaces:
//
//   - cascaded.ASRRecognizer  (ProcessPCM + Set{Text,Error}Callback)
//   - cascaded.LLMService     (StreamReply)
//   - cascaded.TTSService     (Speak + Finalize)
//
// This file wires them onto the existing production-side packages:
//
//   - sipasr.Pipeline   → ASRRecognizer (interface match — direct
//                         pass-through, no adapter needed beyond the
//                         constructor).
//   - llm.LLMProvider   → LLMService    (signature is identical
//                         modulo the ctx argument; thin adapter).
//   - siptts.Pipeline   → TTSService    (Speak/Finalize match in
//                         name; PCM-emission is construction-time
//                         SendPCMFrame so the adapter stashes a
//                         per-Speak onPCM in atomic.Value).
//
// Every adapter is intentionally minimal — no system-prompt loading,
// no transfer-tool registration, no hotword corrector. Those legacy
// features will move out of attachVoiceInner into native stages of
// their own (intent-detection stage, persistence stage, etc.) in
// follow-up PRs. This file only crosses the impedance line between
// existing concrete providers and the engine seam.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LinByte/VoiceServer/pkg/dialog/cascaded"
	"github.com/LinByte/VoiceServer/pkg/llm"
	"github.com/LinByte/VoiceServer/pkg/recognizer"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/synthesizer"
	sipasr "github.com/LinByte/VoiceServer/pkg/voice/asr"
	siprecorder "github.com/LinByte/VoiceServer/pkg/voice/recorder"
	siptts "github.com/LinByte/VoiceServer/pkg/voice/tts"
	"go.uber.org/zap"
)

// enableNativeStereoRecorder mirrors the legacy attachVoiceInner
// recorder bootstrap. SIP_RECORDER_CHUNK_SECS controls rolling
// upload cadence; storage bucket is configured by pkg/stores.
func enableNativeStereoRecorder(cs *sipSession.CallSession, lg *zap.Logger) {
	if cs == nil {
		return
	}
	cfg := siprecorder.Config{Logger: lg}
	if secs, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SIP_RECORDER_CHUNK_SECS"))); err == nil && secs > 0 {
		cfg.ChunkInterval = time.Duration(secs) * time.Second
	}
	if cs.EnableRecorder(cfg) && lg != nil {
		lg.Info("native cascaded: stereo PCM recorder enabled",
			zap.String("call_id", cs.CallID),
			zap.Duration("chunk_interval", cfg.ChunkInterval),
		)
	}
}

// --- ASR adapter ----------------------------------------------------

// buildNativeCascadedASR constructs a sipasr.Pipeline from the
// tenant's QCloud ASR credentials. Today only QCloud is wired; other
// providers will be added by switching on env.ASRProvider when their
// recognizers move out of the legacy helpers.
//
// The returned *sipasr.Pipeline already satisfies cascaded.ASRRecognizer
// (the interface was carved to match it).
func buildNativeCascadedASR(env VoiceEnv, lg *zap.Logger) (cascaded.ASRRecognizer, error) {
	if strings.TrimSpace(env.ASRAppID) == "" ||
		strings.TrimSpace(env.ASRSecretID) == "" ||
		strings.TrimSpace(env.ASRSecretKey) == "" {
		return nil, fmt.Errorf("native cascaded ASR: missing QCloud credentials")
	}
	opt := recognizer.NewQcloudASROption(env.ASRAppID, env.ASRSecretID, env.ASRSecretKey)
	if env.ASRModelType != "" {
		opt.ModelType = env.ASRModelType
	}
	asrOutRate := 16000
	if strings.Contains(strings.ToLower(opt.ModelType), "8k") {
		asrOutRate = 8000
	}
	pipe, err := sipasr.New(sipasr.Options{
		ASR:        recognizer.NewQcloudASR(opt),
		SampleRate: asrOutRate,
		Channels:   1,
		Logger:     lg,
	})
	if err != nil {
		return nil, fmt.Errorf("native cascaded ASR: pipeline: %w", err)
	}
	return pipe, nil
}

// --- LLM adapter ----------------------------------------------------

// nativeCascadedLLM adapts llm.LLMProvider to cascaded.LLMService.
// Lifetime mirrors the underlying provider: one instance per call.
type nativeCascadedLLM struct {
	provider llm.LLMProvider
	model    string
	// callID is the SIP CallID used by the transfer-suppression
	// gate: while a transfer is in progress IsTransferInProgress
	// returns true and StreamReply short-circuits with an empty
	// reply so the caller side hears the agent, not the AI talking
	// over hold music.
	callID string
}

// StreamReply implements cascaded.LLMService. Delegates to
// QueryStream with a streaming options bundle; ctx is honoured by
// short-circuiting onDelta when ctx fires (the provider itself reads
// its construction-time ctx, so we additionally race on the
// per-call ctx here to avoid a stuck stage during teardown).
//
// Pre-flight gate: if a transfer is already in progress for this
// call we return early with an empty reply (the LLM stage still
// emits the obligatory isComplete=true terminal so downstream
// stages close out the turn cleanly).
func (s *nativeCascadedLLM) StreamReply(
	ctx context.Context,
	userText string,
	onDelta func(text string, isComplete bool) error,
) (string, error) {
	if s == nil || s.provider == nil {
		return "", errors.New("native cascaded LLM: nil provider")
	}
	if s.callID != "" && IsTransferInProgress(s.callID) {
		// Suppress LLM during transfer — same gate the legacy
		// triggerTurn closure applies.
		_ = onDelta("", true)
		return "", nil
	}
	// Record transfer-intent occurrence for the heuristic
	// confirm-count machinery (legacy parity).
	if s.callID != "" {
		_ = recordSIPTransferIntent(s.callID, userText)
	}
	options := llm.QueryOptions{Model: s.model, Stream: true}
	cancelled := false
	guarded := func(segment string, isComplete bool) error {
		if cancelled {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			cancelled = true
			return ctx.Err()
		default:
		}
		return onDelta(segment, isComplete)
	}
	return s.provider.QueryStream(userText, options, guarded)
}

// buildNativeCascadedLLM constructs an llm.LLMProvider, registers
// the transfer-to-agent function tool, and wraps it into a
// cascaded.LLMService.
//
// The tool registration is the same one the legacy attachVoiceInner
// installs: the LLM gets a `transfer_to_agent` function it can call
// when the user asks to be transferred. Tool invocation flips an
// in-memory "transfer pending" bit (markSIPTransferPending); the
// post-turn transfer trigger inside buildNativeTurnPersister
// consumes that bit and fires TriggerTransferToAgent.
//
// Returns the wrapped service AND the underlying provider so
// callers can interrogate provider-specific state (Alibaba carries
// its own pending-action queue separate from the function-tool
// machinery).
func buildNativeCascadedLLM(ctx context.Context, env VoiceEnv, callID string, lg *zap.Logger) (cascaded.LLMService, llm.LLMProvider, error) {
	if strings.TrimSpace(env.LLMProvider) == "" {
		return nil, nil, fmt.Errorf("native cascaded LLM: env.LLMProvider unset")
	}
	if strings.TrimSpace(env.LLMAPIKey) == "" && !strings.EqualFold(env.LLMProvider, "ollama") {
		return nil, nil, fmt.Errorf("native cascaded LLM: missing API key for provider %q", env.LLMProvider)
	}
	model := env.LLMModel
	if model == "" {
		model = "qwen-plus"
	}
	systemPrompt := popSIPCallSystemPrompt(callID)
	endpointOrAppID := llmAPIURLForProvider(env)
	provider, err := llm.NewLLMProvider(ctx, env.LLMProvider, env.LLMAPIKey, endpointOrAppID, systemPrompt)
	if err != nil {
		return nil, nil, fmt.Errorf("native cascaded LLM: provider init: %w", err)
	}
	// Mirror legacy attachVoiceInner: install the transfer function
	// tool. nil-safe under the hood when callID is empty.
	registerSIPTransferTool(provider, callID, TransferConfirmRequired(env), lg)
	return &nativeCascadedLLM{provider: provider, model: model, callID: callID}, provider, nil
}

// --- Turn persister adapter -----------------------------------------

// buildNativeTurnPersister returns a cascaded.TurnPersister that
//
//  1. records every completed turn via RecordDialogTurn (CDR row),
//  2. fires post-turn transfer-to-agent if the LLM either invoked
//     the transfer_to_agent function tool (sets the in-memory
//     pending bit via markSIPTransferPending) OR — for Alibaba —
//     left a transfer_to_agent action in its pending-action queue.
//
// Both behaviours match the legacy attachVoiceInner post-stream
// block. The transfer trigger fires AFTER the AI's confirmation
// reply was synthesised (we observe end-of-LLM, which the engine
// emits AFTER all KindAIText deltas, AFTER the TTS stage finished
// flushing) so the user actually hears the AI's "好的，我马上为您
// 转人工" before the leg gets retargeted.
//
// callID is the SIP CallID under which turns are stored. provider
// is the underlying llm.LLMProvider so we can interrogate
// Alibaba-specific pending actions; pass nil for non-Alibaba paths.
func buildNativeTurnPersister(env VoiceEnv, callID string, provider llm.LLMProvider, lg *zap.Logger) cascaded.TurnPersister {
	asrProv := "qcloud_asr"
	if env.ASRModelType != "" {
		asrProv = env.ASRModelType
	}
	ttsProv := strings.TrimSpace(env.TTSProvider)
	if ttsProv == "" {
		ttsProv = "qcloud_tts"
	}
	llmModel := strings.TrimSpace(env.LLMModel)
	if llmModel == "" {
		llmModel = "qwen-plus"
	}
	return cascaded.TurnPersisterFunc(func(ctx context.Context, rec cascaded.TurnRecord) {
		// Drop empty / interrupted turns — no ASR text AND no AI
		// reply means nothing actionable happened.
		hasContent := strings.TrimSpace(rec.UserText) != "" || strings.TrimSpace(rec.AIText) != ""
		if hasContent {
			dt := DialogTurn{
				ASRText:     rec.UserText,
				LLMText:     rec.AIText,
				ASRProvider: asrProv,
				TTSProvider: ttsProv,
				LLMModel:    llmModel,
				Trigger:     "final",
				LLMFirstMs:  rec.LLMFirstMs,
				LLMWallMs:   rec.LLMWallMs,
				PipelineMs:  rec.PipelineMs,
				At:          rec.CompletedAt,
			}
			bg := context.Background()
			_ = ctx
			go RecordDialogTurn(bg, callID, dt)
		}
		// Post-turn transfer trigger — same precedence the legacy
		// path uses: Alibaba's per-provider pending action wins
		// over the generic function-tool pending bit.
		transferNow := false
		if ap, ok := provider.(*llm.AlibabaProvider); ok && ap != nil {
			if action := ap.ConsumePendingAction(); action == "transfer_to_agent" {
				transferNow = true
			}
		} else if consumeSIPTransferPending(callID) {
			transferNow = true
		}
		if transferNow {
			if lg != nil {
				lg.Info("native cascaded: transfer trigger after AI confirmation",
					zap.String("call_id", callID))
			}
			TriggerTransferToAgent(context.Background(), callID, lg)
		}
	})
}

// --- TTS adapter ----------------------------------------------------

// nativeCascadedTTS adapts *siptts.Pipeline to cascaded.TTSService.
// siptts.Pipeline emits PCM via a construction-time SendPCMFrame
// closure; the cascaded TTSService contract wants a per-Speak onPCM
// callback. We bridge by storing the active onPCM in an atomic
// pointer and having SendPCMFrame read it on every frame.
//
// Concurrency: cascaded.ttsStage calls Speak / Finalize serially
// through its single worker goroutine, so the atomic.Pointer load /
// store sequence never races against a concurrent emission. Reads
// from inside SendPCMFrame may run on the siptts.Pipeline's internal
// goroutine — atomic.Pointer makes that safe.
type nativeCascadedTTS struct {
	pipe   *siptts.Pipeline
	onPCM  atomic.Pointer[func(pcm []byte) error]
	ctx    atomic.Pointer[context.Context]
	logger *zap.Logger
}

// Speak implements cascaded.TTSService. Installs the per-call onPCM,
// updates the underlying pipeline's ctx, and dispatches to
// pipe.Speak. Returns ctx.Err() when ctx fires mid-synthesis so the
// stage can filter context.Canceled cleanly.
func (a *nativeCascadedTTS) Speak(ctx context.Context, text string, onPCM func(pcm []byte) error) error {
	if a == nil || a.pipe == nil {
		return errors.New("native cascaded TTS: nil pipeline")
	}
	a.onPCM.Store(&onPCM)
	a.ctx.Store(&ctx)
	defer a.onPCM.Store(nil)
	// siptts.Pipeline reads its own internal ctx, set via Start. We
	// (re)Start here so each Speak honours the caller's ctx; Start
	// is cheap and idempotent for the same ctx.
	a.pipe.Start(ctx)
	if err := a.pipe.Speak(text); err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

// Finalize implements cascaded.TTSService. Flushes residual audio
// from the pipeline's sub-frame buffer.
func (a *nativeCascadedTTS) Finalize(ctx context.Context, onPCM func(pcm []byte) error) error {
	if a == nil || a.pipe == nil {
		return nil
	}
	a.onPCM.Store(&onPCM)
	defer a.onPCM.Store(nil)
	if err := a.pipe.Finalize(); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

// buildNativeCascadedTTS constructs a siptts.Pipeline for the
// tenant's TTS credentials and wraps it as cascaded.TTSService.
// bridgeRate is the SIP-side PCM rate the engine's MediaPort
// expects; siptts will pace at that rate (or resample upstream — the
// engine's output bridge resamples on the way to the port).
//
// recorderTap is invoked synchronously for every PCM frame sent to
// the caller, BEFORE the per-Speak onPCM. The legacy path uses this
// hook to feed the stereo recorder via cs.WriteAIPCM. Pass nil to
// skip recording.
func buildNativeCascadedTTS(env VoiceEnv, bridgeRate int, recorderTap func(pcm []byte), lg *zap.Logger) (cascaded.TTSService, error) {
	if env.TTSConfigRaw == nil {
		return nil, fmt.Errorf("native cascaded TTS: tenant TTSConfig missing")
	}
	ttsHandle, err := synthesizer.NewStreamingFromCredential(env.TTSConfigRaw)
	if err != nil {
		return nil, fmt.Errorf("native cascaded TTS: provider: %w", err)
	}
	ttsCloudSR := sipVoiceTTSCloudSampleRate(env, ttsHandle.SampleRate, bridgeRate)
	adapter := &nativeCascadedTTS{logger: lg}
	pipe, err := siptts.New(siptts.Config{
		Service:       ttsHandle.Stream,
		SampleRate:    ttsCloudSR,
		Channels:      1,
		FrameDuration: 20 * time.Millisecond,
		// PaceRealtime is OFF for native: the engine's outbound
		// bridge owns RTP-time pacing via MediaPort. Leaving this
		// off avoids double-pacing (which would underrun the
		// downstream RTP scheduler).
		PaceRealtime: false,
		SendPCMFrame: func(frame []byte) error {
			if len(frame) == 0 {
				return nil
			}
			// Recorder tap fires unconditionally so stereo
			// recording captures AI audio even when the engine
			// hasn't installed a Speak callback yet (siptts may
			// emit on residual flush at call end).
			if recorderTap != nil {
				recorderTap(frame)
			}
			cbPtr := adapter.onPCM.Load()
			if cbPtr == nil || *cbPtr == nil {
				return nil
			}
			return (*cbPtr)(frame)
		},
		Logger: lg,
	})
	if err != nil {
		return nil, fmt.Errorf("native cascaded TTS: pipeline: %w", err)
	}
	adapter.pipe = pipe
	return adapter, nil
}
