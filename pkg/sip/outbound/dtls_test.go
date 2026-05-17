// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import (
	"strings"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
)

func TestPrepareOutboundDTLSOffer_ReturnsCoherentMaterial(t *testing.T) {
	proto, extras, pending, err := prepareOutboundDTLSOffer()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if proto != "UDP/TLS/RTP/SAVP" {
		t.Errorf("proto = %q, want UDP/TLS/RTP/SAVP", proto)
	}
	if pending == nil || len(pending.CertDER) == 0 || pending.Key == nil {
		t.Fatalf("pending missing material: %+v", pending)
	}
	if pending.OfferedRole != sdp.DTLSRoleActPass {
		t.Errorf("offered role = %q, want actpass", pending.OfferedRole)
	}
	if len(extras) != 2 {
		t.Fatalf("extras = %v", extras)
	}

	// RFC 5763 §3: the fingerprint we advertised in SDP MUST be the
	// SHA-256 of the cert we'll actually present.
	wantFP := rtp.FingerprintSHA256(pending.CertDER)
	var gotFP string
	var hasSetup bool
	for _, l := range extras {
		if strings.HasPrefix(l, "a=fingerprint:sha-256 ") {
			gotFP = strings.TrimPrefix(l, "a=fingerprint:sha-256 ")
		}
		if l == "a=setup:actpass" {
			hasSetup = true
		}
	}
	if !strings.EqualFold(gotFP, wantFP) {
		t.Errorf("SDP advertised fp = %q, cert actual = %q", gotFP, wantFP)
	}
	if !hasSetup {
		t.Errorf("missing a=setup:actpass: %v", extras)
	}
}

func TestResolveOurRoleFromAnswer(t *testing.T) {
	cases := map[sdp.DTLSRole]struct {
		asServer bool
		ok       bool
	}{
		sdp.DTLSRolePassive:  {asServer: false, ok: true}, // peer waits → we initiate
		sdp.DTLSRoleActive:   {asServer: true, ok: true},  // peer initiates → we wait
		sdp.DTLSRoleActPass:  {asServer: false, ok: true}, // best-effort: be active
		sdp.DTLSRoleHoldConn: {asServer: false, ok: false},
		sdp.DTLSRole(""):     {asServer: false, ok: false},
	}
	for in, want := range cases {
		as, ok := resolveOurRoleFromAnswer(in)
		if as != want.asServer || ok != want.ok {
			t.Errorf("resolveOurRoleFromAnswer(%q) = (%v,%v), want (%v,%v)",
				in, as, ok, want.asServer, want.ok)
		}
	}
}

// TestDialRequest_OfferDTLSSRTP_BuildsCorrectOfferSDP verifies the
// full Dial → SDP body path: when OfferDTLSSRTP=true, the SDP body
// uses UDP/TLS/RTP/SAVP + a=fingerprint + a=setup:actpass, NOT
// RTP/SAVPF + a=crypto.
//
// We don't actually run Dial (it requires a live SIP listener), but
// we DO exercise the same offer-construction helper and confirm
// GenerateWithProtoExtras + the extras yield a well-formed body.
func TestDialRequest_OfferDTLSSRTP_BuildsCorrectOfferSDP(t *testing.T) {
	proto, extras, _, err := prepareOutboundDTLSOffer()
	if err != nil {
		t.Fatal(err)
	}
	body := sdp.GenerateWithProtoExtras("1.2.3.4", 5004, proto,
		[]sdp.Codec{{PayloadType: 0, Name: "pcmu", ClockRate: 8000, Channels: 1}}, extras)
	if !strings.Contains(body, "m=audio 5004 UDP/TLS/RTP/SAVP 0") {
		t.Errorf("body missing DTLS m= line:\n%s", body)
	}
	if !strings.Contains(body, "a=fingerprint:sha-256 ") {
		t.Errorf("body missing a=fingerprint:\n%s", body)
	}
	if !strings.Contains(body, "a=setup:actpass") {
		t.Errorf("body missing a=setup:actpass:\n%s", body)
	}
	if strings.Contains(body, "a=crypto:") {
		t.Errorf("DTLS offer must NOT carry SDES a=crypto:\n%s", body)
	}

	// Round-trip: our own parser must agree this is a DTLS offer.
	info, err := sdp.Parse(body)
	if err != nil {
		t.Fatalf("parse round-trip: %v", err)
	}
	if !sdp.IsDTLSTransport(info.Proto) {
		t.Errorf("round-trip proto not recognised as DTLS: %q", info.Proto)
	}
	if len(info.Fingerprints) != 1 {
		t.Errorf("round-trip fingerprints = %v", info.Fingerprints)
	}
	if info.DTLSRole != sdp.DTLSRoleActPass {
		t.Errorf("round-trip setup = %q", info.DTLSRole)
	}
}

func TestStartOutboundDTLSHandshake_RejectsAnswerWithoutFingerprint(t *testing.T) {
	// Exercise the early-exit guard. We can't run a real handshake
	// (no peer) but we CAN verify that the function bails before
	// touching the RTP session when the answer lacks a fingerprint.
	// We pass nil rtpSess + a synthetic pending — the function
	// MUST log + bail rather than NPE.
	answer := &sdp.Info{
		Proto:    "UDP/TLS/RTP/SAVP",
		DTLSRole: sdp.DTLSRolePassive,
		// No fingerprints → should fail early.
	}
	leg := &outLeg{}
	leg.params.CallID = "test-call"
	// rtpSess is nil → without the early-exit on Fingerprints==0,
	// the function would NPE inside StartDTLS. The guard prevents
	// that and routes to cleanupLeg which we wire to a sentinel.
	startOutboundDTLSHandshake(leg, &outboundDTLSPending{}, answer)
	// If we reach here without panic, the guard worked. We don't
	// assert on cleanupLeg side effects because they require a
	// full Manager wiring; this test is about the safety guard.
}

func TestStartOutboundDTLSHandshake_NilSafe(t *testing.T) {
	// Each nil argument should be a silent no-op (no panic).
	startOutboundDTLSHandshake(nil, nil, nil)
	startOutboundDTLSHandshake(&outLeg{}, nil, &sdp.Info{})
}
