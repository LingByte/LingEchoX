// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

// Per-call QoS flush.
//
// At cleanupLeg time we have one last chance to read the RTCP-
// derived statistics off the live Session before we Close() it. We
// fold those numbers into:
//
//   1. The metrics histograms (sip_call_rtt_ms, sip_call_jitter_ms,
//      sip_call_loss_fraction, sip_call_mos_estimate).
//   2. (Future) the CDR record for this call.
//
// This is a once-per-call observation. It must complete in O(1)
// time and never block. We deliberately keep the function tiny so
// it's obviously safe to call from the cleanup path.

import (
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/voice/qos"
)

// flushOutboundCallQoS reads the RTP session's RTCP snapshot and
// pushes the derived metrics. nil-safe at every level.
func flushOutboundCallQoS(leg *outLeg) {
	if leg == nil || leg.rtpSess == nil {
		return
	}
	snap := leg.rtpSess.RTCPSnapshot()
	if !snap.PeerSeenRR && snap.LocalJitter == 0 {
		// No RTCP traffic at all — nothing to record. Avoid
		// polluting histograms with synthetic zero data.
		return
	}

	// Convert peer fraction-lost (Q0.8) to a normalized 0..1 float.
	lossFraction := float64(snap.PeerLossFraction) / 256.0

	// Convert local jitter (RTP clock units) to ms. Codec/clockRate
	// determination happens via leg.rtpSess hints; we default to
	// 8 kHz (G.711 narrowband) which is the dominant case for SIP.
	clockRate := uint32(8000)
	jitterMs := float64(snap.LocalJitter) * 1000.0 / float64(clockRate)

	// MOS estimate from the E-Model.
	mos := qos.Estimate(qos.MOSInput{
		RTTMs:            snap.RTTMs,
		JitterRTPUnits:   snap.LocalJitter,
		JitterClockRate:  clockRate,
		PeerLossFraction: lossFraction,
		Codec:            "pcmu", // TODO: surface negotiated codec from rtpSess
	})

	sipMetrics.ObserveCallQoS(snap.RTTMs, jitterMs, lossFraction, mos.MOS)
}
