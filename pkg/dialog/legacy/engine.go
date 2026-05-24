package legacy

import (
	"context"
	"fmt"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
)

// Engine is the engine.Engine adapter that delegates to a wired
// Attacher. One instance per call; constructed by Factory.Build.
//
// Engine itself is goroutine-safe only in the sense that Mode and the
// (single) Attach call are independent. Calling Attach twice on the
// same Engine returns an error — engines are single-shot, matching the
// per-call lifecycle of the dialog layer.
type Engine struct {
	mode     engine.Mode
	attacher Attacher
	cfg      engine.Config

	attachOnce sync.Once
	attachErr  error
	detach     engine.Detach
}

// Mode returns the mode the engine was constructed for. Matches the
// Factory it came from.
func (e *Engine) Mode() engine.Mode { return e.mode }

// Attach wires the engine to the given MediaPort by invoking the
// wrapped Attacher. The legacy attacher reaches into the port via the
// LegacyHandle escape hatch (see attacher.go) to get the underlying
// CallSession.
//
// Idempotency: Attach is single-shot. Subsequent calls return the
// original (Detach, error) pair. This is intentional: engines are
// per-call objects, and a second Attach would indicate a serious bug
// in the orchestration layer.
func (e *Engine) Attach(ctx context.Context, port engine.MediaPort, lg engine.Logger) (engine.Detach, error) {
	if e.attacher == nil {
		return nil, fmt.Errorf("%w (mode=%q)", ErrNoAttacher, string(e.mode))
	}
	if lg == nil {
		lg = engine.NopLogger{}
	}
	e.attachOnce.Do(func() {
		d, err := e.attacher(ctx, e.cfg, port, lg)
		e.detach = d
		e.attachErr = err
	})
	return e.detach, e.attachErr
}

// Compile-time interface assertion.
var _ engine.Engine = (*Engine)(nil)
