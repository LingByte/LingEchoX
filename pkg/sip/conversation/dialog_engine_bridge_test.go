package conversation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/legacy"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// noopLegacyAttach is the test-only attach function injected into
// perModeLegacyAttacher to verify the closure's guard rails without
// touching real AttachVoicePipeline code.
func noopLegacyAttach(context.Context, *sipSession.CallSession, *zap.Logger) error {
	return nil
}

// TestWireDialogEngineBridge_RegistersBothModes verifies that the
// wire helper installs both cascaded and realtime in the engine
// registry. It does NOT exercise the attach codepath — that needs a
// real *sipSession.CallSession and is exercised by integration tests.
func TestWireDialogEngineBridge_RegistersBothModes(t *testing.T) {
	resetDialogEngineBridgeForTest(t)

	wired, errs := WireDialogEngineBridge()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(wired) != 2 {
		t.Fatalf("wired modes = %v, want both cascaded+realtime", wired)
	}
	got := map[engine.Mode]bool{}
	for _, m := range wired {
		got[m] = true
	}
	if !got[engine.ModeCascaded] || !got[engine.ModeRealtime] {
		t.Errorf("missing mode in %v", wired)
	}
	regs := engine.RegisteredModes()
	if len(regs) < 2 {
		t.Errorf("engine.RegisteredModes = %v, want >=2", regs)
	}
}

// TestWireDialogEngineBridge_IsIdempotent ensures the once-guard works
// — repeated calls don't panic on legacy.SetAttacher double-set.
func TestWireDialogEngineBridge_IsIdempotent(t *testing.T) {
	resetDialogEngineBridgeForTest(t)
	if _, errs := WireDialogEngineBridge(); len(errs) != 0 {
		t.Fatalf("first wire errs: %v", errs)
	}
	wired, errs := WireDialogEngineBridge()
	if len(wired) != 0 || len(errs) != 0 {
		t.Errorf("second wire returned wired=%v errs=%v, want empty (once-guard)", wired, errs)
	}
}

func TestBridgeZapLogger_UnwrapsAndFallsBack(t *testing.T) {
	// Path 1: passing a *zapEngineLogger unwraps to the original.
	z, _ := zap.NewDevelopment()
	wrapped := NewZapEngineLogger(z)
	if got := bridgeZapLogger(wrapped); got != z {
		t.Errorf("bridgeZapLogger(*zapEngineLogger) = %p, want %p (unwrap)", got, z)
	}

	// Path 2: passing engine.NopLogger falls back to a non-nil logger.
	// (Either logger.Lg if initialised, or a fresh dev logger.)
	if got := bridgeZapLogger(engine.NopLogger{}); got == nil {
		t.Error("bridgeZapLogger(NopLogger) returned nil; want fallback")
	}

	// Path 3: passing nil falls back too.
	if got := bridgeZapLogger(nil); got == nil {
		t.Error("bridgeZapLogger(nil) returned nil; want fallback")
	}
}

func TestZapEngineLogger_WithEmptyFieldsShortCircuits(t *testing.T) {
	z, _ := zap.NewDevelopment()
	lg := NewZapEngineLogger(z).(*zapEngineLogger)
	if got := lg.With(); got != lg {
		t.Error("With() with no fields should return receiver unchanged")
	}
}

func TestZapFields_EmptyInputReturnsNil(t *testing.T) {
	if got := zapFields(nil); got != nil {
		t.Errorf("zapFields(nil) = %v, want nil", got)
	}
	if got := zapFields([]engine.Field{}); got != nil {
		t.Errorf("zapFields([]) = %v, want nil", got)
	}
}

func TestZapEngineLogger_AdaptsFields(t *testing.T) {
	// Smoke test — we don't assert log output (no observer), just that
	// every method runs without panicking and With chains correctly.
	z, _ := zap.NewDevelopment()
	lg := NewZapEngineLogger(z)
	lg.Debug("d", engine.F("a", 1))
	lg.Info("i", engine.F("b", "x"))
	lg.Warn("w", engine.F("", nil)) // empty key falls back to "field"
	lg.Error("e", engine.F("err", errors.New("boom")))
	chained := lg.With(engine.F("call_id", "c1"))
	if chained == nil {
		t.Fatal("With returned nil")
	}
	chained.Info("after With", engine.F("k", 2))

	// nil zap logger -> NopLogger, must not panic.
	nop := NewZapEngineLogger(nil)
	nop.Info("nop")
	if _, ok := nop.(engine.NopLogger); !ok {
		t.Errorf("NewZapEngineLogger(nil) = %T, want NopLogger", nop)
	}
}

func TestPerModeLegacyAttacher_RejectsMissingHandle(t *testing.T) {
	att := perModeLegacyAttacher(engine.ModeCascaded, noopLegacyAttach)
	// Use a port that does NOT implement legacy.LegacyHandle.
	if _, err := att(context.Background(), engine.Config{CallID: "c1"}, bareMediaPort{}, engine.NopLogger{}); err == nil {
		t.Fatal("expected error when MediaPort lacks LegacyHandle")
	}
}

func TestPerModeLegacyAttacher_RejectsWrongSessionType(t *testing.T) {
	att := perModeLegacyAttacher(engine.ModeRealtime, noopLegacyAttach)
	port := &handlePort{session: "not-a-call-session"}
	_, err := att(context.Background(), engine.Config{CallID: "c2"}, port, engine.NopLogger{})
	if err == nil {
		t.Fatal("expected error when LegacySession returns wrong type")
	}
}

func TestPerModeLegacyAttacher_RejectsNilSession(t *testing.T) {
	att := perModeLegacyAttacher(engine.ModeCascaded, noopLegacyAttach)
	port := &handlePort{session: nil}
	_, err := att(context.Background(), engine.Config{CallID: "c3"}, port, engine.NopLogger{})
	if !errors.Is(err, legacy.ErrNoLegacySession) {
		t.Errorf("err = %v, want ErrNoLegacySession", err)
	}
}

func TestPerModeLegacyAttacher_DelegatesOnHappyPath(t *testing.T) {
	var got struct {
		called int
		cs     *sipSession.CallSession
	}
	att := perModeLegacyAttacher(engine.ModeCascaded, func(_ context.Context, cs *sipSession.CallSession, _ *zap.Logger) error {
		got.called++
		got.cs = cs
		return nil
	})
	cs := &sipSession.CallSession{CallID: "happy"}
	port := &handlePort{session: cs}
	detach, err := att(context.Background(), engine.Config{Mode: engine.ModeCascaded, CallID: "happy"}, port, engine.NopLogger{})
	if err != nil {
		t.Fatalf("attacher: %v", err)
	}
	if detach != nil {
		t.Errorf("expected nil Detach (legacy lifecycle owned by MediaSession), got %T", detach)
	}
	if got.called != 1 || got.cs != cs {
		t.Errorf("delegate not invoked correctly: called=%d cs=%p (want %p)", got.called, got.cs, cs)
	}
}

func TestPerModeLegacyAttacher_PropagatesError(t *testing.T) {
	wantErr := errors.New("underlying failed")
	att := perModeLegacyAttacher(engine.ModeRealtime, func(context.Context, *sipSession.CallSession, *zap.Logger) error {
		return wantErr
	})
	cs := &sipSession.CallSession{CallID: "err"}
	port := &handlePort{session: cs}
	_, err := att(context.Background(), engine.Config{Mode: engine.ModeRealtime}, port, engine.NopLogger{})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestPerModeLegacyAttacher_LogsModeMismatch(t *testing.T) {
	// Smoke test: cfg.Mode != registered mode should not return an
	// error (it's a defensive warn only). We use a NopLogger to avoid
	// the log assertion machinery; not panicking + propagating to
	// delegate is enough.
	att := perModeLegacyAttacher(engine.ModeCascaded, noopLegacyAttach)
	cs := &sipSession.CallSession{CallID: "mismatch"}
	port := &handlePort{session: cs}
	if _, err := att(context.Background(), engine.Config{Mode: engine.ModeRealtime}, port, engine.NopLogger{}); err != nil {
		t.Errorf("mode mismatch must warn-not-fail; got err=%v", err)
	}
}

// --- helpers -----------------------------------------------------------

// bareMediaPort is a MediaPort that does NOT implement LegacyHandle.
type bareMediaPort struct{}

func (bareMediaPort) InputPCM() <-chan engine.PCMFrame    { return nil }
func (bareMediaPort) SendOutputPCM(engine.PCMFrame) error { return nil }
func (bareMediaPort) OnBargeIn(func())                    {}
func (bareMediaPort) Codec() engine.CodecSpec             { return engine.CodecSpec{} }
func (bareMediaPort) SampleRate() int                     { return 8000 }
func (bareMediaPort) CallID() string                      { return "bare" }
func (bareMediaPort) TenantID() string                    { return "bare" }

// handlePort is a MediaPort that DOES implement legacy.LegacyHandle.
type handlePort struct {
	bareMediaPort
	session any
}

func (p *handlePort) LegacySession() any { return p.session }

// resetDialogEngineBridgeForTest tears down the once-guard, the
// legacy attacher map, and the engine registry so each test starts
// from a clean state. Mirrors the pattern in pkg/dialog/engine and
// pkg/dialog/legacy tests.
func resetDialogEngineBridgeForTest(t *testing.T) {
	t.Helper()
	dialogEngineBridgeOnce = sync.Once{}
	legacy.ResetAttachersForTest()
	engine.ResetRegistryForTest()
}
