package voicedialog

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

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
				return true // protect with VOICE_DIALOG_WS_TOKEN
			},
		},
	}
	conversation.SetTransferPhaseNotifier(deliverConversationTransferPhase)
}

// AttachInbound registers the voice-dialog gateway for this call. Call inside cs.AttachVoiceConversation.
func AttachInbound(_ context.Context, cs *sipSession.CallSession, meta InboundMeta) error {
	if defaultHub == nil {
		return errors.New("voice dialog: InitDefault not called")
	}
	if cs == nil || strings.TrimSpace(meta.CallID) == "" {
		return errors.New("voice dialog: invalid call session or call-id")
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
		return errors.New("voice dialog: nil media session")
	}

	if err := attachGatewayMedia(sess); err != nil {
		h.endCall(meta.CallID, "gateway_attach_error")
		return err
	}

	h.startInboundLoopbackWS(sess)

	logger.Info("voice dialog inbound gateway registered (await WebSocket)",
		zap.String(KeyCallID, meta.CallID),
		zap.Int(KeyPCMSampleRateHz, meta.PCMSampleRate),
		zap.String(KeyCodec, meta.CodecName),
	)
	return nil
}

// AttachInboundVoiceDialog registers the voicedialog gateway on this inbound leg: SIP does RTP/PCM,
// ASR/TTS on the gateway; dialogue uses HTTP WebSocket (loopback and/or external clients).
// Call from SIP ACK handling (pkg/sip/server).
//
// Realtime-mode tenants bypass the gateway entirely and go through the
// embedded SIP voice path (`conversation.AttachVoicePipeline`), which
// dispatches to `attachRealtimeVoiceInner`. The voicedialog gateway WS
// protocol assumes the client drives turns via tts.speak — realtime
// providers (Qwen-Omni, GPT-4o realtime) generate replies internally,
// so there is no per-segment surface for the gateway to forward. Until
// the gateway protocol is extended to passthrough omni events, the
// embedded path is the right home for realtime calls.
func AttachInboundVoiceDialog(ctx context.Context, cs *sipSession.CallSession, from, to, remote, customVoiceWSURL string) error {
	if cs == nil {
		return nil
	}
	// Tenant voice-mode dispatch: when the tenant is on realtime mode,
	// skip the gateway and run the embedded pipeline. This is the same
	// `AttachVoicePipeline` outbound calls already use, which has its
	// own `voiceMode == "realtime"` branch and runs Qwen-Omni etc.
	if voiceEnv, loaded, err := conversation.ResolveTenantVoiceEnv(ctx, cs); err == nil && loaded {
		realtime := strings.EqualFold(voiceEnv.VoiceMode, "realtime") ||
			(voiceEnv.VoiceMode == "" && conversation.TenantRealtimeReady(voiceEnv))
		if realtime {
			logger.Info("voicedialog: tenant on realtime mode → delegating to embedded SIP voice (gateway protocol bypassed)",
				zap.String(KeyCallID, cs.CallID),
				zap.String("realtime_provider", voiceEnv.RealtimeProvider),
			)
			lg := logger.Lg
			return conversation.AttachVoicePipeline(ctx, cs, lg)
		}
	}
	meta := InboundMeta{
		CallID:           cs.CallID,
		FromHeader:       from,
		ToHeader:         to,
		RemoteSig:        remote,
		CodecName:        cs.NegotiatedCodec().Name,
		PCMSampleRate:    cs.PCMSampleRate(),
		CustomVoiceWSURL: strings.TrimSpace(customVoiceWSURL),
	}
	return cs.AttachVoiceConversation(func() error {
		return AttachInbound(ctx, cs, meta)
	})
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
			logger.Info("voice dialog → ws call.pending: no subscriber sockets (fanout skipped; using inbound loopback per-call ws)",
				zap.String(KeyCallID, sess.meta.CallID),
			)
		} else {
			logger.Warn("voice dialog → ws call.pending fanout: no subscribers (HTTP is up but no dialog WebSocket yet)",
				zap.String(KeyCallID, sess.meta.CallID),
				zap.Int("subscribers", 0),
				zap.String("subscriber_ws_path", constants.LingechoVoiceDialogPathPrefix+"/ws"),
				zap.String("hint", "connect subscriber WS without call_id to receive call.pending; then connect with call_id for ASR→LLM→tts.speak"),
			)
		}
	} else {
		logger.Info("voice dialog → ws call.pending fanout",
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

// writeJSON serialises writes to the session's current WebSocket conn.
// This is the ONLY path that may write to sess.conn from goroutines
// other than the read loop (TTS player, media barge-in, endCall). The
// raw `writeJSONDeadline(sess.conn, …)` form is unsafe — see writeMu
// doc in types.go and the gorilla/websocket concurrent-write panic.
func (sess *dialogSession) writeJSON(v any) error {
	if sess == nil {
		return errors.New("nil session")
	}
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	sess.mu.Lock()
	c := sess.conn
	sess.mu.Unlock()
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
	hasConn := sess.conn != nil
	sess.mu.Unlock()
	if hasConn {
		logger.Info("voice dialog → ws call.ended",
			zap.String(KeyCallID, callID),
			zap.String(KeyReason, reason),
		)
		_ = sess.writeJSON(event(EvCallEnded, callID, map[string]any{
			KeyReason: reason,
		}))
		sess.mu.Lock()
		if sess.conn != nil {
			_ = sess.conn.Close()
			sess.conn = nil
		}
		sess.mu.Unlock()
	}
	logger.Info("voicedialog session ended",
		zap.String(KeyCallID, callID),
		zap.String(KeyReason, reason),
	)
}

// envAllowEmptyVoiceDialogToken is the explicit dev-only opt-in to
// keep the legacy "empty token = accept all" behaviour. Production
// must leave it unset so missing VOICE_DIALOG_WS_TOKEN → 401 instead
// of silently disabling auth (matches webseat's strict-by-default).
const envAllowEmptyVoiceDialogToken = "VOICE_DIALOG_ALLOW_EMPTY_TOKEN"

// WebSocketTokenOK validates ?token= against VOICE_DIALOG_WS_TOKEN
// (constant-time when set). Empty env → reject 401, unless
// VOICE_DIALOG_ALLOW_EMPTY_TOKEN=true is set explicitly. The SIP
// inbound loopback dialer (sipapp) reads the same env, so as long as
// VOICE_DIALOG_WS_TOKEN is configured both sides agree.
func WebSocketTokenOK(r *http.Request) bool {
	expected := wsTokenExpected()
	got := strings.TrimSpace(r.URL.Query().Get("token"))
	if expected == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv(envAllowEmptyVoiceDialogToken)), "true") {
			if defaultHub != nil {
				defaultHub.tokenMissingOnce.Do(func() {
					logger.Warn("voice dialog VOICE_DIALOG_WS_TOKEN empty + ALLOW_EMPTY_TOKEN=true → accepting all clients (DEV ONLY)",
						zap.Bool("inbound_loopback_ws", defaultHub.cfg.InboundLoopbackWS),
					)
				})
			}
			return true
		}
		if defaultHub != nil {
			defaultHub.tokenMissingOnce.Do(func() {
				logger.Error("voice dialog VOICE_DIALOG_WS_TOKEN is empty; rejecting all WS upgrades (set VOICE_DIALOG_ALLOW_EMPTY_TOKEN=true to allow in dev)",
					zap.Bool("inbound_loopback_ws", defaultHub.cfg.InboundLoopbackWS),
				)
			})
		}
		return false
	}
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// WebSocketHubReady reports whether InitDefault has created the dialog hub.
func WebSocketHubReady() bool {
	return defaultHub != nil
}

// UpgradeVoiceDialogWebSocket performs the WebSocket handshake using the hub's upgrader.
func UpgradeVoiceDialogWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	if defaultHub == nil {
		return nil, errors.New("voice dialog: hub not initialized")
	}
	return defaultHub.wsUpgrader.Upgrade(w, r, nil)
}

// ServeVoiceDialogWebSocket runs the dialog protocol on an upgraded connection (subscriber fanout or per-call session).
func ServeVoiceDialogWebSocket(conn *websocket.Conn, callID string) {
	if conn == nil || defaultHub == nil {
		if conn != nil {
			_ = conn.Close()
		}
		return
	}
	h := defaultHub
	wsRemote := conn.RemoteAddr().String()
	callID = strings.TrimSpace(callID)
	if callID == "" {
		logger.Info("voice dialog ws: upgraded (subscriber)",
			zap.String("ws_remote", wsRemote),
		)
		h.subMu.Lock()
		h.subs = append(h.subs, conn)
		h.subMu.Unlock()
		h.subscribeReadLoop(conn)
		return
	}
	logger.Info("voice dialog ws: upgraded (call session)",
		zap.String("ws_remote", wsRemote),
		zap.String(KeyCallID, callID),
	)
	h.runCallSocket(callID, conn)
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
	if err := sess.writeJSON(hello); err != nil {
		return
	}
	logger.Info("voice dialog → ws hello sent",
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
			gen := sess.ttsGenSeq.Add(1)
			// NOTE: must be synchronous. handleTTSSpeak only validates + spawns
			// the per-segment prefetch goroutine + does a non-blocking channel
			// send into ttsSegmentCh. Running it in `go` lets multiple concurrent
			// CmdTTSSpeak (e.g. when the LLM emits 3+ sentences within the same
			// SSE chunk) race on enqueueTTSSegment, which produced out-of-order
			// playback (segment N+1 played before N). The reader loop is the
			// single producer of WS commands, so calling synchronously preserves
			// arrival order through the queue.
			sess.handleTTSSpeak(text, uid, gen)
		case CmdTTSCancel:
			sess.handleTTSCancel()
		case CmdInterrupt:
			reason, _ := cmd[KeyReason].(string)
			sess.handleInterruptFromWS(reason)
		case CmdPing:
			_ = sess.writeJSON(event(EvPong, callID, nil))
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

// --- emitGateway (uses media import only for zap in switch; keep here to avoid cycle) ---

func (sess *dialogSession) emitGateway(ev map[string]any) {
	if sess == nil || ev == nil {
		return
	}
	callID := sess.meta.CallID
	t, _ := ev[KeyType].(string)
	switch t {
	case EvASRPartial, EvASRFinal:
		txt, _ := ev[KeyText].(string)
		logger.Info("voice dialog → ws asr",
			zap.String(KeyCallID, callID),
			zap.String("event", t),
			zap.Bool(KeyFinal, t == EvASRFinal),
			zap.Int("text_len", len([]rune(txt))),
			zap.String("text_preview", truncateRunes(txt, 100)),
		)
	case EvASRError:
		msg, _ := ev[KeyMessage].(string)
		fatal, _ := ev[KeyFatal].(bool)
		logger.Warn("voice dialog → ws asr.error",
			zap.String(KeyCallID, callID),
			zap.String(KeyMessage, msg),
			zap.Bool(KeyFatal, fatal),
		)
	case EvDTMF:
		d, _ := ev[KeyDigit].(string)
		logger.Info("voice dialog → ws dtmf",
			zap.String(KeyCallID, callID),
			zap.String(KeyDigit, d),
		)
	case EvInterrupt:
		origin, _ := ev[KeyOrigin].(string)
		cause, _ := ev[KeyCause].(string)
		reason, _ := ev[KeyReason].(string)
		logger.Info("voice dialog → ws interrupt",
			zap.String(KeyCallID, callID),
			zap.String(KeyOrigin, origin),
			zap.String(KeyCause, cause),
			zap.String(KeyReason, reason),
		)
	case EvTTSStarted:
		prev, _ := ev[KeyTextPreview].(string)
		uid, _ := ev[KeyUtteranceID].(string)
		logger.Info("voice dialog → ws tts.started",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, uid),
			zap.String(KeyTextPreview, prev),
		)
	case EvTTSEnded:
		uid, _ := ev[KeyUtteranceID].(string)
		ok, _ := ev[KeyOK].(bool)
		logger.Info("voice dialog → ws tts.ended",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, uid),
			zap.Bool(KeyOK, ok),
		)
	case EvTTSCancelled:
		logger.Info("voice dialog → ws tts.cancelled", zap.String(KeyCallID, callID))
	case EvDialogWelcome, EvDialogTransfer:
		ph, _ := ev[KeyPhase].(string)
		sk, _ := ev[KeySourceKind].(string)
		src, _ := ev[KeySource].(string)
		logger.Info("voice dialog → ws "+t,
			zap.String(KeyCallID, callID),
			zap.String(KeyPhase, ph),
			zap.String(KeySourceKind, sk),
			zap.String(KeySource, src),
		)
	default:
		if strings.HasPrefix(t, "call.") || t == EvHello || t == EvError {
			logger.Debug("voice dialog → ws",
				zap.String(KeyCallID, callID),
				zap.String(KeyType, t),
			)
		} else {
			logger.Info("voice dialog → ws",
				zap.String(KeyCallID, callID),
				zap.String(KeyType, t),
			)
		}
	}

	_ = sess.writeJSON(ev)
}
