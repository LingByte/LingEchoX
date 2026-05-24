package legacy

import (
	"context"
	"errors"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
)

// Attacher is the legacy attach function shape. Implementations live in
// pkg/sip/conversation (or a sibling SIP package) and close over
// whatever per-mode dependencies they need (provider registry, tenant
// loader, transfer hooks, ...).
//
// Contract:
//
//   - port is the new-interface handle. Implementations typically
//     type-assert to LegacyHandle (or a custom local interface) to get
//     back the *sipSession.CallSession; that escape hatch is what makes
//     this package a "bridge" rather than a "rewrite".
//   - Attacher SHOULD return quickly (no blocking on first audio); the
//     returned Detach is the canonical teardown path.
//   - Returning a nil Detach with a nil error is permitted and means
//     "I attached, but the underlying lifecycle is tied to the
//     transport's own context". This matches AttachVoicePipeline's
//     current shape: it returns nil and tear-down happens through
//     ms.GetContext().Done().
type Attacher func(
	ctx context.Context,
	cfg engine.Config,
	port engine.MediaPort,
	lg engine.Logger,
) (engine.Detach, error)

// LegacyHandle is the escape-hatch interface a MediaPort may implement
// to give an Attacher access to the underlying transport object
// (typically *sipSession.CallSession). It exists *solely* so the legacy
// attacher can keep using its existing helpers without first being
// rewritten on top of MediaPort.
//
// New code MUST NOT depend on this interface. Engines written against
// engine.Engine directly should ignore it; only the legacy package's
// callers reach for it.
type LegacyHandle interface {
	// LegacySession returns the transport-specific session object
	// (typically *sipSession.CallSession) or nil if unavailable. The
	// returned value's static type is intentionally untyped to keep
	// this package SIP-free; callers are expected to type-assert.
	LegacySession() any
}

// Errors surfaced by this package.
var (
	// ErrNoAttacher fires when Attach is called on a legacy engine
	// whose Attacher was never wired. Almost always a missing import
	// or a forgotten init() in the SIP layer.
	ErrNoAttacher = errors.New("dialog/legacy: no Attacher wired for this mode")

	// ErrNoLegacySession fires when the wired Attacher needs a
	// LegacySession but the port doesn't implement LegacyHandle (or
	// returned nil). New-style MediaPort implementations that don't
	// carry a CallSession trip this on purpose — they should not be
	// routed through the legacy engine in the first place.
	ErrNoLegacySession = errors.New("dialog/legacy: MediaPort has no LegacySession")
)
