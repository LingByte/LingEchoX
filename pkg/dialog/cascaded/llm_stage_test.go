// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/pipeline"
)

// fakeLLM is a programmable LLMService for tests. Each call to
// StreamReply pulls one scripted response from the queue (or returns
// an error if the queue is empty). A scripted response is a slice of
// deltas; the last delta is emitted with isComplete=true regardless
// of how the test writes it.
type fakeLLM struct {
	mu        sync.Mutex
	scripts   [][]string // each script = one turn's deltas
	calls     atomic.Int32
	errOnNext error
	// onCallStart fires synchronously at the start of each StreamReply
	// so tests can observe / block / cancel.
	onCallStart func(ctx context.Context, userText string)
}

func (f *fakeLLM) push(deltas ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts = append(f.scripts, deltas)
}

func (f *fakeLLM) StreamReply(ctx context.Context, userText string, onDelta func(text string, isComplete bool) error) (string, error) {
	f.calls.Add(1)
	// Dequeue script BEFORE running onCallStart so a blocking hook
	// on call N doesn't starve call N+1 of its own script.
	f.mu.Lock()
	if f.errOnNext != nil {
		err := f.errOnNext
		f.errOnNext = nil
		f.mu.Unlock()
		return "", err
	}
	var deltas []string
	if len(f.scripts) > 0 {
		deltas = f.scripts[0]
		f.scripts = f.scripts[1:]
	}
	f.mu.Unlock()
	if f.onCallStart != nil {
		f.onCallStart(ctx, userText)
	}

	var full strings.Builder
	for i, d := range deltas {
		isLast := i == len(deltas)-1
		if err := onDelta(d, isLast); err != nil {
			return full.String(), err
		}
		full.WriteString(d)
	}
	if len(deltas) == 0 {
		// Emit a single empty terminal so the stage's "always-emit
		// terminal" guarantee is exercised.
		_ = onDelta("", true)
	}
	return full.String(), nil
}

func runLLMStage(t *testing.T, s *llmStage, ctx context.Context, in <-chan pipeline.Frame) (<-chan pipeline.Frame, func() error) {
	t.Helper()
	out := make(chan pipeline.Frame, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(ctx, in, out, engine.NopLogger{})
	}()
	return out, func() error { return <-errCh }
}

// classify counts frames by kind (and collects KindAIText concat) from
// a drained slice. Helper to keep individual asserts terse.
type llmFrames struct {
	aiText        []string
	aiTextDone    int
	userFinalEcho int
	other         int
}

func classifyLLMFrames(got []pipeline.Frame) llmFrames {
	var r llmFrames
	for _, f := range got {
		switch f.Kind {
		case pipeline.KindAIText:
			r.aiText = append(r.aiText, f.Text)
		case pipeline.KindAITextDone:
			r.aiTextDone++
		case pipeline.KindTextFinal:
			r.userFinalEcho++
		default:
			r.other++
		}
	}
	return r
}

// --- Name / nil service --------------------------------------------

func TestLLMStage_Name(t *testing.T) {
	if got := newLLMStage(nil).Name(); got != "llm" {
		t.Errorf("Name() = %q, want %q", got, "llm")
	}
}

func TestLLMStage_NilServicePassthrough(t *testing.T) {
	s := newLLMStage(nil)
	in := make(chan pipeline.Frame, 3)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "hi"}
	in <- pipeline.Frame{Kind: pipeline.KindPCM, PCM: engine.PCMFrame{Data: []byte{1, 2}}}
	in <- pipeline.Frame{Kind: pipeline.KindBargeIn}
	close(in)

	out, wait := runLLMStage(t, s, context.Background(), in)
	got := drainOutput(t, out, time.Second)
	if err := wait(); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d frames, want 3 passthrough", len(got))
	}
}

// --- Turn dispatch -------------------------------------------------

func TestLLMStage_KindTextFinalTriggersGenerate(t *testing.T) {
	svc := &fakeLLM{}
	svc.push("Hello ", "world", "!")
	s := newLLMStage(svc)

	in := make(chan pipeline.Frame, 1)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "what's up?"}

	out, wait := runLLMStage(t, s, context.Background(), in)
	deadline := time.Now().Add(2 * time.Second)
	var got []pipeline.Frame
	for time.Now().Before(deadline) {
		select {
		case f := <-out:
			got = append(got, f)
			if f.Kind == pipeline.KindAITextDone {
				close(in)
				_ = wait()
				goto done
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	close(in)
	_ = wait()
done:
	cls := classifyLLMFrames(got)
	if cls.userFinalEcho != 1 {
		t.Errorf("user-final echo count = %d, want 1", cls.userFinalEcho)
	}
	if cls.aiTextDone != 1 {
		t.Errorf("aiTextDone count = %d, want 1", cls.aiTextDone)
	}
	joined := strings.Join(cls.aiText, "")
	if !strings.Contains(joined, "Hello") || !strings.Contains(joined, "world") {
		t.Errorf("AI text deltas missing expected tokens: %q", joined)
	}
	if svc.calls.Load() != 1 {
		t.Errorf("StreamReply call count = %d, want 1", svc.calls.Load())
	}
}

func TestLLMStage_NonTextFinalFramesPassthroughWithoutDispatch(t *testing.T) {
	svc := &fakeLLM{}
	s := newLLMStage(svc)
	in := make(chan pipeline.Frame, 3)
	in <- pipeline.Frame{Kind: pipeline.KindPCM, PCM: engine.PCMFrame{Data: []byte{1}}}
	in <- pipeline.Frame{Kind: pipeline.KindTextInterim, Text: "partial"}
	in <- pipeline.Frame{Kind: pipeline.KindBargeIn}
	close(in)

	out, wait := runLLMStage(t, s, context.Background(), in)
	got := drainOutput(t, out, time.Second)
	_ = wait()
	if svc.calls.Load() != 0 {
		t.Errorf("StreamReply should not fire for non-final input; calls=%d", svc.calls.Load())
	}
	if len(got) != 3 {
		t.Errorf("got %d frames passthrough, want 3", len(got))
	}
}

func TestLLMStage_EmptyTextFinalSkipped(t *testing.T) {
	svc := &fakeLLM{}
	s := newLLMStage(svc)
	in := make(chan pipeline.Frame, 2)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "   "} // whitespace
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: ""}
	close(in)

	out, wait := runLLMStage(t, s, context.Background(), in)
	_ = drainOutput(t, out, time.Second)
	_ = wait()
	if svc.calls.Load() != 0 {
		t.Errorf("StreamReply should be skipped for empty user text; calls=%d", svc.calls.Load())
	}
}

// --- Pre-emption ---------------------------------------------------

func TestLLMStage_NewTurnCancelsInFlight(t *testing.T) {
	// First turn blocks indefinitely until ctx cancel; second turn
	// runs synchronously. Stage must cancel the first and ONLY emit
	// the second turn's AITextDone (the stale turn's final delta is
	// filtered by turnID).
	var blockStarted sync.WaitGroup
	blockStarted.Add(1)
	svc := &fakeLLM{
		onCallStart: func(ctx context.Context, userText string) {
			if userText == "first" {
				blockStarted.Done()
				<-ctx.Done() // block until cancelled
			}
		},
	}
	svc.push("first-delta") // for "first" — these won't get emitted because onCallStart blocks
	svc.push("second-reply")
	s := newLLMStage(svc)

	in := make(chan pipeline.Frame, 2)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "first"}
	out, wait := runLLMStage(t, s, context.Background(), in)
	blockStarted.Wait() // ensure first turn is in-flight
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "second"}
	// Wait for second turn's final.
	deadline := time.Now().Add(2 * time.Second)
	var sawSecondDone, sawSecondText bool
	for time.Now().Before(deadline) {
		select {
		case f := <-out:
			if f.Kind == pipeline.KindAIText && strings.Contains(f.Text, "second-reply") {
				sawSecondText = true
			}
			if f.Kind == pipeline.KindAITextDone {
				sawSecondDone = true
			}
		case <-time.After(50 * time.Millisecond):
		}
		if sawSecondDone && sawSecondText {
			break
		}
	}
	close(in)
	_ = wait()
	if !sawSecondText {
		t.Error("never saw second turn's AI text")
	}
	if !sawSecondDone {
		t.Error("never saw second turn's AITextDone")
	}
}

// --- Always-emit terminal -----------------------------------------

func TestLLMStage_EmitsAITextDoneEvenOnEmptyReply(t *testing.T) {
	svc := &fakeLLM{}
	svc.push() // empty deltas — stage must still close out the turn
	s := newLLMStage(svc)
	in := make(chan pipeline.Frame, 1)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "hello"}

	out, wait := runLLMStage(t, s, context.Background(), in)
	deadline := time.Now().Add(2 * time.Second)
	var sawDone bool
	for time.Now().Before(deadline) && !sawDone {
		select {
		case f := <-out:
			if f.Kind == pipeline.KindAITextDone {
				sawDone = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	close(in)
	_ = wait()
	if !sawDone {
		t.Error("empty reply did not produce KindAITextDone")
	}
}

func TestLLMStage_EmitsAITextDoneOnProviderError(t *testing.T) {
	var observerHits atomic.Int32
	svc := &fakeLLM{errOnNext: errors.New("provider 500")}
	s := newLLMStage(svc, withLLMTurnErrorObserver(func(turnText string, err error) {
		observerHits.Add(1)
	}))

	in := make(chan pipeline.Frame, 1)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "hello"}

	out, wait := runLLMStage(t, s, context.Background(), in)
	deadline := time.Now().Add(2 * time.Second)
	var sawDone bool
	for time.Now().Before(deadline) && !sawDone {
		select {
		case f := <-out:
			if f.Kind == pipeline.KindAITextDone {
				sawDone = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	close(in)
	_ = wait()
	if !sawDone {
		t.Error("provider error should still emit KindAITextDone for downstream cleanup")
	}
	if observerHits.Load() != 1 {
		t.Errorf("turn error observer hits = %d, want 1", observerHits.Load())
	}
}

// --- Lifecycle ------------------------------------------------------

func TestLLMStage_CtxCancelStopsStageAndCancelsInFlight(t *testing.T) {
	var inFlight sync.WaitGroup
	inFlight.Add(1)
	svc := &fakeLLM{
		onCallStart: func(ctx context.Context, userText string) {
			inFlight.Done()
			<-ctx.Done()
		},
	}
	svc.push("never")
	s := newLLMStage(svc)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan pipeline.Frame, 1)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "hi"}
	out, wait := runLLMStage(t, s, ctx, in)
	inFlight.Wait()
	cancel()

	// out must close after ctx cancel; the in-flight LLM goroutine
	// receives ctx.Done() and exits, the stage's defer waits for it
	// before returning.
	select {
	case _, ok := <-out:
		if ok {
			// drain any pending frames until close
			for range out {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("out did not close after ctx cancel")
	}
	if err := wait(); err == nil {
		t.Error("Run should return ctx.Err() on cancel")
	}
}

func TestLLMStage_InputCloseDrainsInFlightDeltas(t *testing.T) {
	svc := &fakeLLM{}
	svc.push("one ", "two ", "three")
	s := newLLMStage(svc)

	in := make(chan pipeline.Frame, 1)
	in <- pipeline.Frame{Kind: pipeline.KindTextFinal, Text: "go"}
	close(in)

	out, wait := runLLMStage(t, s, context.Background(), in)
	got := drainOutput(t, out, 2*time.Second)
	if err := wait(); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
	cls := classifyLLMFrames(got)
	if cls.aiTextDone != 1 {
		t.Errorf("aiTextDone = %d, want 1 (drain phase must flush terminal)", cls.aiTextDone)
	}
	if strings.Join(cls.aiText, "") == "" {
		t.Error("no AI text emitted; drain phase dropped deltas")
	}
}

// --- Engine integration --------------------------------------------

func TestEngine_WithLLMServiceSwapsStage(t *testing.T) {
	svc := &fakeLLM{}
	svc.push("reply")
	e := New(
		engine.Config{Mode: engine.ModeCascaded, CallID: "c-llm", TenantID: "t1"},
		WithLLMService(svc),
	)
	if e.llmService == nil {
		t.Fatal("WithLLMService did not install service")
	}
	port := newFakePort()
	detach, err := e.Attach(context.Background(), port, nil)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	// The default ASR stub will NOT emit KindTextFinal so we won't
	// actually trigger LLM here without an upstream final. The test
	// asserts wiring only — the stage-level tests cover behaviour.
	close(port.in)
	_ = detach(context.Background())
	if svc.calls.Load() != 0 {
		// Sanity: without a real ASR upstream there's no final to
		// dispatch on.
		t.Errorf("unexpected LLM call without ASR final upstream; calls=%d", svc.calls.Load())
	}
}
