package voicedialog

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/LingByte/SoulNexus/pkg/media"
	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
	"github.com/LingByte/SoulNexus/pkg/synthesizer"
	"github.com/gorilla/websocket"
)

// gatewayQcloudTTSStream is defined in gateway_media.go; declare here as opaque type
// reference for dialogSession.ttsService. Importing synthesizer keeps the dependency
// graph documented (the actual SDK is wrapped inside gatewayQcloudTTSStream).
var _ = (*synthesizer.QCloudService)(nil)

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
	// CustomVoiceWSURL optional ws:// or wss:// URL; gateway merges ?token=&call_id= like loopback.
	CustomVoiceWSURL string
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
	ttsPlayingPtr *atomic.Bool
	ttsStartedNS  *atomic.Int64
	mediaSession  *media.MediaSession

	// --- pipelined TTS player state (see tts_segmenter.go) ----------------
	// Sample rates captured at attachGatewayMedia time so the player goroutine does
	// not have to call back into env / config on every segment.
	ttsCloudSR  int
	ttsBridgeSR int
	// ttsService is the TTS service adapter (Tencent Cloud SDK wrapped). Owned by
	// attachGatewayMedia, consumed by per-segment prefetch goroutines.
	ttsService *gatewayQcloudTTSStream
	// ttsSegmentCh feeds the single player goroutine. Buffered; closed in stopTTSPlayer.
	ttsSegmentCh chan *ttsSegmentJob
	// Lifecycle guards for the player goroutine.
	ttsPlayerOnce     sync.Once
	ttsPlayerStopOnce sync.Once
	ttsPlayerWg       sync.WaitGroup
	// ttsCurrentCancel is set by the player at the start of each segment and reached
	// in by stopGatewayTTSPlayback / barge-in to preempt the in-flight segment.
	ttsCurrentMu     sync.Mutex
	ttsCurrentCancel context.CancelFunc

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

	// pendingTTSCount tracks tts.speak segments emitted but not yet finished playing.
	// Incremented by the loopback writer (beginPendingTTS) before sending each segment, and
	// decremented at the end of handleTTSSpeak. transferAfterNextTTS only fires when this hits 0.
	pendingTTSCount atomic.Int32

	// ttsGenSeq monotonically increments per tts.speak command received. ttsGenInvalidBefore
	// stores the high-water mark assigned at the moment of barge-in / cancel / interrupt;
	// any queued handleTTSSpeak goroutine whose gen <= invalidBefore is dropped before
	// touching the synthesizer (so a single barge-in clears the entire queued segment chain
	// rather than only the currently-playing segment).
	ttsGenSeq           atomic.Uint64
	ttsGenInvalidBefore atomic.Uint64

	// firstAudioHook is invoked once on the very next outbound PCM frame after handleTTSSpeak
	// arms it (used to log tts_first_audio_ms relative to the speak command arrival).
	firstAudioMu   sync.Mutex
	firstAudioHook func()
}
