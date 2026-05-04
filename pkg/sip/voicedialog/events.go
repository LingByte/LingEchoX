package voicedialog

import (
	"strings"
	"time"
)

// JSON field keys (wire protocol).
const (
	KeyType               = "type"
	KeyCallID             = "call_id"
	KeyTS                 = "ts"
	KeyText               = "text"
	KeyFinal              = "final"
	KeyMessage            = "message"
	KeyFatal              = "fatal"
	KeyDigit              = "digit"
	KeyReason             = "reason"
	KeyOrigin             = "origin"
	KeyCause              = "cause"
	KeyUtteranceID        = "utterance_id"
	KeyTextPreview        = "text_preview"
	KeyOK                 = "ok"
	KeyProtocol           = "protocol"
	KeyV                  = "v"
	KeyFrom               = "from"
	KeyTo                 = "to"
	KeyCodec              = "codec"
	KeyPCMSampleRateHz    = "pcm_sample_rate_hz"
	KeyRemoteSig          = "remote_sig"
	KeyUpstreamEvents     = "upstream_events"
	KeyDownstreamCommands = "downstream_commands"
	KeyRole               = "role"
	KeyPCM                = "pcm"
	KeyErr                = "error"
	KeyPhase              = "phase"
	KeySourceKind         = "source_kind"
	KeySource             = "source"
	KeyDetail             = "detail"
	KeyRoute              = "route"
)

// SourceKind values for dialog.welcome / dialog.transfer payloads (playback hint for clients).
const (
	SourceKindURL    = "url"
	SourceKindScript = "script"
)

// Protocol ID sent in hello.
const ProtocolLingechoVoiceDialog = "lingecho-voice-dialog"

// ProtocolVersion is the hello / capability negotiation version.
const ProtocolVersion = 2

// --- Upstream: gateway → WebSocket client ---

const (
	EvHello        = "hello"
	EvCallPending  = "call.pending"
	EvASRPartial   = "asr.partial"
	EvASRFinal     = "asr.final"
	EvASRError     = "asr.error"
	EvDTMF         = "dtmf"
	EvInterrupt    = "interrupt"
	EvTTSStarted   = "tts.started"
	EvTTSEnded     = "tts.ended"
	EvTTSCancelled = "tts.cancelled"
	EvCallEnded    = "call.ended"
	EvPong         = "pong"
	EvError        = "error"
	EvDialogWelcome  = "dialog.welcome"
	EvDialogTransfer = "dialog.transfer"
)

// Dialog welcome phases (dialog.welcome.phase).
const (
	PhaseWelcomeStarted = "started"
	PhaseWelcomePlaying = "playing"
	PhaseWelcomeEnded   = "ended"
	PhaseWelcomeSkipped = "skipped"
	PhaseWelcomeError   = "error"
)

// Dialog transfer phases (dialog.transfer.phase).
const (
	PhaseTransferRequested = "requested"
	PhaseTransferLoading   = "loading"
	PhaseTransferRinging   = "ringing"
	PhaseTransferConnected = "connected"
	PhaseTransferFailed    = "failed"
	PhaseTransferNoAgent   = "no_agent"
)

// --- Downstream: WebSocket client → gateway ---

const (
	CmdHangup    = "hangup"
	CmdTTSSpeak  = "tts.speak"
	CmdTTSCancel = "tts.cancel"
	CmdInterrupt = "interrupt"
	CmdPing      = "ping"
)

// Interrupt / correlation: Origin + Cause on wire.
const (
	OriginGateway = "gateway"
	OriginClient  = "client"
)

const (
	CauseBargeIn       = "barge_in"
	CauseApplied       = "applied"
	CauseClientRequest = "client_request"
)

// tsRFC3339Nano returns UTC timestamp for JSON payloads.
func tsRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// event builds a normalized outbound JSON map with type, call_id, ts.
func event(typ, callID string, extra map[string]any) map[string]any {
	m := map[string]any{
		KeyType: typ,
		KeyTS:   tsRFC3339Nano(),
	}
	if strings.TrimSpace(callID) != "" {
		m[KeyCallID] = callID
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func errorWire(message string) map[string]any {
	return map[string]any{
		KeyType:   EvError,
		KeyMessage: message,
		KeyTS:     tsRFC3339Nano(),
	}
}
