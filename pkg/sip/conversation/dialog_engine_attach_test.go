package conversation

import (
	"context"
	"errors"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/legacy"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
)

// TestAttachVoiceViaEngine_NilCallSessionIsNoOp matches AttachVoicePipeline's
// existing safe-nil contract: a nil CallSession returns nil without
// going anywhere near the registry.
func TestAttachVoiceViaEngine_NilCallSessionIsNoOp(t *testing.T) {
	if err := AttachVoiceViaEngine(context.Background(), nil, nil); err != nil {
		t.Errorf("AttachVoiceViaEngine(nil) = %v, want nil", err)
	}
}

// TestAttachVoiceViaEngine_BridgeNotWired returns ErrUnknownMode when
// the bridge hasn't been wired (registry empty).
func TestAttachVoiceViaEngine_BridgeNotWired(t *testing.T) {
	resetDialogEngineBridgeForTest(t)
	cs := &sipSession.CallSession{CallID: "c-bridge-down"}
	err := AttachVoiceViaEngine(context.Background(), cs, nil)
	if err == nil {
		t.Fatal("expected error when registry empty")
	}
	if !errors.Is(err, engine.ErrUnknownMode) {
		t.Errorf("err = %v, want wraps ErrUnknownMode", err)
	}
}

// TestAttachVoiceViaEngine_HappyPathDelegatesToAttacher swaps in a
// recording attacher (instead of WireDialogEngineBridge) and verifies
// that:
//
//   - engine.New picks our cascaded factory
//   - the port handed to the attacher exposes the right CallID + TenantID
//   - the attacher's returned (Detach, error) round-trips through Attach
func TestAttachVoiceViaEngine_HappyPathDelegatesToAttacher(t *testing.T) {
	resetDialogEngineBridgeForTest(t)

	type observed struct {
		cfg     engine.Config
		callID  string
		tenant  string
		gotCS   *sipSession.CallSession
		invoked int
	}
	var rec observed

	att := func(_ context.Context, cfg engine.Config, port engine.MediaPort, _ engine.Logger) (engine.Detach, error) {
		rec.invoked++
		rec.cfg = cfg
		rec.callID = port.CallID()
		rec.tenant = port.TenantID()
		if h, ok := port.(legacy.LegacyHandle); ok {
			if cs, _ := h.LegacySession().(*sipSession.CallSession); cs != nil {
				rec.gotCS = cs
			}
		}
		return nil, nil
	}
	legacy.SetAttacher(EngineAttachMode, att)
	if err := legacy.Register(EngineAttachMode); err != nil {
		t.Fatalf("legacy.Register: %v", err)
	}

	cs := &sipSession.CallSession{CallID: "c-happy"}
	if err := AttachVoiceViaEngine(context.Background(), cs, nil); err != nil {
		t.Fatalf("AttachVoiceViaEngine: %v", err)
	}

	if rec.invoked != 1 {
		t.Errorf("attacher invoked %d times, want 1", rec.invoked)
	}
	if rec.cfg.Mode != EngineAttachMode {
		t.Errorf("cfg.Mode = %q, want %q", rec.cfg.Mode, EngineAttachMode)
	}
	if rec.callID != "c-happy" {
		t.Errorf("port.CallID = %q, want %q", rec.callID, "c-happy")
	}
	if rec.tenant != "0" {
		t.Errorf("port.TenantID = %q, want %q (cs has no tenant)", rec.tenant, "0")
	}
	if rec.gotCS != cs {
		t.Errorf("LegacySession round-trip: got %p want %p", rec.gotCS, cs)
	}
	if rec.cfg.CallID != "c-happy" || rec.cfg.TenantID != "0" {
		t.Errorf("cfg propagation: got %+v", rec.cfg)
	}
}

// TestAttachVoiceViaEngine_AttacherErrorPropagates ensures that an
// error from the legacy attacher (e.g. tenant config gate) reaches
// the OnACK callback unchanged.
func TestAttachVoiceViaEngine_AttacherErrorPropagates(t *testing.T) {
	resetDialogEngineBridgeForTest(t)

	wantErr := errors.New("tenant config invalid")
	att := func(context.Context, engine.Config, engine.MediaPort, engine.Logger) (engine.Detach, error) {
		return nil, wantErr
	}
	legacy.SetAttacher(EngineAttachMode, att)
	if err := legacy.Register(EngineAttachMode); err != nil {
		t.Fatalf("legacy.Register: %v", err)
	}

	cs := &sipSession.CallSession{CallID: "c-err"}
	err := AttachVoiceViaEngine(context.Background(), cs, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestEngineAttachMode_IsRegisterableMode is a redundant guard:
// EngineAttachMode is a constant and trivially valid, but if a future
// change picks an invalid mode, this test catches it loudly.
func TestEngineAttachMode_IsRegisterableMode(t *testing.T) {
	if !EngineAttachMode.IsValid() {
		t.Fatalf("EngineAttachMode = %q is not a valid engine.Mode", EngineAttachMode)
	}
}
