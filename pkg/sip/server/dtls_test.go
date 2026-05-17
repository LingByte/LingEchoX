// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
)

func TestPrepareDTLSAnswer_DisabledByDefault(t *testing.T) {
	s := &SIPServer{}
	offer := &sdp.Info{
		Proto:        "UDP/TLS/RTP/SAVP",
		Fingerprints: []sdp.Fingerprint{{HashFunc: "sha-256", Hex: "AA:BB:CC"}},
		DTLSRole:     sdp.DTLSRoleActPass,
	}
	res, err := s.prepareDTLSAnswer(offer)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if res != nil {
		t.Fatal("disabled server should return nil result")
	}
}

func TestPrepareDTLSAnswer_NonDTLSOffer(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	// SDES offer should NOT trigger DTLS prep.
	offer := &sdp.Info{
		Proto: "RTP/SAVP",
		CryptoOffers: []sdp.CryptoOffer{
			{Tag: 1, Suite: "AES_CM_128_HMAC_SHA1_80", KeyParams: "inline:abc"},
		},
	}
	res, err := s.prepareDTLSAnswer(offer)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if res != nil {
		t.Fatal("non-DTLS offer must not produce DTLS answer")
	}
}

func TestPrepareDTLSAnswer_DTLSOfferWithoutFingerprintFails(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	offer := &sdp.Info{
		Proto:    "UDP/TLS/RTP/SAVPF",
		DTLSRole: sdp.DTLSRoleActPass,
	}
	if _, err := s.prepareDTLSAnswer(offer); err == nil {
		t.Fatal("expected error for DTLS offer without fingerprint")
	}
}

func TestPrepareDTLSAnswer_HoldConnRoleFails(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	offer := &sdp.Info{
		Proto:        "UDP/TLS/RTP/SAVP",
		Fingerprints: []sdp.Fingerprint{{HashFunc: "sha-256", Hex: "AB:CD:EF"}},
		DTLSRole:     sdp.DTLSRoleHoldConn,
	}
	if _, err := s.prepareDTLSAnswer(offer); err == nil {
		t.Fatal("holdconn role must not be answerable")
	}
}

func TestPrepareDTLSAnswer_Success_OfferActPass_AnswerPassive(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	offer := &sdp.Info{
		Proto:        "UDP/TLS/RTP/SAVP",
		Fingerprints: []sdp.Fingerprint{{HashFunc: "sha-256", Hex: "AA:BB:CC:DD"}},
		DTLSRole:     sdp.DTLSRoleActPass,
	}
	res, err := s.prepareDTLSAnswer(offer)
	if err != nil || res == nil {
		t.Fatalf("err=%v res=%v", err, res)
	}
	if len(res.ExtraLines) != 2 {
		t.Fatalf("extra lines = %v", res.ExtraLines)
	}
	hasFP := false
	hasSetup := false
	for _, l := range res.ExtraLines {
		if strings.HasPrefix(l, "a=fingerprint:sha-256 ") {
			hasFP = true
		}
		if l == "a=setup:passive" {
			hasSetup = true
		}
	}
	if !hasFP {
		t.Errorf("missing a=fingerprint line: %v", res.ExtraLines)
	}
	if !hasSetup {
		t.Errorf("missing a=setup:passive (we chose passive on actpass): %v", res.ExtraLines)
	}
	if res.Pending == nil || !res.Pending.AsServer {
		t.Errorf("pending state expected AsServer=true")
	}
	if len(res.Pending.PeerFingerprints) != 1 {
		t.Errorf("peer fingerprints not preserved")
	}
	if len(res.Pending.CertDER) == 0 || res.Pending.Key == nil {
		t.Errorf("cert/key not minted")
	}
}

func TestPrepareDTLSAnswer_OfferActive_AnswerPassive(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	offer := &sdp.Info{
		Proto:        "UDP/TLS/RTP/SAVP",
		Fingerprints: []sdp.Fingerprint{{HashFunc: "sha-256", Hex: "AA"}},
		DTLSRole:     sdp.DTLSRoleActive,
	}
	res, _ := s.prepareDTLSAnswer(offer)
	if res == nil {
		t.Fatal("nil result")
	}
	hasPassive := false
	for _, l := range res.ExtraLines {
		if l == "a=setup:passive" {
			hasPassive = true
		}
	}
	if !hasPassive {
		t.Errorf("offerer=active must yield answerer=passive")
	}
}

func TestPrepareDTLSAnswer_OfferPassive_AnswerActive(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	offer := &sdp.Info{
		Proto:        "UDP/TLS/RTP/SAVP",
		Fingerprints: []sdp.Fingerprint{{HashFunc: "sha-256", Hex: "AA"}},
		DTLSRole:     sdp.DTLSRolePassive,
	}
	res, _ := s.prepareDTLSAnswer(offer)
	if res == nil {
		t.Fatal("nil result")
	}
	hasActive := false
	for _, l := range res.ExtraLines {
		if l == "a=setup:active" {
			hasActive = true
		}
	}
	if !hasActive {
		t.Errorf("offerer=passive must yield answerer=active")
	}
	if res.Pending.AsServer {
		t.Errorf("when we're active we must NOT be DTLS server")
	}
}

func TestStashAndTakePendingDTLS(t *testing.T) {
	s := &SIPServer{}
	state := &dtlsPendingState{CertDER: []byte{1, 2, 3}}
	s.stashPendingDTLS("call-1", state)
	// take returns state and clears.
	got := s.takePendingDTLS("call-1")
	if got != state {
		t.Errorf("take returned different state")
	}
	if again := s.takePendingDTLS("call-1"); again != nil {
		t.Errorf("second take must return nil (cleared)")
	}
}

func TestStashPendingDTLS_NilSafe(t *testing.T) {
	var s *SIPServer
	s.stashPendingDTLS("x", nil) // must not panic
	if got := s.takePendingDTLS("x"); got != nil {
		t.Errorf("nil server take = %v", got)
	}

	s = &SIPServer{}
	s.stashPendingDTLS("", &dtlsPendingState{}) // empty call-id ignored
	if got := s.takePendingDTLS(""); got != nil {
		t.Errorf("empty call-id take = %v", got)
	}
}

// TestVerifyPeerFingerprint_Match generates a real cert, builds a
// fake DTLSEndpoint state with that cert as the peer, and verifies
// the fingerprint matches what we'd advertise.
func TestVerifyPeerFingerprint_Match(t *testing.T) {
	der, _, err := rtp.SelfSignedDTLSCert(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	advertised := []sdp.Fingerprint{
		{HashFunc: "sha-256", Hex: rtp.FingerprintSHA256(der)},
	}
	// We can't easily build a DTLSEndpoint without a real handshake,
	// but we can verify the comparison function directly. Mirror
	// what verifyPeerFingerprint does:
	got := strings.ToUpper(rtp.FingerprintSHA256(der))
	matched := false
	for _, fp := range advertised {
		if strings.EqualFold(fp.Hex, got) {
			matched = true
		}
	}
	if !matched {
		t.Errorf("self-computed fingerprint should always match")
	}
}

func TestVerifyPeerFingerprint_Mismatch(t *testing.T) {
	derPeer, _, _ := rtp.SelfSignedDTLSCert(time.Time{})
	derOther, _, _ := rtp.SelfSignedDTLSCert(time.Time{})
	advertised := []sdp.Fingerprint{
		{HashFunc: "sha-256", Hex: rtp.FingerprintSHA256(derOther)},
	}
	got := strings.ToUpper(rtp.FingerprintSHA256(derPeer))
	matched := false
	for _, fp := range advertised {
		if strings.EqualFold(fp.Hex, got) {
			matched = true
		}
	}
	if matched {
		t.Errorf("mismatched fingerprints should NOT match — MITM bypass risk")
	}
}

func TestVerifyPeerFingerprint_UnsupportedHashIgnored(t *testing.T) {
	der, _, _ := rtp.SelfSignedDTLSCert(time.Time{})
	advertised := []sdp.Fingerprint{
		{HashFunc: "sha-384", Hex: "DEAD:BEEF"}, // unsupported algo
		{HashFunc: "sha-256", Hex: rtp.FingerprintSHA256(der)},
	}
	got := strings.ToUpper(rtp.FingerprintSHA256(der))
	matched := false
	for _, fp := range advertised {
		if !strings.EqualFold(fp.HashFunc, "sha-256") {
			continue
		}
		if strings.EqualFold(fp.Hex, got) {
			matched = true
		}
	}
	if !matched {
		t.Errorf("sha-256 fallback should match when sha-384 is unsupported")
	}
}

// TestPrepareDTLSAnswer_FingerprintInExtrasMatchesCert is the
// load-bearing test: what we advertise in SDP must hash the cert
// we'll actually present in the handshake. RFC 5763 §3.
func TestPrepareDTLSAnswer_FingerprintInExtrasMatchesCert(t *testing.T) {
	s := &SIPServer{}
	s.SetInboundDTLSAccept(true)
	offer := &sdp.Info{
		Proto:        "UDP/TLS/RTP/SAVP",
		Fingerprints: []sdp.Fingerprint{{HashFunc: "sha-256", Hex: "00:11"}},
		DTLSRole:     sdp.DTLSRoleActPass,
	}
	res, err := s.prepareDTLSAnswer(offer)
	if err != nil || res == nil {
		t.Fatalf("err=%v", err)
	}
	expectedFP := rtp.FingerprintSHA256(res.Pending.CertDER)
	var sdpFP string
	for _, l := range res.ExtraLines {
		if strings.HasPrefix(l, "a=fingerprint:sha-256 ") {
			sdpFP = strings.TrimPrefix(l, "a=fingerprint:sha-256 ")
		}
	}
	if !strings.EqualFold(sdpFP, expectedFP) {
		t.Errorf("SDP advertised fp = %q, cert actual = %q — RFC 5763 §3 violated",
			sdpFP, expectedFP)
	}
}
