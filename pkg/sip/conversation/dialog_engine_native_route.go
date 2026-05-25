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
// What this PR does NOT do:
//
//   - Wire real ASR / LLM / TTS providers into cascaded.Engine. The
//     engine still uses stub stages today (PR-9c shipped the real
//     stage implementations behind options; the production-side
//     adapters that satisfy ASRRecognizer / LLMService / TTSService
//     land in PR-9e). Until then, native-routed traffic produces
//     silence — which is exactly why the feature flag defaults off
//     and ships in a separate PR from the provider wiring.
//
//   - Fall back from native to legacy on engine error. If the
//     feature flag is on for a tenant and engine.New / Attach fails,
//     the call ends. Operators flip the flag off to recover.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

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
	port := NewStreamingCallSessionPort(cs)
	if port == nil {
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("dialog engine native attach: failed to wrap CallSession into StreamingCallSessionPort (call_id=%q)",
			cs.CallID)
	}
	cfg := engine.Config{
		Mode:     engine.ModeCascadedNative,
		CallID:   port.CallID(),
		TenantID: port.TenantID(),
	}
	eng, err := engine.New(cfg)
	if err != nil {
		_ = port.Close()
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("dialog engine native attach: build engine for mode=%q: %w",
			string(cfg.Mode), err)
	}
	lg.Info("dialog engine native attach: routing through cascaded.Engine",
		zap.String("call_id", cs.CallID),
		zap.String("tenant_id", port.TenantID()),
		zap.String("mode", string(cfg.Mode)),
	)
	attachErr := cs.AttachVoiceConversation(func() error {
		_, e := eng.Attach(ctx, port, NewZapEngineLogger(lg))
		return e
	})
	if attachErr != nil {
		_ = port.Close()
		sipMetrics.VoiceAttachNative(false)
		return fmt.Errorf("dialog engine native attach: %w", attachErr)
	}
	sipMetrics.VoiceAttachNative(true)
	return nil
}
