// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"strings"

	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
)

// buildRetargetHeaders constructs the RFC 7044 History-Info and RFC
// 5806 Diversion chains to attach to a B2BUA-retargeted outbound
// INVITE.
//
// Inputs are the three raw header strings captured at inbound INVITE
// intake (via CallSession.InboundRetargetHeaders) plus the new target
// URI we're about to dial. retargetReason is a free-text reason code
// used for both:
//   - RFC 7044 entry's embedded Reason= URI-header (e.g. "SIP;cause=302")
//   - RFC 5806 Diversion `reason=` parameter (e.g. "deflection" /
//     "unconditional"; see historyinfo.DiversionXxx constants)
//
// Returns nil chains when there is nothing useful to emit (e.g. we
// have no inbound To URI to anchor on AND no upstream chain to
// extend, meaning the call originated here so there's no retarget
// history to surface).
//
// The function is deliberately lenient: malformed inbound headers
// degrade to "treat as absent" rather than aborting the transfer.
func buildRetargetHeaders(
	rawTo, rawHistoryInfo, rawDiversion string,
	newTargetURI string,
	historyReason string,
	diversionReason string,
) ([]historyinfo.Entry, []historyinfo.Diversion) {
	newTargetURI = strings.TrimSpace(newTargetURI)
	originalTo := historyinfo.ExtractURIFromToHeader(rawTo)
	if newTargetURI == "" {
		return nil, nil
	}

	inboundHistory := historyinfo.ParseChain(rawHistoryInfo)
	inboundDiversion := historyinfo.ParseDiversionChain(rawDiversion)

	if originalTo == "" && len(inboundHistory) == 0 && len(inboundDiversion) == 0 {
		// Nothing to anchor or extend — the call has no retarget
		// history. Emitting bare entries for newTargetURI alone would
		// be misleading (it would imply a redirect that didn't happen
		// at the protocol layer).
		return nil, nil
	}

	hi := historyinfo.AppendTransferEntry(inboundHistory, originalTo, newTargetURI, historyReason)
	dv := historyinfo.AppendDiversionEntry(inboundDiversion, originalTo, diversionReason)
	return hi, dv
}

// applyRetargetHeaders mutates req in place to include History-Info
// and Diversion chains representing the retarget from the inbound
// call (rawTo / rawHistoryInfo / rawDiversion) to req.Target.RequestURI.
//
// historyReason / diversionReason are the cause annotations:
//   - For AI/ACD-driven transfers, use:
//       historyReason   = "SIP;cause=302;text=\"Transfer\""
//       diversionReason = historyinfo.DiversionUnconditional
//   - For inbound REFER, use:
//       historyReason   = "SIP;cause=302;text=\"REFER\""
//       diversionReason = historyinfo.DiversionDeflection
//
// No-op if the new target URI is empty or there's no inbound history
// to extend (see buildRetargetHeaders).
func applyRetargetHeaders(
	req *outbound.DialRequest,
	rawTo, rawHistoryInfo, rawDiversion string,
	historyReason, diversionReason string,
) {
	if req == nil {
		return
	}
	hi, dv := buildRetargetHeaders(rawTo, rawHistoryInfo, rawDiversion, req.Target.RequestURI, historyReason, diversionReason)
	if len(hi) > 0 {
		req.HistoryInfo = hi
	}
	if len(dv) > 0 {
		req.Diversion = dv
	}
}
