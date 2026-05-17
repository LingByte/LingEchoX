// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/sip/stack"
	"github.com/LinByte/VoiceServer/pkg/sip/stir"
)

// stirTestEnv builds a self-contained signer + matched verifier
// (mock fetcher → in-memory cert) so tests don't need real HTTPS.
type stirTestEnv struct {
	signKey  *ecdsa.PrivateKey
	cert     *x509.Certificate
	verifier *stir.Verifier
}

type fixedFetcher struct{ certs map[string]*x509.Certificate }

func (f *fixedFetcher) Fetch(_ context.Context, url string) (*x509.Certificate, error) {
	if c, ok := f.certs[url]; ok {
		return c, nil
	}
	return nil, errFetchMiss
}

var errFetchMiss = errStr("no cert in fixed fetcher")

type errStr string

func (e errStr) Error() string { return string(e) }

func newSTIRTestEnv(t *testing.T) *stirTestEnv {
	t.Helper()
	now := time.Now()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "stir-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)

	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	leaf, _ := x509.ParseCertificate(leafDER)

	x5u := "https://sti.example/cert.pem"
	v := &stir.Verifier{
		Fetcher: &fixedFetcher{certs: map[string]*x509.Certificate{x5u: leaf}},
		MaxAge:  time.Minute,
		Now:     time.Now,
	}
	return &stirTestEnv{signKey: leafKey, cert: leaf, verifier: v}
}

// signedIdentityHeader produces a fully-formed Identity header value
// using the env's matched signer/verifier pair.
func (e *stirTestEnv) signedIdentityHeader(t *testing.T, fromTN, destTN string) string {
	t.Helper()
	hdr := stir.PassportHeader{
		Alg: stir.AlgES256, Ppt: stir.PptShaken, Typ: "passport",
		X5u: "https://sti.example/cert.pem",
	}
	claims := stir.PassportClaims{
		IAT:    time.Now().Unix(),
		Orig:   stir.PassportParty{TN: fromTN},
		Dest:   stir.PassportDestSet{TN: []string{destTN}},
		Attest: "A", OrigID: "uuid",
	}
	signed, err := stir.SignPassport(hdr, claims, e.signKey)
	if err != nil {
		t.Fatal(err)
	}
	val, _ := stir.FormatIdentityHeader(stir.IdentityHeader{
		Passport: signed.Compact,
		Info:     "https://sti.example/cert.pem",
		Alg:      stir.AlgES256,
		Ppt:      stir.PptShaken,
	})
	return val
}

func newInviteForTest(fromTN, destTN, identityHeader string) *stack.Message {
	m := &stack.Message{
		IsRequest:  true,
		Method:     "INVITE",
		RequestURI: "sip:" + destTN + "@carrier.example",
		Version:    "SIP/2.0",
	}
	m.SetHeader("Call-ID", "stir-test@h")
	m.SetHeader("From", "<sip:"+fromTN+"@example.com>;tag=fr")
	m.SetHeader("To", "<sip:"+destTN+"@carrier.example>")
	m.SetHeader("CSeq", "1 INVITE")
	m.SetHeader("Via", "SIP/2.0/UDP 1.2.3.4:5060;branch=z9hG4bKtest")
	if identityHeader != "" {
		m.SetHeader("Identity", identityHeader)
	}
	return m
}

// ---------------------------------------------------------------------------
// verifyInboundIdentity tests
// ---------------------------------------------------------------------------

func TestVerifyInboundIdentity_DisabledByDefault(t *testing.T) {
	s := &SIPServer{}
	msg := newInviteForTest("+15551234567", "+15559876543", "")
	if r := s.verifyInboundIdentity(msg); r != nil {
		t.Errorf("STIR disabled by default; should not reject: %v", r)
	}
}

func TestVerifyInboundIdentity_PassWithValidHeader(t *testing.T) {
	env := newSTIRTestEnv(t)
	var got stir.Verdict
	s := &SIPServer{
		listenHost: "127.0.0.1",
		stir: &STIRConfig{
			Verifier:  env.verifier,
			OnVerdict: func(_ string, v stir.Verdict) { got = v },
		},
	}
	id := env.signedIdentityHeader(t, "+15551234567", "+15559876543")
	msg := newInviteForTest("+15551234567", "+15559876543", id)
	if r := s.verifyInboundIdentity(msg); r != nil {
		t.Fatalf("valid Identity must not reject: %v", r)
	}
	if !got.Pass() {
		t.Errorf("verdict = %s (%s), want pass", got.Code, got.Reason)
	}
}

func TestVerifyInboundIdentity_MissingHeader_SoftAccept(t *testing.T) {
	env := newSTIRTestEnv(t)
	var got stir.Verdict
	s := &SIPServer{
		stir: &STIRConfig{
			Verifier:  env.verifier,
			OnVerdict: func(_ string, v stir.Verdict) { got = v },
		},
	}
	msg := newInviteForTest("+15551234567", "+15559876543", "")
	if r := s.verifyInboundIdentity(msg); r != nil {
		t.Fatalf("soft mode must accept missing header: %v", r)
	}
	if got.Code != stir.VerdictBadIdentity {
		t.Errorf("verdict = %s, want bad_identity (synthetic missing)", got.Code)
	}
}

func TestVerifyInboundIdentity_MissingHeader_HardReject(t *testing.T) {
	env := newSTIRTestEnv(t)
	s := &SIPServer{
		listenHost: "127.0.0.1",
		stir: &STIRConfig{
			Verifier:        env.verifier,
			RequireIdentity: true,
		},
	}
	msg := newInviteForTest("+15551234567", "+15559876543", "")
	r := s.verifyInboundIdentity(msg)
	if r == nil {
		t.Fatal("RequireIdentity should hard-reject missing header")
	}
	if r.StatusCode != 438 {
		t.Errorf("expected 438, got %d", r.StatusCode)
	}
	if !strings.Contains(r.GetHeader("Reason"), "stir/invalid_identity") {
		t.Errorf("Reason header missing verdict tag: %q", r.GetHeader("Reason"))
	}
}

func TestVerifyInboundIdentity_TamperedSignature_SoftAccept(t *testing.T) {
	env := newSTIRTestEnv(t)
	var got stir.Verdict
	s := &SIPServer{
		stir: &STIRConfig{
			Verifier:  env.verifier,
			OnVerdict: func(_ string, v stir.Verdict) { got = v },
		},
	}
	id := env.signedIdentityHeader(t, "+15551234567", "+15559876543")
	parts := strings.Split(strings.SplitN(id, ";", 2)[0], ".")
	parts[2] = strings.Repeat("A", len(parts[2]))
	tampered := strings.Join(parts, ".") + ";" + strings.SplitN(id, ";", 2)[1]
	msg := newInviteForTest("+15551234567", "+15559876543", tampered)
	if r := s.verifyInboundIdentity(msg); r != nil {
		t.Errorf("soft mode must not reject tampered: %v", r)
	}
	if got.Code != stir.VerdictBadIdentity {
		t.Errorf("verdict = %s, want bad_identity", got.Code)
	}
}

func TestVerifyInboundIdentity_TamperedSignature_HardReject(t *testing.T) {
	env := newSTIRTestEnv(t)
	s := &SIPServer{
		listenHost: "127.0.0.1",
		stir: &STIRConfig{
			Verifier:     env.verifier,
			RejectOnFail: true,
		},
	}
	id := env.signedIdentityHeader(t, "+15551234567", "+15559876543")
	parts := strings.Split(strings.SplitN(id, ";", 2)[0], ".")
	parts[2] = strings.Repeat("A", len(parts[2]))
	tampered := strings.Join(parts, ".") + ";" + strings.SplitN(id, ";", 2)[1]
	msg := newInviteForTest("+15551234567", "+15559876543", tampered)
	r := s.verifyInboundIdentity(msg)
	if r == nil || r.StatusCode != 438 {
		t.Fatalf("expected 438 reject, got %v", r)
	}
}

func TestVerifyInboundIdentity_TNMismatch_HardReject(t *testing.T) {
	env := newSTIRTestEnv(t)
	s := &SIPServer{
		listenHost: "127.0.0.1",
		stir: &STIRConfig{
			Verifier:     env.verifier,
			RejectOnFail: true,
		},
	}
	// PASSporT claims +15551234567 but From says +19998887777.
	id := env.signedIdentityHeader(t, "+15551234567", "+15559876543")
	msg := newInviteForTest("+19998887777", "+15559876543", id)
	r := s.verifyInboundIdentity(msg)
	if r == nil || r.StatusCode != 438 {
		t.Fatalf("expected 438 on TN mismatch, got %v", r)
	}
}

func TestVerifyInboundIdentity_OnVerdictHookPanicSafe(t *testing.T) {
	env := newSTIRTestEnv(t)
	s := &SIPServer{
		stir: &STIRConfig{
			Verifier:  env.verifier,
			OnVerdict: func(string, stir.Verdict) { panic("oops") },
		},
	}
	msg := newInviteForTest("+15551234567", "+15559876543", "")
	// Must not panic; soft-accept the call.
	if r := s.verifyInboundIdentity(msg); r != nil {
		t.Errorf("hook panic should not produce reject: %v", r)
	}
}

// ---------------------------------------------------------------------------
// helper-function tests
// ---------------------------------------------------------------------------

func TestExtractFromTN(t *testing.T) {
	cases := []struct {
		from string
		want string
	}{
		{`<sip:+15551234567@example.com>;tag=fr`, "+15551234567"},
		{`"Alice" <sip:+15551234567@example.com>;tag=fr`, "+15551234567"},
		{`sip:+15551234567@example.com;tag=fr`, "+15551234567"},
		{`<sip:alice@example.com>`, ""},   // non-E.164 user
		{`<sip:5551234567@example.com>`, ""}, // missing leading +
		{``, ""},
	}
	for _, c := range cases {
		m := &stack.Message{}
		m.SetHeader("From", c.from)
		if got := extractFromTN(m); got != c.want {
			t.Errorf("extractFromTN(%q) = %q, want %q", c.from, got, c.want)
		}
	}
}

func TestExtractURIFromFromHeader(t *testing.T) {
	cases := map[string]string{
		`<sip:alice@example.com>;tag=fr`:                     "sip:alice@example.com",
		`"Alice" <sip:alice@example.com>;tag=fr`:             "sip:alice@example.com",
		`sip:alice@example.com;tag=fr`:                       "sip:alice@example.com",
		`<sips:+15551234567@host>`:                            "sips:+15551234567@host",
		``:                                                   "",
	}
	for in, want := range cases {
		if got := extractURIFromFromHeader(in); got != want {
			t.Errorf("extractURIFromFromHeader(%q) = %q, want %q", in, got, want)
		}
	}
}
