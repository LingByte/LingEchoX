// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package server

// VoiceServer-style decoupled handler interfaces.
//
// Ported from VoiceServer/pkg/sip/server/interfaces.go (the Config type
// portion is intentionally omitted — LingEchoX's existing Config in
// sip_server.go remains authoritative).
//
// Business code in pkg/sip/conversation, pkg/sip/voicedialog, etc. may
// register implementations through SIPServer.Set{Invite,DTMFSink,
// Transfer,CallLifecycle}Handler. The server's existing direct calls
// into those packages remain in place; the new interfaces are an
// additive opt-in path that lets non-LEX consumers (or future refactors)
// drive the SIP server without importing the LEX business tree.

import (
	"context"
	"net"

	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
	"github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

// IncomingCall carries the inbound INVITE for InviteHandler.OnIncomingCall.
type IncomingCall struct {
	CallID              string
	FromURI             string
	ToURI               string
	RemoteSignalingAddr *net.UDPAddr
	SDP                 *sdp.Info
	RawMessage          *stack.Message

	// RTPSession is the server-allocated RTP socket whose port has already
	// been (or will be) advertised in the 200 OK SDP answer. Business
	// handlers MUST pass this same session to session.NewMediaLeg.
	RTPSession *rtp.Session
}

// Decision tells the SIP server how to answer an INVITE.
type Decision struct {
	Accept       bool
	StatusCode   int
	ReasonPhrase string

	// MediaLeg is the fully-configured audio leg for this call. Required
	// when Accept=true.
	MediaLeg *session.MediaLeg

	// OnTerminate is invoked once when the call is torn down. Optional.
	OnTerminate func(reason string)
}

// InviteHandler is the primary business-layer entry point for the SIP
// server. Register via SIPServer.SetInviteHandler.
type InviteHandler interface {
	OnIncomingCall(ctx context.Context, inv *IncomingCall) (Decision, error)
}

// DTMFSink receives DTMF events from both SIP INFO bodies and
// RFC 2833 telephone-event RTP payloads.
type DTMFSink interface {
	OnDTMF(callID string, digit string, end bool)
}

// TransferHandler handles SIP REFER requests (call transfer).
//
// notify is a callback to send NOTIFY progress back to the caller (frag
// is the sipfrag body, subState is the Subscription-State header value
// like "active;expires=60" or "terminated;reason=noresource").
type TransferHandler interface {
	OnRefer(ctx context.Context, callID, referTo string, notify func(frag, subState string))
}

// CallLifecycleObserver observes call teardown in a cross-cutting way.
//
// OnCallPreHangup lets the observer claim the hangup: return true to
// signal "I've already torn down everything for this call, don't send
// BYE". Useful for WebSeat / transfer bridges whose teardown is owned
// elsewhere.
type CallLifecycleObserver interface {
	OnCallPreHangup(callID string) (handled bool)
	OnCallCleanup(callID string)
}
