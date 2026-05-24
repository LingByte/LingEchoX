package legacy

import (
	"context"
	"errors"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
)

// fakePort is a minimal MediaPort that optionally exposes a legacy
// session value via the LegacyHandle escape hatch.
type fakePort struct {
	session any
}

func (p *fakePort) InputPCM() <-chan engine.PCMFrame { return nil }
func (p *fakePort) SendOutputPCM(engine.PCMFrame) error {
	return errors.New("not implemented")
}
func (p *fakePort) OnBargeIn(func())     {}
func (p *fakePort) Codec() engine.CodecSpec { return engine.CodecSpec{} }
func (p *fakePort) SampleRate() int        { return 8000 }
func (p *fakePort) CallID() string         { return "call-fake" }
func (p *fakePort) TenantID() string       { return "tenant-fake" }
func (p *fakePort) LegacySession() any     { return p.session }

func TestNewFactory_PanicsOnInvalid(t *testing.T) {
	t.Run("invalid mode", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on invalid mode")
			}
		}()
		NewFactory(engine.Mode("nope"), func(context.Context, engine.Config, engine.MediaPort, engine.Logger) (engine.Detach, error) {
			return nil, nil
		})
	})

	t.Run("nil attacher", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on nil attacher")
			}
		}()
		NewFactory(engine.ModeCascaded, nil)
	})
}

func TestFactory_Build_PropagatesConfigAndMode(t *testing.T) {
	ResetAttachersForTest()

	var seen engine.Config
	a := func(_ context.Context, cfg engine.Config, _ engine.MediaPort, _ engine.Logger) (engine.Detach, error) {
		seen = cfg
		return nil, nil
	}
	f := NewFactory(engine.ModeCascaded, a)

	t.Run("mode mismatch is rejected", func(t *testing.T) {
		_, err := f.Build(engine.Config{Mode: engine.ModeRealtime})
		if err == nil {
			t.Fatal("expected mode mismatch error")
		}
	})

	t.Run("empty mode is filled in", func(t *testing.T) {
		eng, err := f.Build(engine.Config{CallID: "c1", TenantID: "t1"})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if eng.Mode() != engine.ModeCascaded {
			t.Errorf("Mode = %q, want %q", eng.Mode(), engine.ModeCascaded)
		}
		port := &fakePort{}
		if _, err := eng.Attach(context.Background(), port, engine.NopLogger{}); err != nil {
			t.Fatalf("Attach: %v", err)
		}
		if seen.CallID != "c1" || seen.TenantID != "t1" {
			t.Errorf("config not propagated: %+v", seen)
		}
		if seen.Mode != engine.ModeCascaded {
			t.Errorf("config Mode not normalised: %q", seen.Mode)
		}
	})
}

func TestEngine_Attach_IsSingleShot(t *testing.T) {
	calls := 0
	wantDetachErr := errors.New("teardown")
	a := func(context.Context, engine.Config, engine.MediaPort, engine.Logger) (engine.Detach, error) {
		calls++
		return func(context.Context) error { return wantDetachErr }, nil
	}
	eng, err := NewFactory(engine.ModeCascaded, a).Build(engine.Config{Mode: engine.ModeCascaded})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	port := &fakePort{}
	d1, err1 := eng.Attach(context.Background(), port, nil) // nil logger -> NopLogger
	d2, err2 := eng.Attach(context.Background(), port, nil)
	if err1 != nil || err2 != nil {
		t.Fatalf("attach errors: %v / %v", err1, err2)
	}
	if calls != 1 {
		t.Errorf("attacher invoked %d times, want 1", calls)
	}
	if d1 == nil || d2 == nil {
		t.Fatal("expected non-nil Detach on both calls")
	}
	if got := d1(context.Background()); !errors.Is(got, wantDetachErr) {
		t.Errorf("Detach err = %v, want %v", got, wantDetachErr)
	}
}

func TestEngine_Attach_LegacyHandleEscapeHatch(t *testing.T) {
	type sentinel struct{ tag string }
	want := &sentinel{tag: "call-session"}

	var got any
	a := func(_ context.Context, _ engine.Config, port engine.MediaPort, _ engine.Logger) (engine.Detach, error) {
		if h, ok := port.(LegacyHandle); ok {
			got = h.LegacySession()
		}
		return nil, nil
	}
	eng, err := NewFactory(engine.ModeRealtime, a).Build(engine.Config{Mode: engine.ModeRealtime})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := eng.Attach(context.Background(), &fakePort{session: want}, nil); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if got != want {
		t.Errorf("LegacySession = %#v, want %#v", got, want)
	}
}

func TestEngine_Attach_NilAttacher(t *testing.T) {
	// Constructed manually to bypass NewFactory's nil check.
	eng := &Engine{mode: engine.ModeCascaded, cfg: engine.Config{Mode: engine.ModeCascaded}}
	_, err := eng.Attach(context.Background(), &fakePort{}, nil)
	if !errors.Is(err, ErrNoAttacher) {
		t.Errorf("err = %v, want ErrNoAttacher", err)
	}
}

func TestSetAttacher_PanicsOnDoubleSet(t *testing.T) {
	ResetAttachersForTest()
	a := func(context.Context, engine.Config, engine.MediaPort, engine.Logger) (engine.Detach, error) {
		return nil, nil
	}
	SetAttacher(engine.ModeCascaded, a)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on double-set")
		}
	}()
	SetAttacher(engine.ModeCascaded, a)
}

func TestRegister_RoundTrip(t *testing.T) {
	ResetAttachersForTest()
	engine.ResetRegistryForTest()

	if err := Register(engine.ModeCascaded); !errors.Is(err, ErrNoAttacher) {
		t.Errorf("expected ErrNoAttacher before SetAttacher, got %v", err)
	}

	SetAttacher(engine.ModeCascaded, func(context.Context, engine.Config, engine.MediaPort, engine.Logger) (engine.Detach, error) {
		return func(context.Context) error { return nil }, nil
	})
	if err := Register(engine.ModeCascaded); err != nil {
		t.Fatalf("Register: %v", err)
	}

	eng, err := engine.New(engine.Config{Mode: engine.ModeCascaded, CallID: "c", TenantID: "t"})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	if eng.Mode() != engine.ModeCascaded {
		t.Errorf("mode = %q", eng.Mode())
	}
	if _, ok := eng.(*Engine); !ok {
		t.Errorf("expected *legacy.Engine, got %T", eng)
	}
}

func TestAttacherFor_Empty(t *testing.T) {
	ResetAttachersForTest()
	if a := AttacherFor(engine.ModeCascaded); a != nil {
		t.Errorf("expected nil before SetAttacher, got %T", a)
	}
}
