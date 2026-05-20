package server

// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Inbound CDR (Call Detail Record) emission.
//
// The outbound side has its own per-leg CDR plumbing in
// pkg/sip/outbound/cdr.go — that one tracks dial-out attempts
// (Manager.Dial → BYE). This file is the symmetric inbound path:
// every INVITE that the server accepts (2xx) and that later ends
// via BYE / timeout produces one JSON-Lines record into the same
// CDR sink, so downstream pipelines see both directions in one
// stream.
//
// State model:
//
//   - inboundCDRState is a process-wide sync.Map keyed by Call-ID.
//     Entries are created on the first ACK we successfully handle
//     (handleAck → CallStarted) and removed on either BYE 2xx or
//     handleBye's local-initiated paths (transfer bridge teardown).
//
//   - Allocation is lazy: when no CDR sink has been configured,
//     trackInboundCallStart is a no-op and the state map stays
//     empty. Production paths therefore pay zero overhead when CDR
//     emission isn't wired.
//
// Why ACK as the "start" anchor rather than INVITE? Because INVITE
// may end in a non-2xx (BUSY, declined, etc.) — those failures are
// recorded as INVITE result metrics (sip_invite_result_total) and
// don't deserve a CDR row. A CDR row represents a CALL that
// happened, which by RFC 3261 §13 means "ACK has been received".

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LinByte/VoiceServer/pkg/voice/cdr"
)

// inboundCDRSinkRef holds the optional CDR writer. Using atomic
// avoids contention on the hot path (every ACK reads it once); the
// pointer is set at most once at bootstrap.
var inboundCDRSinkRef atomic.Pointer[cdr.Writer]

// SetInboundCDRSink wires the CDR writer to the inbound pipeline.
// Pass nil to disable. Safe to call concurrently with calls in
// flight: new calls pick up the new sink, in-flight calls finish
// against whatever sink they observed at trackInboundCallStart.
func SetInboundCDRSink(w *cdr.Writer) {
	inboundCDRSinkRef.Store(w)
}

// inboundCDREntry is the per-call state. Lives only between ACK
// and BYE; the map entry is deleted on emit so memory is bounded
// to in-flight calls regardless of process lifetime.
type inboundCDREntry struct {
	mu           sync.Mutex
	startedAt    time.Time
	answeredMs   int64
	codec        string
	fromUser     string // optional: for log/debug correlation
	toUser       string
	correlation  string
	endStatus    string
	hangupBy     string
	hangupReason string
	errors       []string
}

var inboundCDRState sync.Map // key: callID, value: *inboundCDREntry

// trackInboundCallStart records the moment a call answered (first
// ACK we processed for this Call-ID). No-op if no CDR sink is wired
// — keeps the map empty in builds that don't want CDR.
//
// codec is the negotiated codec name (lowercase preferred).
// answeredMs is INVITE→200 OK setup time in milliseconds, or 0 if
// unknown.
func trackInboundCallStart(callID, codec, fromUser, toUser, correlation string, answeredMs int64) {
	if inboundCDRSinkRef.Load() == nil {
		return
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	entry := &inboundCDREntry{
		startedAt:   time.Now(),
		codec:       strings.ToLower(strings.TrimSpace(codec)),
		fromUser:    fromUser,
		toUser:      toUser,
		correlation: correlation,
		answeredMs:  answeredMs,
		endStatus:   "ok",
	}
	// Idempotent against ACK retransmits: only the first ACK wins.
	inboundCDRState.LoadOrStore(callID, entry)
}

// trackInboundCallEnd finalises and emits the CDR for a call. Safe
// to call multiple times — only the first wins. hangupBy is "local"
// or "remote"; reasonClass is the bounded enum from
// classifyBYEReason (empty = default "normal"). hangupReason is the
// free-text Reason header value (optional).
func trackInboundCallEnd(callID, hangupBy, reasonClass, hangupReason string) {
	v, ok := inboundCDRState.LoadAndDelete(strings.TrimSpace(callID))
	if !ok {
		return
	}
	entry := v.(*inboundCDREntry)
	sink := inboundCDRSinkRef.Load()
	if sink == nil {
		return
	}
	entry.mu.Lock()
	if hangupBy != "" {
		entry.hangupBy = hangupBy
	}
	if hangupReason != "" {
		entry.hangupReason = hangupReason
	}
	// End-status mapping mirrors classToCallEndedStatus so the CDR
	// row and the voiceserver_calls_total counter agree on labels.
	if entry.endStatus == "ok" && reasonClass != "" {
		entry.endStatus = classToCallEndedStatus(reasonClass)
	}
	rec := cdr.NewCallRecord(callID, "sip", entry.startedAt)
	rec.CorrelationID = entry.correlation
	rec.Codec = entry.codec
	rec.AnsweredMs = entry.answeredMs
	rec.EndStatus = entry.endStatus
	rec.HangupBy = entry.hangupBy
	rec.HangupReason = entry.hangupReason
	if len(entry.errors) > 0 {
		rec.Errors = append([]string(nil), entry.errors...)
	}
	entry.mu.Unlock()
	rec.Finalize(time.Now())
	sink.Emit(rec)
}

// recordInboundCallError appends a short error tag to the in-flight
// CDR entry. Cheap no-op if the call isn't tracked. Use sparingly —
// the slice is unbounded in principle.
func recordInboundCallError(callID, tag string) {
	if tag == "" {
		return
	}
	v, ok := inboundCDRState.Load(strings.TrimSpace(callID))
	if !ok {
		return
	}
	entry := v.(*inboundCDREntry)
	entry.mu.Lock()
	entry.errors = append(entry.errors, tag)
	entry.mu.Unlock()
}
