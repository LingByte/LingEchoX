// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

// dtls.go: outbound DTLS-SRTP (RFC 5763 + RFC 5764) — UAC side.
//
// Lifecycle:
//
//   1. Dial mints a per-call ECDSA cert + computes its SHA-256
//      fingerprint when DialRequest.OfferDTLSSRTP is true. The
//      INVITE body uses `m=audio … UDP/TLS/RTP/SAVP` plus the
//      fingerprint and `a=setup:actpass`.
//
//   2. The cert / key / our role are stashed on outLeg so the 2xx
//      handler can:
//
//        a. Parse the answer's `a=fingerprint` + `a=setup`. The
//           peer's `a=setup` decides our DTLS role (actpass means
//           we picked passive ourselves; active forces us passive;
//           passive forces us active).
//
//        b. Kick off rtpSess.StartDTLS on a goroutine with a hard
//           timeout. After handshake, verify peer cert against the
//           advertised fingerprint (RFC 5763 §3) and install SRTP
//           contexts via EnableDTLSSRTP.
//
//      On failure the leg is BYE'd via cleanupLeg.
//
// Notes:
//
//   * outbound DTLS coexists with SDES on the same Manager — when
//     OfferDTLSSRTP is false we keep the existing RTP/SAVPF + SDES
//     offer path.
//   * The signaling thread never blocks on the handshake — it runs
//     in startOutboundDTLSHandshake's goroutine.

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
	"go.uber.org/zap"
)

// outboundDTLSHandshakeTimeout caps how long we'll wait for the
// peer to complete the DTLS handshake after our INVITE got 2xx.
// Same rationale as the inbound side (RFC 6347 + retries).
const outboundDTLSHandshakeTimeout = 8 * time.Second

// outboundDTLSPending is the per-leg state carried from Dial (where
// we wrote the SDP offer) to handleResponse (where we run the
// handshake). All fields are immutable post-Dial.
type outboundDTLSPending struct {
	CertDER []byte
	Key     *ecdsa.PrivateKey
	// OfferedRole is whatever we put in a=setup of the OFFER —
	// "actpass" for the default outbound case.
	OfferedRole sdp.DTLSRole
}

// prepareOutboundDTLSOffer mints the cert + builds the SDP extras
// for an outbound DTLS-SRTP offer. Returns:
//
//   - mediaProto: "UDP/TLS/RTP/SAVP" (we don't offer SAVPF without
//     RTCP-FB support, which we'd need to advertise separately)
//   - extras: the a=fingerprint + a=setup lines to append in SDP
//   - pending: state to stash on outLeg for the 2xx handler
//
// Returns a non-nil error only on internal failure (cert mint /
// render). Callers should fall back to SDES on such errors.
func prepareOutboundDTLSOffer() (mediaProto string, extras []string, pending *outboundDTLSPending, err error) {
	certDER, key, err := rtp.SelfSignedDTLSCert(time.Time{})
	if err != nil {
		return "", nil, nil, fmt.Errorf("dtls cert: %w", err)
	}
	fp := sdp.Fingerprint{
		HashFunc: "sha-256",
		Hex:      rtp.FingerprintSHA256(certDER),
	}
	fpLine := sdp.FormatFingerprintLine(fp)
	// "actpass" lets the peer pick — most carriers / WebRTC
	// gateways prefer to be the DTLS server.
	setupLine := sdp.FormatSetupLine(sdp.DTLSRoleActPass)
	if fpLine == "" || setupLine == "" {
		return "", nil, nil, errors.New("dtls answer line render failed")
	}
	return "UDP/TLS/RTP/SAVP",
		[]string{fpLine, setupLine},
		&outboundDTLSPending{
			CertDER:     certDER,
			Key:         key,
			OfferedRole: sdp.DTLSRoleActPass,
		},
		nil
}

// resolveOurRoleFromAnswer interprets the peer's a=setup against
// our offer to decide who drives the handshake. Symmetric to
// sdp.AnswerRole but viewed from the offerer's seat:
//
//   - We offered actpass; peer chose passive → we're DTLS client (active)
//   - We offered actpass; peer chose active  → we're DTLS server (passive)
//   - We offered actpass; peer left actpass  → spec violation; we
//     default to active (safe — most peers will accept ClientHello).
//   - peer holdconn → no handshake (caller should treat as failure).
func resolveOurRoleFromAnswer(peerRole sdp.DTLSRole) (asServer bool, ok bool) {
	switch peerRole {
	case sdp.DTLSRolePassive:
		return false, true // peer waits → we initiate
	case sdp.DTLSRoleActive:
		return true, true // peer initiates → we wait
	case sdp.DTLSRoleActPass:
		// Peer should have picked but didn't — best-effort: be the
		// active side ourselves.
		return false, true
	}
	return false, false
}

// startOutboundDTLSHandshake drives the post-2xx handshake. Runs
// on its own goroutine — handleResponse continues without blocking.
// On any failure the leg is BYE'd via cleanupLeg.
func startOutboundDTLSHandshake(leg *outLeg, pending *outboundDTLSPending, answer *sdp.Info) {
	if leg == nil || leg.rtpSess == nil || pending == nil || answer == nil {
		return
	}
	if len(answer.Fingerprints) == 0 {
		logger.Warn("sip outbound dtls answer missing a=fingerprint (RFC 5763 §3)",
			zap.String("call_id", leg.params.CallID))
		leg.cleanupLeg()
		return
	}
	asServer, ok := resolveOurRoleFromAnswer(answer.DTLSRole)
	if !ok {
		logger.Warn("sip outbound dtls answer setup unanswerable",
			zap.String("call_id", leg.params.CallID),
			zap.String("peer_setup", string(answer.DTLSRole)))
		leg.cleanupLeg()
		return
	}
	peerFPs := append([]sdp.Fingerprint(nil), answer.Fingerprints...)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), outboundDTLSHandshakeTimeout)
		defer cancel()
		resCh := leg.rtpSess.StartDTLS(ctx, asServer, pending.CertDER, pending.Key, nil)
		res := <-resCh
		if res.Err != nil {
			logger.Warn("sip outbound dtls handshake failed",
				zap.String("call_id", leg.params.CallID),
				zap.Error(res.Err))
			sipMetrics.DTLSHandshake(sipMetrics.DTLSResultFail)
			leg.cleanupLeg()
			return
		}
		defer res.Endpoint.Close()
		certs := res.Endpoint.PeerCertificates()
		if len(certs) == 0 {
			logger.Warn("sip outbound dtls peer presented no cert",
				zap.String("call_id", leg.params.CallID))
			leg.cleanupLeg()
			return
		}
		if err := sdp.VerifyDTLSCertFingerprint(certs[0], peerFPs); err != nil {
			logger.Warn("sip outbound dtls fingerprint mismatch (RFC 5763 §3)",
				zap.String("call_id", leg.params.CallID),
				zap.Error(err))
			sipMetrics.DTLSHandshake(sipMetrics.DTLSResultFingerprintMismatch)
			leg.cleanupLeg()
			return
		}
		rx, tx, err := res.Endpoint.SRTPContexts(res.Keys)
		if err != nil {
			logger.Warn("sip outbound dtls srtp context build failed",
				zap.String("call_id", leg.params.CallID),
				zap.Error(err))
			leg.cleanupLeg()
			return
		}
		if err := leg.rtpSess.EnableDTLSSRTP(rx, tx); err != nil {
			logger.Warn("sip outbound dtls srtp install failed",
				zap.String("call_id", leg.params.CallID),
				zap.Error(err))
			leg.cleanupLeg()
			return
		}
		sipMetrics.DTLSHandshake(sipMetrics.DTLSResultOK)
		logger.Info("sip outbound dtls-srtp established",
			zap.String("call_id", leg.params.CallID),
			zap.String("profile", string(res.Keys.Profile)))
	}()
}
