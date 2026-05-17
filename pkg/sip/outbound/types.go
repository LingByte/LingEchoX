package outbound

import (
	"context"
	"time"

	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
)

// Scenario classifies why an outbound leg exists. Extensible without changing core SIP types.
type Scenario string

const (
	// ScenarioCampaign is a proactive outbound call (manual trigger or job queue) with an optional script.
	ScenarioCampaign Scenario = "campaign"
	// ScenarioTransferAgent is the agent leg after inbound user requests human support.
	ScenarioTransferAgent Scenario = "transfer_agent"
	// ScenarioCallback is a scheduled return call (same runtime as campaign, distinct for analytics).
	ScenarioCallback Scenario = "callback"
)

// DialTarget is a minimal description of where to send INVITE.
type DialTarget struct {
	// WebSeat is true when routing selects a WebSeat pool target (browser agent),
	// not a SIP INVITE to user "web".
	WebSeat bool
	// RequestURI is the SIP request URI, e.g. sip:+8613800138000@carrier.example;user=phone
	RequestURI string
	// SignalingAddr is the UDP address of the next SIP hop (proxy or UAS).
	SignalingAddr string // host:port
	// Optional From/Contact user part (e.g. ACD pool sipCallerId). Empty → Dial uses Manager/env default.
	CallerUser string
	// Optional quoted display-name in From when CallerUser is set (or alone if DialRequest sets caller).
	CallerDisplayName string

	// ACDPoolTargetID is set when the target comes from acd_pool_targets (transfer routing); used for retries / bookkeeping.
	ACDPoolTargetID uint `json:"-"`

	// Transport is the trunk-configured signaling transport for this
	// target (UDP / TCP / TLS). Empty (TransportUnset) means "use
	// default precedence": Request-URI's ;transport= wins, else this
	// field, else TransportUDP. See ResolveTransport.
	//
	// Wire-up status (2026-05-17): the routing layer can populate
	// this; outbound dialing logic (manager.go) currently still
	// hardcodes UDP. Slice 2 of batch 2A-out will switch the
	// connection layer over.
	Transport Transport
}

// DialRequest is one outbound attempt.
type DialRequest struct {
	Scenario Scenario
	Target   DialTarget

	// ScriptID optional reference for campaign runner (DB/job id).
	ScriptID string

	// CorrelationID ties this leg to CRM, inbound Call-ID, etc.
	CorrelationID string

	// MediaProfile selects which media/AI hooks run after connect (see MediaProfile).
	MediaProfile MediaProfile

	// CallerUser / CallerDisplayName override Target.* and Manager defaults when CallerUser non-empty.
	CallerUser        string
	CallerDisplayName string

	// DialTenantID scopes per-tenant trunk-number outbound concurrency (campaign worker sets this).
	DialTenantID uint `json:"-"`

	// AssertedIdentityURI (RFC 3325) is the carrier-verified caller URI
	// that the platform is authorised to assert on this outbound leg.
	// Typical value: the trunk_number that owns this outbound channel
	// expressed as a sip URI ("sip:+8613800138000@<trunk-host>"), OR a
	// tel: URI ("tel:+8613800138000"). Leave empty to omit the PAI
	// header entirely; the From header still carries the displayed CLI.
	//
	// IMPORTANT: never derive this automatically from user input — the
	// caller (router / handler) must have a trust path that justifies
	// asserting this identity, otherwise we're leaking unverifiable
	// claims as if they were operator-validated.
	AssertedIdentityURI         string
	AssertedIdentityDisplayName string
	// PrivacyTokens are RFC 3323 Privacy header tokens applied to this
	// leg. The common case is []string{"id"} for a "withheld-CLI" call
	// (PSTN displays "anonymous" while the trust domain still sees the
	// PAI for routing / abuse-tracing). Nil/empty omits the header.
	PrivacyTokens []string

	// HistoryInfo (RFC 7044) — see inviteParams.HistoryInfo.
	// Conversation / transfer code should build this via
	// historyinfo.AppendTransferEntry to chain on any inbound history.
	HistoryInfo []historyinfo.Entry
	// Diversion (RFC 5806) — see inviteParams.Diversion.
	Diversion []historyinfo.Diversion

	// OfferDTLSSRTP enables outbound DTLS-SRTP (RFC 5763 + RFC 5764)
	// instead of SDES. When true:
	//
	//   * INVITE body uses `m=audio … UDP/TLS/RTP/SAVP`
	//   * `a=fingerprint:sha-256 …` + `a=setup:actpass` extras
	//   * post-2xx the manager runs the DTLS handshake on the RTP
	//     socket and derives SRTP keys (RFC 5764 §4.2)
	//
	// Cannot be combined with MediaProfileTransferBridge (whose
	// downstream peers reject SAVP+ entirely). Carrier trunks
	// typically reject this with 488; flip it on only for WebRTC
	// gateway or known DTLS-capable endpoints. Default false keeps
	// the existing SDES offer path intact.
	OfferDTLSSRTP bool
}

// MediaProfile selects post-connect behavior on the established CallSession.
type MediaProfile string

const (
	// MediaProfileAI attaches the same ASR→LLM→TTS pipeline as inbound (env-driven).
	MediaProfileAI MediaProfile = "ai_voice"
	// MediaProfileScript runs a scripted IVR-style flow (prompts, DTMF) — orchestration TBD.
	MediaProfileScript MediaProfile = "script"
	// MediaProfileTransferBridge hands RTP to StartTransferBridge after ACK (raw G.711 relay or PCM transcode).
	MediaProfileTransferBridge MediaProfile = "transfer_bridge"
	// MediaProfileNone only brings RTP up (testing or custom hooks via callback).
	MediaProfileNone MediaProfile = "none"
)

// EstablishedLeg is passed to script/transfer hooks after 200 OK + ACK.
type EstablishedLeg struct {
	CallID        string
	Scenario      Scenario
	CorrelationID string
	Session       *sipSession.CallSession
	CreatedAt     time.Time

	// SIP headers / signaling (for analytics / DB persistence on outbound legs).
	FromHeader          string
	ToHeader            string
	RemoteSignalingAddr string
	CSeqInvite          string
}

// MediaAttachFunc wires ASR/LLM/TTS or other processors after RTP is live.
// Typically set to conversation.AttachVoicePipeline for MediaProfileAI.
type MediaAttachFunc func(ctx context.Context, cs *sipSession.CallSession) error

const (
	DialEventInvited     = "invited"
	DialEventProvisional = "provisional"
	DialEventEstablished = "established"
	DialEventFailed      = "failed"
)

// DialEvent streams lightweight dial lifecycle transitions for queue/observability.
type DialEvent struct {
	CallID        string
	CorrelationID string
	Scenario      Scenario
	MediaProfile  MediaProfile
	State         string
	StatusCode    int
	Reason        string
	At            time.Time

	// RequestURI is the INVITE Request-URI (set on invite + dialog events when known).
	RequestURI string
	// StatusText is the SIP reason phrase (e.g. "Trying", "Ringing").
	StatusText string
	// RemoteAddr is the UDP signaling peer: INVITE destination for "invited";
	// source address of SIP responses for provisional / failure / established.
	RemoteAddr string
}
