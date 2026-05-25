// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// PR-10b — SIP-side adapter that bridges pkg/realtime.Agent (the
// transport-level full-duplex multimodal client) into the
// pkg/dialog/realtime.Agent contract the native realtime engine
// consumes. No SIP routing change happens here; this is pure
// plumbing so PR-10c can wire it to AttachVoiceViaEngine without
// also having to land the adapter at the same time.
//
// Design notes
//
//   - The adapter is a SIP-side concern, not part of pkg/dialog/realtime,
//     because it touches tenant credentials (env.RealtimeConfigRaw),
//     the realtime tool registry, and per-call instructions / system
//     prompt — all of which live near SIP, not near the engine.
//   - The translation function (translateRealtimeEvent) is exposed for
//     tests; the runtime adapter holds it inside a closure so the
//     event sink lifecycle is clear.

package conversation

import (
	"context"
	"os"

	dialogrealtime "github.com/LinByte/VoiceServer/pkg/dialog/realtime"
	"github.com/LinByte/VoiceServer/pkg/realtime"
)

// nativeRealtimeAgentAdapter wraps a pkg/realtime.Agent so the
// native realtime engine sees the local Agent interface only. The
// adapter is intentionally minimal — it forwards each method
// directly. Future cross-cutting concerns (metric instrumentation,
// circuit-breaker, retry-once-on-Start) belong here, not inside the
// engine.
type nativeRealtimeAgentAdapter struct {
	inner realtime.Agent
}

// Start implements pkg/dialog/realtime.Agent.
func (a *nativeRealtimeAgentAdapter) Start(ctx context.Context) error {
	return a.inner.Start(ctx)
}

// PushAudio implements pkg/dialog/realtime.Agent.
func (a *nativeRealtimeAgentAdapter) PushAudio(pcm []byte) error {
	return a.inner.PushAudio(pcm)
}

// Cancel implements pkg/dialog/realtime.Agent.
func (a *nativeRealtimeAgentAdapter) Cancel() error { return a.inner.Cancel() }

// Close implements pkg/dialog/realtime.Agent.
func (a *nativeRealtimeAgentAdapter) Close() error { return a.inner.Close() }

// translateRealtimeEvent maps one pkg/realtime.Event to the engine's
// local Event. Returning a pointer with ok=false signals "drop this
// event" — used for vendor-specific events that have no downstream
// equivalent (e.g. EventSessionOpen, which the engine does not
// surface as a frame).
func translateRealtimeEvent(ev realtime.Event) (dialogrealtime.Event, bool) {
	switch ev.Type {
	case realtime.EventSessionOpen:
		// The engine has no use for "WS handshake done"; the
		// arrival of the first event is implicit confirmation.
		return dialogrealtime.Event{}, false

	case realtime.EventSessionClose:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventSessionClose,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventUserTranscript:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventUserTranscript,
			Text:   ev.Text,
			Final:  ev.Final,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventUserSpeechStarted:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventUserSpeechStarted,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventUserSpeechEnded:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventUserSpeechEnded,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventAssistantText:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventAssistantText,
			Text:   ev.Text,
			Final:  ev.Final,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventAssistantAudio:
		// SampleRate is not on pkg/realtime.Event today — the
		// adapter caller (the BuilderProvider) attaches the
		// configured OutputSampleRate via the closure that wraps
		// translateRealtimeEvent. See nativeRealtimeBuilder.
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventAssistantAudio,
			Audio:  ev.AudioPC,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventAssistantTurnEnd:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventAssistantTurnEnd,
			Vendor: ev.Vendor,
		}, true

	case realtime.EventError:
		return dialogrealtime.Event{
			Kind:   dialogrealtime.EventError,
			Err:    ev.Err,
			Fatal:  ev.Fatal,
			Vendor: ev.Vendor,
		}, true
	}
	return dialogrealtime.Event{}, false
}

// nativeRealtimeBuilderConfig pins the fields the SIP adapter needs
// to build a pkg/realtime.Agent. Kept narrow so test fixtures can
// supply mocks without touching DB or env loaders.
type nativeRealtimeBuilderConfig struct {
	// CredentialCfg is the parsed RealtimeConfigRaw map used by
	// pkg/realtime.NewAgentFromCredential to resolve a Provider.
	CredentialCfg map[string]any

	// Options is forwarded to NewAgentFromCredential. The adapter
	// overrides Options.OnEvent with its own translating callback;
	// callers MUST NOT set OnEvent themselves (it would be ignored).
	Options realtime.Options

	// NewAgent allows tests to inject an in-memory Agent without
	// going through the registry. Production wires it to
	// realtime.NewAgentFromCredential.
	NewAgent func(cfg map[string]any, opts realtime.Options) (realtime.Agent, error)
}

// newNativeRealtimeBuilder returns a dialogrealtime.AgentBuilder
// that, when invoked by the engine, constructs a pkg/realtime.Agent
// configured to forward translated events through the supplied sink.
//
// The returned builder is single-use — calling Build twice on the
// same builder will produce two independent agents. The engine only
// builds once per Engine instance, so this is fine.
func newNativeRealtimeBuilder(cfg nativeRealtimeBuilderConfig) dialogrealtime.AgentBuilder {
	newAgent := cfg.NewAgent
	if newAgent == nil {
		newAgent = realtime.NewAgentFromCredential
	}
	outRate := cfg.Options.OutputSampleRate
	return dialogrealtime.AgentBuilderFunc(func(sink dialogrealtime.EventSink) (dialogrealtime.Agent, error) {
		opts := cfg.Options
		opts.OnEvent = func(ev realtime.Event) {
			out, ok := translateRealtimeEvent(ev)
			if !ok {
				return
			}
			// Stamp the audio sample rate from the configured
			// OutputSampleRate — pkg/realtime.Event doesn't
			// carry the rate inline (every chunk in a session
			// has the same rate negotiated at session.update
			// time, so there's nothing to disambiguate).
			if out.Kind == dialogrealtime.EventAssistantAudio && out.SampleRate == 0 {
				out.SampleRate = outRate
			}
			sink.Emit(out)
		}
		ag, err := newAgent(cfg.CredentialCfg, opts)
		if err != nil {
			return nil, err
		}
		return &nativeRealtimeAgentAdapter{inner: ag}, nil
	})
}

// useNativeRealtime is the per-tenant feature gate for the native
// realtime engine. UNLIKE useNativeCascaded (which is default-on
// with a kill-switch), realtime native is default-OFF: the legacy
// path still owns mandatory features (recorder, welcome WAV,
// transfer state machine) that the native engine does not
// reproduce yet (PR-10c+). Tenants opt-in via
// DIALOG_NATIVE_REALTIME_TENANTS, individual call IDs via
// DIALOG_NATIVE_REALTIME_CALLS — both are comma-separated.
//
// Not yet wired into AttachVoiceViaEngine. PR-10c flips the gate
// after parity work lands.
func useNativeRealtime(tenantID string) bool {
	return defaultNativeRealtimeRouter().useNativeRealtime(tenantID)
}

// nativeRealtimeRouter is the env-reader indirection that mirrors
// nativeCascadedRouter. Tests inject a getenv to drive the
// predicate without touching real env vars.
type nativeRealtimeRouter struct {
	getenv func(string) string
}

const (
	envNativeRealtimeTenants = "DIALOG_NATIVE_REALTIME_TENANTS"
)

func defaultNativeRealtimeRouter() nativeRealtimeRouter {
	return nativeRealtimeRouter{getenv: os.Getenv}
}

func (r nativeRealtimeRouter) useNativeRealtime(tenantID string) bool {
	if tenantID == "" {
		return false
	}
	for _, id := range parseTenantList(r.getenv(envNativeRealtimeTenants)) {
		if id == tenantID {
			return true
		}
	}
	return false
}
