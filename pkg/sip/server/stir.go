// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package server

// server/stir.go: inbound RFC 8224 Identity header verification.
//
// The full verification pipeline (parse → fetch x5u → verify JWS →
// info=x5u check → staleness → From-TN match) lives in pkg/sip/stir.
// This file is the **adapter** between an inbound INVITE and the
// stir.Verifier: it pulls the Identity header value off the SIP
// message, extracts the From-TN, calls Verify, and decides what to
// do with the verdict.
//
// Policy decisions (defaults per product call 2026-05-17):
//
//   * Missing Identity header → policy decides. Default: soft-warn
//     and accept (most carriers haven't fully rolled out SHAKEN).
//     Set STIRRequireIdentity to flip to hard-reject.
//   * Verification failure → soft-warn + emit hook with verdict.
//     Set STIRRejectOnFail to flip to hard-reject (carrier-style).
//
// The hook lets the call-center layer attach the verdict to the
// CallSession / CDR even when not rejecting, so reps see "this
// caller is unverified" in their UI without us blocking the call.

import (
	"context"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
	"github.com/LinByte/VoiceServer/pkg/sip/stir"
	"go.uber.org/zap"
)

// STIRConfig is the per-server inbound verification policy. Attach
// to Config.STIR to enable; nil disables verification entirely.
type STIRConfig struct {
	// Verifier holds the dependencies (X5UFetcher with the deployment's
	// trust pool, MaxAge for replay protection, etc.). Required.
	Verifier *stir.Verifier

	// RequireIdentity: when true, an INVITE with no Identity header
	// is rejected as VerdictBadIdentity → 438. Default false (soft
	// mode): missing header → soft-warn + Verdict{Code: missing} +
	// accept.
	RequireIdentity bool

	// RejectOnFail: when true, any non-pass Verdict is converted to
	// the SIP response code in VerdictCode.SIPResponseCode() and the
	// INVITE is rejected. Default false: log + hook + accept.
	RejectOnFail bool

	// VerifyTimeout caps one verification (x5u fetch is the bulk of
	// it). Zero → 3s. Beyond this the verdict becomes BadCert.
	VerifyTimeout time.Duration

	// RequiredPpt is the PASSporT extension we insist on (most
	// deployments want "shaken"). Empty allows any.
	RequiredPpt string

	// RequiredAttests is the set of attestation levels we accept.
	// Empty allows any (A/B/C). Set []{"A"} to reject B/C-attested
	// calls (very strict, common only inside SP networks).
	RequiredAttests []string

	// OnVerdict fires after each verification (pass OR fail). The
	// CallSession hasn't been built yet — caller is expected to stash
	// the verdict against the Call-ID for later correlation. Hook
	// errors / panics never block call routing.
	OnVerdict func(callID string, v stir.Verdict)
}

// VerdictMissing is the synthetic verdict emitted when an INVITE has
// no Identity header. RFC 8224 doesn't define a code for "absent";
// we map this to BadIdentity for consistency with the
// SIPResponseCode → 438 mapping.
var VerdictMissing = stir.Verdict{
	Code:   stir.VerdictBadIdentity,
	Reason: "no Identity header on inbound INVITE",
}

// verifyInboundIdentity is the integration point called from
// handleInvite. Returns nil when the call should proceed, or a
// non-nil response message when the call is being rejected.
//
// Errors / hook panics are swallowed; STIR misconfiguration must
// never bring down call-routing.
//
// Observability: one metric increment per call regardless of which
// branch we take (no-identity / pass / fail / config-error). Done
// via a small classifier closure so adding new branches doesn't
// silently skip the counter.
func (s *SIPServer) verifyInboundIdentity(msg *stack.Message) *stack.Message {
	if s == nil || msg == nil {
		return nil
	}
	cfg := s.stirCfg()
	if cfg == nil || cfg.Verifier == nil {
		return nil // STIR disabled
	}
	callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
	idHeader := strings.TrimSpace(msg.GetHeader("Identity"))

	if idHeader == "" {
		verdict := VerdictMissing
		s.fireSTIRVerdict(cfg, callID, verdict)
		sipMetrics.STIRVerify(sipMetrics.STIRResultNoIdent)
		if cfg.RequireIdentity {
			return s.stirReject(msg, callID, verdict)
		}
		return nil
	}

	timeout := cfg.VerifyTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	verdict, err := cfg.Verifier.Verify(ctx, idHeader, stir.VerifyOptions{
		FromTN:          extractFromTN(msg),
		FromURI:         extractFromURI(msg),
		RequiredPpt:     cfg.RequiredPpt,
		RequiredAttests: cfg.RequiredAttests,
	})
	if err != nil {
		// Programming fault (e.g. nil fetcher) — log and accept,
		// don't synthesise a verdict that misleads downstream code.
		logger.Error("sip inbound STIR verify error (accepting call)",
			zap.String("call_id", callID),
			zap.Error(err))
		sipMetrics.STIRVerify(sipMetrics.STIRResultFailed)
		return nil
	}

	s.fireSTIRVerdict(cfg, callID, verdict)

	if verdict.Pass() {
		sipMetrics.STIRVerify(sipMetrics.STIRResultVerified)
		return nil
	}
	if cfg.RejectOnFail {
		sipMetrics.STIRVerify(sipMetrics.STIRResultFailed)
		return s.stirReject(msg, callID, verdict)
	}
	// Verification failed but policy is soft — call still goes
	// through. Tag as soft-fail so dashboards can split "rejected"
	// vs "warned" outcomes.
	sipMetrics.STIRVerify(sipMetrics.STIRResultSoftFail)
	logger.Warn("sip inbound STIR verify failed (accepting in soft mode)",
		zap.String("call_id", callID),
		zap.String("verdict", verdict.Code.String()),
		zap.Int("sip_code", verdict.Code.SIPResponseCode()),
		zap.String("reason", verdict.Reason))
	return nil
}

func (s *SIPServer) stirCfg() *STIRConfig {
	if s == nil {
		return nil
	}
	return s.stir
}

func (s *SIPServer) fireSTIRVerdict(cfg *STIRConfig, callID string, v stir.Verdict) {
	if cfg == nil || cfg.OnVerdict == nil {
		return
	}
	defer func() {
		// User-provided hook; never let it crash the SIP path.
		if r := recover(); r != nil {
			logger.Error("sip inbound STIR OnVerdict hook panic",
				zap.Any("recover", r),
				zap.String("call_id", callID))
		}
	}()
	cfg.OnVerdict(callID, v)
}

// stirReject builds the SIP response for a failed verification. The
// reason string goes into the Reason header (RFC 3326) so packet
// captures show why we rejected without leaking cert chain details.
func (s *SIPServer) stirReject(msg *stack.Message, callID string, v stir.Verdict) *stack.Message {
	code := v.Code.SIPResponseCode()
	phrase := stirReasonPhrase(v.Code)
	resp := s.makeResponse(msg, code, phrase, "", "")
	if resp != nil {
		// RFC 3326 Reason header — opaque to most softswitches but
		// useful for our own debugging / CDR aggregation.
		resp.SetHeader("Reason",
			"SIP;cause="+itoa(code)+";text=\"stir/"+v.Code.String()+"\"")
	}
	logger.Warn("sip inbound STIR rejected",
		zap.String("call_id", callID),
		zap.String("verdict", v.Code.String()),
		zap.Int("sip_code", code),
		zap.String("reason", v.Reason))
	return resp
}

func stirReasonPhrase(code stir.VerdictCode) string {
	switch code.SIPResponseCode() {
	case 437:
		return "Unsupported Credential"
	case 438:
		return "Invalid Identity Header"
	case 403:
		return "Stale Date"
	}
	return "Identity Header Failed"
}

// extractFromTN pulls the user-part of the From header URI, runs it
// through E.164 canonicalisation, and returns "" when the user-part
// isn't a phone number (URI-style From). Robust against display-
// name + angle-bracket + tag-param variations.
func extractFromTN(msg *stack.Message) string {
	from := msg.GetHeader("From")
	uri := extractURIFromFromHeader(from)
	user := userPartOfSIPURI(uri)
	return canonicalE164(user)
}

// extractFromURI returns the cleaned SIP URI from the From header
// (with angle brackets / parameters stripped) for URI-style PASSporT
// origin comparison.
func extractFromURI(msg *stack.Message) string {
	from := msg.GetHeader("From")
	return extractURIFromFromHeader(from)
}

// extractURIFromFromHeader handles all common renderings:
//
//	"Alice" <sip:alice@example.com>;tag=abc
//	<sip:+15551234567@example.com>;tag=abc
//	sip:alice@example.com;tag=abc
func extractURIFromFromHeader(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '<'); i >= 0 {
		if j := strings.IndexByte(s[i+1:], '>'); j >= 0 {
			return strings.TrimSpace(s[i+1 : i+1+j])
		}
	}
	if i := strings.IndexByte(s, ';'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// userPartOfSIPURI extracts "user" from "sip:user@host". Returns ""
// when no user-part is present.
func userPartOfSIPURI(uri string) string {
	u := strings.TrimSpace(uri)
	if i := strings.IndexByte(u, ':'); i >= 0 {
		u = u[i+1:]
	}
	if at := strings.IndexByte(u, '@'); at > 0 {
		return u[:at]
	}
	return ""
}

// canonicalE164 mirrors the same helper in pkg/sip/outbound/stir.go.
// Kept package-local rather than exported because the two packages
// have subtly different needs (server is more lenient about leading-
// + canonicalisation than the signer).
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
	if !strings.HasPrefix(out, "+") || len(out) < 4 {
		return ""
	}
	return out
}

// itoa avoids strconv import in this file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
