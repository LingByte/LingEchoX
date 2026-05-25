package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Phase 3 PR-9d — feature-flag routing of single-tenant cascaded
// traffic to the native cascaded.Engine (registered under
// engine.ModeCascadedNative) instead of the legacy bridge.
//
// Two env knobs:
//
//	DIALOG_NATIVE_CASCADED_ALL=1
//	  → every cascaded call goes through cascaded.Engine. Use only
//	    in dev / canary clusters; bypasses the per-tenant gate.
//
//	DIALOG_NATIVE_CASCADED_TENANTS=tenant-a,tenant-b
//	  → comma- or whitespace-separated allow-list of tenant IDs.
//	    Matched against MediaPort.TenantID() (string equality,
//	    case-sensitive). Empty list = nobody opted in.
//
// Both knobs are evaluated lazily on every AttachVoiceViaEngine call
// so config can be flipped without restarting the SIP server (e.g.
// via Kubernetes ConfigMap rollout). The cost is one os.Getenv +
// strings.Split per ACK; negligible compared to the cost of building
// the engine.
//
// The flag only affects ModeCascaded. Realtime traffic is untouched.
// When the flag fires, the native attach path:
//
//   - Constructs a StreamingCallSessionPort (real bi-directional
//     PCM, ctx-cancel teardown).
//   - Calls engine.New(cfg{Mode: ModeCascadedNative}) so the native
//     factory builds a cascaded.Engine.
//   - Passes the streaming port to engine.Attach.
//   - Counts the routing decision in
//     sip_voice_attach_native_total{result} so we can monitor opt-in
//     rollout from Grafana.
//
// PR-9e adds production-side provider adapters (see
// dialog_engine_native_providers.go) so opted-in tenants actually
// produce audio. ASR/LLM/TTS are loaded from the tenant's VoiceEnv
// (cached on ctx by ResolveAttachModeWithEnv) and passed as
// cascaded.With{ASRRecognizer,LLMService,TTSService} options. The
// registry-based engine.New(ModeCascadedNative) path is preserved
// for any caller that doesn't need provider injection; the
// feature-flag attach below uses cascaded.New directly to leverage
// the option pattern.
//
// What this PR does NOT do:
//
//   - Fall back from native to legacy on engine error. If the
//     feature flag is on for a tenant and provider/engine init
//     fails, the call ends. Operators flip the flag off to recover.
//
//   - Wire intent detection / transfer tool / hotword corrector
//     into the native pipeline. Those are next on the roadmap as
//     dedicated pipeline.Stages.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/dialog/cascaded"
	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// envNativeCascadedAll is the global override env var. Any non-empty
// value other than "0" / "false" enables native cascaded routing for
// every tenant.
const envNativeCascadedAll = "DIALOG_NATIVE_CASCADED_ALL"

// envNativeCascadedTenants is the per-tenant allow-list env var.
// Comma-, semicolon- or whitespace-separated tenant IDs.
const envNativeCascadedTenants = "DIALOG_NATIVE_CASCADED_TENANTS"

// nativeCascadedRouter is the small struct that owns the env
// lookups. Pulled out so tests can swap in a deterministic source
// without process-level os.Setenv juggling.
type nativeCascadedRouter struct {
	getenv func(string) string
}

// defaultNativeCascadedRouter is the production singleton; tests use
// withNativeCascadedRouter to install a fake.
var (
	nativeRouterMu      sync.RWMutex
	defaultNativeRouter = nativeCascadedRouter{getenv: os.Getenv}
)

// useNativeCascaded reports whether the cascaded call for tenantID
// should be routed through the native cascaded.Engine instead of the
// legacy bridge. The decision is:
//
//	DIALOG_NATIVE_CASCADED_ALL truthy        → true
//	tenantID ∈ DIALOG_NATIVE_CASCADED_TENANTS → true
//	otherwise                                → false
//
// Empty tenantID matches nothing (defensive — we don't want a
// missing tenant header to silently route through the experimental
// path).
func useNativeCascaded(tenantID string) bool {
	nativeRouterMu.RLock()
	r := defaultNativeRouter
	nativeRouterMu.RUnlock()
	return r.useNativeCascaded(tenantID)
}

func (r nativeCascadedRouter) useNativeCascaded(tenantID string) bool {
	if isTruthyEnv(r.getenv(envNativeCascadedAll)) {
		return true
	}
	if tenantID == "" {
		return false
	}
	for _, id := range parseTenantList(r.getenv(envNativeCascadedTenants)) {
		if id == tenantID {
			return true
		}
	}
	return false
}

// withNativeCascadedRouter swaps the singleton router for the
// duration of a test. Returns a cleanup func the caller must defer.
// Production code never calls this; it exists in the non-test file
// so test files in pkg/sip/conversation can use it without exporting
// router internals.
func withNativeCascadedRouter(getenv func(string) string) func() {
	nativeRouterMu.Lock()
	prev := defaultNativeRouter
	defaultNativeRouter = nativeCascadedRouter{getenv: getenv}
	nativeRouterMu.Unlock()
	return func() {
		nativeRouterMu.Lock()
		defaultNativeRouter = prev
		nativeRouterMu.Unlock()
	}
}

// isTruthyEnv treats "1", "true", "TRUE", "yes", "on" (case-folded)
// as truthy. Empty / "0" / "false" / "no" / "off" are falsy. Any
// other non-empty value is treated as truthy too — the env contract
// is "set the var to enable", and we err on the side of obeying the
// operator.
func isTruthyEnv(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// parseTenantList splits raw on comma, semicolon, or whitespace and
// returns the trimmed non-empty entries. Order-preserving; duplicates
// are kept (the lookup is O(n) and lists are tiny in practice).
func parseTenantList(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// attachVoiceViaNativeCascaded is the native-engine attach path
// invoked by AttachVoiceViaEngine when the feature flag fires for
// this tenant + cascaded mode.
//
// Behavioural contract:
//
//   - Returns nil for nil cs (matches AttachVoiceViaEngine's
//     existing safe-nil contract).
//   - Constructs a StreamingCallSessionPort. Failure to build it
//     (nil MediaSession, etc.) yields an error — the call ends. We
//     intentionally do NOT fall back to legacy: the operator opted
//     this tenant into the native path, so a build failure is a
//     real error worth surfacing.
//   - Builds an engine via engine.New(ModeCascadedNative) — relies
//     on cascaded.RegisterNative() having run at bootstrap.
//   - Calls eng.Attach inside cs.AttachVoiceConversation so the
//     CallSession's voice-conversation lifecycle still owns
//     mid-call cancellation / detach.
//
// Metrics:
//
//   - sip_voice_attach_native_total{result=ok|err} for the routing
//     decision outcome (separate from sip_voice_attach_total which
//     keeps tracking by mode label).
func attachVoiceViaNativeCascaded(
	ctx context.Context,
	cs *sipSession.CallSession,
	lg *zap.Logger,
) error {
	if cs == nil {
		return nil
	}
	lg = ensureVoiceLogger(lg)
	env, ok := loadVoiceEnvOrConfigError(ctx, cs, lg)
	if !ok {
		sipMetrics.VoiceAttachNative(false)
		return attachTenantConfigErrorPlayback(cs, lg)
	}
	if !pipelineCredsUsable(env) {
		sipMetrics.VoiceAttachNative(false)
		lg.Warn("native cascaded attach: tenant pipeline creds unusable; playing config error",
			zap.String("call_id", cs.CallID),
			zap.Uint("tenant_id", cs.TenantID()),
		)
		return attachTenantConfigErrorPlayback(cs, lg)
	}
	port := NewStreamingCallSessionPort(cs)
	if port == nil {
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("native cascaded attach: failed to wrap CallSession (call_id=%q)", cs.CallID)
	}
	// Build the three production-side adapters. Each failure path
	// closes the port + bumps the err metric so dashboards can tell
	// "feature flag on but tenant misconfigured" from "feature flag
	// on and engine bug". We do NOT fall back to legacy — the
	// operator opted this tenant in.
	asrSvc, err := buildNativeCascadedASR(env, lg)
	if err != nil {
		_ = port.Close()
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("native cascaded attach: ASR: %w", err)
	}
	llmSvc, err := buildNativeCascadedLLM(ctx, env, cs.CallID)
	if err != nil {
		_ = port.Close()
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("native cascaded attach: LLM: %w", err)
	}
	ttsSvc, err := buildNativeCascadedTTS(env, port.SampleRate(), lg)
	if err != nil {
		_ = port.Close()
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("native cascaded attach: TTS: %w", err)
	}
	cfg := engine.Config{
		Mode:     engine.ModeCascadedNative,
		CallID:   port.CallID(),
		TenantID: port.TenantID(),
	}
	// cascaded.New (rather than engine.New) so we can pass the
	// per-call provider Options. The registry-registered factory
	// stays in place for callers that don't need injection.
	eng := cascaded.New(cfg,
		cascaded.WithASRRecognizer(asrSvc),
		cascaded.WithLLMService(llmSvc),
		cascaded.WithTTSService(ttsSvc),
	)
	lg.Info("native cascaded attach: routing through cascaded.Engine",
		zap.String("call_id", cs.CallID),
		zap.String("tenant_id", port.TenantID()),
		zap.String("mode", string(cfg.Mode)),
		zap.String("asr_provider", env.ASRProvider),
		zap.String("llm_provider", env.LLMProvider),
		zap.String("tts_provider", env.TTSProvider),
	)
	attachErr := cs.AttachVoiceConversation(func() error {
		_, e := eng.Attach(ctx, port, NewZapEngineLogger(lg))
		return e
	})
	if attachErr != nil {
		_ = port.Close()
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("native cascaded attach: %w", attachErr)
	}
	sipMetrics.VoiceAttachNative(true)
	return nil
}
