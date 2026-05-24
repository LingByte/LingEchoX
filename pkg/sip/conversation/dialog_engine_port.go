package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// CallSessionPort — adapts *sipSession.CallSession to engine.MediaPort
// + legacy.LegacyHandle so the legacy attach path can be reached
// through engine.New(...).Attach(ctx, port, lg).
//
// Phase 1 PR-6 contract
// ---------------------
//
// The legacy attach (AttachVoicePipeline) does NOT consume MediaPort's
// streaming methods (InputPCM / SendOutputPCM / OnBargeIn). It pulls
// the raw *sipSession.CallSession out via LegacyHandle.LegacySession()
// and registers media processors on it directly. So this port:
//
//   - Reports correct values for the SIP-side metadata methods that
//     are cheap to provide: CallID, TenantID, SampleRate, Codec.
//   - Surfaces a clear ErrLegacyBridgeOnly when callers attempt to use
//     the streaming methods (InputPCM / SendOutputPCM / OnBargeIn).
//     This makes "I forgot this is a legacy bridge port" a fast,
//     loud failure rather than silent stall on a nil channel.
//   - Implements LegacyHandle by returning the underlying CallSession.
//
// Phase 3+ ships a real, streaming MediaPort that owns its channels
// and bridges to media.MediaSession; that port will NOT implement
// LegacyHandle. Engine implementations should program against
// engine.MediaPort and use LegacyHandle only as a transitional crutch.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
	"github.com/LinByte/VoiceServer/pkg/dialog/legacy"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
)

// ErrLegacyBridgeOnly is returned by streaming methods on a
// CallSessionPort. The bridge attacher never invokes them; native
// engines that DO need streaming must use a future port type that
// owns its channels.
var ErrLegacyBridgeOnly = errors.New("dialog engine port: legacy bridge only — streaming methods not implemented")

// CallSessionPort adapts a *sipSession.CallSession to engine.MediaPort
// and legacy.LegacyHandle. Construct via NewCallSessionPort; do not
// zero-initialise (a nil cs trips clearly-marked guards).
type CallSessionPort struct {
	cs *sipSession.CallSession
}

// NewCallSessionPort wraps cs as a MediaPort. Returns nil when cs is
// nil so callers can `if port == nil { … }` instead of nil-dereferencing
// inside Attach.
func NewCallSessionPort(cs *sipSession.CallSession) *CallSessionPort {
	if cs == nil {
		return nil
	}
	return &CallSessionPort{cs: cs}
}

// Compile-time assertions: every exported wrap must keep the dual
// interface satisfaction.
var (
	_ engine.MediaPort    = (*CallSessionPort)(nil)
	_ legacy.LegacyHandle = (*CallSessionPort)(nil)
)

// LegacySession returns the wrapped *sipSession.CallSession. Returning
// nil on a nil receiver makes "wrapped a nil cs" trip the wired
// attacher's ErrNoLegacySession guard cleanly.
func (p *CallSessionPort) LegacySession() any {
	if p == nil || p.cs == nil {
		return nil
	}
	return p.cs
}

// CallID returns the unique call identifier. Empty string on a nil
// receiver — engines should validate non-empty before persisting.
func (p *CallSessionPort) CallID() string {
	if p == nil || p.cs == nil {
		return ""
	}
	return p.cs.CallID
}

// TenantID returns the owning tenant as a decimal string ("0" when
// no tenant is bound — matches CallSession.TenantID's legacy semantic).
// engine.MediaPort.TenantID is a string because the dialog layer is
// transport-agnostic and tenants may be UUIDs / strings in non-SIP
// transports.
func (p *CallSessionPort) TenantID() string {
	if p == nil || p.cs == nil {
		return ""
	}
	return strconv.FormatUint(uint64(p.cs.TenantID()), 10)
}

// SampleRate returns the bridge-side mono PCM rate (CallSession's
// internal decode rate, fed to ASR processors). Falls back to 16000
// on a nil receiver — same default CallSession.PCMSampleRate uses.
func (p *CallSessionPort) SampleRate() int {
	if p == nil || p.cs == nil {
		return 16000
	}
	return p.cs.PCMSampleRate()
}

// Codec returns the SIP-negotiated codec for this call. Engines
// shouldn't normally care; recorder / passthrough optimisations do.
func (p *CallSessionPort) Codec() engine.CodecSpec {
	if p == nil || p.cs == nil {
		return engine.CodecSpec{}
	}
	c := p.cs.NegotiatedCodec()
	name := strings.ToUpper(strings.TrimSpace(c.Name))
	channels := c.Channels
	if channels <= 0 {
		channels = 1
	}
	return engine.CodecSpec{
		Name:       name,
		SampleRate: c.ClockRate,
		Channels:   channels,
	}
}

// InputPCM is intentionally unimplemented for the legacy bridge. See
// ErrLegacyBridgeOnly. We return a closed channel so a caller that
// somehow ignores the contract spins on a closed-receive instead of
// blocking forever — fail loud, not silent.
func (p *CallSessionPort) InputPCM() <-chan engine.PCMFrame {
	ch := make(chan engine.PCMFrame)
	close(ch)
	return ch
}

// SendOutputPCM is intentionally unimplemented for the legacy bridge.
// Returns ErrLegacyBridgeOnly so callers see a clear error rather
// than a silent drop.
func (p *CallSessionPort) SendOutputPCM(engine.PCMFrame) error {
	return fmt.Errorf("%w (call_id=%q)", ErrLegacyBridgeOnly, p.CallID())
}

// OnBargeIn is intentionally a no-op for the legacy bridge. The
// legacy attach owns its own barge-in plumbing inside
// pkg/sip/conversation/voice*.go and does not consult MediaPort. We
// silently drop the registration rather than erroring because the
// engine.MediaPort interface signature has no error return — making
// this loud would require a contract change. Phase-3 ports will fire
// these callbacks for real.
func (p *CallSessionPort) OnBargeIn(_ func()) {
	// no-op (see comment above)
}
