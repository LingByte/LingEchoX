package conversation

import (
	"errors"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/legacy"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
)

// TestNewCallSessionPort_NilCallSession verifies the safe-nil contract.
// Wrapping nil returns nil; downstream `if port == nil` is the
// canonical way to skip the engine path.
func TestNewCallSessionPort_NilCallSession(t *testing.T) {
	if p := NewCallSessionPort(nil); p != nil {
		t.Fatalf("expected nil, got %T", p)
	}
}

// TestCallSessionPort_NilReceiverAccessors covers every accessor on a
// nil port (pre-nil-check defensive programming). Each must return its
// zero value without panicking.
func TestCallSessionPort_NilReceiverAccessors(t *testing.T) {
	var p *CallSessionPort
	if got := p.CallID(); got != "" {
		t.Errorf("CallID() = %q, want empty", got)
	}
	if got := p.TenantID(); got != "" {
		t.Errorf("TenantID() = %q, want empty", got)
	}
	if got := p.SampleRate(); got != 16000 {
		t.Errorf("SampleRate() = %d, want 16000", got)
	}
	if got := p.Codec(); got != (engine.CodecSpec{}) {
		t.Errorf("Codec() = %+v, want zero", got)
	}
	if got := p.LegacySession(); got != nil {
		t.Errorf("LegacySession() = %#v, want nil", got)
	}
	// InputPCM must return a closed channel (drains immediately).
	ch := p.InputPCM()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("InputPCM channel produced a value, want closed")
		}
	default:
		t.Error("InputPCM channel not closed on nil receiver")
	}
	// OnBargeIn is no-op; just verify it doesn't panic.
	p.OnBargeIn(func() {})
}

// TestCallSessionPort_NilReceiverSendOutput verifies that the streaming
// send method returns the bridge-only sentinel even on a nil port.
func TestCallSessionPort_NilReceiverSendOutput(t *testing.T) {
	var p *CallSessionPort
	if err := p.SendOutputPCM(engine.PCMFrame{}); !errors.Is(err, ErrLegacyBridgeOnly) {
		t.Errorf("SendOutputPCM err = %v, want ErrLegacyBridgeOnly", err)
	}
}

// TestCallSessionPort_AccessorsFromCallSession exercises the happy
// path: a CallSession with CallID set must be reflected by the port.
// We cannot construct a fully wired CallSession here (tons of deps),
// but the exported field CallID + the nil-safe TenantID/PCMSampleRate
// methods are enough to validate the port forwards correctly.
func TestCallSessionPort_AccessorsFromCallSession(t *testing.T) {
	cs := &sipSession.CallSession{CallID: "abc-123"}
	p := NewCallSessionPort(cs)
	if p == nil {
		t.Fatal("NewCallSessionPort returned nil for a non-nil CallSession")
	}
	if got := p.CallID(); got != "abc-123" {
		t.Errorf("CallID() = %q, want %q", got, "abc-123")
	}
	// TenantID() falls back to "0" because cs.TenantID() returns 0
	// when no tenant binding is set on the zero struct.
	if got := p.TenantID(); got != "0" {
		t.Errorf("TenantID() = %q, want %q", got, "0")
	}
	// PCMSampleRate falls back to 16000 when pcmSampleRate is zero.
	if got := p.SampleRate(); got != 16000 {
		t.Errorf("SampleRate() = %d, want 16000 (CallSession default)", got)
	}
	// Codec on a fresh CallSession is the zero codec → zero CodecSpec.
	if got := p.Codec(); got != (engine.CodecSpec{Channels: 1}) {
		// NegotiatedCodec returns sdp.Codec{} which has Channels=0;
		// our adapter normalises to Channels=1.
		t.Errorf("Codec() = %+v, want {Channels:1}", got)
	}
	// LegacySession round-trips the CallSession pointer.
	got := p.LegacySession()
	if got == nil {
		t.Fatal("LegacySession() = nil")
	}
	csOut, ok := got.(*sipSession.CallSession)
	if !ok || csOut != cs {
		t.Errorf("LegacySession() = %#v, want %#v", csOut, cs)
	}
}

// TestCallSessionPort_StreamingMethodsAreLegacyBridgeOnly hardens the
// "fail loud" contract on streaming.
func TestCallSessionPort_StreamingMethodsAreLegacyBridgeOnly(t *testing.T) {
	cs := &sipSession.CallSession{CallID: "x"}
	p := NewCallSessionPort(cs)

	t.Run("InputPCM closed", func(t *testing.T) {
		ch := p.InputPCM()
		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected closed channel")
			}
		default:
			t.Error("channel not closed")
		}
	})

	t.Run("SendOutputPCM errors", func(t *testing.T) {
		err := p.SendOutputPCM(engine.PCMFrame{Data: []byte{1, 2}, SampleRate: 8000})
		if !errors.Is(err, ErrLegacyBridgeOnly) {
			t.Errorf("err = %v, want ErrLegacyBridgeOnly", err)
		}
	})

	t.Run("OnBargeIn never fires", func(t *testing.T) {
		fired := false
		p.OnBargeIn(func() { fired = true })
		// No way to trigger barge-in via this port; ensure it didn't
		// fire synchronously (catches accidental immediate-call bugs).
		if fired {
			t.Error("OnBargeIn callback fired during registration")
		}
	})
}

// TestCallSessionPort_ImplementsLegacyHandle is a redundant guard for
// the var _ legacy.LegacyHandle = ... compile-time assertion. Keeps
// future refactors honest.
func TestCallSessionPort_ImplementsLegacyHandle(t *testing.T) {
	var _ legacy.LegacyHandle = (*CallSessionPort)(nil)
}
