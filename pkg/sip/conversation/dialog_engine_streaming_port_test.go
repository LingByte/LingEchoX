package conversation

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/media"
)

// newTestMS builds a bare *media.MediaSession suitable for the
// streaming-port unit tests. It has no RTP transports wired in, so
// SendToOutput is a no-op (puts into a closed/empty queue) and
// inbound packets must be injected by directly invoking the processor
// callback — we test that path via media.ProcessorRegistry instead of
// a full decode chain.
func newTestMS(t *testing.T) (*media.MediaSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ms := media.NewDefaultSession().Context(ctx).SetSessionID("streaming-port-test")
	return ms, cancel
}

// --- Constructor nil safety ----------------------------------------

func TestNewStreamingCallSessionPort_NilCS(t *testing.T) {
	if p := NewStreamingCallSessionPort(nil); p != nil {
		t.Errorf("NewStreamingCallSessionPort(nil) = %v, want nil", p)
	}
}

func Test_newStreamingPort_NilMS(t *testing.T) {
	if p := newStreamingPort(nil, nil, 16000); p != nil {
		t.Errorf("newStreamingPort(nil ms) = %v, want nil", p)
	}
}

func Test_newStreamingPort_DefaultsBridgeRate(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 0) // 0 → defaults to 16000
	if p == nil {
		t.Fatal("newStreamingPort returned nil with valid ms")
	}
	if got := p.SampleRate(); got != 16000 {
		t.Errorf("SampleRate = %d, want 16000 fallback", got)
	}
}

// --- Metadata accessors --------------------------------------------

func TestStreamingPort_MetadataAccessors_NilCS(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 8000)
	if p == nil {
		t.Fatal("port nil")
	}
	if got := p.CallID(); got != "" {
		t.Errorf("CallID() = %q, want empty (nil cs)", got)
	}
	if got := p.TenantID(); got != "" {
		t.Errorf("TenantID() = %q, want empty (nil cs)", got)
	}
	if got := p.Codec(); got != (engine.CodecSpec{}) {
		t.Errorf("Codec() = %+v, want zero (nil cs)", got)
	}
	if got := p.SampleRate(); got != 8000 {
		t.Errorf("SampleRate() = %d, want 8000 (overridden)", got)
	}
}

func TestStreamingPort_NilReceiverDegradesGracefully(t *testing.T) {
	var p *StreamingCallSessionPort
	if p.SampleRate() != 16000 {
		t.Error("nil port SampleRate() should fall back to 16000")
	}
	if p.CallID() != "" {
		t.Error("nil port CallID() should be empty")
	}
	if p.TenantID() != "" {
		t.Error("nil port TenantID() should be empty")
	}
	if p.Codec() != (engine.CodecSpec{}) {
		t.Error("nil port Codec() should be zero")
	}
	// InputPCM on nil port: closed channel, range-loop terminates.
	for range p.InputPCM() {
		t.Error("nil port InputPCM should emit nothing")
	}
	// SendOutputPCM on nil port: ErrPortClosed.
	if err := p.SendOutputPCM(engine.PCMFrame{Data: []byte{1, 2}}); !errors.Is(err, ErrPortClosed) {
		t.Errorf("nil port SendOutputPCM = %v, want ErrPortClosed", err)
	}
	// Close on nil port: nil, no panic.
	if err := p.Close(); err != nil {
		t.Errorf("nil port Close = %v, want nil", err)
	}
	// OnBargeIn / TriggerBargeIn on nil port: no panic.
	p.OnBargeIn(func() {})
	p.TriggerBargeIn()
}

// --- SendOutputPCM behaviour ---------------------------------------

func TestStreamingPort_SendOutputPCM_ClosedReturnsErr(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	_ = p.Close()
	if err := p.SendOutputPCM(engine.PCMFrame{Data: []byte{0, 0}}); !errors.Is(err, ErrPortClosed) {
		t.Errorf("closed port SendOutputPCM = %v, want ErrPortClosed", err)
	}
}

func TestStreamingPort_SendOutputPCM_EmptyFrameNoop(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()
	if err := p.SendOutputPCM(engine.PCMFrame{Data: nil}); err != nil {
		t.Errorf("empty frame SendOutputPCM = %v, want nil (silent drop)", err)
	}
}

func TestStreamingPort_SendOutputPCM_HappyPath(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()
	// 20ms @ 16kHz = 640 bytes. Send a frame at the bridge rate;
	// no resample, no error. ms has no Output transport so the
	// packet ends up in MediaSession's internal queue and is dropped
	// during GC — we just assert we don't error.
	frame := engine.PCMFrame{
		Data:       make([]byte, 640),
		SampleRate: 16000,
	}
	if err := p.SendOutputPCM(frame); err != nil {
		t.Errorf("SendOutputPCM = %v, want nil", err)
	}
}

func TestStreamingPort_SendOutputPCM_ResampleOnRateMismatch(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()
	// 20ms @ 8kHz = 320 bytes. Port bridge is 16kHz → resample to 640.
	frame := engine.PCMFrame{
		Data:       make([]byte, 320),
		SampleRate: 8000,
	}
	if err := p.SendOutputPCM(frame); err != nil {
		t.Errorf("SendOutputPCM with resample = %v, want nil", err)
	}
}

func TestStreamingPort_SendOutputPCM_UnspecifiedSampleRateBypassesResample(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()
	// SampleRate == 0 means "trust the producer; already at bridge
	// rate". We send 640 bytes (one 20ms @ 16kHz frame); no error.
	frame := engine.PCMFrame{Data: make([]byte, 640)}
	if err := p.SendOutputPCM(frame); err != nil {
		t.Errorf("SendOutputPCM = %v, want nil", err)
	}
}

// --- Close / lifecycle ----------------------------------------------

func TestStreamingPort_CloseIdempotent(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	if err := p.Close(); err != nil {
		t.Errorf("first Close = %v, want nil", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close = %v, want nil (idempotent)", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("third Close = %v, want nil (idempotent)", err)
	}
	// Input channel must be closed.
	select {
	case _, ok := <-p.InputPCM():
		if ok {
			t.Error("InputPCM channel should be closed after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("InputPCM did not close within deadline")
	}
}

func TestStreamingPort_ClosesOnMediaSessionCtxCancel(t *testing.T) {
	ms, cancel := newTestMS(t)
	p := newStreamingPort(nil, ms, 16000)

	cancel() // simulates call end

	// InputPCM must close within a short deadline.
	select {
	case _, ok := <-p.InputPCM():
		if ok {
			t.Error("InputPCM should close after media session ctx cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("InputPCM did not close after media session ctx cancel")
	}

	// SendOutputPCM should now return ErrPortClosed.
	if err := p.SendOutputPCM(engine.PCMFrame{Data: []byte{0, 0}}); !errors.Is(err, ErrPortClosed) {
		t.Errorf("SendOutputPCM after ctx cancel = %v, want ErrPortClosed", err)
	}
}

// --- OnBargeIn ------------------------------------------------------

func TestStreamingPort_OnBargeIn_StoresAndFires(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()

	var hits atomic.Int32
	p.OnBargeIn(func() { hits.Add(1) })
	p.TriggerBargeIn()
	if got := hits.Load(); got != 1 {
		t.Errorf("hits after one fire = %d, want 1", got)
	}
	p.TriggerBargeIn()
	if got := hits.Load(); got != 2 {
		t.Errorf("hits after two fires = %d, want 2", got)
	}
}

func TestStreamingPort_OnBargeIn_NilClearsRegistration(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()

	var hits atomic.Int32
	p.OnBargeIn(func() { hits.Add(1) })
	p.OnBargeIn(nil)
	p.TriggerBargeIn() // should NOT increment
	if got := hits.Load(); got != 0 {
		t.Errorf("hits after clear+fire = %d, want 0", got)
	}
}

func TestStreamingPort_OnBargeIn_NoCallbackFireIsSafe(t *testing.T) {
	ms, cancel := newTestMS(t)
	defer cancel()
	p := newStreamingPort(nil, ms, 16000)
	defer func() { _ = p.Close() }()
	// No OnBargeIn registered yet; TriggerBargeIn must not panic.
	p.TriggerBargeIn()
}

// --- engine.MediaPort compliance ------------------------------------

func TestStreamingPort_SatisfiesMediaPort(t *testing.T) {
	// Compile-time check is already in the implementation file; this
	// runtime assertion is a belt-and-braces guard against an
	// accidental method-removal in a future refactor.
	var _ engine.MediaPort = (*StreamingCallSessionPort)(nil)
}
