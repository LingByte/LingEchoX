package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Phase 1 PR-7 — per-mode legacy attach helpers.
//
// AttachVoicePipeline (in voice.go) is the historical entry point that
// loads tenant env, decides cascaded vs realtime by inspecting
// env.VoiceMode, and dispatches to the right inner attach. That single
// entry served the legacy SIP layer fine, but the new engine.Mode
// registry wants ONE attacher per mode so cfg.Mode is load-bearing,
// metrics are mode-tagged, and future native engines can replace one
// mode without disturbing the other.
//
// This file splits the dispatch into two public per-mode helpers
// (AttachCascadedLegacy / AttachRealtimeLegacy) and exposes a
// resolver (ResolveAttachMode) that callers use to pick the right
// mode BEFORE invoking engine.New.
//
// Coexistence: AttachVoicePipeline is untouched and remains the entry
// point for non-OnACK callers (e.g. voicedialog hub). The new helpers
// are slotted in by dialog_engine_bridge.go for the OnACK path.

import (
	"context"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/tenantcfg"
	"github.com/LinByte/VoiceServer/pkg/logger"
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// ensureVoiceLogger picks a *zap.Logger when the caller passes nil.
// Mirrors the (private) pattern at the top of AttachVoicePipeline.
func ensureVoiceLogger(lg *zap.Logger) *zap.Logger {
	if lg != nil {
		return lg
	}
	if logger.Lg != nil {
		return logger.Lg
	}
	z, _ := zap.NewDevelopment()
	return z
}

// loadVoiceEnvOrConfigError is the shared preamble for both per-mode
// attach helpers. Returns (env, true) on success and (zero, false)
// when the caller should bounce the call to config_error.wav. Every
// failure path emits the same lg.Info / lg.Warn line as the original
// AttachVoicePipeline so SRE log alerts keep working unchanged.
func loadVoiceEnvOrConfigError(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) (VoiceEnv, bool) {
	// Fast path: a previous step in this OnACK call chain may have
	// already loaded the VoiceEnv (typically ResolveAttachMode does
	// this so it can pick cascaded vs realtime). Reusing that load
	// saves one DB hit per call (~10ms behind the loader cache).
	if env, ok := tenantcfg.VoiceEnvFromContext(ctx); ok {
		return env, true
	}
	tid := cs.TenantID()
	if tid == 0 {
		lg.Info("sip voice pipeline skipped (no tenant_id on call; play config_error)",
			zap.String("call_id", cs.CallID))
		return VoiceEnv{}, false
	}
	env, loaded, err := ResolveTenantVoiceEnv(ctx, cs)
	if err != nil {
		lg.Warn("sip voice tenant env load error",
			zap.String("call_id", cs.CallID),
			zap.Uint("tenant_id", tid),
			zap.Error(err),
		)
		return VoiceEnv{}, false
	}
	if !loaded {
		lg.Info("sip voice tenant row missing or loader not ok",
			zap.String("call_id", cs.CallID),
			zap.Uint("tenant_id", tid),
		)
		return VoiceEnv{}, false
	}
	return env, true
}

// ResolveAttachModeWithEnv is like ResolveAttachMode but also publishes
// the loaded VoiceEnv onto the returned context. Per-mode attachers
// called later in the same OnACK call chain skip a redundant tenant
// row lookup by reading the cached env via tenantcfg.VoiceEnvFromContext
// inside loadVoiceEnvOrConfigError. Returns the original ctx unchanged
// when the env could not be loaded (tenant gate / DB error) — the
// per-mode attacher will then re-attempt and surface the same
// config_error.wav fallback it always did.
func ResolveAttachModeWithEnv(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) (engine.Mode, context.Context) {
	if cs == nil {
		return engine.ModeCascaded, ctx
	}
	lg = ensureVoiceLogger(lg)
	env, ok := loadVoiceEnvOrConfigError(ctx, cs, lg)
	if !ok {
		return engine.ModeCascaded, ctx
	}
	// Stash for downstream per-mode attachers BEFORE returning.
	ctx = tenantcfg.WithVoiceEnv(ctx, env)
	if strings.EqualFold(env.VoiceMode, "realtime") {
		return engine.ModeRealtime, ctx
	}
	if strings.EqualFold(env.VoiceMode, "pipeline") && !pipelineCredsUsable(env) && TenantRealtimeReady(env) {
		lg.Info("sip voice: auto-resolving to realtime (pipeline creds unusable, realtime ready)",
			zap.String("call_id", cs.CallID),
			zap.Uint("tenant_id", cs.TenantID()),
		)
		sipMetrics.VoiceAttachModeFallback(sipMetrics.VoiceAttachModeCascaded, sipMetrics.VoiceAttachModeRealtime)
		return engine.ModeRealtime, ctx
	}
	return engine.ModeCascaded, ctx
}

// ResolveAttachMode picks the engine.Mode that best matches the tenant's
// configuration. The decision tree mirrors AttachVoicePipeline's
// "last-mile mode normalisation" so the on-ACK seam preserves the
// auto-fallback contract:
//
//	tenant row missing            → ModeCascaded (caller's per-mode
//	                                attacher will hit config_error.wav)
//	env.VoiceMode == "realtime"   → ModeRealtime
//	env.VoiceMode == "pipeline" + pipeline creds unusable
//	                              + realtime ready
//	                              → ModeRealtime (auto-fallback)
//	otherwise                     → ModeCascaded
//
// The lg argument is optional; nil falls back to logger.Lg.
//
// Callers that don't need the publishing behaviour of
// ResolveAttachModeWithEnv use this thin wrapper. The OnACK path
// (AttachVoiceViaEngine) prefers ResolveAttachModeWithEnv so the
// per-mode attacher re-uses the load.
func ResolveAttachMode(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) engine.Mode {
	mode, _ := ResolveAttachModeWithEnv(ctx, cs, lg)
	return mode
}

// AttachCascadedLegacy forces the cascaded (3-step ASR + LLM + TTS)
// attach path. Loads tenant env, validates pipeline credentials, and
// either runs the inner attach with env.VoiceMode pinned to "pipeline"
// or falls back to attachTenantConfigErrorPlayback on any config
// problem. Returns nil for nil cs (matches AttachVoicePipeline's
// existing safe-nil contract).
func AttachCascadedLegacy(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	lg = ensureVoiceLogger(lg)
	env, ok := loadVoiceEnvOrConfigError(ctx, cs, lg)
	if !ok {
		return attachTenantConfigErrorPlayback(cs, lg)
	}
	if !pipelineCredsUsable(env) {
		lg.Info("sip voice cascaded requested but pipeline credentials unusable",
			zap.String("call_id", cs.CallID),
			zap.Uint("tenant_id", cs.TenantID()),
			zap.String("asr_provider", env.ASRProvider),
			zap.String("tts_provider", env.TTSProvider),
			zap.String("llm_provider", env.LLMProvider),
		)
		return attachTenantConfigErrorPlayback(cs, lg)
	}
	// Pin mode so attachVoiceInner's internal "if VoiceMode==realtime"
	// branch cannot fire — this attacher is mode-honest.
	env.VoiceMode = "pipeline"
	return cs.AttachVoiceConversation(func() error {
		return attachVoiceInner(ctx, cs, env, lg)
	})
}

// AttachRealtimeLegacy forces the realtime (multimodal) attach path.
// Same shape as AttachCascadedLegacy but validates realtime creds and
// pins env.VoiceMode = "realtime" so attachRealtimeVoiceInner runs.
func AttachRealtimeLegacy(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	lg = ensureVoiceLogger(lg)
	env, ok := loadVoiceEnvOrConfigError(ctx, cs, lg)
	if !ok {
		return attachTenantConfigErrorPlayback(cs, lg)
	}
	if !TenantRealtimeReady(env) {
		lg.Info("sip voice realtime requested but realtime config incomplete",
			zap.String("call_id", cs.CallID),
			zap.Uint("tenant_id", cs.TenantID()),
			zap.String("realtime_provider", env.RealtimeProvider),
			zap.Bool("realtime_config_present", len(env.RealtimeConfigRaw) > 0),
		)
		return attachTenantConfigErrorPlayback(cs, lg)
	}
	env.VoiceMode = "realtime"
	return cs.AttachVoiceConversation(func() error {
		// attachVoiceInner will short-circuit to
		// attachRealtimeVoiceInner because env.VoiceMode == "realtime".
		// We don't call attachRealtimeVoiceInner directly to keep one
		// canonical entry point in voice.go, even though it would
		// shave a function frame.
		return attachVoiceInner(ctx, cs, env, lg)
	})
}
