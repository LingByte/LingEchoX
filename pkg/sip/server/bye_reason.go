package server

// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// RFC 3326 Reason header classification.
//
// SBCs and softswitches attach a Reason header to BYE requests to
// explain WHY the call is ending. Two header families dominate:
//
//	Reason: SIP ;cause=486 ;text="Busy Here"
//	Reason: Q.850 ;cause=16 ;text="Normal call clearing"
//
// We collapse this into the same bounded enum used by outbound's
// CDR / metrics layer (sipMetrics.ByeReason*). Unparseable / missing
// headers map to "normal" — the safe default that won't pollute
// dashboards with synthetic "error" spikes.
//
// Why a bounded enum?  Cardinality. A free-form Reason text could
// be anything ("Outbound capacity exhausted, retry in 15s") and
// would explode the metrics registry. The exact human text still
// flows to the CDR Errors[] slice for debugging — see
// outboundReasonTextForCDR.

import (
	"strconv"
	"strings"

	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

// classifyBYEReason returns the bounded-enum reason class for a SIP
// BYE request, plus the raw text suitable for the CDR. Both outputs
// are safe defaults when the Reason header is absent.
//
// Conservative: anything we don't explicitly recognize maps to
// "normal" rather than "error". An unexpected cause from a new peer
// SHOULDN'T look like a system failure on the dashboard.
func classifyBYEReason(msg *stack.Message) (reasonClass, rawText string) {
	if msg == nil {
		return sipMetrics.ByeReasonNormal, ""
	}
	reasonHdr := strings.TrimSpace(msg.GetHeader("Reason"))
	if reasonHdr == "" {
		return sipMetrics.ByeReasonNormal, ""
	}
	proto, cause, text := parseRFC3326Reason(reasonHdr)
	rawText = strings.TrimSpace(text)

	switch strings.ToUpper(proto) {
	case "Q.850":
		// ITU-T Q.850 cause codes. The subset we care about:
		//   16  Normal call clearing       → normal
		//   17  User busy                  → normal (the call did happen)
		//   18  No user responding         → normal
		//   19  No answer                  → normal
		//   21  Call rejected              → user-hangup
		//   34  No circuit available       → error
		//   38  Network out of order       → error
		//   41  Temporary failure          → error
		//   102 Recovery on timer expiry   → timer-expired
		switch cause {
		case 16, 17, 18, 19:
			return sipMetrics.ByeReasonNormal, rawText
		case 21:
			return sipMetrics.ByeReasonUserHangup, rawText
		case 102:
			return sipMetrics.ByeReasonTimerExpired, rawText
		case 34, 38, 41, 42, 47:
			return sipMetrics.ByeReasonError, rawText
		}
	case "SIP":
		// SIP response code embedded in the Reason header. Same
		// classes as a normal 4xx/5xx, but typically the BYE side
		// is rarer — usually session timer (408 Request Timeout)
		// or service unavailability.
		switch cause {
		case 200:
			return sipMetrics.ByeReasonNormal, rawText
		case 408:
			return sipMetrics.ByeReasonTimerExpired, rawText
		default:
			if cause >= 400 {
				return sipMetrics.ByeReasonError, rawText
			}
		}
	}
	return sipMetrics.ByeReasonNormal, rawText
}

// parseRFC3326Reason extracts protocol, cause, and text from a
// Reason header. Format per RFC 3326 §2:
//
//	Reason-Value = protocol *(SEMI reason-param)
//	reason-param = "cause=" 1*DIGIT
//	             | "text=" quoted-string
//	             | extension-param
//
// The parser is lenient: whitespace tolerated, params in any order,
// quoting on text optional (some implementations omit quotes).
func parseRFC3326Reason(hdr string) (proto string, cause int, text string) {
	// Split on the first semicolon to separate the protocol token
	// from the params list.
	parts := strings.SplitN(hdr, ";", 2)
	proto = strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return proto, 0, ""
	}
	for _, kv := range strings.Split(parts[1], ";") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(kv[:eq]))
		val := strings.TrimSpace(kv[eq+1:])
		switch key {
		case "cause":
			// Cause code is a decimal integer per RFC 3326. Strip
			// any wrapping quotes some implementations send.
			val = strings.Trim(val, `"`)
			if n, err := strconv.Atoi(val); err == nil {
				cause = n
			}
		case "text":
			text = strings.Trim(val, `"`)
		}
	}
	return proto, cause, text
}
