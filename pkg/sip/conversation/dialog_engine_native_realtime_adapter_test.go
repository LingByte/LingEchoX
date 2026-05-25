// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dialogrealtime "github.com/LinByte/VoiceServer/pkg/dialog/realtime"
	"github.com/LinByte/VoiceServer/pkg/realtime"
)

// fakeRealtimeAgent satisfies pkg/realtime.Agent for tests. The
// production wiring uses NewAgentFromCredential to obtain one of
// these (via aliyunomni); in tests we hand-build it so the adapter
// is exercised in isolation.
type fakeRealtimeAgent struct {
	startErr error
	pushErr  error

	mu        sync.Mutex
	pushed    [][]byte
	cancelled atomic.Int32
	closed    atomic.Int32

	onEvent func(realtime.Event)
}

func (a *fakeRealtimeAgent) Start(_ context.Context) error { return a.startErr }
func (a *fakeRealtimeAgent) PushAudio(pcm []byte) error {
	if a.pushErr != nil {
		return a.pushErr
	}
	a.mu.Lock()
	a.pushed = append(a.pushed, append([]byte(nil), pcm...))
	a.mu.Unlock()
	return nil
}
func (a *fakeRealtimeAgent) CommitInputAudio() error              { return nil }
func (a *fakeRealtimeAgent) Cancel() error                        { a.cancelled.Add(1); return nil }
func (a *fakeRealtimeAgent) Close() error                         { a.closed.Add(1); return nil }
func (a *fakeRealtimeAgent) UpdateInstructions(_ string) error    { return nil }

// fakeSink captures events so tests can assert what the translator
// produced. Implements dialogrealtime.EventSink.
type fakeSink struct {
	mu   sync.Mutex
	got  []dialogrealtime.Event
	open atomic.Bool
}

func newFakeSink() *fakeSink {
	s := &fakeSink{}
	s.open.Store(true)
	return s
}

func (s *fakeSink) Emit(ev dialogrealtime.Event) bool {
	if !s.open.Load() {
		return false
	}
	s.mu.Lock()
	s.got = append(s.got, ev)
	s.mu.Unlock()
	return true
}

func (s *fakeSink) snapshot() []dialogrealtime.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]dialogrealtime.Event, len(s.got))
	copy(out, s.got)
	return out
}

// --- translateRealtimeEvent ------------------------------------------

func TestTranslateRealtimeEvent_KindMapping(t *testing.T) {
	cases := []struct {
		in       realtime.EventType
		wantKind dialogrealtime.EventKind
		wantOk   bool
	}{
		{realtime.EventSessionOpen, 0, false},
		{realtime.EventSessionClose, dialogrealtime.EventSessionClose, true},
		{realtime.EventUserTranscript, dialogrealtime.EventUserTranscript, true},
		{realtime.EventUserSpeechStarted, dialogrealtime.EventUserSpeechStarted, true},
		{realtime.EventUserSpeechEnded, dialogrealtime.EventUserSpeechEnded, true},
		{realtime.EventAssistantText, dialogrealtime.EventAssistantText, true},
		{realtime.EventAssistantAudio, dialogrealtime.EventAssistantAudio, true},
		{realtime.EventAssistantTurnEnd, dialogrealtime.EventAssistantTurnEnd, true},
		{realtime.EventError, dialogrealtime.EventError, true},
	}
	for _, c := range cases {
		got, ok := translateRealtimeEvent(realtime.Event{Type: c.in})
		if ok != c.wantOk {
			t.Errorf("%v: ok=%v, want %v", c.in, ok, c.wantOk)
			continue
		}
		if ok && got.Kind != c.wantKind {
			t.Errorf("%v: kind=%v, want %v", c.in, got.Kind, c.wantKind)
		}
	}
}

func TestTranslateRealtimeEvent_FieldsForwarded(t *testing.T) {
	in := realtime.Event{
		Type:    realtime.EventAssistantText,
		Text:    "hello",
		Final:   true,
		Vendor:  "aliyun_omni",
	}
	got, ok := translateRealtimeEvent(in)
	if !ok {
		t.Fatal("translation failed for AssistantText")
	}
	if got.Text != "hello" || !got.Final || got.Vendor != "aliyun_omni" {
		t.Errorf("got %+v", got)
	}
}

func TestTranslateRealtimeEvent_AudioPayload(t *testing.T) {
	in := realtime.Event{
		Type:    realtime.EventAssistantAudio,
		AudioPC: []byte{1, 2, 3, 4},
	}
	got, ok := translateRealtimeEvent(in)
	if !ok {
		t.Fatal("translation failed for AssistantAudio")
	}
	if string(got.Audio) != "\x01\x02\x03\x04" {
		t.Errorf("Audio = %v", got.Audio)
	}
	// SampleRate is stamped by the builder closure, NOT by
	// translateRealtimeEvent — at this layer it should be zero.
	if got.SampleRate != 0 {
		t.Errorf("SampleRate = %d, want 0 (set by builder)", got.SampleRate)
	}
}

// --- newNativeRealtimeBuilder ----------------------------------------

func TestNativeRealtimeBuilder_StampsAudioSampleRate(t *testing.T) {
	fa := &fakeRealtimeAgent{}
	cfg := nativeRealtimeBuilderConfig{
		CredentialCfg: map[string]any{"provider": "fake"},
		Options:       realtime.Options{OutputSampleRate: 24000},
		NewAgent: func(_ map[string]any, opts realtime.Options) (realtime.Agent, error) {
			fa.onEvent = opts.OnEvent
			return fa, nil
		},
	}
	builder := newNativeRealtimeBuilder(cfg)
	sink := newFakeSink()
	agent, err := builder.Build(sink)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if agent == nil {
		t.Fatal("nil agent")
	}
	// Drive an audio event through the WS callback path.
	fa.onEvent(realtime.Event{Type: realtime.EventAssistantAudio, AudioPC: []byte{9}})

	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Kind != dialogrealtime.EventAssistantAudio {
		t.Errorf("kind = %v, want AssistantAudio", got[0].Kind)
	}
	if got[0].SampleRate != 24000 {
		t.Errorf("SampleRate = %d, want 24000", got[0].SampleRate)
	}
}

func TestNativeRealtimeBuilder_DropsSessionOpen(t *testing.T) {
	fa := &fakeRealtimeAgent{}
	cfg := nativeRealtimeBuilderConfig{
		CredentialCfg: map[string]any{"provider": "fake"},
		NewAgent: func(_ map[string]any, opts realtime.Options) (realtime.Agent, error) {
			fa.onEvent = opts.OnEvent
			return fa, nil
		},
	}
	sink := newFakeSink()
	if _, err := newNativeRealtimeBuilder(cfg).Build(sink); err != nil {
		t.Fatalf("Build: %v", err)
	}
	fa.onEvent(realtime.Event{Type: realtime.EventSessionOpen})
	if got := sink.snapshot(); len(got) != 0 {
		t.Errorf("got %d events, want 0 (SessionOpen dropped)", len(got))
	}
}

func TestNativeRealtimeBuilder_PropagatesNewAgentError(t *testing.T) {
	wantErr := errors.New("agent build failed")
	cfg := nativeRealtimeBuilderConfig{
		CredentialCfg: map[string]any{"provider": "fake"},
		NewAgent: func(_ map[string]any, _ realtime.Options) (realtime.Agent, error) {
			return nil, wantErr
		},
	}
	if _, err := newNativeRealtimeBuilder(cfg).Build(newFakeSink()); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// --- nativeRealtimeAgentAdapter ----------------------------------------

func TestNativeRealtimeAdapter_ForwardsCalls(t *testing.T) {
	fa := &fakeRealtimeAgent{}
	a := &nativeRealtimeAgentAdapter{inner: fa}

	if err := a.Start(context.Background()); err != nil {
		t.Errorf("Start: %v", err)
	}
	if err := a.PushAudio([]byte{1, 2}); err != nil {
		t.Errorf("PushAudio: %v", err)
	}
	if err := a.Cancel(); err != nil {
		t.Errorf("Cancel: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	fa.mu.Lock()
	pushedN := len(fa.pushed)
	fa.mu.Unlock()
	if pushedN != 1 {
		t.Errorf("pushed = %d, want 1", pushedN)
	}
	if fa.cancelled.Load() != 1 {
		t.Errorf("cancelled = %d, want 1", fa.cancelled.Load())
	}
	if fa.closed.Load() != 1 {
		t.Errorf("closed = %d, want 1", fa.closed.Load())
	}
}

// --- useNativeRealtime predicate --------------------------------------

func TestUseNativeRealtime_DefaultOff(t *testing.T) {
	r := nativeRealtimeRouter{getenv: func(string) string { return "" }}
	if r.useNativeRealtime("tenant-7") {
		t.Error("default should be OFF for realtime native (legacy still owns parity features)")
	}
}

func TestUseNativeRealtime_AllowList(t *testing.T) {
	r := nativeRealtimeRouter{getenv: func(k string) string {
		if k == envNativeRealtimeTenants {
			return "tenant-a, tenant-b"
		}
		return ""
	}}
	if !r.useNativeRealtime("tenant-a") {
		t.Error("tenant-a in allow-list should be ON")
	}
	if !r.useNativeRealtime("tenant-b") {
		t.Error("tenant-b in allow-list should be ON")
	}
	if r.useNativeRealtime("tenant-c") {
		t.Error("tenant-c not in allow-list should be OFF")
	}
}

func TestUseNativeRealtime_EmptyTenantOff(t *testing.T) {
	r := nativeRealtimeRouter{getenv: func(string) string { return "ALL" }}
	if r.useNativeRealtime("") {
		t.Error("empty tenant must never route through native")
	}
}

// Sanity: builder must not deadlock if Emit returns false for every
// event (closed sink).
func TestNativeRealtimeBuilder_ClosedSinkNoDeadlock(t *testing.T) {
	fa := &fakeRealtimeAgent{}
	cfg := nativeRealtimeBuilderConfig{
		CredentialCfg: map[string]any{"provider": "fake"},
		NewAgent: func(_ map[string]any, opts realtime.Options) (realtime.Agent, error) {
			fa.onEvent = opts.OnEvent
			return fa, nil
		},
	}
	sink := newFakeSink()
	if _, err := newNativeRealtimeBuilder(cfg).Build(sink); err != nil {
		t.Fatalf("Build: %v", err)
	}
	sink.open.Store(false)
	done := make(chan struct{})
	go func() {
		fa.onEvent(realtime.Event{Type: realtime.EventAssistantText, Text: "x"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("translator deadlocked on closed sink")
	}
}
