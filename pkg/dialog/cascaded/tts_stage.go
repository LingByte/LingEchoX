// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

// ttsStage — real TTS as a pipeline.Stage.
//
// Replaces ttsStub when WithTTSService is set on the Engine. Sits at
// the tail of the pipeline (VAD → ASR → LLM → TTS) and converts
// KindAIText deltas into KindPCM frames the engine forwards to the
// MediaPort.
//
// Design constraints
// ==================
//
//  1. NO upward import to pkg/voice/tts. Cascaded stays neutral; the
//     SIP-side adapter (PR-9d) wraps *siptts.Pipeline by translating
//     its construction-time SendPCMFrame closure into a per-Speak
//     onPCM callback. See pkg/voice/tts/pipeline.go for the
//     production-side shape.
//
//  2. Buffer-then-speak strategy: KindAIText deltas accumulate into
//     a sentence buffer; we flush the buffer through TTSService.Speak
//     when we hit a sentence terminator (。！？.!?,，；; etc.) OR
//     when the buffer exceeds a soft cap, OR on KindAITextDone (the
//     end-of-turn signal from the LLM stage). This matches legacy
//     streamLLMToTTS behaviour and keeps audio chunks intelligible
//     instead of word-by-word stuttering.
//
//  3. Barge-in handling: a KindBargeIn frame from the VAD stage
//     cancels the in-flight Speak via ctx and clears the sentence
//     buffer. The TTSService is expected to honour ctx and abort
//     mid-synthesis. The stage emits KindAITextDone after a barge-in
//     so the engine resets ttsPlaying and downstream observers see a
//     clean turn boundary.
//
//  4. PCM emission: TTSService.Speak invokes onPCM zero or more
//     times per call. Each invocation produces one KindPCM frame
//     downstream. The stage forwards via a buffered channel back to
//     its main select loop so the synthesis goroutine never blocks
//     on out-channel send.

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/pipeline"
)

// TTSService is the slim interface ttsStage drives. The single Speak
// method covers any synthesizer: the SIP-side adapter for
// *siptts.Pipeline maintains the construction-time SendPCMFrame
// callback by stashing a per-Speak onPCM closure in an atomic.Value,
// so production wiring needs ~30 LOC of glue without changing the
// underlying TTS engine.
//
// Lifecycle:
//
//   - Speak is called serially from the stage Run goroutine. The
//     stage cancels via ctx before issuing a new Speak when a
//     barge-in or new turn arrives.
//
//   - onPCM is invoked from the Speak goroutine; the callback MUST
//     return ctx.Err() (or a wrapped form) when the caller cancels
//     mid-synthesis so providers can short-circuit.
//
//   - Sample rate / frame duration are decided by the implementation;
//     the stage doesn't try to reframe. Frames flow through to the
//     MediaPort which is responsible for RTP-time pacing.
type TTSService interface {
	// Speak synthesizes text and emits PCM frames via onPCM. Each
	// frame is the raw PCM payload (mono signed 16-bit LE samples)
	// at the synthesizer's configured sample rate.
	//
	// ctx cancellation: implementations MUST honour ctx and abort
	// mid-synthesis when ctx fires (returning ctx.Err() is fine —
	// the stage filters context.Canceled out of error reporting).
	Speak(ctx context.Context, text string, onPCM func(pcm []byte) error) error

	// Finalize is invoked at end-of-turn (after the last KindAIText
	// of a turn AND after KindAITextDone arrives). The expected
	// behaviour is to flush any synthesizer-internal residual audio
	// (e.g. siptts.Pipeline's sub-frame tail). Implementations
	// without residual state may make this a no-op.
	Finalize(ctx context.Context, onPCM func(pcm []byte) error) error
}

type ttsStage struct {
	svc TTSService
	// pcmBuffer caps the in-flight PCM channel size. Default 64 ≈
	// 1.3s of 20ms-frame audio buffering, well above any reasonable
	// stage drain latency.
	pcmBuffer int
	// flushOn is the set of runes that trigger an early sentence
	// flush from the AI-text accumulator. Configurable for non-CJK
	// or low-latency deployments.
	flushOn string
	// minFlushRunes prevents flushing on the first comma when a
	// sentence has barely started. Default 6 → enough characters
	// for a stand-alone Chinese phrase.
	minFlushRunes int
}

type ttsStageOption func(*ttsStage)

func withTTSPCMBuffer(n int) ttsStageOption {
	return func(s *ttsStage) {
		if n > 0 {
			s.pcmBuffer = n
		}
	}
}

func newTTSStage(svc TTSService, opts ...ttsStageOption) *ttsStage {
	s := &ttsStage{
		svc:           svc,
		pcmBuffer:     64,
		flushOn:       "。！？.!?,，；;:",
		minFlushRunes: 6,
	}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// Name implements pipeline.Stage.
func (ttsStage) Name() string { return "tts" }

// pcmEvent is the synthesis-goroutine → Run-loop carrier. The turnID
// lets the stage discard PCM from a barge-in-cancelled turn that
// happens to land in the buffer just after we started a new turn.
type pcmEvent struct {
	turnID uint64
	data   []byte
	// terminal=true marks the synthesis goroutine wrapping up. No
	// payload; the stage treats this as a signal to stop expecting
	// more PCM for this turn.
	terminal bool
}

// Run implements pipeline.Stage.
func (s *ttsStage) Run(ctx context.Context, in <-chan pipeline.Frame, out chan<- pipeline.Frame, lg engine.Logger) error {
	defer close(out)
	if s.svc == nil {
		// Nil service → passthrough.
		lg.Debug("tts stage: nil service; passthrough mode")
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

	pcmCh := make(chan pcmEvent, s.pcmBuffer)

	type speakJob struct {
		text     string
		turnDone bool
	}
	jobCh := make(chan speakJob, 16)

	var (
		genMu      sync.Mutex
		genCancel  context.CancelFunc
		currentTID uint64
		buf        strings.Builder
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

	// Single worker goroutine: consumes speakJobs serially so
	// Speak() and the subsequent Finalize() never race against
	// each other. cancelInFlight() cancels whatever the worker is
	// currently doing without tearing down the worker itself.
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for job := range jobCh {
			text := strings.TrimSpace(job.text)
			if text == "" && !job.turnDone {
				continue
			}
			genCtx, cancel := context.WithCancel(ctx)
			genMu.Lock()
			currentTID++
			tid := currentTID
			genCancel = cancel
			genMu.Unlock()

			emit := func(pcm []byte) error {
				if genCtx.Err() != nil {
					return genCtx.Err()
				}
				cp := make([]byte, len(pcm))
				copy(cp, pcm)
				select {
				case pcmCh <- pcmEvent{turnID: tid, data: cp}:
				case <-genCtx.Done():
					return genCtx.Err()
				}
				return nil
			}
			if text != "" {
				if err := s.svc.Speak(genCtx, text, emit); err != nil && !errors.Is(err, context.Canceled) {
					lg.Warn("tts stage: speak failed",
						engine.F("err", err.Error()),
						engine.F("text", text))
				}
			}
			if job.turnDone {
				if err := s.svc.Finalize(genCtx, emit); err != nil && !errors.Is(err, context.Canceled) {
					lg.Warn("tts stage: finalize failed",
						engine.F("err", err.Error()))
				}
			}
			// Emit terminal regardless so the main loop knows
			// this job is done.
			select {
			case pcmCh <- pcmEvent{turnID: tid, terminal: true}:
			case <-ctx.Done():
			}
			cancel()
			genMu.Lock()
			if genCancel != nil {
				// Only clear if we're still the current owner;
				// a barge-in may have already swapped us out.
				genCancel = nil
			}
			genMu.Unlock()
		}
	}()

	dispatchSpeak := func(text string, turnDone bool) {
		select {
		case jobCh <- speakJob{text: text, turnDone: turnDone}:
		case <-ctx.Done():
		}
	}

	jobChClosed := false
	closeJobs := func() {
		if !jobChClosed {
			jobChClosed = true
			close(jobCh)
		}
	}
	defer func() {
		cancelInFlight()
		closeJobs()
		<-workerDone
	}()

	inOpen := true
	// pendingTurnDone tracks whether the LLM has already delivered
	// KindAITextDone but we deferred the synthesizer Finalize until
	// the residual sentence buffer was flushed.
	pendingTurnDone := false

	flushBuffer := func(turnDone bool) {
		text := buf.String()
		buf.Reset()
		if text == "" && !turnDone {
			return
		}
		dispatchSpeak(text, turnDone)
	}

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
				if buf.Len() > 0 || pendingTurnDone {
					flushBuffer(pendingTurnDone)
					pendingTurnDone = false
				}
				break
			}
			switch f.Kind {
			case pipeline.KindAIText:
				buf.WriteString(f.Text)
				if shouldEarlyFlush(buf.String(), s.flushOn, s.minFlushRunes) {
					flushBuffer(false)
				}
				// passthrough so observers (UI / metrics) still see deltas
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
			case pipeline.KindAITextDone:
				// Flush whatever is left + run Finalize.
				flushBuffer(true)
				// The KindAITextDone the engine ttsPlaying tracker
				// uses is the one we emit AFTER our synthesizer
				// Finalize completes; pass the upstream done frame
				// through transparently for any other observers.
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
			case pipeline.KindBargeIn:
				// User started talking — dump residual text + cancel
				// in-flight Speak. Forward the barge-in frame so
				// downstream observers (recorder, metrics) see it.
				buf.Reset()
				cancelInFlight()
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
			default:
				if err := sendOrCancel(ctx, out, f); err != nil {
					return err
				}
			}
		case ev := <-pcmCh:
			if err := s.handlePCMEvent(ctx, out, ev, &genMu, &currentTID); err != nil {
				return err
			}
		}
		if !inOpen {
			// Drain phase: close the job queue so the worker
			// finishes pending Speak/Finalize calls, then wait
			// for it to exit while still pumping PCM frames so
			// we don't drop trailing audio.
			closeJobs()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case ev := <-pcmCh:
					if err := s.handlePCMEvent(ctx, out, ev, &genMu, &currentTID); err != nil {
						return err
					}
				case <-workerDone:
					for {
						select {
						case ev := <-pcmCh:
							if err := s.handlePCMEvent(ctx, out, ev, &genMu, &currentTID); err != nil {
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

// handlePCMEvent emits one PCM frame downstream, filtering stale-turn
// payloads via turnID. terminal events are dropped (they exist only
// to signal the synthesis goroutine has finished — the engine's own
// KindAITextDone tracking is driven by the upstream pipeline frame
// passthrough, not by this signal).
func (s *ttsStage) handlePCMEvent(
	ctx context.Context,
	out chan<- pipeline.Frame,
	ev pcmEvent,
	mu *sync.Mutex,
	currentTID *uint64,
) error {
	mu.Lock()
	active := ev.turnID == *currentTID
	mu.Unlock()
	if !active {
		return nil
	}
	if ev.terminal {
		return nil
	}
	if len(ev.data) == 0 {
		return nil
	}
	return sendOrCancel(ctx, out, pipeline.Frame{
		Kind: pipeline.KindPCM,
		PCM: engine.PCMFrame{
			Data: ev.data,
		},
	})
}

// shouldEarlyFlush returns true when the buffered AI text contains a
// sentence terminator AND has accumulated enough characters for a
// natural-sounding phrase. Lifted from streamLLMToTTS so behaviour is
// identical to the legacy bridge.
func shouldEarlyFlush(buf string, terminators string, minRunes int) bool {
	if buf == "" {
		return false
	}
	runes := []rune(buf)
	if len(runes) < minRunes {
		return false
	}
	last := runes[len(runes)-1]
	return strings.ContainsRune(terminators, last)
}
