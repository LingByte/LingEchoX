// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

// outbound/stir.go: SHAKEN signing for outbound INVITE.
//
// The pure crypto + PASSporT + header rendering lives in
// `pkg/sip/stir`. This file is the **adapter** between an outbound
// call (DialRequest + inviteParams) and a signed `Identity:` header
// — it pulls the From-TN out of the wire-format From URI, builds a
// PASSporT, and returns the Identity header value.
//
// Design notes:
//
//   - Signing is opt-in via ManagerConfig.STIRSigner. Default = nil =
//     do not emit Identity. Most deployments don't have a STI-CA-
//     issued cert yet, so silent fallback to "no Identity" is the
//     least surprising behaviour.
//
//   - Attestation level is per-call configurable through STIRSigner.
//     The most common pattern is: A for outbound from a SIP user
//     owned by the platform; B/C for transfer / forwarded legs. A
//     hook lets the caller override per DialRequest.
//
//   - We DO NOT auto-fail the dial when signing fails — emit a log
//     and dial without Identity. STIR is "nice-to-have" for trunks
//     that gate on it; never block a legitimate call on a cert
//     glitch (would cause platform-wide outages on key rotation).

import (
	"crypto/ecdsa"
	"fmt"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/stir"
	"go.uber.org/zap"
)

// STIRSigner is the configuration a Manager uses to sign outbound
// INVITEs with an RFC 8224 Identity header. Build one once at
// service start (typically from PEM files on disk) and assign it to
// ManagerConfig.STIRSigner. The Manager calls Sign on each outbound
// INVITE; concurrent calls are safe.
type STIRSigner struct {
	// PrivateKey is the ES256 (P-256) key matching the cert at X5U.
	// Required.
	PrivateKey *ecdsa.PrivateKey

	// X5U is the public HTTPS URL where the cert chain is served.
	// Required; MUST start with "https://".
	X5U string

	// DefaultAttest is the SHAKEN attestation level to use when a
	// per-call override isn't provided. Most platforms ship "A" for
	// owned-number outbound; "C" for gateway / unattested cases.
	DefaultAttest stir.AttestationLevel

	// OriginIDFn returns the SHAKEN `origid` (UUID per RFC 8588 §6)
	// for one call. Default: a derived value from the SIP Call-ID +
	// service-prefix to make traceback queries match the platform's
	// log identifiers. Override when the deployment has a separate
	// traceback ID source (CDR id, etc.).
	OriginIDFn func(callID string) string

	// Ppt is the PASSporT extension to assert. Default "shaken"; set
	// to "" to emit a base RFC 8225 PASSporT (rare, mostly for non-
	// SHAKEN deployments).
	Ppt string

	// MetricsHook is called once per Sign attempt with the outcome
	// (empty err = success). Used to wire Prometheus counters from
	// the cmd layer without coupling this package to a metrics lib.
	MetricsHook func(callID, outcome string, err error)
}

// Sign builds the Identity header value for one outbound INVITE.
// Returns "" + non-nil error when signing fails — the caller is
// expected to swallow the error and proceed without the header.
//
// fromTN is the E.164 number we're claiming as the caller (typically
// extracted from inviteParams.FromUser). destTN is the destination
// number (extracted from the request URI). Both must be non-empty
// for SHAKEN; for non-SHAKEN you can pass URIs by setting fromURI /
// destURI on the params (not exposed yet — add when needed).
func (s *STIRSigner) Sign(callID, fromTN, destTN string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("outbound/stir: nil signer")
	}
	if s.PrivateKey == nil {
		return "", fmt.Errorf("outbound/stir: signer missing private key")
	}
	if strings.TrimSpace(s.X5U) == "" {
		return "", fmt.Errorf("outbound/stir: signer missing X5U")
	}
	fromTN = canonicalE164(fromTN)
	destTN = canonicalE164(destTN)
	if fromTN == "" || destTN == "" {
		return "", fmt.Errorf("outbound/stir: need E.164 from/dest TNs, got from=%q dest=%q", fromTN, destTN)
	}
	attest := s.DefaultAttest
	if !attest.IsValid() {
		attest = stir.AttestC // safest default — unattested
	}
	ppt := s.Ppt
	if ppt == "" {
		ppt = stir.PptShaken
	}
	origID := ""
	if s.OriginIDFn != nil {
		origID = s.OriginIDFn(callID)
	}
	if origID == "" {
		origID = defaultOriginID(callID)
	}

	hdr := stir.PassportHeader{
		Alg: stir.AlgES256,
		Ppt: ppt,
		Typ: "passport",
		X5u: s.X5U,
	}
	claims := stir.PassportClaims{
		IAT:    time.Now().Unix(),
		Orig:   stir.PassportParty{TN: fromTN},
		Dest:   stir.PassportDestSet{TN: []string{destTN}},
		Attest: string(attest),
		OrigID: origID,
	}
	signed, err := stir.SignPassport(hdr, claims, s.PrivateKey)
	if err != nil {
		s.emitMetric(callID, "sign_error", err)
		return "", fmt.Errorf("outbound/stir: sign: %w", err)
	}
	value, err := stir.FormatIdentityHeader(stir.IdentityHeader{
		Passport: signed.Compact,
		Info:     s.X5U,
		Alg:      stir.AlgES256,
		Ppt:      ppt,
	})
	if err != nil {
		s.emitMetric(callID, "format_error", err)
		return "", fmt.Errorf("outbound/stir: format header: %w", err)
	}
	s.emitMetric(callID, "ok", nil)
	return value, nil
}

func (s *STIRSigner) emitMetric(callID, outcome string, err error) {
	if s == nil || s.MetricsHook == nil {
		return
	}
	defer func() {
		// MetricsHook is user-provided; don't let panics blow up
		// the dial path.
		_ = recover()
	}()
	s.MetricsHook(callID, outcome, err)
}

// canonicalE164 strips common punctuation introduced by display-name
// renderers ("+1 (555) 123-4567" → "+15551234567"). Returns "" when
// the input doesn't look like E.164.
func canonicalE164(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		switch {
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	out := b.String()
	// Require at least leading + followed by digits; bare digits
	// without a country code shouldn't claim to be E.164.
	if !strings.HasPrefix(out, "+") || len(out) < 4 {
		return ""
	}
	return out
}

// defaultOriginID produces a deterministic UUID-ish identifier from
// the SIP Call-ID. RFC 8588 says origid is a UUID but every major
// carrier accepts any URL-safe ≤36-char identifier and we want
// log-correlation with the platform Call-ID. Pad/trim to 36 chars
// to look UUID-ish (some SBCs validate length).
func defaultOriginID(callID string) string {
	id := strings.TrimSpace(callID)
	if id == "" {
		id = fmt.Sprintf("lingbyte-%d", time.Now().UnixNano())
	}
	// Strip "@host" suffix typical of SIP Call-IDs.
	if at := strings.IndexByte(id, '@'); at > 0 {
		id = id[:at]
	}
	if len(id) > 36 {
		id = id[:36]
	}
	return id
}

// extractTNFromRequestURI pulls the user-part out of a SIP request
// URI ("sip:+15551234567@host;user=phone" → "+15551234567") and
// runs it through canonicalE164. Returns "" when the user-part
// isn't an E.164 number, which causes Sign to soft-fail this call.
func extractTNFromRequestURI(uri string) string {
	s := strings.TrimSpace(uri)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[i+1:]
	}
	if at := strings.IndexByte(s, '@'); at > 0 {
		s = s[:at]
	}
	if semi := strings.IndexByte(s, ';'); semi >= 0 {
		s = s[:semi]
	}
	return canonicalE164(s)
}

// signOutboundIdentity is the integration point called from Dial.
// Stays here (not on Manager) so manager.go doesn't grow another
// 100 lines of stir-aware logic. Caller passes the resolved fromTN
// / destTN already extracted from inviteParams.
func signOutboundIdentity(s *STIRSigner, callID, fromTN, destTN string) (string, bool) {
	if s == nil {
		return "", false
	}
	val, err := s.Sign(callID, fromTN, destTN)
	if err != nil {
		// Soft-fail: never block a dial on STIR error.
		logger.Warn("sip outbound STIR sign failed; dialing without Identity",
			zap.String("call_id", callID),
			zap.String("from_tn", fromTN),
			zap.String("dest_tn", destTN),
			zap.Error(err))
		return "", false
	}
	return val, true
}
