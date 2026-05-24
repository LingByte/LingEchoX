// Package mediator hosts the CallMediator interface — the single
// point of contact between the SIP protocol layer (pkg/sip) and the
// business orchestration layer (pkg/contactcenter, pkg/dialog).
//
// Goals:
//
//   - sip/server holds ONE CallMediator field and emits events to it;
//     it never imports pkg/dialog or pkg/contactcenter directly.
//   - mediator's default implementation owns the wiring: it builds a
//     dialog Engine, attaches it on ACK, handles transfer / DTMF /
//     hangup, persists CDR.
//   - Tests can swap in a fake mediator to exercise sip/server in
//     isolation, and a fake SIP layer to exercise the mediator.
package mediator

import (
	"context"
	"time"

	"github.com/LinByte/VoiceServer/pkg/dialog/engine"
)

// CallMediator is the SIP-side observer. Methods are called by the
// SIP layer on protocol events and SHOULD return quickly (no blocking
// network I/O); long-running work runs in mediator-owned goroutines.
type CallMediator interface {
	// OnIncomingCall fires after INVITE is accepted (200 OK sent,
	// awaiting ACK). The mediator typically pre-loads tenant config
	// here. After ACK, OnAnswered is called.
	OnIncomingCall(IncomingCallEvent)

	// OnAnswered fires when the call reaches established media (ACK
	// processed). The mediator attaches the dialog Engine here.
	OnAnswered(AnsweredEvent)

	// OnDTMF fires for each DTMF digit decoded (RFC 2833 / SIP INFO).
	OnDTMF(DTMFEvent)

	// OnRefer fires when a REFER request asks the platform to retarget
	// the call (e.g. PBX-initiated blind transfer).
	OnRefer(ReferEvent)

	// OnHangup fires when either side terminates the call (PSTN BYE
	// or local BYE). Mediator runs Detach on the engine, persists
	// the final turn, and emits CDR.
	OnHangup(HangupEvent)

	// OnRecordingComplete fires after the post-call recording flush
	// finishes (or fails). Used to persist the recording metadata.
	OnRecordingComplete(RecordingEvent)
}

// IncomingCallEvent captures the data SIP knows at INVITE-accept time.
type IncomingCallEvent struct {
	CallID    string
	TenantID  string
	DID       string // dialed-in DID (To-URI user)
	Caller    string // calling party number (From-URI user)
	CallerDN  string // optional display name
	Transport string // "udp" / "tcp" / "tls"
	At        time.Time
}

// AnsweredEvent fires once media is ready.
type AnsweredEvent struct {
	CallID   string
	TenantID string

	// MediaPort is the engine's media abstraction over the live
	// CallSession. The mediator passes it to Engine.Attach.
	MediaPort engine.MediaPort

	At time.Time
}

// DTMFEvent describes one decoded DTMF digit.
type DTMFEvent struct {
	CallID string
	Digit  byte    // '0'-'9', '*', '#', 'A'-'D'
	Source string // "rfc2833" | "sip_info"
	At     time.Time
}

// ReferEvent describes an incoming REFER (blind transfer request).
type ReferEvent struct {
	CallID  string
	ReferTo string // Refer-To header value

	// NotifyTerminal is the sipfrag callback to invoke when the
	// retarget reaches its final state. mediator hands it off to
	// the transfer orchestrator so the NOTIFY reflects real state.
	NotifyTerminal func(sipfragLine, subscriptionState string)
}

// HangupEvent captures who hung up and why.
type HangupEvent struct {
	CallID     string
	Initiator  engine.Initiator // who hung up
	Reason     string           // RFC 3326 Reason header class or platform string
	StatusCode int              // SIP final status when triggered by failure
	At         time.Time
}

// RecordingEvent reports the result of the post-call recording flush.
type RecordingEvent struct {
	CallID    string
	WAVKey    string  // object store key
	SHA256Hex string  // recording integrity hash
	Duration  time.Duration
	Err       error
}

// Hangup is the reverse direction — mediator/business asking SIP to
// terminate a call. Provided as a separate interface to keep
// CallMediator a one-way contract (SIP → business). The SIP layer
// implements Hangup; cmd wires it to the mediator via constructor
// injection.
type Hangup interface {
	// HangupCall asks the SIP layer to BYE the given call. Idempotent.
	HangupCall(ctx context.Context, callID string, reason string) error
}

// NoopMediator is a safe default that drops all events. Used in
// tests and during early bootstrap before the real mediator is wired.
type NoopMediator struct{}

func (NoopMediator) OnIncomingCall(IncomingCallEvent) {}
func (NoopMediator) OnAnswered(AnsweredEvent)         {}
func (NoopMediator) OnDTMF(DTMFEvent)                 {}
func (NoopMediator) OnRefer(ReferEvent)               {}
func (NoopMediator) OnHangup(HangupEvent)             {}
func (NoopMediator) OnRecordingComplete(RecordingEvent) {}

// Compile-time interface check.
var _ CallMediator = NoopMediator{}
