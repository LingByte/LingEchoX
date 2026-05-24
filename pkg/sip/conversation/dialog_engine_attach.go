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

// EngineAttachFallbackMode is the safe default mode used when
// ResolveAttachMode cannot determine a mode (nil cs, tenant config
// load error). The per-mode attacher registered under this mode will
// emit its own config_error.wav fallback, so the call leg still ends
// cleanly. Cascaded is the safer default because every supported
// tenant has historically had pipeline credentials configured even
// when realtime is the operational choice.
const EngineAttachFallbackMode = engine.ModeCascaded

// AttachVoiceViaEngine wires the call through engine.New + Attach.
// PR-7: mode is resolved up-front (ResolveAttachMode) so the
// per-mode attacher receives the right mode and registry metrics are
// mode-honest. Behaviour stays equivalent to AttachVoicePipeline:
// the resolver applies the same "pipeline mode + pipeline creds
// unusable + realtime ready → realtime" auto-fallback the historical
// dispatcher did.
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
	mode := ResolveAttachMode(ctx, cs, lg)
	if !mode.IsValid() {
		mode = EngineAttachFallbackMode
	}
	cfg := engine.Config{
		Mode:     mode,
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
