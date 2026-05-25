// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

// llmStage — real LLM as a pipeline.Stage.
//
// Replaces llmStub when WithLLMService is set on the Engine. Sits
// between the ASR stage (KindTextFinal user transcripts) and the TTS
// stage (KindAIText deltas + KindAITextDone end-of-turn marker).
//
// Design constraints
// ==================
//
//  1. NO upward import to pkg/llm. Cascaded stays vendor-neutral.
//     The slim LLMService interface below covers OpenAI/Coze/Ollama/
//     Alibaba via a single shape. Production wiring at the SIP/dialog
//     seam (PR-9d) adapts pkg/llm.LLMProvider → LLMService with a
//     small shim that picks the right method per provider type
//     (Alibaba → Query, others → QueryStream).
//
//  2. The stage triggers LLM dispatch only on KindTextFinal (final
//     user transcript). KindTextInterim is forwarded unchanged so
//     downstream observers can see partial state but doesn't kick
//     off duplicate generations.
//
//  3. One generation per turn, serialised. While a previous turn is
//     still streaming, a new KindTextFinal cancels the in-flight ctx
//     and starts a new one. This matches legacy attachVoiceInner
//     behaviour where the user's barge-in / new utterance always
//     wins over a still-speaking AI turn.
//
//  4. LLM errors are observability-only. A failed generation logs +
//     emits KindAITextDone (so the TTS stage can flush residual
//     audio and the engine clears its ttsPlaying bit) and then waits
//     for the next turn. The pipeline does NOT abort on LLM errors —
//     the call continues and the user can try again.
//
// Lifecycle
// =========
//
//   Run loop:
//     ctx done                 → cancel any in-flight gen, return ctx.Err()
//     in → KindTextFinal       → cancel any in-flight gen, dispatch new
//     in → other Kind          → passthrough
//     in closed                → cancel in-flight gen, drain, exit
//     gen → delta              → emit KindAIText
//     gen → done               → emit KindAITextDone

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/pipeline"
)

// LLMService is the slim interface llmStage drives. The single
// StreamReply method covers both real streaming providers (OpenAI,
// Ollama, Coze, etc. — invoked with onDelta firing per token chunk)
// and single-shot providers (Alibaba — onDelta fires once with the
// full text and isComplete=true).
//
// Concurrency:
//
//   - StreamReply is called serially from the stage Run goroutine
//     (one in-flight call at a time). The stage cancels via ctx
//     before issuing a new call.
//
//   - onDelta is invoked from the StreamReply goroutine; the stage
//     forwards via a buffered channel back to its main select loop.
//     onDelta MUST return ctx.Err() (or wrap it) when the caller
//     cancels mid-stream so providers can short-circuit.
type LLMService interface {
	// StreamReply generates a response for userText and invokes
	// onDelta zero or more times with isComplete=false (intermediate
	// fragments) and exactly once with isComplete=true (terminal
	// marker, payload may be empty).
	//
	// Return value: full response text + error. The full text is
	// used for logging / metrics; the streaming side emits via
	// onDelta deltas, NOT via the return value.
	//
	// ctx cancellation: implementations MUST honour ctx and return
	// ctx.Err() (possibly wrapped) so the stage can teardown
	// promptly on call hangup or new-turn pre-emption.
	StreamReply(ctx context.Context, userText string, onDelta func(text string, isComplete bool) error) (full string, err error)
}

type llmStage struct {
	svc       LLMService
	bufSize   int
	onErrTurn func(turnText string, err error) // optional observer for failed turns
}

type llmStageOption func(*llmStage)

// withLLMDeltaBuffer caps the in-flight delta queue. Most LLMs emit
// 30-100 tokens/sec; default 256 ≈ 2.5s of buffer at the high end,
// well above the worst-case stage drain latency. Tunable for tests.
func withLLMDeltaBuffer(n int) llmStageOption {
	return func(s *llmStage) {
		if n > 0 {
			s.bufSize = n
		}
	}
}

// withLLMTurnErrorObserver installs a callback invoked when a turn
// fails. The handler runs synchronously from the Run goroutine; do
// not block. Default nil = log only (via the stage logger).
func withLLMTurnErrorObserver(fn func(turnText string, err error)) llmStageOption {
	return func(s *llmStage) { s.onErrTurn = fn }
}

func newLLMStage(svc LLMService, opts ...llmStageOption) *llmStage {
	s := &llmStage{
		svc:     svc,
		bufSize: 256,
	}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// Name implements pipeline.Stage.
func (llmStage) Name() string { return "llm" }

// llmDelta is the internal generator → Run-loop carrier. Wraps the
// callback signature plus a turn-id so out-of-order deliveries from
// a cancelled-but-still-emitting goroutine can be filtered.
type llmDelta struct {
	turnID  uint64
	text    string
	isFinal bool
}

// Run implements pipeline.Stage.
func (s *llmStage) Run(ctx context.Context, in <-chan pipeline.Frame, out chan<- pipeline.Frame, lg engine.Logger) error {
	defer close(out)
	if s.svc == nil {
		// Nil service → passthrough (mirrors vad / asr nil handling).
		lg.Debug("llm stage: nil service; passthrough mode")
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case f, ok := <-in:
				if !ok {
					return nil
				}
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
			}
		}
	}

	deltas := make(chan llmDelta, s.bufSize)

	// In-flight turn state. The mutex guards against the rare race
	// where a stale turn's onDelta is still firing while we're
	// installing the next turn's ctx.
	var (
		genMu      sync.Mutex
		genCancel  context.CancelFunc
		genWG      sync.WaitGroup
		currentTID uint64
	)

	cancelInFlight := func() {
		genMu.Lock()
		c := genCancel
		genCancel = nil
		genMu.Unlock()
		if c != nil {
			c()
		}
	}

	dispatchTurn := func(parent context.Context, userText string) {
		userText = strings.TrimSpace(userText)
		if userText == "" {
			return
		}
		// Pre-empt any in-flight turn before installing the new one.
		cancelInFlight()
		// Wait briefly for the previous goroutine to wind down so
		// its trailing callbacks can't poison the new turn's
		// delta stream. Worst case: the previous gen never returns —
		// we don't hold genMu during the Wait so deadlock is
		// impossible, but we DO bound it with the parent ctx so
		// stage shutdown still works.
		// Note: we don't actually call genWG.Wait() here because the
		// turnID filter on the deltas channel already protects
		// downstream. Skipping the wait keeps turn-pre-emption fast.

		genCtx, cancel := context.WithCancel(parent)
		genMu.Lock()
		currentTID++
		tid := currentTID
		genCancel = cancel
		genMu.Unlock()

		genWG.Add(1)
		go func() {
			defer genWG.Done()
			defer cancel()
			// sawTerminal: once the provider has delivered an
			// isComplete=true delta we MUST NOT fire the safety-net
			// terminal below — that would cause the stage to emit
			// two KindAITextDone frames per turn (the TTS stage's
			// Finalize() would then run twice, adding a spurious
			// audio cliff). The safety net exists solely for
			// providers that return without ever invoking onDelta
			// (e.g. alibaba single-shot on empty replies, or any
			// transient error path).
			var sawTerminal bool
			_, err := s.svc.StreamReply(genCtx, userText, func(text string, isComplete bool) error {
				if genCtx.Err() != nil {
					return genCtx.Err()
				}
				if isComplete {
					sawTerminal = true
				}
				select {
				case deltas <- llmDelta{turnID: tid, text: text, isFinal: isComplete}:
				case <-genCtx.Done():
					return genCtx.Err()
				}
				return nil
			})
			if !sawTerminal {
				select {
				case deltas <- llmDelta{turnID: tid, text: "", isFinal: true}:
				case <-genCtx.Done():
				}
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				lg.Warn("llm stage: turn failed",
					engine.F("err", err.Error()),
					engine.F("turn_text", userText))
				if s.onErrTurn != nil {
					s.onErrTurn(userText, err)
				}
			}
		}()
	}

	// On exit: cancel in-flight + wait so stale callbacks don't write
	// into a closed `deltas` channel after we return. We DO close
	// deltas to allow the dispatch goroutine to drop sends gracefully
	// via the genCtx-done branch.
	defer func() {
		cancelInFlight()
		genWG.Wait()
	}()

	inOpen := true
	for {
		var inCh <-chan pipeline.Frame
		if inOpen {
			inCh = in
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f, ok := <-inCh:
			if !ok {
				inOpen = false
				break
			}
			switch f.Kind {
			case pipeline.KindTextFinal:
				// Pass the user transcript through (UI / metrics
				// observers may want it) AND kick off generation.
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
				dispatchTurn(ctx, f.Text)
			default:
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
			}
		case d := <-deltas:
			// Drop stale turn deltas — a previously-cancelled turn
			// can keep emitting briefly while the new turn is
			// already starting.
			genMu.Lock()
			active := d.turnID == currentTID
			genMu.Unlock()
			if !active {
				continue
			}
			if d.isFinal {
				// Emit any non-empty trailing text first, then the
				// turn-done marker.
				if t := strings.TrimSpace(d.text); t != "" {
					if err := sendOrCancel(ctx, out, pipeline.Frame{
						Kind: pipeline.KindAIText,
						Text: t,
					}); err != nil {
						return err
					}
				}
				if err := sendOrCancel(ctx, out, pipeline.Frame{
					Kind: pipeline.KindAITextDone,
				}); err != nil {
					return err
				}
			} else {
				if t := d.text; t != "" {
					if err := sendOrCancel(ctx, out, pipeline.Frame{
						Kind: pipeline.KindAIText,
						Text: t,
					}); err != nil {
						return err
					}
				}
			}
		}
		if !inOpen {
			// Drain phase. Input is closed and we MUST flush any
			// in-flight generator's deltas before exiting — a
			// non-blocking drain would miss deltas that haven't
			// been produced yet because the LLM goroutine is still
			// running. Block on (generator-done OR delta-arrived OR
			// ctx-done) until the generator has emitted its
			// terminal isFinal marker AND the deltas channel is
			// observably empty.
			genDone := make(chan struct{})
			go func() {
				genWG.Wait()
				close(genDone)
			}()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case d := <-deltas:
					if err := s.emitDelta(ctx, out, d, &genMu, &currentTID); err != nil {
						return err
					}
				case <-genDone:
					// Generator finished. Drain any deltas that
					// landed in the buffer between the last select
					// iteration and the close, then exit.
					for {
						select {
						case d := <-deltas:
							if err := s.emitDelta(ctx, out, d, &genMu, &currentTID); err != nil {
								return err
							}
						default:
							return nil
						}
					}
				}
			}
		}
	}
}

// emitDelta is the shared delta-emission path used by both the main
// loop and the drain phase. Filters stale-turn deltas via turnID,
// emits KindAIText for non-empty text, and KindAITextDone on
// isFinal=true. Pulling this out of the main switch reduces ~30 LOC
// of duplication and keeps the drain code at the bottom of Run
// readable.
func (s *llmStage) emitDelta(
	ctx context.Context,
	out chan<- pipeline.Frame,
	d llmDelta,
	mu *sync.Mutex,
	currentTID *uint64,
) error {
	mu.Lock()
	active := d.turnID == *currentTID
	mu.Unlock()
	if !active {
		return nil
	}
	if d.isFinal {
		if t := strings.TrimSpace(d.text); t != "" {
			if err := sendOrCancel(ctx, out, pipeline.Frame{
				Kind: pipeline.KindAIText,
				Text: t,
			}); err != nil {
				return err
			}
		}
		return sendOrCancel(ctx, out, pipeline.Frame{
			Kind: pipeline.KindAITextDone,
		})
	}
	if t := d.text; t != "" {
		return sendOrCancel(ctx, out, pipeline.Frame{
			Kind: pipeline.KindAIText,
			Text: t,
		})
	}
	return nil
}
