// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package server

// dtls.go: inbound DTLS-SRTP (RFC 5763 + RFC 5764) — server side.
//
// Lifecycle:
//
//   1. handleInvite parses the offer. If proto is UDP/TLS/RTP/SAVP[F]
//      AND there's at least one a=fingerprint AND a=setup is
//      active|actpass, prepareDTLSAnswer mints a per-call ECDSA cert
//      and returns the SDP attribute lines for the answer.
//
//   2. The pending state (cert, key, peer fingerprints, our chosen
//      role = passive) is stashed on SIPServer keyed by Call-ID.
//
//   3. handleAck consumes that state and kicks off StartDTLS on the
//      RTP session — on a goroutine, with a hard timeout, so the
//      signaling thread isn't blocked. After handshake:
//
//        a. Verify peer cert SHA-256 matches one of the advertised
//           fingerprints (RFC 5763 §3 — without this the cert is
//           uncommitted and a passive MITM owns the call).
//
//        b. Derive SRTP contexts via Endpoint.SRTPContexts and
//           install them on the Session via EnableDTLSSRTP.
//
//      On failure, log + tear down: the call is half-established
//      (ACK already sent) so we BYE it.
//
// Outbound DTLS-SRTP offer support (we initiate to a WebRTC peer
// from the customer leg) is a separate slice — see TODO_OUTBOUND
// at the bottom.

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
	"go.uber.org/zap"
)

// dtlsHandshakeTimeout caps how long we'll wait for the client to
// complete the DTLS handshake. RFC 6347 default flight timer is 1s
// with exponential backoff, so 8s lets a fully retried handshake
// complete; longer is just dead-call holding-pattern.
const dtlsHandshakeTimeout = 8 * time.Second

// dtlsPendingState is the per-Call-ID state that survives between
// handleInvite (where we negotiated DTLS in SDP) and handleAck
// (where we actually run the handshake). All fields are immutable
// once handleInvite returns; the goroutine reads them concurrently
// with the next request on this server, hence keeping them on a
// distinct allocation rather than mutating the CallSession.
type dtlsPendingState struct {
	// CertDER is our DER-encoded cert presented in the handshake.
	// The SDP a=fingerprint we sent to the peer is SHA-256 of this.
	CertDER []byte
	// Key is the matching private key for CertDER.
	Key *ecdsa.PrivateKey
	// PeerFingerprints are EVERY a=fingerprint the peer offered.
	// Acceptance: at least one must match the peer's actual cert
	// after handshake (RFC 5763 §3). We accept multiple because
	// some peers list both an EC and an RSA fingerprint.
	PeerFingerprints []sdp.Fingerprint
	// AsServer is true for inbound — we always answer with
	// a=setup:passive so we wait for ClientHello.
	AsServer bool
}

// SetInboundDTLSAccept toggles inbound DTLS-SRTP. Default off.
// Safe to call before or after Start.
func (s *SIPServer) SetInboundDTLSAccept(enabled bool) {
	if s == nil {
		return
	}
	s.dtlsAcceptInbound.Store(enabled)
}

// dtlsAnswerResult holds the rendered SDP attributes + pending
// state to install on the server. Returned from prepareDTLSAnswer.
type dtlsAnswerResult struct {
	// ExtraLines are the `a=fingerprint:` and `a=setup:` SDP lines
	// to append to the 200 OK SDP via GenerateWithProtoExtras.
	ExtraLines []string
	// Pending is the state to stash for handleAck.
	Pending *dtlsPendingState
}

// prepareDTLSAnswer evaluates whether the offer is a valid
// DTLS-SRTP offer this server can answer. Returns nil when:
//
//   - inbound DTLS isn't enabled (SetInboundDTLSAccept(false));
//   - the proto isn't UDP/TLS/RTP/SAVP[F] (e.g. plain RTP/SAVP →
//     SDES path takes over);
//   - the peer didn't send any a=fingerprint (RFC 5763 §3 forbids
//     us answering DTLS-SRTP without it);
//   - a=setup is missing or holdconn (we have no policy for
//     half-renegotiation right now).
//
// Returns a non-nil error only on internal failure (e.g. cert
// minting failure) — all "this offer isn't DTLS" cases return
// (nil, nil) so the caller can fall through to other offer paths.
func (s *SIPServer) prepareDTLSAnswer(offer *sdp.Info) (*dtlsAnswerResult, error) {
	if s == nil || offer == nil {
		return nil, nil
	}
	if !s.dtlsAcceptInbound.Load() {
		return nil, nil
	}
	if !sdp.IsDTLSTransport(offer.Proto) {
		return nil, nil
	}
	if len(offer.Fingerprints) == 0 {
		// RFC 5763 §3: an offer using UDP/TLS/RTP/SAVP without a
		// fingerprint is malformed. Don't accept it as DTLS-SRTP;
		// caller will return 488 since we have no SDES path for
		// this proto.
		return nil, errors.New("dtls offer missing a=fingerprint")
	}
	answerRole := sdp.AnswerRole(offer.DTLSRole)
	if answerRole == sdp.DTLSRoleHoldConn || !answerRole.IsValid() {
		return nil, fmt.Errorf("dtls offer setup role %q not answerable", offer.DTLSRole)
	}

	certDER, key, err := rtp.SelfSignedDTLSCert(time.Time{})
	if err != nil {
		return nil, fmt.Errorf("dtls cert mint: %w", err)
	}
	ourFP := sdp.Fingerprint{
		HashFunc: "sha-256",
		Hex:      rtp.FingerprintSHA256(certDER),
	}
	fpLine := sdp.FormatFingerprintLine(ourFP)
	setupLine := sdp.FormatSetupLine(answerRole)
	if fpLine == "" || setupLine == "" {
		return nil, errors.New("dtls answer line render failed")
	}
	return &dtlsAnswerResult{
		ExtraLines: []string{fpLine, setupLine},
		Pending: &dtlsPendingState{
			CertDER:          certDER,
			Key:              key,
			PeerFingerprints: append([]sdp.Fingerprint(nil), offer.Fingerprints...),
			AsServer:         answerRole == sdp.DTLSRolePassive,
		},
	}, nil
}

// stashPendingDTLS associates state with a Call-ID so handleAck can
// pick it up. Concurrent INVITEs on the same Call-ID overwrite —
// but only one should ever be in flight per dialog.
func (s *SIPServer) stashPendingDTLS(callID string, p *dtlsPendingState) {
	if s == nil || p == nil || callID == "" {
		return
	}
	s.pendingDTLSMu.Lock()
	if s.pendingDTLS == nil {
		s.pendingDTLS = make(map[string]*dtlsPendingState)
	}
	s.pendingDTLS[callID] = p
	s.pendingDTLSMu.Unlock()
}

// takePendingDTLS atomically reads + clears state. Called from
// handleAck. Returns nil when nothing's pending (the call wasn't
// negotiated for DTLS).
func (s *SIPServer) takePendingDTLS(callID string) *dtlsPendingState {
	if s == nil || callID == "" {
		return nil
	}
	s.pendingDTLSMu.Lock()
	defer s.pendingDTLSMu.Unlock()
	p := s.pendingDTLS[callID]
	if p != nil {
		delete(s.pendingDTLS, callID)
	}
	return p
}

// runInboundDTLSHandshake drives the post-ACK handshake. Runs on
// its own goroutine — callers do not wait. On any failure the call
// is logged + BYE'd via SIPServer.HangupInboundCall so we don't
// leave a zombie dialog whose media never starts.
//
// Concurrency: pre-ACK media isn't supposed to flow on a DTLS-SRTP
// call (peer waits for handshake before sending SRTP), so racing
// with rtpSess reads from a CallSession that hasn't started media
// yet is fine. The Session's ReceiveRTP demux puts DTLS bytes onto
// the route; SRTP install only happens once the handshake is done.
func (s *SIPServer) runInboundDTLSHandshake(callID string, rtpSess *rtp.Session, p *dtlsPendingState) {
	if s == nil || rtpSess == nil || p == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), dtlsHandshakeTimeout)
	defer cancel()
	resCh := rtpSess.StartDTLS(ctx, p.AsServer, p.CertDER, p.Key, nil)
	res := <-resCh
	if res.Err != nil {
		logger.Warn("sip inbound dtls handshake failed",
			zap.String("call_id", callID),
			zap.Error(res.Err))
		s.HangupInboundCall(callID)
		return
	}
	defer res.Endpoint.Close()
	if err := verifyPeerFingerprint(res.Endpoint, p.PeerFingerprints); err != nil {
		logger.Warn("sip inbound dtls fingerprint mismatch (RFC 5763 §3)",
			zap.String("call_id", callID),
			zap.Error(err))
		s.HangupInboundCall(callID)
		return
	}
	rx, tx, err := res.Endpoint.SRTPContexts(res.Keys)
	if err != nil {
		logger.Warn("sip inbound dtls srtp context build failed",
			zap.String("call_id", callID),
			zap.Error(err))
		s.HangupInboundCall(callID)
		return
	}
	if err := rtpSess.EnableDTLSSRTP(rx, tx); err != nil {
		logger.Warn("sip inbound dtls srtp install failed",
			zap.String("call_id", callID),
			zap.Error(err))
		s.HangupInboundCall(callID)
		return
	}
	logger.Info("sip inbound dtls-srtp established",
		zap.String("call_id", callID),
		zap.String("profile", string(res.Keys.Profile)))
}

// verifyPeerFingerprint enforces the RFC 5763 §3 binding between
// SDP a=fingerprint and the cert presented during handshake. Thin
// wrapper over sdp.VerifyDTLSCertFingerprint that pulls the leaf
// cert off the pion endpoint.
func verifyPeerFingerprint(ep *rtp.DTLSEndpoint, advertised []sdp.Fingerprint) error {
	if ep == nil {
		return errors.New("dtls: nil endpoint")
	}
	certs := ep.PeerCertificates()
	if len(certs) == 0 {
		return errors.New("dtls: peer presented no certificate")
	}
	return sdp.VerifyDTLSCertFingerprint(certs[0], advertised)
}
