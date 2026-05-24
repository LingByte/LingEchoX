package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Phase 1 PR-6 step B — AttachVoiceViaEngine, the entry point that
// drives the existing legacy attach through engine.New(...).Attach.
//
// This is the seam-flip helper. internal/sipserver/sipapp.go can swap
// its OnACK callback body from
//
//     return conversation.AttachVoicePipeline(ctx, cs, voiceLog)
//
// to
//
//     return conversation.AttachVoiceViaEngine(ctx, cs, voiceLog)
//
// and the runtime behaviour stays identical because the cascaded /
// realtime attachers wired in PR-4 ultimately call AttachVoicePipeline.
// The flip is what gives us mode-typed observability + a place for
// phase-3 native engines to land without churning callers again.

import (
	"context"
	"fmt"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// EngineAttachMode picks which engine.Mode the OnACK path requests
// from the registry. The legacy attacher dispatches by env.VoiceMode
// internally regardless, so this constant is informational today; it
// becomes load-bearing in phase 3 when native cascaded / realtime
// engines diverge.
//
// Defaulting to ModeCascaded is the safe choice: if the registry
// somehow only has ModeCascaded wired (e.g. a future feature flag
// disables realtime registration), realtime tenants still resolve
// through the cascaded slot and AttachVoicePipeline routes them
// correctly via env.VoiceMode.
const EngineAttachMode = engine.ModeCascaded

// AttachVoiceViaEngine wires the call through engine.New + Attach
// instead of calling AttachVoicePipeline directly. Behaviour-neutral
// today (delegates to the same legacy code via the wired attacher);
// the difference is purely architectural — it puts the on-ACK path
// on the same seam future engines will use.
//
// Errors:
//
//   - cs == nil → no-op, returns nil (matches AttachVoicePipeline's
//     existing safe-nil contract).
//   - bridge not wired (WireDialogEngineBridge never called or failed
//     to register the requested mode) → returns engine.ErrUnknownMode.
//   - underlying attacher fails (config error, tenant gate) → returns
//     the wrapped error.
func AttachVoiceViaEngine(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	port := NewCallSessionPort(cs)
	if port == nil {
		// Defensive: NewCallSessionPort already nil-checks but we
		// guard explicitly so a future change to that constructor
		// can't silently break this path.
		return fmt.Errorf("dialog engine attach: failed to wrap CallSession (call_id=%q)", cs.CallID)
	}
	cfg := engine.Config{
		Mode:     EngineAttachMode,
		CallID:   port.CallID(),
		TenantID: port.TenantID(),
	}
	eng, err := engine.New(cfg)
	if err != nil {
		return fmt.Errorf("dialog engine attach: build engine for mode=%q: %w", string(cfg.Mode), err)
	}
	_, err = eng.Attach(ctx, port, NewZapEngineLogger(lg))
	return err
}
