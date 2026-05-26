// Package webseat bridges an inbound SIP call to a browser over WebRTC when routing selects a WebSeat pool target.
package webseat

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/sip/bridge"
	siprtp "github.com/LinByte/VoiceServer/pkg/sip/rtp"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

const (
	// EnvWSToken is the shared secret for GET /webseat/v1/ws?token=... (empty = accept any client; not recommended for production).
	EnvWSToken = "SIP_WEBSEAT_WS_TOKEN"
)

var (
	wsUpgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	wsTokenMissingOnce sync.Once
)

// Config wires SIP teardown and store updates from cmd/sip (avoid import cycles).
type Config struct {
	RemoveCallSession     func(callID string)
	ForgetUASDialog       func(callID string)
	SendUASBye            func(callID string) error
	ReleaseTransferDedupe func(callID string)
	// SetACDWebSeatWorkState updates acd_pool_targets.work_state for the web seat row (targetID) when nil, ACD state is skipped.
	SetACDWebSeatWorkState func(ctx context.Context, targetID uint, workState string) error
	// FinalizeInboundPersist runs once when a web-seat handoff ends (BYE, hangup, or bridge teardown).
	// callID is the inbound PSTN Call-ID; initiator is "remote" (customer BYE) or "local" (operator / full hangup).
	// wavRec is the (already-uploaded) stereo WAV from pkg/voice/recorder; zero-valued (Bucket=="") when the
	// new recorder was disabled / no PCM was captured — the persister should fall back to RawPayload (SN3) decoding.
	FinalizeInboundPersist func(ctx context.Context, callID, initiator string, raw []byte, codecName string, recordSampleRate, recordOpusChannels int, wavRec gateway.RecordingInfo)
	// OnWebSeatBridgeEstablished runs after the browser SDP answer is accepted (bridge setup begins).
	OnWebSeatBridgeEstablished func(callID string)
	// OnWebSeatJoinTimeout runs when no browser join arrives within SIP_WEBSEAT_JOIN_TIMEOUT (after dedupe release).
	OnWebSeatJoinTimeout func(callID string, acdPoolTargetID uint)
	// PlayTransferAgentBrief plays optional trunk transfer_agent_brief_text TTS on browser downlink
	// before PSTN↔WebRTC bridge (caller keeps transfer ringing until bridge starts).
	PlayTransferAgentBrief func(ctx context.Context, inboundCallID string, agentDownlink media.MediaTransport) (played bool, err error)
	// StopTransferRinging cancels inbound hold/ring loop before web bridge (mirrors SIP StartTransferBridge).
	StopTransferRinging func(inboundCallID string)
}

// Hub tracks pending joins and active bridges.
type Hub struct {
	cfg Config

	mu       sync.Mutex
	awaiting map[string]*awaitEntry   // inbound Call-ID
	active   map[string]*activeBridge // inbound Call-ID

	// acdBinding maps inbound PSTN Call-ID → acd_pool_targets.id for the web seat row picked for this transfer.
	acdBinding sync.Map

	wsMu    sync.Mutex
	wsConns map[*websocket.Conn]struct{}
}

type awaitEntry struct {
	cs *sipSession.CallSession
	lg *zap.Logger
	at time.Time
}

type activeBridge struct {
	callID  string
	inbound *sipSession.CallSession
	br      *bridge.TwoLegPCMBridge
	pc      *webrtc.PeerConnection
}

var defaultHub *Hub

// InitDefault configures the process-wide hub (call once from main).
func InitDefault(cfg Config) {
	defaultHub = &Hub{
		cfg:      cfg,
		awaiting: make(map[string]*awaitEntry),
		active:   make(map[string]*activeBridge),
		wsConns:  make(map[*websocket.Conn]struct{}),
	}
}

// BindInboundCallToWebACD records which ACD pool row was offered for this inbound call
// (after PickTransferDialTarget marked it ringing).
func BindInboundCallToWebACD(callID string, acdTargetID uint) {
	if defaultHub == nil || strings.TrimSpace(callID) == "" || acdTargetID == 0 {
		return
	}
	defaultHub.acdBinding.Store(strings.TrimSpace(callID), acdTargetID)
}

// ReleaseInboundWebACDOffer sets the bound web ACD row back to available and clears the binding
// (e.g. register-awaiting failed or transfer aborted before webseat took the call).
func ReleaseInboundWebACDOffer(callID string) {
	if defaultHub == nil || strings.TrimSpace(callID) == "" {
		return
	}
	defaultHub.acdReleaseBindingToAvailable(strings.TrimSpace(callID))
}

func (h *Hub) acdSetStateForCall(callID string, workState string) {
	if h == nil || strings.TrimSpace(callID) == "" || h.cfg.SetACDWebSeatWorkState == nil {
		return
	}
	v, ok := h.acdBinding.Load(strings.TrimSpace(callID))
	if !ok {
		return
	}
	id, ok := v.(uint)
	if !ok || id == 0 {
		return
	}
	ctx := context.Background()
	_ = h.cfg.SetACDWebSeatWorkState(ctx, id, workState)
}

func (h *Hub) acdReleaseBindingToAvailable(callID string) {
	if h == nil || strings.TrimSpace(callID) == "" {
		return
	}
	cid := strings.TrimSpace(callID)
	v, loaded := h.acdBinding.LoadAndDelete(cid)
	if !loaded {
		return
	}
	if h.cfg.SetACDWebSeatWorkState == nil {
		return
	}
	id, ok := v.(uint)
	if !ok || id == 0 {
		return
	}
	ctx := context.Background()
	_ = h.cfg.SetACDWebSeatWorkState(ctx, id, "available")
}

// JoinHTTP serves POST join (browser WebRTC offer). Mount on Gin as WrapF(JoinHTTP).
func JoinHTTP(w http.ResponseWriter, r *http.Request) {
	if defaultHub == nil {
		http.Error(w, "webseat not initialized", http.StatusServiceUnavailable)
		return
	}
	defaultHub.handleJoin(w, r)
}

// HangupHTTP serves POST hangup (JSON body call_id).
func HangupHTTP(w http.ResponseWriter, r *http.Request) {
	if defaultHub == nil {
		http.Error(w, "webseat not initialized", http.StatusServiceUnavailable)
		return
	}
	defaultHub.handleAgentHangup(w, r)
}

// RejectHTTP serves POST reject (JSON body call_id).
func RejectHTTP(w http.ResponseWriter, r *http.Request) {
	if defaultHub == nil {
		http.Error(w, "webseat not initialized", http.StatusServiceUnavailable)
		return
	}
	defaultHub.handleAgentReject(w, r)
}

// WebSocketHTTP serves GET WebSocket upgrade (?token=...).
func WebSocketHTTP(w http.ResponseWriter, r *http.Request) {
	if defaultHub == nil {
		http.Error(w, "webseat not initialized", http.StatusServiceUnavailable)
		return
	}
	defaultHub.handleWebSocket(w, r)
}

// RegisterAwaiting marks inbound as waiting for browser WebRTC join (after AI media stopped).
func RegisterAwaiting(callID string, cs *sipSession.CallSession, lg *zap.Logger) error {
	if defaultHub == nil {
		return errors.New("webseat: InitDefault not called")
	}
	callID = strings.TrimSpace(callID)
	if callID == "" || cs == nil {
		return errors.New("webseat: invalid call or session")
	}
	h := defaultHub
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[callID] != nil {
		return fmt.Errorf("webseat: call %q already bridged", callID)
	}
	h.awaiting[callID] = &awaitEntry{cs: cs, lg: lg, at: time.Now()}
	logger.SafeGo("webseat-await-watchdog", func() { h.awaitWatchdog(callID) })
	if lg != nil {
		lg.Info("webseat: awaiting browser join", zap.String("call_id", callID))
	}
	logger.SafeGo("webseat-broadcast-incoming", func() { h.broadcastIncoming(callID) })
	return nil
}

// HeaderWebseatToken lets browsers POST without leaking the token in
// query strings (which can land in access logs / referrers). Both
// query (?token=...) and header are accepted; header wins when both
// are set.
const HeaderWebseatToken = "X-Webseat-Token"

// EnvAllowEmptyToken makes empty SIP_WEBSEAT_WS_TOKEN behave like
// "accept any client" — only meant for local/dev. Production must
// keep this unset so a missing/empty token results in 401 instead
// of silently disabling auth.
const EnvAllowEmptyToken = "SIP_WEBSEAT_ALLOW_EMPTY_TOKEN"

// webseatTokenOK gates every webseat HTTP / WS endpoint. It used to
// only check ?token=... and **silently allowed all clients** when
// SIP_WEBSEAT_WS_TOKEN was empty — which means in production an
// attacker could POST to /join (taking over a pending PSTN call) or
// /hangup (DoS any active call) by guessing a Call-ID. Now:
//
//   - read token from X-Webseat-Token header first, fall back to ?token=
//   - empty env → reject 401, unless SIP_WEBSEAT_ALLOW_EMPTY_TOKEN=true
//     (explicit local-dev opt-in)
//   - constant-time compare to defeat timing oracles
func webseatTokenOK(r *http.Request) bool {
	expected := strings.TrimSpace(utils.GetEnv(EnvWSToken))
	got := strings.TrimSpace(r.Header.Get(HeaderWebseatToken))
	if got == "" {
		got = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if expected == "" {
		if strings.EqualFold(strings.TrimSpace(utils.GetEnv(EnvAllowEmptyToken)), "true") {
			wsTokenMissingOnce.Do(func() {
				if logger.Lg != nil {
					logger.Lg.Warn("webseat: SIP_WEBSEAT_WS_TOKEN empty + ALLOW_EMPTY_TOKEN=true → accepting all clients (DEV ONLY)")
				}
			})
			return true
		}
		// Strict-by-default: production must configure a token. Logged
		// once so misconfiguration is loud at startup.
		wsTokenMissingOnce.Do(func() {
			if logger.Lg != nil {
				logger.Lg.Error("webseat: SIP_WEBSEAT_WS_TOKEN is empty; rejecting all webseat requests (set SIP_WEBSEAT_ALLOW_EMPTY_TOKEN=true to allow in dev)")
			}
		})
		return false
	}
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// webseatWSTokenOK kept as a thin alias for callers that wired the old
// name; new code should call webseatTokenOK.
func webseatWSTokenOK(r *http.Request) bool { return webseatTokenOK(r) }

// HTTPTokenOK is the exported wrapper for callers in the handlers
// package that need to enforce the webseat token gate (e.g. the
// status endpoint mounted directly on the gin router rather than
// through the package-internal http.Handler chain).
func HTTPTokenOK(r *http.Request) bool { return webseatTokenOK(r) }

func (h *Hub) wsAdd(c *websocket.Conn) {
	if h == nil || c == nil {
		return
	}
	h.wsMu.Lock()
	h.wsConns[c] = struct{}{}
	n := len(h.wsConns)
	h.wsMu.Unlock()
	if logger.Lg != nil {
		logger.Lg.Info("webseat: websocket client connected", zap.Int("clients", n))
	}
	h.broadcastPresence()
}

func (h *Hub) wsRemove(c *websocket.Conn) {
	if h == nil || c == nil {
		return
	}
	h.wsMu.Lock()
	delete(h.wsConns, c)
	h.wsMu.Unlock()
}

// broadcastIncoming notifies all WS clients that a call is waiting for join (JSON: type=incoming, call_id).
func (h *Hub) broadcastIncoming(callID string) {
	if h == nil || strings.TrimSpace(callID) == "" {
		return
	}
	msg, err := json.Marshal(map[string]any{
		"type":    "incoming",
		"call_id": callID,
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}
	h.wsMu.Lock()
	list := make([]*websocket.Conn, 0, len(h.wsConns))
	for c := range h.wsConns {
		list = append(list, c)
	}
	h.wsMu.Unlock()
	for _, c := range list {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			_ = c.Close()
			h.wsRemove(c)
		}
	}
}

// broadcastIncomingEnd notifies agents that the incoming card/ringing for call_id should stop.
func (h *Hub) broadcastIncomingEnd(callID, reason string) {
	if h == nil || strings.TrimSpace(callID) == "" {
		return
	}
	msg, err := json.Marshal(map[string]any{
		"type":    "incoming_end",
		"call_id": strings.TrimSpace(callID),
		"reason":  strings.TrimSpace(reason),
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}
	h.wsMu.Lock()
	list := make([]*websocket.Conn, 0, len(h.wsConns))
	for c := range h.wsConns {
		list = append(list, c)
	}
	h.wsMu.Unlock()
	for _, c := range list {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			_ = c.Close()
			h.wsRemove(c)
		}
	}
}

// broadcastPresence notifies all agent WS clients of current listener count (replaces a separate HTTP presence probe).
func (h *Hub) broadcastPresence() {
	if h == nil {
		return
	}
	h.wsMu.Lock()
	n := len(h.wsConns)
	list := make([]*websocket.Conn, 0, n)
	for c := range h.wsConns {
		list = append(list, c)
	}
	h.wsMu.Unlock()
	msg, err := json.Marshal(map[string]any{
		"type":       "presence",
		"ws_clients": n,
		"online":     n > 0,
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}
	for _, c := range list {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			_ = c.Close()
			h.wsRemove(c)
		}
	}
}

// handleWebSocket: GET /webseat/v1/ws?token=... — push incoming call_id to the agent page.
func (h *Hub) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !webseatWSTokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.wsAdd(conn)
	go func(c *websocket.Conn) {
		defer func() {
			_ = c.Close()
			h.wsRemove(c)
			h.broadcastPresence()
		}()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}(conn)
}

func (h *Hub) awaitWatchdog(callID string) {
	wait := 30 * time.Second
	if v := utils.GetEnv("SIP_WEBSEAT_JOIN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			wait = d
		}
	}
	time.Sleep(wait)
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.awaiting[callID]; ok {
		delete(h.awaiting, callID)
		if e.lg != nil {
			e.lg.Warn("webseat: join timeout, releasing slot", zap.String("call_id", callID))
		}
		var acdID uint
		if v, ok := h.acdBinding.Load(callID); ok {
			if id, ok := v.(uint); ok {
				acdID = id
			}
		}
		h.acdReleaseBindingToAvailable(callID)
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		if h.cfg.OnWebSeatJoinTimeout != nil {
			h.cfg.OnWebSeatJoinTimeout(callID, acdID)
		}
		h.broadcastIncomingEnd(callID, "join_timeout")
	}
}

// IsPendingOrActive is true while waiting for WebRTC or while a bridge is running (suppress late ACK voice attach).
func IsPendingOrActive(callID string) bool {
	if defaultHub == nil || callID == "" {
		return false
	}
	h := defaultHub
	h.mu.Lock()
	defer h.mu.Unlock()
	_, p := h.awaiting[callID]
	_, a := h.active[callID]
	return p || a
}

// IsActive is true only when WebRTC bridge is active (browser already joined).
func IsActive(callID string) bool {
	if defaultHub == nil || callID == "" {
		return false
	}
	h := defaultHub
	h.mu.Lock()
	defer h.mu.Unlock()
	_, a := h.active[callID]
	return a
}

// HangupIfCustomerBye tears down Web seat when the PSTN side sends BYE. Returns true if handled.
func HangupIfCustomerBye(callID string) bool {
	return teardownWebSeat(callID, false)
}

// HangupFull tears down Web seat and BYE the customer (browser left or operator hangup).
func HangupFull(callID string) bool {
	return teardownWebSeat(callID, true)
}

func persistSnapshotInbound(cs *sipSession.CallSession) (raw []byte, codecName string, recSR, recOpusCh int, wavRec gateway.RecordingInfo) {
	if cs == nil {
		return nil, "", 0, 0, gateway.RecordingInfo{}
	}
	raw = cs.TakeRecording()
	codecName = cs.NegotiatedCodec().Name
	src := cs.SourceCodec()
	recSR = src.SampleRate
	recOpusCh = src.OpusDecodeChannels
	if recOpusCh < 1 {
		recOpusCh = src.Channels
	}
	// Best-effort flush of the stereo PCM recorder — if not enabled or
	// upload failed we still return the SN3 RawPayload above as fallback.
	if info, ok := cs.FlushRecorder(context.Background()); ok {
		wavRec = info
	}
	return raw, codecName, recSR, recOpusCh, wavRec
}

func (h *Hub) emitFinalizePersist(callID, initiator string, cs *sipSession.CallSession) {
	if h == nil || strings.TrimSpace(callID) == "" || h.cfg.FinalizeInboundPersist == nil {
		return
	}
	init := strings.TrimSpace(initiator)
	if init == "" {
		init = "remote"
	}
	raw, codec, sr, ch, wavRec := persistSnapshotInbound(cs)
	go h.cfg.FinalizeInboundPersist(context.Background(), callID, init, raw, codec, sr, ch, wavRec)
}

// cleanupInboundAfterJoinFailure runs when completeJoin fails after we removed the call from awaiting.
// Without this, the PSTN leg can stay up (no BYE) while the web join never completes.
func (h *Hub) cleanupInboundAfterJoinFailure(callID string, cs *sipSession.CallSession, lg *zap.Logger) {
	if h == nil || strings.TrimSpace(callID) == "" {
		return
	}
	h.broadcastIncomingEnd(callID, "ended")
	h.emitFinalizePersist(callID, "local", cs)
	if cs != nil {
		cs.Stop()
	}
	if h.cfg.SendUASBye != nil {
		if err := h.cfg.SendUASBye(callID); err != nil && lg != nil {
			lg.Warn("webseat: join failed; SendUASBye failed", zap.String("call_id", callID), zap.Error(err))
		} else if err == nil && h.cfg.ForgetUASDialog != nil {
			h.cfg.ForgetUASDialog(callID)
		}
	} else if h.cfg.ForgetUASDialog != nil {
		h.cfg.ForgetUASDialog(callID)
	}
	if h.cfg.RemoveCallSession != nil {
		h.cfg.RemoveCallSession(callID)
	}
}

func teardownWebSeat(callID string, sendByeToCustomer bool) bool {
	if defaultHub == nil {
		return false
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	h := defaultHub
	lg := logger.Lg
	if lg == nil {
		lg = zap.NewNop()
	}

	h.mu.Lock()
	ab, activeOK := h.active[callID]
	if !activeOK {
		entry, waiting := h.awaiting[callID]
		if waiting {
			if sendByeToCustomer && h.cfg.SendUASBye != nil {
				if err := h.cfg.SendUASBye(callID); err != nil {
					h.mu.Unlock()
					lg.Warn("webseat: SendUASBye failed (awaiting join path)", zap.String("call_id", callID), zap.Error(err))
					return false
				}
			}
			delete(h.awaiting, callID)
			h.mu.Unlock()
			h.broadcastIncomingEnd(callID, "ended")
			initiator := "remote"
			if sendByeToCustomer {
				initiator = "local"
			}
			var cs *sipSession.CallSession
			if entry != nil {
				cs = entry.cs
			}
			h.emitFinalizePersist(callID, initiator, cs)
			if cs != nil {
				cs.Stop()
			}
			if h.cfg.RemoveCallSession != nil {
				h.cfg.RemoveCallSession(callID)
			}
			if h.cfg.ForgetUASDialog != nil {
				h.cfg.ForgetUASDialog(callID)
			}
			h.acdReleaseBindingToAvailable(callID)
			if h.cfg.ReleaseTransferDedupe != nil {
				h.cfg.ReleaseTransferDedupe(callID)
			}
			lg.Info("webseat: session torn down", zap.String("call_id", callID), zap.Bool("bye_customer", sendByeToCustomer), zap.String("phase", "awaiting"))
			return true
		}
		h.mu.Unlock()
		return false
	}
	h.mu.Unlock()

	if sendByeToCustomer && h.cfg.SendUASBye != nil {
		if err := h.cfg.SendUASBye(callID); err != nil {
			lg.Warn("webseat: SendUASBye failed (active bridge path)", zap.String("call_id", callID), zap.Error(err))
			return false
		}
	}

	h.mu.Lock()
	if cur, still := h.active[callID]; !still || cur != ab {
		h.mu.Unlock()
		return false
	}
	delete(h.active, callID)
	h.mu.Unlock()

	h.broadcastIncomingEnd(callID, "ended")

	initiator := "remote"
	if sendByeToCustomer {
		initiator = "local"
	}
	h.emitFinalizePersist(callID, initiator, ab.inbound)

	if ab.br != nil {
		ab.br.Stop()
	}
	if ab.pc != nil {
		_ = ab.pc.Close()
	}
	if ab.inbound != nil {
		ab.inbound.CloseRTPOnly()
	}
	if h.cfg.RemoveCallSession != nil {
		h.cfg.RemoveCallSession(callID)
	}
	if h.cfg.ForgetUASDialog != nil {
		h.cfg.ForgetUASDialog(callID)
	}
	h.acdReleaseBindingToAvailable(callID)
	if h.cfg.ReleaseTransferDedupe != nil {
		h.cfg.ReleaseTransferDedupe(callID)
	}
	lg.Info("webseat: session torn down", zap.String("call_id", callID), zap.Bool("bye_customer", sendByeToCustomer))
	return true
}

type joinBody struct {
	CallID     string                    `json:"call_id"`
	SDP        string                    `json:"sdp"`
	Type       string                    `json:"type"`
	Candidates []webrtc.ICECandidateInit `json:"candidates"`
}

func (h *Hub) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !webseatTokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body joinBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "json", http.StatusBadRequest)
		return
	}
	callID := strings.TrimSpace(body.CallID)
	if callID == "" || strings.TrimSpace(body.SDP) == "" {
		http.Error(w, "call_id and sdp required", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	entry, ok := h.awaiting[callID]
	if ok {
		delete(h.awaiting, callID)
	}
	h.mu.Unlock()
	if !ok {
		http.Error(w, "unknown or expired call_id", http.StatusNotFound)
		return
	}

	lg := entry.lg
	if lg == nil && logger.Lg != nil {
		lg = logger.Lg
	}
	if lg == nil {
		lg = zap.NewNop()
	}

	answer, err := h.completeJoin(r.Context(), callID, entry.cs, body, lg)
	if err != nil {
		h.cleanupInboundAfterJoinFailure(callID, entry.cs, lg)
		h.acdReleaseBindingToAvailable(callID)
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		lg.Warn("webseat: join failed", zap.String("call_id", callID), zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(answer)
}

// handleAgentHangup ends the web seat leg and sends BYE to the PSTN customer (JSON body: { "call_id": "..." }).
func (h *Hub) handleAgentHangup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !webseatTokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		CallID string `json:"call_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "json", http.StatusBadRequest)
		return
	}
	callID := strings.TrimSpace(body.CallID)
	if callID == "" {
		http.Error(w, "call_id required", http.StatusBadRequest)
		return
	}
	if !HangupFull(callID) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAgentReject declines an awaiting or active web seat (same effect as hangup: BYE customer, teardown).
func (h *Hub) handleAgentReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !webseatTokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		CallID string `json:"call_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "json", http.StatusBadRequest)
		return
	}
	callID := strings.TrimSpace(body.CallID)
	if callID == "" {
		http.Error(w, "call_id required", http.StatusBadRequest)
		return
	}
	if !HangupFull(callID) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type joinAnswer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func (h *Hub) completeJoin(ctx context.Context, callID string, inbound *sipSession.CallSession, body joinBody, lg *zap.Logger) (*joinAnswer, error) {
	m := newMediaEngine()
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: defaultICEServers(),
	})
	if err != nil {
		return nil, err
	}

	// OnTrack fires only after the browser applies our answer and RTP flows — never block the HTTP response on this.
	trackCh := make(chan *webrtc.TrackRemote, 1)
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		select {
		case trackCh <- tr:
		default:
		}
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			_ = teardownWebSeat(callID, true)
		}
	})

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: body.SDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("SetRemoteDescription: %w", err)
	}
	for _, c := range body.Candidates {
		_ = pc.AddICECandidate(c)
	}

	// Must match codecs registered in newMediaEngine() (PCMA/PCMU only). Using Opus here
	// causes AddTrack to fail: "codec is not supported by remote" and join returns 500.
	// Prefer PCMA on the downlink track to align with browser setCodecPreferences (PCMA first).
	txCap := webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypePCMA,
		ClockRate: 8000,
	}
	txLocal, err := webrtc.NewTrackLocalStaticSample(txCap, "audio", "soulnexus")
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if _, err := pc.AddTrack(txLocal); err != nil {
		_ = pc.Close()
		return nil, err
	}

	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := pc.SetLocalDescription(ans); err != nil {
		_ = pc.Close()
		return nil, err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	select {
	case <-gatherComplete:
	case <-time.After(15 * time.Second):
		_ = pc.Close()
		return nil, errors.New("ICE gather timeout")
	case <-ctx.Done():
		_ = pc.Close()
		return nil, ctx.Err()
	}

	h.mu.Lock()
	h.active[callID] = &activeBridge{
		callID:  callID,
		inbound: inbound,
		br:      nil,
		pc:      pc,
	}
	h.mu.Unlock()

	if h.cfg.OnWebSeatBridgeEstablished != nil {
		h.cfg.OnWebSeatBridgeEstablished(callID)
	}

	webTxCodec := mediaFromRTPCapability(txCap)
	logger.SafeGo("webseat-bridge-wait-track", func() {
		h.waitRemoteTrackAndBridge(callID, inbound, pc, txLocal, webTxCodec, trackCh, lg)
	})

	ld := pc.LocalDescription()
	lg.Info("webseat: answer sent, waiting for browser RTP / OnTrack", zap.String("call_id", callID))
	return &joinAnswer{Type: ld.Type.String(), SDP: ld.SDP}, nil
}

func (h *Hub) waitRemoteTrackAndBridge(
	callID string,
	inbound *sipSession.CallSession,
	pc *webrtc.PeerConnection,
	txLocal *webrtc.TrackLocalStaticSample,
	webTxCodec media.CodecConfig,
	trackCh <-chan *webrtc.TrackRemote,
	lg *zap.Logger,
) {
	var remoteTrack *webrtc.TrackRemote
	select {
	case remoteTrack = <-trackCh:
	case <-time.After(90 * time.Second):
		if lg != nil {
			lg.Warn("webseat: no remote audio track after answer (ICE or mic?)", zap.String("call_id", callID), zap.Duration("wait", 90*time.Second))
		}
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		_ = teardownWebSeat(callID, true)
		return
	}
	if remoteTrack == nil {
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		_ = teardownWebSeat(callID, true)
		return
	}

	h.mu.Lock()
	ab, ok := h.active[callID]
	if !ok || ab == nil || ab.pc != pc {
		h.mu.Unlock()
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		_ = teardownWebSeat(callID, true)
		_ = pc.Close()
		return
	}
	h.mu.Unlock()

	webRxCodec := mediaFromRemoteTrack(remoteTrack)
	wt := NewTransport(remoteTrack, txLocal, webRxCodec, webTxCodec)

	if h.cfg.PlayTransferAgentBrief != nil {
		playCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		played, err := h.cfg.PlayTransferAgentBrief(playCtx, callID, wt)
		cancel()
		if err != nil && lg != nil {
			lg.Warn("webseat: agent brief TTS failed, bridging anyway",
				zap.String("call_id", callID),
				zap.Error(err),
			)
		}
		if played && lg != nil {
			lg.Info("webseat: agent brief finished", zap.String("call_id", callID))
		}
	}
	if h.cfg.StopTransferRinging != nil {
		h.cfg.StopTransferRinging(callID)
	}

	h.mu.Lock()
	ab, ok = h.active[callID]
	if !ok || ab == nil || ab.pc != pc {
		h.mu.Unlock()
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		_ = teardownWebSeat(callID, true)
		_ = pc.Close()
		return
	}
	h.mu.Unlock()

	// Keep caller media alive during "awaiting join" so transfer ringing can be played.
	// Stop MediaSession right before building bridge transports to avoid dual RTP readers.
	inbound.StopMediaPreserveRTP()

	ccIn := inbound.SourceCodec()
	callerRx := siprtp.NewSIPRTPTransport(inbound.RTPSession(), ccIn, media.DirectionInput, inbound.DTMFPayloadType())
	callerTx := siprtp.NewSIPRTPTransport(inbound.RTPSession(), ccIn, media.DirectionOutput, 0)
	inbound.WireTransferBridgeRecording(callerRx, callerTx)

	br, err := bridge.NewTwoLegPCMBridge(callerRx, callerTx, wt, wt)
	if err != nil {
		if lg != nil {
			lg.Warn("webseat: pcm bridge build failed", zap.String("call_id", callID), zap.Error(err))
		}
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		_ = teardownWebSeat(callID, true)
		return
	}
	// 把 WebSeat 桥接的双向 PCM 同步喂进 inbound 的新版立体声录音器：
	// 否则 OnBye 时 voice/recorder 产出的 WAV 在转接后整段静音（recorder
	// 不消化 SN3 raw / WireTransferBridgeRecording 喂的字节）。
	//
	// 桥接 mid SR 由两侧码方协商：WebSeat 浏览器侧固定 G.711/8k，inbound
	// 侧若也是 G.711 走 8k，Opus 走 16k；recorder 在 EnableRecorder 时按
	// inbound.PCMSampleRate() 配置，与 TwoLegPCMBridge.MidSampleRate() 一致，
	// 无需重采样。
	br.SetDirectionalPCMTap(func(dir bridge.BridgeDirection, pcm []byte) {
		switch dir {
		case bridge.DirectionCallerToAgent:
			inbound.WriteCallerPCM(pcm)
		case bridge.DirectionAgentToCaller:
			inbound.WriteAIPCM(pcm)
		}
	})

	h.mu.Lock()
	ab, ok = h.active[callID]
	if !ok || ab == nil || ab.pc != pc {
		h.mu.Unlock()
		if h.cfg.ReleaseTransferDedupe != nil {
			h.cfg.ReleaseTransferDedupe(callID)
		}
		br.Stop()
		_ = teardownWebSeat(callID, true)
		_ = pc.Close()
		return
	}
	ab.br = br
	h.mu.Unlock()

	br.Start()
	h.acdSetStateForCall(callID, "busy")
	if lg != nil {
		lg.Info("webseat: bridge started",
			zap.String("call_id", callID),
			zap.String("in_codec", ccIn.Codec),
			zap.Int("in_sr", ccIn.SampleRate),
			zap.String("web_rx_codec", webRxCodec.Codec),
			zap.String("web_tx_codec", webTxCodec.Codec),
			zap.Int("mid_sr", br.MidSampleRate()),
		)
	}
}

func mediaFromRTPCapability(cap webrtc.RTPCodecCapability) media.CodecConfig {
	mime := strings.ToLower(cap.MimeType)
	switch {
	case strings.Contains(mime, "pcmu") || strings.Contains(mime, "g711u"):
		return media.CodecConfig{Codec: "pcmu", SampleRate: 8000, Channels: 1, BitDepth: 8, FrameDuration: "20ms"}
	case strings.Contains(mime, "pcma") || strings.Contains(mime, "g711a"):
		return media.CodecConfig{Codec: "pcma", SampleRate: 8000, Channels: 1, BitDepth: 8, FrameDuration: "20ms"}
	default:
		return media.CodecConfig{Codec: "pcma", SampleRate: 8000, Channels: 1, BitDepth: 8, FrameDuration: "20ms"}
	}
}

func newMediaEngine() *webrtc.MediaEngine {
	me := &webrtc.MediaEngine{}
	// G.711 only for web seat (no Opus): PCMA preferred for CN/EU-style carriers; PCMU fallback.
	_ = me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA, ClockRate: 8000},
		PayloadType:        8,
	}, webrtc.RTPCodecTypeAudio)
	_ = me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
		PayloadType:        0,
	}, webrtc.RTPCodecTypeAudio)
	return me
}

func defaultICEServers() []webrtc.ICEServer {
	raw := utils.GetEnv("SIP_WEBSEAT_ICE_SERVERS")
	if raw != "" {
		var servers []webrtc.ICEServer
		if err := json.Unmarshal([]byte(raw), &servers); err == nil && len(servers) > 0 {
			return servers
		}
	}
	return []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}
}

func mediaFromRemoteTrack(tr *webrtc.TrackRemote) media.CodecConfig {
	c := tr.Codec()
	mime := strings.ToLower(c.MimeType)
	ch := int(c.Channels)
	if ch < 1 {
		ch = 1
	}
	switch {
	case strings.Contains(mime, "opus"):
		// Web seat MediaEngine no longer offers Opus; kept for forward-compat if SDP still lists it.
		decodeCh := ch
		if decodeCh > 2 {
			decodeCh = 2
		}
		return media.CodecConfig{
			Codec:              "opus",
			SampleRate:         int(c.ClockRate),
			Channels:           1,
			BitDepth:           16,
			FrameDuration:      "20ms",
			OpusDecodeChannels: decodeCh,
		}
	case strings.Contains(mime, "pcmu") || strings.Contains(mime, "g711u"):
		sr := int(c.ClockRate)
		if sr <= 0 {
			sr = 8000
		}
		return media.CodecConfig{
			Codec:         "pcmu",
			SampleRate:    sr,
			Channels:      1,
			BitDepth:      8,
			FrameDuration: "20ms",
		}
	case strings.Contains(mime, "pcma") || strings.Contains(mime, "g711a"):
		sr := int(c.ClockRate)
		if sr <= 0 {
			sr = 8000
		}
		return media.CodecConfig{
			Codec:         "pcma",
			SampleRate:    sr,
			Channels:      1,
			BitDepth:      8,
			FrameDuration: "20ms",
		}
	default:
		return media.CodecConfig{Codec: "pcma", SampleRate: 8000, Channels: 1, BitDepth: 8, FrameDuration: "20ms"}
	}
}
