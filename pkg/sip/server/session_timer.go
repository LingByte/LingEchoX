// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package server

// This file wires the pure-logic pkg/sip/session_timer into the UAS
// (inbound) request path. The watchdog goroutine + arming live on
// CallSession (see pkg/sip/session/call_session.go); here we just
// parse the request, run the negotiation, and provide helpers to
// echo headers onto the 200 OK.
//
// Outbound (UAC) timer handling lives in pkg/sip/outbound; for the
// current pass we only emit `Supported: timer` on outbound INVITE
// without proposing a Session-Expires (we accept the peer's
// proposal if any). See docs/sip_gap_analysis.md §1C for follow-up
// scope (UAC-side refresh sending).

import (
	"strings"

	"github.com/LinByte/VoiceServer/pkg/sip/session_timer"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

// negotiateInboundSessionTimer extracts the relevant headers from an
// inbound INVITE and runs the UAS-side negotiation. The returned
// Decision tells the INVITE handler whether to 422 the call, what
// headers to echo on the 200 OK, and what watchdog interval to arm.
//
// We currently use the package defaults (Min-SE=90, preferred SE=1800).
// These could become per-trunk configuration later, but a fixed pair
// keeps real-world behaviour predictable across tenants.
func (s *SIPServer) negotiateInboundSessionTimer(msg *stack.Message) session_timer.Decision {
	if msg == nil {
		return session_timer.Decision{}
	}
	peerSE, peerRefresher, _ := session_timer.ParseSessionExpires(msg.GetHeader("Session-Expires"))
	peerMinSE := session_timer.ParseMinSE(msg.GetHeader("Min-SE"))
	supported := session_timer.ParseTokenList(msg.GetHeader("Supported"))
	require := session_timer.ParseTokenList(msg.GetHeader("Require"))

	return session_timer.NegotiateUAS(
		peerSE,
		peerRefresher,
		peerMinSE,
		session_timer.HasToken(supported, session_timer.SupportedTokenTimer),
		session_timer.HasToken(require, session_timer.SupportedTokenTimer),
		session_timer.DefaultMinSE,
		session_timer.DefaultSE,
	)
}

// mergeSupportedToken appends tok to msg's existing Supported header,
// preserving any other tokens already there. SIP allows duplicates,
// but most stacks deduplicate; we do the same here to be polite.
//
// Empty existing header → set to just `tok`.
func mergeSupportedToken(msg *stack.Message, tok string) {
	tok = strings.TrimSpace(tok)
	if msg == nil || tok == "" {
		return
	}
	existing := session_timer.ParseTokenList(msg.GetHeader("Supported"))
	for _, t := range existing {
		if t == strings.ToLower(tok) {
			// Already present.
			return
		}
	}
	existing = append(existing, strings.ToLower(tok))
	msg.SetHeader("Supported", strings.Join(existing, ", "))
}
