package voicedialog

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/media"
	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
	siptts "github.com/LingByte/SoulNexus/pkg/voice/tts"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Config wires SIP teardown from the HTTP/dialog side.
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

type dialogSession struct {
	h *Hub

	meta InboundMeta
	cs   *sipSession.CallSession

	mu            sync.Mutex
	conn          *websocket.Conn
	clientSeen    bool

	gatewayMu     sync.Mutex
	ttsSpeakMu    sync.Mutex
	ttsPipe       *siptts.Pipeline
	ttsPlayingPtr *atomic.Bool
	ttsStartedNS  *atomic.Int64
	mediaSession  *media.MediaSession

	transferLoadingMu     sync.Mutex
	transferLoadingCancel context.CancelFunc
}

var defaultHub *Hub

// InitDefault configures the global hub (call once from process init).
func InitDefault(cfg Config) {
	defaultHub = &Hub{
		cfg:      cfg,
		sessions: make(map[string]*dialogSession),
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  WSReadBufferSize,
			WriteBufferSize: WSWriteBufferSize,
			CheckOrigin: func(*http.Request) bool {
				return true // same-origin policy relaxed; protect with VOICE_DIALOG_WS_TOKEN.
			},
		},
	}
	RegisterTransferPhaseNotifier()
}

// AttachInbound registers the voice-dialog gateway for this call. Call inside cs.AttachVoiceConversation.
func AttachInbound(_ context.Context, cs *sipSession.CallSession, meta InboundMeta) error {
	if defaultHub == nil {
		return errors.New("voicedialog: InitDefault not called")
	}
	if cs == nil || strings.TrimSpace(meta.CallID) == "" {
		return errors.New("voicedialog: invalid call session or call-id")
	}
	meta.CallID = strings.TrimSpace(meta.CallID)

	h := defaultHub
	sess := &dialogSession{
		h:    h,
		meta: meta,
		cs:   cs,
	}

	h.mu.Lock()
	if h.sessions[meta.CallID] != nil {
		h.mu.Unlock()
		return fmt.Errorf("voicedialog: call %q already has a dialog bridge", meta.CallID)
	}
	h.sessions[meta.CallID] = sess
	h.mu.Unlock()

	h.broadcastPending(sess)

	if cs.MediaSession() == nil {
		h.endCall(meta.CallID, "error")
		return errors.New("voicedialog: nil media session")
	}

	if err := attachGatewayMedia(sess); err != nil {
		h.endCall(meta.CallID, "gateway_attach_error")
		return err
	}

	h.startInboundLoopbackWS(sess)

	logger.Info("voicedialog inbound gateway registered (await WebSocket)",
		zap.String(KeyCallID, meta.CallID),
		zap.Int(KeyPCMSampleRateHz, meta.PCMSampleRate),
		zap.String(KeyCodec, meta.CodecName),
	)
	return nil
}

func (h *Hub) broadcastPending(sess *dialogSession) {
	if h == nil || sess == nil {
		return
	}
	msg := event(EvCallPending, sess.meta.CallID, map[string]any{
		KeyFrom:            sess.meta.FromHeader,
		KeyTo:              sess.meta.ToHeader,
		KeyRemoteSig:       sess.meta.RemoteSig,
		KeyCodec:           sess.meta.CodecName,
		KeyPCMSampleRateHz: sess.meta.PCMSampleRate,
		KeyV:               ProtocolVersion,
	})
	h.subMu.Lock()
	list := append([]*websocket.Conn(nil), h.subs...)
	h.subMu.Unlock()
	if len(list) == 0 {
		if h.cfg.InboundLoopbackWS {
			// Loopback dials .../ws?call_id= in-process; subscriber sockets (no call_id) are optional monitors.
			logger.Info("voicedialog → ws call.pending: no subscriber sockets (fanout skipped; using inbound loopback per-call ws)",
				zap.String(KeyCallID, sess.meta.CallID),
			)
		} else {
			logger.Warn("voicedialog → ws call.pending fanout: no subscribers (HTTP is up but no dialog WebSocket yet)",
				zap.String(KeyCallID, sess.meta.CallID),
				zap.Int("subscribers", 0),
				zap.String("subscriber_ws_path", constants.LingechoVoiceDialogPathPrefix+"/ws"),
				zap.String("hint", "connect subscriber WS without call_id to receive call.pending; then connect with call_id for ASR→LLM→tts.speak"),
			)
		}
	} else {
		logger.Info("voicedialog → ws call.pending fanout",
			zap.String(KeyCallID, sess.meta.CallID),
			zap.Int("subscribers", len(list)),
		)
	}
	for _, c := range list {
		_ = writeJSONDeadline(c, msg)
	}
}

func writeJSONDeadline(c *websocket.Conn, v any) error {
	if c == nil {
		return errors.New("nil conn")
	}
	_ = c.SetWriteDeadline(time.Now().Add(60 * time.Second))
	return c.WriteJSON(v)
}

// EndCall tears down the bridge for callID (no SIP BYE unless caller invokes HangupInbound).
func EndCall(callID string) {
	if defaultHub == nil {
		return
	}
	defaultHub.endCall(callID, "server")
}

func (h *Hub) endCall(callID, reason string) {
	if h == nil || strings.TrimSpace(callID) == "" {
		return
	}
	callID = strings.TrimSpace(callID)
	h.mu.Lock()
	sess := h.sessions[callID]
	delete(h.sessions, callID)
	h.mu.Unlock()
	if sess == nil {
		return
	}
	sess.gatewayShutdown()
	sess.mu.Lock()
	if sess.conn != nil {
		logger.Info("voicedialog → ws call.ended",
			zap.String(KeyCallID, callID),
			zap.String(KeyReason, reason),
		)
		_ = writeJSONDeadline(sess.conn, event(EvCallEnded, callID, map[string]any{
			KeyReason: reason,
		}))
		_ = sess.conn.Close()
		sess.conn = nil
	}
	sess.mu.Unlock()
	logger.Info("voicedialog session ended",
		zap.String(KeyCallID, callID),
		zap.String(KeyReason, reason),
	)
}

// IsPendingOrActive is true while a voice-dialog bridge exists for this inbound Call-ID (suppress duplicate AI attach).
func IsPendingOrActive(callID string) bool {
	if defaultHub == nil || strings.TrimSpace(callID) == "" {
		return false
	}
	defaultHub.mu.Lock()
	defer defaultHub.mu.Unlock()
	_, ok := defaultHub.sessions[strings.TrimSpace(callID)]
	return ok
}

func wsTokenOK(r *http.Request) bool {
	expected := wsTokenExpected()
	got := strings.TrimSpace(r.URL.Query().Get("token"))
	if expected == "" {
		if defaultHub != nil {
			defaultHub.tokenMissingOnce.Do(func() {
				logger.Warn("voicedialog VOICE_DIALOG_WS_TOKEN is empty; WebSocket accepts any client (set VOICE_DIALOG_WS_TOKEN in production)",
					zap.Bool("inbound_loopback_ws", defaultHub.cfg.InboundLoopbackWS),
				)
			})
		}
		return true
	}
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// WebSocketHTTP serves GET WebSocket (?token=… [&call_id=…]). Performs RFC 6455 upgrade via wsUpgrader.
func WebSocketHTTP(w http.ResponseWriter, r *http.Request) {
	remote := r.RemoteAddr
	if defaultHub == nil {
		logger.Warn("voicedialog ws: upgrade refused (hub not initialized)", zap.String("remote", remote))
		http.Error(w, "voice dialog not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		logger.Warn("voicedialog ws: wrong method", zap.String("remote", remote), zap.String("method", r.Method))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !wsTokenOK(r) {
		logger.Warn("voicedialog ws: upgrade refused (token)", zap.String("remote", remote))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callID := strings.TrimSpace(r.URL.Query().Get("call_id"))
	conn, err := defaultHub.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn("voicedialog ws: Upgrade failed", zap.String("remote", remote), zap.String(KeyCallID, callID), zap.Error(err))
		return
	}
	wsRemote := conn.RemoteAddr().String()
	if callID == "" {
		logger.Info("voicedialog ws: upgraded (subscriber)",
			zap.String("http_remote", remote),
			zap.String("ws_remote", wsRemote),
		)
		defaultHub.subMu.Lock()
		defaultHub.subs = append(defaultHub.subs, conn)
		defaultHub.subMu.Unlock()
		go defaultHub.subscribeReadLoop(conn)
		return
	}
	logger.Info("voicedialog ws: upgraded (call session)",
		zap.String("http_remote", remote),
		zap.String("ws_remote", wsRemote),
		zap.String(KeyCallID, callID),
	)
	go defaultHub.runCallSocket(callID, conn)
}

func (h *Hub) subRemove(c *websocket.Conn) {
	if h == nil || c == nil {
		return
	}
	h.subMu.Lock()
	defer h.subMu.Unlock()
	out := h.subs[:0]
	for _, x := range h.subs {
		if x != c {
			out = append(out, x)
		}
	}
	h.subs = out
}

func (h *Hub) subscribeReadLoop(c *websocket.Conn) {
	defer func() {
		_ = c.Close()
		h.subRemove(c)
	}()
	for {
		mt, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		preview := string(data)
		if len(preview) > 300 {
			preview = preview[:300] + "…"
		}
		logger.Info("voicedialog ← ws subscribe client message",
			zap.Int("mt", mt),
			zap.Int("bytes", len(data)),
			zap.String("preview", preview),
		)
	}
}

func (h *Hub) runCallSocket(callID string, conn *websocket.Conn) {
	defer func() { _ = conn.Close() }()

	h.mu.Lock()
	sess := h.sessions[callID]
	h.mu.Unlock()
	if sess == nil {
		_ = writeJSONDeadline(conn, errorWire("unknown or expired call_id"))
		return
	}

	sess.mu.Lock()
	if sess.conn != nil {
		_ = sess.conn.Close()
		sess.conn = nil
	}
	sess.conn = conn
	sess.clientSeen = true
	sess.mu.Unlock()

	caps := []string{
		EvASRPartial, EvASRFinal, EvASRError, EvDTMF, EvInterrupt,
		EvTTSStarted, EvTTSEnded, EvTTSCancelled, EvCallEnded,
		EvDialogWelcome, EvDialogTransfer,
	}
	downCmds := []string{CmdTTSSpeak, CmdTTSCancel, CmdInterrupt, CmdHangup, CmdPing}
	sort.Strings(caps)
	sort.Strings(downCmds)

	hello := map[string]any{
		KeyType:               EvHello,
		KeyV:                  ProtocolVersion,
		KeyProtocol:           ProtocolLingechoVoiceDialog,
		KeyCallID:             sess.meta.CallID,
		KeyPCMSampleRateHz:    sess.meta.PCMSampleRate,
		KeyCodec:              sess.meta.CodecName,
		KeyFrom:               sess.meta.FromHeader,
		KeyTo:                 sess.meta.ToHeader,
		KeyRole:               "sip_gateway",
		KeyUpstreamEvents:     caps,
		KeyDownstreamCommands: downCmds,
		KeyTS:                 tsRFC3339Nano(),
	}
	if err := writeJSONDeadline(conn, hello); err != nil {
		return
	}
	logger.Info("voicedialog → ws hello sent",
		zap.String(KeyCallID, sess.meta.CallID),
		zap.Strings(KeyUpstreamEvents, caps),
		zap.Strings(KeyDownstreamCommands, downCmds),
	)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if h.cfg.HangupInbound != nil {
				h.cfg.HangupInbound(callID)
			}
			return
		}
		var cmd map[string]any
		if err := json.Unmarshal(data, &cmd); err != nil {
			logger.Warn("voicedialog ← ws invalid json",
				zap.String(KeyCallID, callID),
				zap.Int("bytes", len(data)),
			)
			continue
		}
		t, _ := cmd[KeyType].(string)
		t = strings.ToLower(strings.TrimSpace(t))
		logWSIn(callID, t, cmd)

		switch t {
		case CmdHangup:
			h.endCall(callID, "hangup")
			if h.cfg.HangupInbound != nil {
				h.cfg.HangupInbound(callID)
			}
			return
		case CmdTTSSpeak:
			text, _ := cmd[KeyText].(string)
			uid, _ := cmd[KeyUtteranceID].(string)
			if strings.TrimSpace(uid) == "" {
				uid = fmt.Sprintf("u-%d", time.Now().UnixNano())
			}
			go sess.handleTTSSpeak(text, uid)
		case CmdTTSCancel:
			sess.handleTTSCancel()
		case CmdInterrupt:
			reason, _ := cmd[KeyReason].(string)
			sess.handleInterruptFromWS(reason)
		case CmdPing:
			_ = writeJSONDeadline(conn, event(EvPong, callID, nil))
		}
	}
}

func logWSIn(callID, typ string, cmd map[string]any) {
	fields := []zap.Field{zap.String(KeyCallID, callID), zap.String(KeyType, typ)}
	switch typ {
	case CmdTTSSpeak:
		txt, _ := cmd[KeyText].(string)
		uid, _ := cmd[KeyUtteranceID].(string)
		fields = append(fields,
			zap.String(KeyUtteranceID, uid),
			zap.Int("text_len", len([]rune(txt))),
			zap.String("text_preview", truncateRunes(txt, 120)),
		)
	case CmdInterrupt:
		r, _ := cmd[KeyReason].(string)
		fields = append(fields, zap.String(KeyReason, r))
	default:
		fields = append(fields, zap.Strings("payload_keys", cmdKeysSorted(cmd)))
	}
	logger.Info("voicedialog ← ws", fields...)
}

func cmdKeysSorted(cmd map[string]any) []string {
	if len(cmd) == 0 {
		return nil
	}
	out := make([]string, 0, len(cmd))
	for k := range cmd {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
