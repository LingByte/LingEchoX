package voicedialog

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/LingByte/SoulNexus/pkg/media"
	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
	siptts "github.com/LingByte/SoulNexus/pkg/voice/tts"
	"github.com/gorilla/websocket"
)

// --- Wire protocol: JSON field keys (gateway ↔ WebSocket client) ---

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

const (
	ProtocolLingechoVoiceDialog = "lingecho-voice-dialog"
	ProtocolVersion             = 2
)

// Upstream event types (gateway → client).
const (
	EvHello          = "hello"
	EvCallPending    = "call.pending"
	EvASRPartial     = "asr.partial"
	EvASRFinal       = "asr.final"
	EvASRError       = "asr.error"
	EvDTMF           = "dtmf"
	EvInterrupt      = "interrupt"
	EvTTSStarted     = "tts.started"
	EvTTSEnded       = "tts.ended"
	EvTTSCancelled   = "tts.cancelled"
	EvCallEnded      = "call.ended"
	EvPong           = "pong"
	EvError          = "error"
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

// Downstream command types (client → gateway).
const (
	CmdHangup    = "hangup"
	CmdTTSSpeak  = "tts.speak"
	CmdTTSCancel = "tts.cancel"
	CmdInterrupt = "interrupt"
	CmdPing      = "ping"
)

const (
	OriginGateway = "gateway"
	OriginClient  = "client"
)

const (
	CauseBargeIn       = "barge_in"
	CauseApplied       = "applied"
	CauseClientRequest = "client_request"
)

// WebSocket hijack buffer sizes (gorilla/websocket).
const (
	WSReadBufferSize  = 1024 * 1024
	WSWriteBufferSize = 1024 * 1024
)

// Config wires SIP teardown from the HTTP/dialog side and optional in-process loopback.
type Config struct {
	HangupInbound func(callID string)

	// InboundLoopbackWS: on inbound attach, dial this process's HTTP voice-dialog WebSocket with ?call_id=
	// so emitGateway has a peer without an external browser (sipapp sets true).
	InboundLoopbackWS             bool
	LoopbackUseTLS                bool
	LoopbackTLSInsecureSkipVerify bool
	LoopbackHTTPHostPort          string // e.g. 127.0.0.1:8080
	APIPrefix                     string // e.g. /api
}

// InboundMeta is SIP snapshot metadata for dialog clients (From/To as received on INVITE).
type InboundMeta struct {
	CallID        string
	FromHeader    string
	ToHeader      string
	RemoteSig     string
	CodecName     string
	PCMSampleRate int
}

// Hub coordinates pending subscribers and per-call media bridges.
type Hub struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]*dialogSession

	subMu sync.Mutex
	subs  []*websocket.Conn

	wsUpgrader websocket.Upgrader

	tokenMissingOnce sync.Once
}

// dialogSession is one inbound call bridged to at most one WebSocket client.
type dialogSession struct {
	h *Hub

	meta InboundMeta
	cs   *sipSession.CallSession

	mu         sync.Mutex
	conn       *websocket.Conn
	clientSeen bool

	gatewayMu     sync.Mutex
	ttsSpeakMu    sync.Mutex
	ttsPipe       *siptts.Pipeline
	ttsPlayingPtr *atomic.Bool
	ttsStartedNS  *atomic.Int64
	mediaSession  *media.MediaSession

	transferLoadingMu     sync.Mutex
	transferLoadingCancel context.CancelFunc

	// Last ASR final transcript for correlating with the next tts.speak (persisted as SIPCall turns).
	dialogTurnMu       sync.Mutex
	lastASRFinal       string
	voicedialogASRProv string

	// Loopback LLM metadata consumed on the next gateway TTS playback.
	pendingTurnMu    sync.Mutex
	pendingLLMModel  string
	pendingLLMWallMs int

	// Defer TriggerTransferToAgent until handleTTSSpeak finishes (avoid ringback over assistant audio).
	transferAfterNextTTS atomic.Bool
}
