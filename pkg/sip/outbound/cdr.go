// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

// Call-Detail-Record (CDR) plumbing for outbound legs.
//
// Lifecycle:
//
//   1. Dial() initializes leg.cdr via beginCDR — captures Call-ID,
//      transport, scenario, correlation ID, start time.
//   2. handleResponse() updates classification on each response:
//        - 1xx (provisional) → no CDR change
//        - 2xx OK INVITE     → leg.cdrSetAnswered()
//        - non-2xx final     → leg.cdrSetError(code, reason)
//        - 2xx BYE           → leg.cdrSetHangup("local", "normal")
//   3. CleanupLegIfPresent() (remote BYE) → leg.cdrSetHangup("remote", ...)
//   4. cleanupLeg() flushes RTCP QoS, then emitCDR() writes one
//      JSONL record asynchronously via cdr.Writer.Emit. Producers
//      never block — drop-on-full at the writer.
//
// The CDR sink is optional; if SetCDRSink was never called, every
// emit is a no-op. This keeps tests / dev runs free of disk I/O
// unless explicitly enabled by the bootstrap layer.

import (
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/voice/cdr"
	voiceMetrics "github.com/LinByte/VoiceServer/pkg/voice/metrics"
	"github.com/LinByte/VoiceServer/pkg/voice/qos"
)

// SetCDRSink configures the destination Writer for Call-Detail-
// Records. nil disables CDR emission. Safe to call concurrently
// with in-flight legs — the new sink applies to subsequent emits.
// Idempotent.
func (m *Manager) SetCDRSink(w *cdr.Writer) {
	if m == nil {
		return
	}
	m.cdrSinkMu.Lock()
	m.cdrSinkV = w
	m.cdrSinkMu.Unlock()
}

// cdrSink returns the currently-configured writer or nil. Hot path
// (called from cleanupLeg) so we use an RLock.
func (m *Manager) cdrSink() *cdr.Writer {
	if m == nil {
		return nil
	}
	m.cdrSinkMu.RLock()
	w := m.cdrSinkV
	m.cdrSinkMu.RUnlock()
	return w
}

// beginCDR stamps the start time on a freshly-built leg. Called
// from Dial() after the leg is allocated but before the INVITE is
// put on the wire — that timestamp is the closest we can get to
// "user pressed dial".
func (leg *outLeg) beginCDR() {
	if leg == nil {
		return
	}
	leg.cdr.mu.Lock()
	if leg.cdr.startedAt.IsZero() {
		leg.cdr.startedAt = time.Now()
	}
	leg.cdr.mu.Unlock()
}

// endCDRActiveCount decrements the voiceserver_active_calls gauge
// and bumps voiceserver_calls_total exactly once per leg. Idempotent
// against repeat cleanup paths (e.g. remote BYE racing dialog
// teardown). Safe to call on legs that never reached "answered" —
// in that case there's nothing to decrement and we no-op.
func (leg *outLeg) endCDRActiveCount() {
	if leg == nil {
		return
	}
	leg.cdr.mu.Lock()
	shouldEnd := leg.cdr.activeCounted && !leg.cdr.activeEnded
	if shouldEnd {
		leg.cdr.activeEnded = true
	}
	endStatus := leg.cdr.endStatus
	leg.cdr.mu.Unlock()
	if !shouldEnd {
		return
	}
	if endStatus == "" {
		endStatus = "ok"
	}
	voiceMetrics.CallEnded("sip", endStatus)
}

// cdrState holds the lifecycle data we accumulate over a single
// outbound leg's lifetime. It's a sub-struct of outLeg (held by
// value) rather than a pointer so we don't allocate when no sink
// is configured.
type cdrState struct {
	mu sync.Mutex

	startedAt time.Time
	// answered marks the moment we received 2xx OK INVITE. Used to
	// compute setup time (INVITE → 200 OK).
	answeredAt time.Time
	// activeCounted is true after CallStarted has fired so emitCDR
	// can pair it with exactly one CallEnded (idempotent against
	// double-cleanup paths like remote BYE + dialog teardown).
	activeCounted bool
	activeEnded   bool

	// codec is the negotiated codec name (lowercase). Set in
	// handleResponse after the answer SDP is parsed. Default "pcmu"
	// is conservative; the qos package treats unknown values as
	// G.711 anyway.
	codec string

	// endStatus + hangupBy + sipFinalCode classify how the leg
	// ended. Defaults are deliberately benign: if the leg ends
	// cleanly nothing has to mutate them.
	endStatus    string // ok / signaling-error / pipeline-error / timer-expired
	hangupBy     string // local / remote
	reasonClass  string
	sipFinalCode int

	// errors collects short tags accumulated during the call (e.g.
	// "dtls-fingerprint-mismatch", "srtp-install-failed").
	errors []string
}

// cdrSnapshot is a lock-free, point-in-time copy of cdrState. We use a
// distinct type so go vet -copylocks doesn't flag the embedded mutex,
// and so callers can't accidentally mutate the snapshot expecting it
// to be visible to other readers.
type cdrSnapshot struct {
	startedAt     time.Time
	answeredAt    time.Time
	activeCounted bool
	activeEnded   bool
	codec         string
	endStatus     string
	hangupBy      string
	reasonClass   string
	sipFinalCode  int
	errors        []string
}

func (s *cdrState) snapshot() cdrSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := cdrSnapshot{
		startedAt:     s.startedAt,
		answeredAt:    s.answeredAt,
		activeCounted: s.activeCounted,
		activeEnded:   s.activeEnded,
		codec:         s.codec,
		endStatus:     s.endStatus,
		hangupBy:      s.hangupBy,
		reasonClass:   s.reasonClass,
		sipFinalCode:  s.sipFinalCode,
	}
	if len(s.errors) > 0 {
		out.errors = append([]string(nil), s.errors...)
	}
	return out
}

// cdrSetAnswered records the moment the call moved to media-flow.
// Idempotent: only the first 2xx OK matters. Also bumps the
// voiceserver_active_calls gauge — the symmetric decrement happens
// in emitCDR / cleanupLeg.
func (leg *outLeg) cdrSetAnswered(codec string) {
	if leg == nil {
		return
	}
	leg.cdr.mu.Lock()
	wasFirstAnswer := leg.cdr.answeredAt.IsZero()
	if wasFirstAnswer {
		leg.cdr.answeredAt = time.Now()
		leg.cdr.activeCounted = true
	}
	if codec != "" {
		leg.cdr.codec = strings.ToLower(codec)
	}
	if leg.cdr.endStatus == "" {
		leg.cdr.endStatus = "ok"
	}
	leg.cdr.mu.Unlock()
	if wasFirstAnswer {
		// Active-calls gauge: one ++ per leg that goes live. The
		// matching -- happens in emitCDR (cleanup path).
		voiceMetrics.CallStarted("sip")
	}
}

// cdrSetError records a signaling failure (non-2xx final or build
// error before the leg ever answered).
func (leg *outLeg) cdrSetError(sipCode int, reason string) {
	if leg == nil {
		return
	}
	leg.cdr.mu.Lock()
	defer leg.cdr.mu.Unlock()
	if leg.cdr.endStatus == "" || leg.cdr.endStatus == "ok" {
		leg.cdr.endStatus = "signaling-error"
	}
	if sipCode > 0 {
		leg.cdr.sipFinalCode = sipCode
	}
	if reason != "" {
		leg.cdr.errors = append(leg.cdr.errors, reason)
	}
}

// cdrSetHangup records who initiated the BYE / cleanup. Idempotent:
// the first hangup wins so a remote-BYE → local-cleanup race tags
// the cause correctly.
func (leg *outLeg) cdrSetHangup(by, reasonClass string) {
	if leg == nil {
		return
	}
	leg.cdr.mu.Lock()
	defer leg.cdr.mu.Unlock()
	if leg.cdr.hangupBy == "" {
		leg.cdr.hangupBy = by
	}
	if leg.cdr.reasonClass == "" {
		leg.cdr.reasonClass = reasonClass
	}
	if leg.cdr.endStatus == "" {
		leg.cdr.endStatus = "ok"
	}
}

// cdrAddError appends an error tag (e.g. "dtls-fail",
// "rtp-allocation-failed"). Free-form; downstream pipelines slice
// on this for failure-mode dashboards.
func (leg *outLeg) cdrAddError(tag string) {
	if leg == nil || tag == "" {
		return
	}
	leg.cdr.mu.Lock()
	defer leg.cdr.mu.Unlock()
	leg.cdr.errors = append(leg.cdr.errors, tag)
}

// emitCDR builds the call record and hands it to the writer. Must
// be called AFTER flushOutboundCallQoS so the RTCP snapshot is
// available; reads it directly off rtpSess one more time here to
// avoid duplicating QoS state. Hot-path-safe: one channel send to
// the cdr.Writer drain.
func (leg *outLeg) emitCDR() {
	if leg == nil || leg.m == nil {
		return
	}
	sink := leg.m.cdrSink()
	if sink == nil {
		return
	}
	snap := leg.cdr.snapshot()
	if snap.startedAt.IsZero() {
		// Never properly initialised — skip rather than emit a
		// half-record. Possible only if Dial bypassed beginCDR.
		return
	}

	now := time.Now()
	rec := cdr.NewCallRecord(leg.params.CallID, "sip", snap.startedAt)
	rec.CorrelationID = strings.TrimSpace(leg.req.CorrelationID)
	rec.Scenario = string(leg.req.Scenario)
	rec.Codec = snap.codec
	rec.Finalize(now)
	if !snap.answeredAt.IsZero() {
		rec.AnsweredMs = snap.answeredAt.Sub(snap.startedAt).Milliseconds()
	}

	// Classification.
	if snap.endStatus == "" {
		rec.EndStatus = "ok"
	} else {
		rec.EndStatus = snap.endStatus
	}
	rec.HangupBy = snap.hangupBy
	rec.HangupReason = snap.reasonClass
	rec.SIPFinalCode = snap.sipFinalCode
	rec.Errors = snap.errors

	// Per-call QoS. Reading RTCP one more time is cheap (one mutex
	// take inside RTCPSnapshot) and keeps emitCDR robust against
	// call sites that forget to run the metrics flush.
	if leg.rtpSess != nil {
		rtcp := leg.rtpSess.RTCPSnapshot()
		if rtcp.PeerSeenRR || rtcp.LocalJitter > 0 {
			lossFraction := float64(rtcp.PeerLossFraction) / 256.0
			clockRate := uint32(8000)
			jitterMs := float64(rtcp.LocalJitter) * 1000.0 / float64(clockRate)
			mos := qos.Estimate(qos.MOSInput{
				RTTMs:            rtcp.RTTMs,
				JitterRTPUnits:   rtcp.LocalJitter,
				JitterClockRate:  clockRate,
				PeerLossFraction: lossFraction,
				Codec:            rec.Codec,
			})
			rec.RTTMsP95 = rtcp.RTTMs
			rec.JitterRTPUnits = rtcp.LocalJitter
			rec.PeerLossFraction = float32(lossFraction)
			rec.PeerCumulativeLost = rtcp.PeerCumulativeLost
			rec.MOSEstimate = float32(mos.MOS)
			_ = jitterMs // reserved for future _ms field
		}
	}

	sink.Emit(rec)
}
