// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package cascaded

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/pipeline"
)

// fakeTTS is a programmable TTSService for tests. Speak emits one PCM
// frame per character of the input text (so output PCM length encodes
// the synthesized text). Finalize emits a fixed sentinel frame so we
// can detect end-of-turn finalization.
//
// fakeTTS records all spoken texts (in order) so tests can assert
// flushing/buffering behaviour.
type fakeTTS struct {
	mu          sync.Mutex
	spoken      []string
	finalizeHit atomic.Int32
	speakHit    atomic.Int32

	// onSpeakStart blocks Speak from emitting until the test signals
	// (used to test barge-in cancel).
	onSpeakStart func(ctx context.Context, text string)
}

func (f *fakeTTS) Speak(ctx context.Context, text string, onPCM func(pcm []byte) error) error {
	f.speakHit.Add(1)
	f.mu.Lock()
	f.spoken = append(f.spoken, text)
	f.mu.Unlock()
	if f.onSpeakStart != nil {
		f.onSpeakStart(ctx, text)
	}
	for i := 0; i < len(text); i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := onPCM([]byte{text[i]}); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeTTS) Finalize(ctx context.Context, onPCM func(pcm []byte) error) error {
	f.finalizeHit.Add(1)
	if onPCM != nil {
		_ = onPCM([]byte{0xFF}) // sentinel "finalize fired"
	}
	return nil
}

func (f *fakeTTS) spokenSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.spoken))
	copy(out, f.spoken)
	return out
}

func runTTSStage(t *testing.T, s *ttsStage, ctx context.Context, in <-chan pipeline.Frame) (<-chan pipeline.Frame, func() error) {
	t.Helper()
	out := make(chan pipeline.Frame, 256)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(ctx, in, out, engine.NopLogger{})
	}()
	return out, func() error { return <-errCh }
}

// classifyTTSFrames sums PCM bytes (proxy for synthesized character
// count) and counts terminal frames.
func classifyTTSFrames(got []pipeline.Frame) (pcmBytes int, doneCount int, aiTextEcho int, bargeIn int) {
	for _, f := range got {
		switch f.Kind {
		case pipeline.KindPCM:
			pcmBytes += len(f.PCM.Data)
		case pipeline.KindAITextDone:
			doneCount++
		case pipeline.KindAIText:
			aiTextEcho++
		case pipeline.KindBargeIn:
			bargeIn++
		}
	}
	return
}

// --- Name / nil service --------------------------------------------

func TestTTSStage_Name(t *testing.T) {
	if got := newTTSStage(nil).Name(); got != "tts" {
		t.Errorf("Name() = %q, want %q", got, "tts")
	}
}

func TestTTSStage_NilServicePassthrough(t *testing.T) {
	s := newTTSStage(nil)
	in := make(chan pipeline.Frame, 3)
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "hi"}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	in <- pipeline.Frame{Kind: pipeline.KindBargeIn}
	close(in)

	out, wait := runTTSStage(t, s, context.Background(), in)
	got := drainOutput(t, out, time.Second)
	if err := wait(); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d frames, want 3 passthrough", len(got))
	}
}

// --- Basic synthesis -----------------------------------------------

func TestTTSStage_AITextDoneTriggersFinalize(t *testing.T) {
	tts := &fakeTTS{}
	s := newTTSStage(tts)
	in := make(chan pipeline.Frame, 3)
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "hi"}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	close(in)

	out, wait := runTTSStage(t, s, context.Background(), in)
	got := drainOutput(t, out, 2*time.Second)
	if err := wait(); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
	pcm, doneCount, _, _ := classifyTTSFrames(got)
	if tts.finalizeHit.Load() != 1 {
		t.Errorf("Finalize calls = %d, want 1", tts.finalizeHit.Load())
	}
	if doneCount != 1 {
		t.Errorf("KindAITextDone count = %d, want 1 (passthrough)", doneCount)
	}
	// "hi" → 2 chars + 1 finalize sentinel = 3 PCM bytes
	if pcm < 2 {
		t.Errorf("PCM bytes = %d, want >= 2 ('hi' synth)", pcm)
	}
}

func TestTTSStage_EarlyFlushOnSentenceTerminator(t *testing.T) {
	tts := &fakeTTS{}
	s := newTTSStage(tts)
	in := make(chan pipeline.Frame, 5)
	// Build a long enough sentence to clear minFlushRunes (default 6)
	// then end with "！" — should flush immediately.
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "Hello "}
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "world!"}
	// More text after the flush — should accumulate again.
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: " more"}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	close(in)

	out, wait := runTTSStage(t, s, context.Background(), in)
	_ = drainOutput(t, out, 2*time.Second)
	if err := wait(); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
	spoken := tts.spokenSnapshot()
	if len(spoken) < 2 {
		t.Errorf("Speak calls = %d, want >= 2 (early flush + final)", len(spoken))
	}
	if !strings.Contains(spoken[0], "world!") {
		t.Errorf("first Speak should include 'world!' (early flush); got %q", spoken[0])
	}
}

func TestTTSStage_NoFlushBeforeMinRunes(t *testing.T) {
	// Short text with terminator but below minFlushRunes (6) → no
	// early flush; everything synthesizes only at AITextDone.
	tts := &fakeTTS{}
	s := newTTSStage(tts)
	in := make(chan pipeline.Frame, 2)
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "hi!"}
	// Wait briefly so any early-flush race resolves before close.
	out, wait := runTTSStage(t, s, context.Background(), in)
	time.Sleep(80 * time.Millisecond)
	if got := tts.speakHit.Load(); got != 0 {
		t.Errorf("Speak fired before AITextDone with short text; calls=%d", got)
	}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	close(in)
	_ = drainOutput(t, out, 2*time.Second)
	_ = wait()
	if tts.speakHit.Load() != 1 {
		t.Errorf("Speak final calls = %d, want 1", tts.speakHit.Load())
	}
}

// --- Barge-in ------------------------------------------------------

func TestTTSStage_BargeInCancelsInFlightSpeak(t *testing.T) {
	var blocking sync.WaitGroup
	blocking.Add(1)
	var firstSpeakOnce sync.Once
	tts := &fakeTTS{
		onSpeakStart: func(ctx context.Context, text string) {
			// Only the first Speak blocks; subsequent ones are
			// allowed to proceed normally so the second turn can
			// run after barge-in.
			fired := false
			firstSpeakOnce.Do(func() {
				fired = true
				blocking.Done()
				<-ctx.Done()
			})
			_ = fired
		},
	}
	s := newTTSStage(tts)
	in := make(chan pipeline.Frame, 4)
	// Note: text MUST end on a flushOn rune so shouldEarlyFlush
	// triggers before AITextDone — otherwise the buffer stays put
	// and Speak never fires.
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "First sentence finished."}
	out, wait := runTTSStage(t, s, context.Background(), in)
	blocking.Wait() // first Speak is in-flight & blocking
	in <- pipeline.Frame{Kind: pipeline.KindBargeIn}
	// Followed by a new turn. The cancelled goroutine will exit on ctx.Done.
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "Second turn here."}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	close(in)
	got := drainOutput(t, out, 2*time.Second)
	_ = wait()

	_, _, _, bargeIn := classifyTTSFrames(got)
	if bargeIn != 1 {
		t.Errorf("KindBargeIn passthrough count = %d, want 1", bargeIn)
	}
	// The first Speak was cancelled; the second turn should still
	// fire its own Speak. We can't assert exact call counts because
	// the early-flush + final-flush split is text-length dependent,
	// but speakHit must be >= 2 (one cancelled, one new).
	if tts.speakHit.Load() < 2 {
		t.Errorf("speakHit = %d, want >= 2 (cancelled + new turn)", tts.speakHit.Load())
	}
}

func TestTTSStage_BargeInClearsBuffer(t *testing.T) {
	tts := &fakeTTS{}
	s := newTTSStage(tts)
	in := make(chan pipeline.Frame, 4)
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "abc"} // below minFlushRunes
	in <- pipeline.Frame{Kind: pipeline.KindBargeIn}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	close(in)

	out, wait := runTTSStage(t, s, context.Background(), in)
	_ = drainOutput(t, out, 2*time.Second)
	_ = wait()
	for _, s := range tts.spokenSnapshot() {
		if strings.Contains(s, "abc") {
			t.Errorf("buffered text 'abc' was synthesized despite barge-in; spoken=%q", s)
		}
	}
}

// --- Lifecycle ------------------------------------------------------

func TestTTSStage_CtxCancelStopsStage(t *testing.T) {
	tts := &fakeTTS{}
	s := newTTSStage(tts)
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan pipeline.Frame)
	out, wait := runTTSStage(t, s, ctx, in)
	cancel()
	select {
	case _, ok := <-out:
		if ok {
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

func TestTTSStage_InputCloseDrainsTrailingPCM(t *testing.T) {
	tts := &fakeTTS{}
	s := newTTSStage(tts)
	in := make(chan pipeline.Frame, 3)
	in <- pipeline.Frame{Kind: pipeline.KindAIText, Text: "trailing"}
	in <- pipeline.Frame{Kind: pipeline.KindAITextDone}
	close(in)

	out, wait := runTTSStage(t, s, context.Background(), in)
	got := drainOutput(t, out, 2*time.Second)
	if err := wait(); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
	pcm, doneCount, _, _ := classifyTTSFrames(got)
	if pcm < len("trailing") {
		t.Errorf("PCM bytes = %d, want >= %d (drain dropped frames)", pcm, len("trailing"))
	}
	if doneCount != 1 {
		t.Errorf("KindAITextDone count = %d, want 1", doneCount)
	}
}

// --- Engine integration --------------------------------------------

func TestEngine_WithTTSServiceSwapsStage(t *testing.T) {
	tts := &fakeTTS{}
	e := New(
		engine.Config{Mode: engine.ModeCascaded, CallID: "c-tts", TenantID: "t1"},
		WithTTSService(tts),
	)
	if e.ttsService == nil {
		t.Fatal("WithTTSService did not install service")
	}
	port := newFakePort()
	detach, err := e.Attach(context.Background(), port, nil)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	close(port.in)
	_ = detach(context.Background())
	// Stub LLM doesn't emit AIText so Speak should never fire.
	if tts.speakHit.Load() != 0 {
		t.Errorf("unexpected Speak with stub LLM; calls=%d", tts.speakHit.Load())
	}
}

// --- All stages together -------------------------------------------

func TestEngine_AllRealStagesEndToEnd(t *testing.T) {
	// Wire ASR → LLM → TTS together with deterministic fakes and
	// confirm one PCM in produces synthesized PCM out via the full
	// chain. This is the smoke test that proves the stage swap
	// matrix is wired correctly.
	asr := &fakeASR{}
	llm := &fakeLLM{}
	llm.push("hello reply!")
	tts := &fakeTTS{}

	e := New(
		engine.Config{Mode: engine.ModeCascaded, CallID: "c-all", TenantID: "t1"},
		WithASRRecognizer(asr),
		WithLLMService(llm),
		WithTTSService(tts),
	)
	port := newFakePort()
	detach, err := e.Attach(context.Background(), port, nil)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	// Push PCM so the ASR sees something.
	port.in <- engine.PCMFrame{Data: []byte{1, 2, 3, 4}, SampleRate: 16000}
	// Wait for ASR to register the frame, then emit a final
	// transcript that triggers LLM → TTS.
	deadline := time.Now().Add(time.Second)
	for asr.pcmCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	asr.emit("hi there", true)
	// Wait for LLM to fire its turn.
	deadline = time.Now().Add(2 * time.Second)
	for llm.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Wait for TTS to finalize.
	deadline = time.Now().Add(2 * time.Second)
	for tts.finalizeHit.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	close(port.in)
	_ = detach(context.Background())

	if asr.pcmCount() == 0 {
		t.Error("ASR never received PCM through the chain")
	}
	if llm.calls.Load() == 0 {
		t.Error("LLM never received the user transcript")
	}
	if tts.finalizeHit.Load() == 0 {
		t.Error("TTS never finalized the AI turn")
	}
	// Confirm the AI turn's synthesized PCM reached the port.
	outCount := func() int {
		port.outMu.Lock()
		defer port.outMu.Unlock()
		return len(port.out)
	}
	deadline = time.Now().Add(time.Second)
	for outCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if outCount() == 0 {
		t.Error("synthesized PCM never reached the MediaPort output")
	}
}
