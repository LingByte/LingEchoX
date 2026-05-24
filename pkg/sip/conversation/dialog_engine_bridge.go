package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// SIP-side wiring for the dialog refactor's new engine.Engine
// interface (see docs/refactor-architecture.md §5.1, phase 1 PR-4).
//
// This file registers the conversation-package legacy attach path
// (AttachVoicePipeline) under both ModeCascaded and ModeRealtime in
// the engine.Mode registry, so future code can target engine.New +
// engine.Engine.Attach instead of calling AttachVoicePipeline directly.
//
// PRODUCTION CALL PATHS ARE NOT SWITCHED IN THIS PR. The SIP server
// bootstrap (internal/sipserver) still invokes AttachVoicePipeline via
// the existing on-ACK callback; engine.New is only reachable from
// tests and future PRs. The registry wiring is observable via
// engine.RegisteredModes() for health endpoints.
//
// When phase 5+ switches the call path, that PR will:
//   1. Build the MediaPort adapter on top of *sipSession.CallSession
//      (implementing engine.MediaPort + legacy.LegacyHandle).
//   2. Replace the on-ACK callback body with:
//          eng, err := engine.New(engine.Config{Mode: mode, ...})
//          if err != nil { return err }
//          _, err = eng.Attach(ctx, port, engineLogger(voiceLog))
//          return err
//   3. Delete this file's "behaviour-neutral" disclaimer.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/legacy"
	"github.com/LinByte/VoiceServer/pkg/logger"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// dialogEngineBridgeOnce guards Wire* so repeated bootstraps in tests
// (or hot reload, theoretical) don't panic the legacy.SetAttacher
// double-set guard.
var dialogEngineBridgeOnce sync.Once

// WireDialogEngineBridge registers per-mode legacy attachers
// (AttachCascadedLegacy + AttachRealtimeLegacy) in the engine.Mode
// registry. Safe to call multiple times; subsequent calls are no-ops.
//
// Returns the modes successfully wired (in registration order) and
// any per-mode error encountered. An error on one mode does not abort
// registration of the other.
//
// PR-7 split: cascaded and realtime now own DISTINCT attachers. The
// registered mode is load-bearing — the cascaded attacher will not
// silently fall back to realtime via env.VoiceMode anymore; the OnACK
// caller must resolve mode up-front (ResolveAttachMode in
// dialog_engine_attach.go). This makes metrics and logs mode-honest.
func WireDialogEngineBridge() (wired []engine.Mode, errs []error) {
	dialogEngineBridgeOnce.Do(func() {
		legacy.SetAttacher(engine.ModeCascaded, perModeLegacyAttacher(engine.ModeCascaded, AttachCascadedLegacy))
		legacy.SetAttacher(engine.ModeRealtime, perModeLegacyAttacher(engine.ModeRealtime, AttachRealtimeLegacy))
		for _, m := range []engine.Mode{engine.ModeCascaded, engine.ModeRealtime} {
			if err := legacy.Register(m); err != nil {
				errs = append(errs, fmt.Errorf("dialog engine bridge: register %q: %w", string(m), err))
				continue
			}
			wired = append(wired, m)
		}
	})
	return wired, errs
}

// perModeLegacyAttacher builds the legacy.Attacher closure that
// satisfies the engine seam for ONE mode by delegating to the matching
// AttachCascadedLegacy / AttachRealtimeLegacy entry point.
//
// Responsibilities:
//   - Extract *sipSession.CallSession via the LegacyHandle escape hatch.
//   - Adapt engine.Logger -> *zap.Logger.
//   - Log a mode-mismatch warning when cfg.Mode disagrees with the
//     registered mode (should never happen post-PR-7 because
//     ResolveAttachMode picks before engine.New; treat as a defensive
//     observability hook).
//   - Translate the *zap.Logger-returning attach signature into the
//     (Detach, error) shape the legacy bridge expects.
func perModeLegacyAttacher(mode engine.Mode, fn func(context.Context, *sipSession.CallSession, *zap.Logger) error) legacy.Attacher {
	return func(ctx context.Context, cfg engine.Config, port engine.MediaPort, lg engine.Logger) (engine.Detach, error) {
		h, ok := port.(legacy.LegacyHandle)
		if !ok {
			return nil, fmt.Errorf("dialog engine bridge: MediaPort missing LegacyHandle (mode=%q call_id=%q)",
				string(mode), cfg.CallID)
		}
		sessVal := h.LegacySession()
		if sessVal == nil {
			return nil, fmt.Errorf("%w (mode=%q call_id=%q)", legacy.ErrNoLegacySession, string(mode), cfg.CallID)
		}
		cs, ok := sessVal.(*sipSession.CallSession)
		if !ok {
			return nil, fmt.Errorf("dialog engine bridge: LegacySession is %T, want *sipSession.CallSession (mode=%q)",
				sessVal, string(mode))
		}
		voiceLog := bridgeZapLogger(lg).Named("sip-voice")
		if cfg.Mode != mode && cfg.Mode != "" {
			voiceLog.Warn("dialog engine bridge: cfg.Mode does not match registered mode",
				zap.String("call_id", cs.CallID),
				zap.String("cfg_mode", string(cfg.Mode)),
				zap.String("registered_mode", string(mode)),
			)
		} else {
			voiceLog.Debug("dialog engine bridge attach (per-mode legacy)",
				zap.String("call_id", cs.CallID),
				zap.String("mode", string(mode)),
			)
		}
		// Legacy lifecycle: attach functions wire processors onto the
		// MediaSession; teardown is driven by ms.GetContext().Done(),
		// not by an explicit Detach handle. Returning nil from the
		// Detach slot is part of the bridge contract documented in
		// pkg/dialog/legacy/attacher.go.
		if err := fn(ctx, cs, voiceLog); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

// bridgeZapLogger pulls a *zap.Logger out of the engine.Logger we were
// handed. In production the caller passes our zapEngineLogger adapter
// (so we just unwrap); in tests / mocks it may pass NopLogger, in which
// case we fall back to the process default. We never return nil.
func bridgeZapLogger(lg engine.Logger) *zap.Logger {
	if zw, ok := lg.(*zapEngineLogger); ok && zw != nil && zw.z != nil {
		return zw.z
	}
	if logger.Lg != nil {
		return logger.Lg
	}
	z, _ := zap.NewDevelopment()
	return z
}

// zapEngineLogger adapts *zap.Logger to engine.Logger so the SIP layer
// can pass its zap loggers across the engine seam without breaking the
// engine package's "no zap dependency" invariant.
type zapEngineLogger struct{ z *zap.Logger }

// NewZapEngineLogger wraps z as an engine.Logger. nil z returns a
// no-op logger so callers don't have to nil-check.
func NewZapEngineLogger(z *zap.Logger) engine.Logger {
	if z == nil {
		return engine.NopLogger{}
	}
	return &zapEngineLogger{z: z}
}

func (l *zapEngineLogger) Debug(msg string, fields ...engine.Field) {
	l.z.Debug(msg, zapFields(fields)...)
}
func (l *zapEngineLogger) Info(msg string, fields ...engine.Field) {
	l.z.Info(msg, zapFields(fields)...)
}
func (l *zapEngineLogger) Warn(msg string, fields ...engine.Field) {
	l.z.Warn(msg, zapFields(fields)...)
}
func (l *zapEngineLogger) Error(msg string, fields ...engine.Field) {
	l.z.Error(msg, zapFields(fields)...)
}
func (l *zapEngineLogger) With(fields ...engine.Field) engine.Logger {
	if len(fields) == 0 {
		return l
	}
	return &zapEngineLogger{z: l.z.With(zapFields(fields)...)}
}

func zapFields(in []engine.Field) []zap.Field {
	if len(in) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(in))
	for _, f := range in {
		key := strings.TrimSpace(f.Key)
		if key == "" {
			key = "field"
		}
		out = append(out, zap.Any(key, f.Value))
	}
	return out
}
