package legacy

import (
	"errors"
	"fmt"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
)

// Factory implements engine.Factory by producing legacy.Engine values.
// Construct one per mode and register it with engine.Register.
//
// Factories are immutable after construction; the wrapped Attacher is
// captured at NewFactory time. To swap implementations (e.g. for tests
// that mock the legacy attach path), call SetAttacher on this package
// BEFORE the factory is registered, or build a fresh Factory.
type Factory struct {
	mode     engine.Mode
	attacher Attacher
}

// Build implements engine.Factory.Build. The returned Engine wraps the
// factory's Attacher; cfg is forwarded as-is.
func (f Factory) Build(cfg engine.Config) (engine.Engine, error) {
	if !f.mode.IsValid() {
		return nil, fmt.Errorf("dialog/legacy: invalid mode on factory: %q", string(f.mode))
	}
	if f.attacher == nil {
		return nil, fmt.Errorf("%w (factory mode=%q)", ErrNoAttacher, string(f.mode))
	}
	if cfg.Mode != "" && cfg.Mode != f.mode {
		return nil, fmt.Errorf("dialog/legacy: factory mode mismatch: factory=%q cfg=%q",
			string(f.mode), string(cfg.Mode))
	}
	cfg.Mode = f.mode
	return &Engine{mode: f.mode, attacher: f.attacher, cfg: cfg}, nil
}

// NewFactory builds a Factory bound to mode m and the given attacher.
// Both arguments are required; nil/invalid inputs panic at construction
// time so misconfiguration shows up at process start, not at first call.
func NewFactory(m engine.Mode, a Attacher) Factory {
	if !m.IsValid() {
		panic(fmt.Errorf("dialog/legacy: invalid mode %q", string(m)))
	}
	if a == nil {
		panic(fmt.Errorf("dialog/legacy: nil Attacher for mode %q", string(m)))
	}
	return Factory{mode: m, attacher: a}
}

// --- package-level Attacher registry (set at SIP-side init) ----------
//
// The SIP layer can call SetAttacher(mode, fn) once at init() time and
// then call Register(mode) without having to thread a Factory value
// through application bootstrap. This is the convenience path most
// call sites will use; tests may bypass it via NewFactory directly.

var (
	attachersMu sync.RWMutex
	attachers   = map[engine.Mode]Attacher{}
)

// ErrAttacherAlreadySet is returned (and panicked on) when SetAttacher
// is called twice for the same mode without an intervening
// ResetAttachersForTest.
var ErrAttacherAlreadySet = errors.New("dialog/legacy: Attacher already set for mode")

// SetAttacher installs a as the legacy attacher for mode m. Panics on
// double-set — intended to fail fast at init() time. Tests that need
// to swap attachers should use ResetAttachersForTest.
func SetAttacher(m engine.Mode, a Attacher) {
	if !m.IsValid() {
		panic(fmt.Errorf("dialog/legacy: invalid mode %q", string(m)))
	}
	if a == nil {
		panic(fmt.Errorf("dialog/legacy: nil Attacher for mode %q", string(m)))
	}
	attachersMu.Lock()
	defer attachersMu.Unlock()
	if _, ok := attachers[m]; ok {
		panic(fmt.Errorf("%w: %q", ErrAttacherAlreadySet, string(m)))
	}
	attachers[m] = a
}

// AttacherFor returns the wired attacher for m, or nil if none. Useful
// for diagnostics and for tests that want to invoke the attacher
// directly without going through a Factory.
func AttacherFor(m engine.Mode) Attacher {
	attachersMu.RLock()
	defer attachersMu.RUnlock()
	return attachers[m]
}

// Register builds a Factory for the wired attacher of mode m and
// installs it in the engine.Mode registry. Returns an error (rather
// than panicking) when the attacher is missing so the bootstrap layer
// can decide between hard-fail and graceful degradation per mode.
//
// Typical bootstrap shape:
//
//	legacy.SetAttacher(engine.ModeCascaded, conversation.LegacyCascadedAttacher)
//	legacy.SetAttacher(engine.ModeRealtime, conversation.LegacyRealtimeAttacher)
//	if err := legacy.Register(engine.ModeCascaded); err != nil { ... }
//	if err := legacy.Register(engine.ModeRealtime); err != nil { ... }
func Register(m engine.Mode) error {
	a := AttacherFor(m)
	if a == nil {
		return fmt.Errorf("%w (mode=%q)", ErrNoAttacher, string(m))
	}
	engine.Register(m, NewFactory(m, a))
	return nil
}

// ResetAttachersForTest clears every registered attacher. ONLY for use
// in tests; calling it from production code voids the warranty.
func ResetAttachersForTest() {
	attachersMu.Lock()
	defer attachersMu.Unlock()
	attachers = map[engine.Mode]Attacher{}
}
