package server

// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Inbound active-call accounting for the voice/metrics gauge.
//
// We bump voiceserver_active_calls{transport="sip"} by +1 when the
// inbound dialog reaches "answered" (handleAck → StartOnACK fires
// for the first time on this Call-ID) and -1 when the matching
// teardown happens (any BYE 2xx, or a transaction timeout that
// implicitly ends the leg).
//
// The pairing is enforced via a sync.Map of "this Call-ID has been
// counted up". Without it, double-cleanup paths (remote BYE racing
// dialog teardown) would double-decrement the gauge and produce
// spurious negative values.
//
// We also bump voiceserver_calls_total{transport="sip",status="ok"}
// once per ended call so dashboards have a counter that matches the
// outbound side's accounting.

import (
	"sync"

	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	voiceMetrics "github.com/LinByte/VoiceServer/pkg/voice/metrics"
)

// classToCallEndedStatus maps the RFC 3326 reasonClass enum onto
// the bounded "status" label used by voiceserver_calls_total. The
// two enums share most values; this mapping makes the divergence
// explicit so future additions don't silently break dashboards.
func classToCallEndedStatus(reasonClass string) string {
	switch reasonClass {
	case sipMetrics.ByeReasonNormal, "":
		return "ok"
	case sipMetrics.ByeReasonTimerExpired:
		return "timer-expired"
	case sipMetrics.ByeReasonError:
		return "pipeline-error"
	case sipMetrics.ByeReasonUserHangup:
		return "dialog-hangup"
	}
	return reasonClass
}

// inboundActiveCalls is a process-wide set: callID → struct{}.
// Entries are added in markInboundCallStarted and removed in
// markInboundCallEnded. Both are idempotent: a duplicate "started"
// is a no-op, a duplicate "ended" is a no-op.
var inboundActiveCalls sync.Map // key: callID, value: struct{}

// markInboundCallStarted bumps the active-calls gauge ONCE per
// Call-ID. Returns true if the gauge was actually incremented (i.e.
// this is the first time we see this Call-ID).
func markInboundCallStarted(callID string) bool {
	if callID == "" {
		return false
	}
	if _, loaded := inboundActiveCalls.LoadOrStore(callID, struct{}{}); loaded {
		return false
	}
	voiceMetrics.CallStarted("sip")
	return true
}

// markInboundCallEnded decrements the active-calls gauge and bumps
// the calls_total counter ONCE per Call-ID. Returns true on the
// first call; subsequent calls for the same Call-ID are no-ops.
//
// status is the bounded enum used by voiceserver_calls_total: "ok",
// "dialog-hangup", "timer-expired", "pipeline-error", etc. Empty
// defaults to "ok" so callers don't have to fill it for normal
// teardown.
func markInboundCallEnded(callID, status string) bool {
	if callID == "" {
		return false
	}
	if _, loaded := inboundActiveCalls.LoadAndDelete(callID); !loaded {
		return false
	}
	if status == "" {
		status = "ok"
	}
	voiceMetrics.CallEnded("sip", status)
	return true
}
