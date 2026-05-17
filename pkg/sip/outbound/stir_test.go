// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/stir"
)

func newSignerForTest(t *testing.T) *STIRSigner {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &STIRSigner{
		PrivateKey:    key,
		X5U:           "https://sti.example/cert.pem",
		DefaultAttest: stir.AttestA,
		Ppt:           stir.PptShaken,
	}
}

func TestSTIRSigner_Sign_RoundTrip(t *testing.T) {
	s := newSignerForTest(t)
	val, err := s.Sign("call-1@h", "+15551234567", "+15559876543")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	hdr, err := stir.ParseIdentityHeader(val)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	signed, err := stir.VerifyPassport(hdr.Passport, &s.PrivateKey.PublicKey)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if signed.Claims.Orig.TN != "+15551234567" {
		t.Errorf("orig.tn = %q", signed.Claims.Orig.TN)
	}
	if len(signed.Claims.Dest.TN) != 1 || signed.Claims.Dest.TN[0] != "+15559876543" {
		t.Errorf("dest.tn = %v", signed.Claims.Dest.TN)
	}
	if signed.Claims.Attest != "A" {
		t.Errorf("attest = %q", signed.Claims.Attest)
	}
	if hdr.Ppt != stir.PptShaken {
		t.Errorf("ppt = %q", hdr.Ppt)
	}
}

func TestSTIRSigner_Sign_RejectsNonE164(t *testing.T) {
	s := newSignerForTest(t)
	cases := [][2]string{
		{"", "+15559876543"},
		{"+15551234567", ""},
		{"alice", "+15559876543"},      // non-E.164 from
		{"+15551234567", "bob"},        // non-E.164 dest
		{"5551234567", "+15559876543"}, // missing leading +
	}
	for _, c := range cases {
		if _, err := s.Sign("c", c[0], c[1]); err == nil {
			t.Errorf("expected error from=%q dest=%q", c[0], c[1])
		}
	}
}

func TestSTIRSigner_Sign_FormatNormalisation(t *testing.T) {
	s := newSignerForTest(t)
	// Display-name-style formatting should still produce a valid sign.
	val, err := s.Sign("c", "+1 (555) 123-4567", "+1-555-987-6543")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	hdr, _ := stir.ParseIdentityHeader(val)
	signed, _ := stir.VerifyPassport(hdr.Passport, &s.PrivateKey.PublicKey)
	if signed.Claims.Orig.TN != "+15551234567" {
		t.Errorf("normalised orig.tn = %q", signed.Claims.Orig.TN)
	}
	if signed.Claims.Dest.TN[0] != "+15559876543" {
		t.Errorf("normalised dest.tn = %q", signed.Claims.Dest.TN[0])
	}
}

func TestSTIRSigner_MissingKey(t *testing.T) {
	s := &STIRSigner{X5U: "https://x.example/c.pem"}
	if _, err := s.Sign("c", "+1234567", "+7654321"); err == nil {
		t.Fatal("missing key should error")
	}
}

func TestSTIRSigner_MetricsHook(t *testing.T) {
	// MetricsHook fires for signing-pipeline outcomes only; input
	// validation errors are caller bugs and are intentionally NOT
	// counted (would inflate "STIR failure" metrics with garbage).
	var got string
	s := newSignerForTest(t)
	s.MetricsHook = func(_, outcome string, _ error) { got = outcome }
	_, _ = s.Sign("c", "+15551234567", "+15559876543")
	if got != "ok" {
		t.Errorf("metrics outcome = %q, want ok", got)
	}
	got = ""
	_, _ = s.Sign("c", "", "+1") // input validation error
	if got != "" {
		t.Errorf("metrics hook should NOT fire on input validation error, got %q", got)
	}
}

func TestExtractTNFromRequestURI(t *testing.T) {
	cases := map[string]string{
		"sip:+15551234567@carrier.example":            "+15551234567",
		"sip:+15551234567@carrier.example;user=phone": "+15551234567",
		"sips:+15551234567@host:5060":                 "+15551234567",
		"sip:alice@example.com":                       "", // non-E.164
		"sip:5551234567@host":                         "", // missing +
		"":                                            "",
		"  sip:+1 555 123 4567@host  ":                "+15551234567",
	}
	for in, want := range cases {
		if got := extractTNFromRequestURI(in); got != want {
			t.Errorf("extractTN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignOutboundIdentity_SoftFail(t *testing.T) {
	// Nil signer → no header, no panic.
	val, ok := signOutboundIdentity(nil, "c", "+1", "+2")
	if ok || val != "" {
		t.Errorf("nil signer must soft-fail: ok=%v val=%q", ok, val)
	}
	// Bad input on real signer → no header, no panic, no error return
	// (logged + counted internally).
	s := newSignerForTest(t)
	val, ok = signOutboundIdentity(s, "c", "alice", "+1")
	if ok || val != "" {
		t.Errorf("non-E.164 from should soft-fail: ok=%v val=%q", ok, val)
	}
}

func TestBuildINVITE_WithIdentityHeader(t *testing.T) {
	p := inviteParams{
		LocalIP: "127.0.0.1", SIPHost: "127.0.0.1", SIPPort: 6050,
		RequestURI: "sip:+15559876543@carrier.example",
		CallID:     "c@h", FromTag: "t", Branch: "br", CSeq: 1,
		FromUser:       "+15551234567",
		IdentityHeader: "fake.jwt.value;info=<https://x.example/c.pem>;alg=ES256;ppt=shaken",
	}
	msg := buildINVITE(p)
	got := msg.GetHeader("Identity")
	if !strings.Contains(got, "fake.jwt.value") {
		t.Errorf("Identity header missing: %q", got)
	}
}

func TestBuildINVITE_NoIdentityHeaderWhenEmpty(t *testing.T) {
	p := inviteParams{
		LocalIP: "127.0.0.1", SIPHost: "127.0.0.1", SIPPort: 6050,
		RequestURI: "sip:bob@example.com",
		CallID:     "c@h", FromTag: "t", Branch: "br", CSeq: 1,
		FromUser: "alice",
	}
	msg := buildINVITE(p)
	if h := msg.GetHeader("Identity"); h != "" {
		t.Errorf("Identity should be absent when empty: %q", h)
	}
}
