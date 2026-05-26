package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/sip/session_timer"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
	"github.com/LinByte/VoiceServer/pkg/sip/transaction"
	"github.com/LinByte/VoiceServer/pkg/sip/voicedialog"
	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
	"go.uber.org/zap"
)

// SIPServer is the inbound SIP UAS (UDP primary; optional TCP/TLS in tcp_sig.go).
//
// Core methods include INVITE/ACK/BYE, outbound-oriented REFER (RFC 3515) with NOTIFY/sipfrag,
// REGISTER, SUBSCRIBE/NOTIFY + PUBLISH presence fan-out, session UPDATE, MESSAGE, SRTP SDES,
// re-INVITE for same-codec refresh, and partial RFC 3261 server-transaction behavior via pkg/sip/transaction.
//
// On ACK, inbound media attaches the HTTP WebSocket dialogue bridge (pkg/sip/voicedialog) unless transfer/WebSeat owns media.
type SIPServer struct {
	ep *stack.Endpoint

	txMgr        *transaction.Manager
	pendingInvMu sync.Mutex
	pendingInv   map[string]pendingInviteSnap

	localIP    string
	listenHost string
	listenPort int

	mu        sync.Mutex
	callStore map[string]*sipSession.CallSession // Call-ID -> call session

	regStoreMu sync.RWMutex
	regStore   SIPRegisterStore // optional: persisted REGISTER (sip_users), set via SetRegisterStore

	callPersistMu sync.RWMutex
	callPersist   SIPCallPersistStore // optional: SIPCall / recording / dialog persistence

	inboundTenantMu sync.RWMutex
	// Optional: resolve DID → tenant + sip_trunk_numbers row for inbound INVITEs.
	inboundDIDBindingLookup func(msg *stack.Message) InboundDIDBinding

	inboundCapMu sync.RWMutex
	// Optional: per-DID inbound capacity for this process (see TrunkCapacityTracker). Return ok=false to reject INVITE.
	inboundCapacityGate    func(callID, calledUser string) (ok bool, sipStatus int, reason string)
	inboundCapacityRelease func(callID string)

	// Default false: reject INVITE when inbound DID binding yields tenant_id=0 (unknown DID). Set true for demo / legacy single-tenant passthrough.
	inboundAllowUnknownDID atomic.Bool

	voiceWSLookupMu sync.RWMutex
	voiceWSLookup   func(callID string) string

	// Optional: outbound.Manager — drop UAC leg after BYE is fully
	// handled (transfer bridge / generic). The reasonClass argument
	// is the bounded enum from classifyBYEReason() so outbound can
	// stamp the CDR / metrics without re-parsing. Empty string means
	// "use default" (normal).
	outboundBYELegCleanup func(callID, reasonClass string)
	// Optional: map established transfer_bridge outbound Call-ID → inbound PSTN Call-ID (DialRequest.CorrelationID).
	transferBridgeInboundFromOutbound func(outboundCallID string) (inboundCallID string, ok bool)

	dlgMu  sync.RWMutex
	uasDlg map[string]*uasDialogState // inbound Call-ID -> dialog (for server-initiated BYE)

	inviteAllowNets  []*net.IPNet
	inviteRate       inviteRateState
	inviteRatePerSec float64
	inviteBurst      int
	inviteDigest     *sipDigestAuth

	inviteEnv inviteEnvConfig

	inviteFlights sync.Map // inviteFlightKey(req) -> *inviteFlightState (RFC 3262 / INVITE retransmit)
	inviteByCall  sync.Map // Call-ID -> *inviteFlightState

	// Final 200 OK replay for INVITE retransmissions (sync path and post-flight teardown).
	inviteFinal200Raw     sync.Map // inviteFlightKey -> raw 200 OK message string
	inviteFlightKeyByCall sync.Map // Call-ID -> inviteFlightKey (BYE clears cached final)

	sigCtx    context.Context
	sigCancel context.CancelFunc

	inviteBriefMu sync.RWMutex
	inviteBrief   map[string]inviteBrief // inbound Call-ID -> INVITE snapshot for dialog WS meta

	// VoiceServer-style decoupled handler interfaces (additive; nil-safe).
	// Registered via Set{Invite,DTMFSink,Transfer,CallLifecycle}Handler.
	// When nil, the existing direct calls into pkg/sip/conversation /
	// pkg/sip/voicedialog remain authoritative — these only take effect when
	// business code opts in. See pkg/sip/server/compat.go.
	inviteHandlerMu sync.RWMutex
	inviteHandler   InviteHandler

	dtmfSinkMu sync.RWMutex
	dtmfSink   DTMFSink

	transferHandlerMu sync.RWMutex
	transferHandler   TransferHandler

	callObserverMu sync.RWMutex
	callObserver   CallLifecycleObserver

	terminateMu    sync.Mutex
	terminateHooks map[string]func(reason string)

	// stir holds the inbound RFC 8224 Identity verification policy.
	// nil = STIR disabled (default). See server/stir.go for the
	// hook + soft/hard rejection knobs.
	stir *STIRConfig

	// dtlsAcceptInbound, when true, accepts DTLS-SRTP offers
	// (UDP/TLS/RTP/SAVP[F] + a=fingerprint + a=setup). Default off
	// because most carriers still use SDES — flip to on when
	// peering with WebRTC gateways. See dtls.go for the full
	// handshake lifecycle.
	dtlsAcceptInbound atomic.Bool

	// pendingDTLSMu / pendingDTLS hold per-call state populated by
	// handleInvite (cert, key, peer fingerprints, role) and consumed
	// by handleAck — the handshake itself runs on a goroutine off
	// the SIP signaling path so a 30-byte ClientHello can't stall
	// other dialogs.
	pendingDTLSMu sync.Mutex
	pendingDTLS   map[string]*dtlsPendingState
}

// SetSTIRConfig installs the inbound STIR/SHAKEN verification
// policy. Call before Start; concurrent modification after Start is
// safe but the new policy applies only to subsequent INVITEs.
func (s *SIPServer) SetSTIRConfig(cfg *STIRConfig) {
	if s == nil {
		return
	}
	s.stir = cfg
}

var (
	rtpPortAllocMu sync.Mutex
	rtpPortNext    int
)

// newInboundRTPSession allocates RTP UDP port based on env:
// - SIP_RTP_PORT: fixed single port
// - SIP_RTP_PORT_START/SIP_RTP_PORT_END: rotating range
// - fallback: ephemeral (port 0)
func newInboundRTPSession() (*rtp.Session, error) {
	if fixed, ok := envInt("SIP_RTP_PORT"); ok && fixed > 0 {
		logger.Info("sip rtp port policy: fixed", zap.Int("port", fixed))
		return rtp.NewSession(fixed)
	}
	start, hasStart := envInt("SIP_RTP_PORT_START")
	end, hasEnd := envInt("SIP_RTP_PORT_END")
	if hasStart && hasEnd && start > 0 && end >= start {
		logger.Info("sip rtp port policy: range", zap.Int("start", start), zap.Int("end", end))
		return newRTPSessionFromRange(start, end)
	}
	logger.Info("sip rtp port policy: ephemeral")
	return rtp.NewSession(0)
}

func envInt(name string) (int, bool) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func newRTPSessionFromRange(start, end int) (*rtp.Session, error) {
	rtpPortAllocMu.Lock()
	defer rtpPortAllocMu.Unlock()
	span := end - start + 1
	if span <= 0 {
		return nil, fmt.Errorf("invalid RTP port range: %d-%d", start, end)
	}
	if rtpPortNext < start || rtpPortNext > end {
		rtpPortNext = start
	}
	var lastErr error
	for i := 0; i < span; i++ {
		p := rtpPortNext
		rtpPortNext++
		if rtpPortNext > end {
			rtpPortNext = start
		}
		sess, err := rtp.NewSession(p)
		if err == nil {
			return sess, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("rtp range exhausted %d-%d: %w", start, end, lastErr)
}

func isPrivateIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 10:
		return true
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return true
	case v4[0] == 192 && v4[1] == 168:
		return true
	default:
		return false
	}
}

type Config struct {
	Host string
	Port int

	// localIP is used in SDP response's c=IN IP4 <localIP>.
	// If empty, server will try to use 127.0.0.1.
	LocalIP string

	// OnSIPResponse is optional: SIP responses on the listen socket (UAC / outbound legs).
	// Typically set to outbound.Manager.HandleSIPResponse.
	OnSIPResponse func(resp *stack.Message, addr *net.UDPAddr)

	// OutboundBYELegCleanup drops outbound.Manager leg state for
	// this Call-ID after BYE handling. The second argument is the
	// RFC 3326 reasonClass enum extracted from the BYE request
	// (sipMetrics.ByeReason*); empty string means caller didn't
	// know and the implementor should default to "normal". Optional.
	OutboundBYELegCleanup func(callID, reasonClass string)
	// TransferBridgeInboundFromOutbound maps established transfer_bridge outbound Call-ID to inbound PSTN Call-ID. Optional.
	TransferBridgeInboundFromOutbound func(outboundCallID string) (inboundCallID string, ok bool)
}

func New(cfg Config) *SIPServer {
	s := &SIPServer{
		localIP:                           strings.TrimSpace(cfg.LocalIP),
		listenHost:                        strings.TrimSpace(cfg.Host),
		listenPort:                        cfg.Port,
		callStore:                         make(map[string]*sipSession.CallSession),
		inviteBrief:                       make(map[string]inviteBrief),
		outboundBYELegCleanup:             cfg.OutboundBYELegCleanup,
		transferBridgeInboundFromOutbound: cfg.TransferBridgeInboundFromOutbound,
	}
	if s.localIP == "" {
		s.localIP = "127.0.0.1"
	}

	epCfg := stack.EndpointConfig{
		Host: cfg.Host,
		Port: cfg.Port,
		OnSIPResponse: func(resp *stack.Message, addr *net.UDPAddr) {
			if resp != nil && logger.Lg != nil {
				status := resp.StatusCode
				if status >= 180 || status >= 300 {
					logger.Lg.Info("sip response dispatch",
						zap.String("remote", addrString(addr)),
						zap.String("call_id", resp.GetHeader("Call-ID")),
						zap.String("cseq", resp.GetHeader("CSeq")),
						zap.Int("status", status),
						zap.String("reason", strings.TrimSpace(resp.StatusText)),
						zap.String("content_type", strings.TrimSpace(resp.GetHeader("Content-Type"))),
						zap.Int("content_length", len(resp.Body)),
						zap.String("body_preview", preview(resp.Body, 500)),
					)
				}
			}
			if cfg.OnSIPResponse != nil {
				cfg.OnSIPResponse(resp, addr)
			}
		},
		OnEvent: func(e stack.Event) {
			switch e.Type {
			case stack.EventDatagramReceived:
				logger.Debug("sip datagram received",
					zap.String("remote", addrString(e.Addr)),
					zap.Int("bytes", len(e.Raw)),
				)
			case stack.EventParseError:
				logger.Warn("sip parse error",
					zap.String("remote", addrString(e.Addr)),
					zap.Int("bytes", len(e.Raw)),
					zap.Error(e.Err),
				)
			case stack.EventRequestReceived:
				req := e.Request
				logger.Info("sip request received",
					zap.String("remote", addrString(e.Addr)),
					zap.String("method", safe(req, func(m *stack.Message) string { return m.Method })),
					zap.String("uri", safe(req, func(m *stack.Message) string { return m.RequestURI })),
					zap.String("call_id", safe(req, func(m *stack.Message) string { return m.GetHeader("Call-ID") })),
					zap.String("from", safe(req, func(m *stack.Message) string { return m.GetHeader("From") })),
					zap.String("to", safe(req, func(m *stack.Message) string { return m.GetHeader("To") })),
					zap.String("cseq", safe(req, func(m *stack.Message) string { return m.GetHeader("CSeq") })),
				)
			case stack.EventResponseSent:
				req := e.Request
				resp := e.Response
				logger.Info("sip response sent",
					zap.String("remote", addrString(e.Addr)),
					zap.String("method", safe(req, func(m *stack.Message) string { return m.Method })),
					zap.String("call_id", safe(req, func(m *stack.Message) string { return m.GetHeader("Call-ID") })),
					zap.Int("status", safeI(resp, func(m *stack.Message) int { return m.StatusCode })),
					zap.String("reason", safe(resp, func(m *stack.Message) string { return m.StatusText })),
				)
			case stack.EventResponseReceived:
				if e.Response != nil {
					logger.Debug("sip response received",
						zap.String("remote", addrString(e.Addr)),
						zap.String("call_id", e.Response.GetHeader("Call-ID")),
						zap.Int("status", e.Response.StatusCode),
					)
				}
			}
		},
	}
	s.ep = stack.NewEndpoint(epCfg)
	// Inbound counters wrap the real handlers so every response
	// flows through one observation point regardless of how many
	// early-return paths the handler has. Nil response = absorbed
	// retransmit or stateful 100 — we don't count those because
	// the underlying transaction already counted the first response.
	s.ep.RegisterHandler(stack.MethodInvite, func(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
		resp := s.handleInvite(msg, addr)
		if resp != nil && resp.StatusCode > 0 {
			sipMetrics.InviteResult(sipMetrics.DirectionInbound, resp.StatusCode)
		}
		return resp
	})
	s.ep.RegisterHandler(stack.MethodAck, s.handleAck)
	s.ep.RegisterHandler(stack.MethodBye, func(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
		resp := s.handleBye(msg, addr)
		// An inbound BYE arriving here means the REMOTE side
		// initiated the hangup. Classify by RFC 3326 Reason header
		// when present so dashboards split "normal hangup" from
		// "session timer expired" / "carrier rejected" cleanly.
		if resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			reasonClass, _ := classifyBYEReason(msg)
			sipMetrics.BYE(sipMetrics.DirectionInbound,
				sipMetrics.ByeByRemote, reasonClass)
			// Active-calls gauge / calls_total counter: pair the +1
			// from markInboundCallStarted (handleAck) with exactly
			// one -1 here. Status maps the bounded reason class onto
			// the calls_total status enum.
			callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
			markInboundCallEnded(callID, classToCallEndedStatus(reasonClass))
			// Inbound CDR: emit one JSON-Lines row per completed call.
			// No-op if CDR sink was never set.
			_, rawReasonText := classifyBYEReason(msg)
			trackInboundCallEnd(callID, "remote", reasonClass, rawReasonText)
		}
		return resp
	})
	s.ep.RegisterHandler(stack.MethodOptions, s.handleOptions)
	s.ep.RegisterHandler(stack.MethodRegister, s.handleRegister)
	s.ep.RegisterHandler(stack.MethodInfo, s.handleInfo)
	s.ep.RegisterHandler(stack.MethodCancel, s.handleCancel)
	s.ep.RegisterHandler(stack.MethodPublish, s.handlePublishPresence)
	s.ep.RegisterHandler(stack.MethodPrack, s.handlePrack)
	s.ep.RegisterHandler(stack.MethodSubscribe, s.handleSubscribe)
	s.ep.RegisterHandler(stack.MethodNotify, s.handleNotify)
	s.ep.RegisterHandler(stack.MethodRefer, s.handleRefer)
	s.ep.RegisterHandler(stack.MethodUpdate, s.handleUpdate)
	s.ep.RegisterHandler(stack.MethodMessage, s.handleMessage)
	s.ep.SetNoRouteHandler(func(_ *stack.Message, _ *net.UDPAddr) *stack.Message {
		return &stack.Message{
			IsRequest:  false,
			Version:    "SIP/2.0",
			StatusCode: 404,
			StatusText: "Not Found",
		}
	})

	s.inviteAllowNets = parseIPCIDRList(strings.TrimSpace(os.Getenv("SIP_INVITE_ALLOW_CIDRS")))
	if v := strings.TrimSpace(os.Getenv("SIP_INVITE_RATE_PER_SEC")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			s.inviteRatePerSec = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("SIP_INVITE_RATE_BURST")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.inviteBurst = n
		}
	}
	s.inviteDigest = newSIPDigest(
		os.Getenv("SIP_DIGEST_REALM"),
		os.Getenv("SIP_DIGEST_USER"),
		os.Getenv("SIP_DIGEST_PASSWORD"),
	)
	s.inviteEnv = parseInviteEnvConfig()
	s.wireTransactionLayer()
	return s
}

// SetRegisterStore wires DB-backed REGISTER persistence and INVITE proxy lookup.
// Safe to call before Start; typically once at process init.
func (s *SIPServer) SetRegisterStore(st SIPRegisterStore) {
	if s == nil {
		return
	}
	s.regStoreMu.Lock()
	defer s.regStoreMu.Unlock()
	s.regStore = st
}

func (s *SIPServer) registerStore() SIPRegisterStore {
	if s == nil {
		return nil
	}
	s.regStoreMu.RLock()
	defer s.regStoreMu.RUnlock()
	return s.regStore
}

// SetCallPersist wires DB-backed SIP call / session persistence and recording upload on BYE.
func (s *SIPServer) SetCallPersist(st SIPCallPersistStore) {
	if s == nil {
		return
	}
	s.callPersistMu.Lock()
	defer s.callPersistMu.Unlock()
	s.callPersist = st
}

func (s *SIPServer) callPersistStore() SIPCallPersistStore {
	if s == nil {
		return nil
	}
	s.callPersistMu.RLock()
	defer s.callPersistMu.RUnlock()
	return s.callPersist
}

func (s *SIPServer) resolveInboundTenant(msg *stack.Message) uint {
	return s.resolveInboundDIDBinding(msg).TenantID
}

func (s *SIPServer) resolveInboundDIDBinding(msg *stack.Message) InboundDIDBinding {
	if s == nil || msg == nil {
		return InboundDIDBinding{}
	}
	s.inboundTenantMu.RLock()
	fn := s.inboundDIDBindingLookup
	s.inboundTenantMu.RUnlock()
	if fn == nil {
		return InboundDIDBinding{}
	}
	b := fn(msg)
	return b
}

// SetInboundDIDBindingResolver wires tenant + trunk_number resolution from the called-party (DID).
func (s *SIPServer) SetInboundDIDBindingResolver(fn func(msg *stack.Message) InboundDIDBinding) {
	if s == nil {
		return
	}
	s.inboundTenantMu.Lock()
	defer s.inboundTenantMu.Unlock()
	s.inboundDIDBindingLookup = fn
}

// SetInboundCapacityGate wires trunk-number inbound concurrency enforcement (process-local; pair with SetInboundCapacityRelease).
func (s *SIPServer) SetInboundCapacityGate(fn func(callID, calledUser string) (ok bool, sipStatus int, reason string)) {
	if s == nil {
		return
	}
	s.inboundCapMu.Lock()
	defer s.inboundCapMu.Unlock()
	s.inboundCapacityGate = fn
}

// SetInboundCapacityRelease releases a capacity slot acquired by the gate (idempotent per Call-ID).
func (s *SIPServer) SetInboundCapacityRelease(fn func(callID string)) {
	if s == nil {
		return
	}
	s.inboundCapMu.Lock()
	defer s.inboundCapMu.Unlock()
	s.inboundCapacityRelease = fn
}

func (s *SIPServer) releaseInboundCapacity(callID string) {
	if s == nil || strings.TrimSpace(callID) == "" {
		return
	}
	s.inboundCapMu.RLock()
	fn := s.inboundCapacityRelease
	s.inboundCapMu.RUnlock()
	if fn != nil {
		fn(strings.TrimSpace(callID))
	}
}

// SetInboundAllowUnknownDID controls INVITE handling when tenant/DID resolution yields 0.
// false (default): respond 404; true: accept and persist tenant_id=0 (legacy demo behavior).
func (s *SIPServer) SetInboundAllowUnknownDID(v bool) {
	if s == nil {
		return
	}
	s.inboundAllowUnknownDID.Store(v)
}

func (s *SIPServer) inboundAllowsUnknownDID() bool {
	if s == nil {
		return false
	}
	return s.inboundAllowUnknownDID.Load()
}

// SetVoiceDialogWSLookup returns optional per-call wss/ws base URL for voicedialog client dial (merged with token & call_id query).
func (s *SIPServer) SetVoiceDialogWSLookup(fn func(callID string) string) {
	if s == nil {
		return
	}
	s.voiceWSLookupMu.Lock()
	defer s.voiceWSLookupMu.Unlock()
	s.voiceWSLookup = fn
}

func (s *SIPServer) lookupVoiceDialogWS(callID string) string {
	if s == nil {
		return ""
	}
	s.voiceWSLookupMu.RLock()
	fn := s.voiceWSLookup
	s.voiceWSLookupMu.RUnlock()
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(fn(callID))
}

func addrString(a *net.UDPAddr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

func safe(m *stack.Message, f func(*stack.Message) string) string {
	if m == nil || f == nil {
		return ""
	}
	return f(m)
}

func safeI(m *stack.Message, f func(*stack.Message) int) int {
	if m == nil || f == nil {
		return 0
	}
	return f(m)
}

func preview(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || s == "" {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func ensureToTag(to string) string {
	to = strings.TrimSpace(to)
	if to == "" {
		return to
	}
	if strings.Contains(strings.ToLower(to), "tag=") {
		return to
	}
	return to + ";tag=" + newTag()
}

func newTag() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "srvtag"
	}
	return hex.EncodeToString(b[:])
}

func (s *SIPServer) ensureSigCtx() {
	if s == nil || s.sigCancel != nil {
		return
	}
	s.sigCtx, s.sigCancel = context.WithCancel(context.Background())
}

func (s *SIPServer) Start() error {
	if s == nil || s.ep == nil {
		return fmt.Errorf("sip: server not ready")
	}
	s.ensureSigCtx()
	if err := s.ep.Open(); err != nil {
		return fmt.Errorf("sip: endpoint open: %w", err)
	}
	if la := s.ep.ListenAddr(); la != nil {
		if u, ok := la.(*net.UDPAddr); ok && u != nil && u.Port > 0 {
			s.listenPort = u.Port
		}
	}
	go func() {
		_ = s.ep.Serve(s.sigCtx)
	}()
	s.startSigTransportListeners()
	return nil
}

func (s *SIPServer) Stop() error {
	if s == nil {
		return nil
	}

	if s.sigCancel != nil {
		s.sigCancel()
		s.sigCancel = nil
		s.sigCtx = nil
	}

	s.mu.Lock()
	for callID, cs := range s.callStore {
		s.endVoiceDialogBridge(callID)
		conversation.CleanupCallState(callID)
		if cs != nil {
			cs.Stop()
		}
		delete(s.callStore, callID)
	}
	s.mu.Unlock()

	if s.ep != nil {
		_ = s.ep.Close()
	}
	return nil
}

// StartInviteHandler is exported for unit tests.
func (s *SIPServer) StartInviteHandler(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	return s.handleInvite(msg, addr)
}

// StartByeHandler is exported for unit tests.
func (s *SIPServer) StartByeHandler(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	return s.handleBye(msg, addr)
}

func (s *SIPServer) StartAckHandler(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	return s.handleAck(msg, addr)
}

func (s *SIPServer) handleInvite(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != "INVITE" {
		return nil
	}

	callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
	if callID == "" {
		// SIP requires Call-ID; if absent respond 400.
		return s.makeResponse(msg, 400, "Bad Request", "", "")
	}

	if s.absorbInviteRetransmit(msg, addr) {
		return nil
	}

	if addr != nil && addr.IP != nil {
		if !ipAllowed(s.inviteAllowNets, addr.IP) {
			return s.makeResponse(msg, 403, "Forbidden", "", "")
		}
		if !s.inviteRate.allow(addr.IP, s.inviteRatePerSec, s.inviteBurst) {
			return s.makeResponse(msg, 503, "Service Unavailable", "", "")
		}
	}
	if s.inviteDigest != nil && !s.inviteDigest.verifyINVITE(msg) {
		resp, err := s.inviteDigest.challenge401(msg)
		if err != nil || resp == nil {
			return s.makeResponse(msg, 500, "Internal Server Error", "", "")
		}
		return resp
	}

	if cs := s.GetCallSession(callID); cs != nil && headerHasToTag(msg.GetHeader("To")) {
		if s.ep != nil && addr != nil {
			trying := s.makeResponse(msg, 100, "Trying", "", "")
			_ = s.ep.Send(trying, addr)
		}
		return s.handleReInvite(msg, addr, cs)
	}

	// Provisional response: 100 Trying (helps many clients' state machines).
	if s.ep != nil && addr != nil {
		trying := s.makeResponse(msg, 100, "Trying", "", "")
		_ = s.ep.Send(trying, addr)
	}

	fk := inviteFlightKey(msg)
	if fk != "" {
		if raw, ok := s.inviteFinal200Raw.Load(fk); ok {
			if rs, ok2 := raw.(string); ok2 && strings.TrimSpace(rs) != "" && s.ep != nil && addr != nil {
				if m, err := stack.Parse(rs); err == nil && m != nil {
					_ = s.ep.Send(m, addr)
				}
			}
			return nil
		}
		if v, ok := s.inviteFlights.Load(fk); ok {
			s.resendInviteProgress(v.(*inviteFlightState), addr)
			return nil
		}
	}

	// RFC 4028 Session-Timer negotiation. Done BEFORE SDP parsing so a
	// 422 "Session Interval Too Small" gets out fast without allocating
	// RTP / CallSession (peer is expected to retry with a higher SE).
	stDecision := s.negotiateInboundSessionTimer(msg)
	if stDecision.Reject422 {
		resp := s.makeResponse(msg, 422, "Session Interval Too Small", "", "")
		resp.SetHeader("Min-SE", session_timer.FormatMinSE(stDecision.MinSE))
		return resp
	}

	// RFC 8224 SHAKEN inbound verification (no-op when STIR disabled).
	// Done BEFORE SDP parsing for the same reason as Session-Timer:
	// a hard-rejected call doesn't need RTP allocation. The hook
	// inside fires for every verdict (pass + fail) so the call-center
	// layer can still attach the verdict to its CDR even on accept.
	if reject := s.verifyInboundIdentity(msg); reject != nil {
		return reject
	}

	// Parse remote RTP endpoint from SDP.
	offer, err := sdp.Parse(msg.Body)
	if err != nil {
		logger.Warn("sip invite rejected (sdp not acceptable)",
			zap.String("call_id", callID),
			zap.String("content_type", msg.GetHeader("Content-Type")),
			zap.Int("content_length", len(msg.Body)),
			zap.Error(err),
			zap.String("sdp_preview", preview(msg.Body, 800)),
		)
		return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
	}

	// SAVP-family acceptance check.
	//
	// Two flavours both share "SAVP" in proto:
	//   * RTP/SAVP[F]            — SDES (RFC 4568); a=crypto required
	//   * UDP/TLS/RTP/SAVP[F]    — DTLS-SRTP (RFC 5764); a=fingerprint
	//
	// We check DTLS first because IsDTLSTransport is a stricter
	// match — when it's true we skip the SDES check entirely.
	if sdp.IsDTLSTransport(offer.Proto) {
		if !s.dtlsAcceptInbound.Load() {
			logger.Warn("sip invite rejected (DTLS-SRTP not enabled on this server)",
				zap.String("call_id", callID),
				zap.String("proto", offer.Proto))
			return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
		}
		if len(offer.Fingerprints) == 0 {
			logger.Warn("sip invite rejected (DTLS-SRTP offer missing a=fingerprint)",
				zap.String("call_id", callID))
			return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
		}
	} else if strings.Contains(strings.ToUpper(offer.Proto), "SAVP") {
		if _, ok := sdp.PickSupportedSDESOffer(offer.CryptoOffers); !ok {
			logger.Warn("sip invite rejected (SRTP media without usable a=crypto)",
				zap.String("call_id", callID),
				zap.String("proto", offer.Proto),
			)
			return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
		}
	}

	// REGISTERed AOR: proxy INVITE to that UA (same host: different user in Request-URI).
	if u, h, ok := parseURIUserHost(msg.RequestURI); ok {
		if st := s.registerStore(); st != nil {
			dst, found, lerr := st.LookupRegister(context.Background(), u, h)
			if lerr != nil {
				logger.Warn("sip invite lookup register failed",
					zap.String("call_id", callID),
					zap.String("aor", registrationKey(u, h)),
					zap.Error(lerr),
				)
			} else if found && dst != nil {
				if err := s.proxyInviteToRegistrar(msg, dst); err != nil {
					logger.Warn("sip invite proxy to registered UA failed",
						zap.String("call_id", callID),
						zap.String("aor", registrationKey(u, h)),
						zap.Error(err),
					)
				} else {
					logger.Info("sip invite proxied to registered UA",
						zap.String("call_id", callID),
						zap.String("aor", registrationKey(u, h)),
						zap.String("dst", dst.String()),
					)
					return nil
				}
			}
		}
	}

	tidInbound := s.resolveInboundTenant(msg)
	// DID not bound to any tenant: still answer the call; ACK path plays scripts/not_bind.wav then BYE.
	if tidInbound == 0 {
		logger.Warn("sip invite: inbound DID not bound to a tenant (will play not_bind on ACK)",
			zap.String("call_id", callID),
			zap.String("request_uri", msg.RequestURI),
		)
	}

	rawCalled := InboundCalledPartyUser(msg)
	capacityAcquired := false
	if rawCalled != "" {
		s.inboundCapMu.RLock()
		gate := s.inboundCapacityGate
		s.inboundCapMu.RUnlock()
		if gate != nil {
			ok, code, reason := gate(callID, rawCalled)
			if !ok {
				if code <= 0 {
					code = 486
				}
				r := strings.TrimSpace(reason)
				if r == "" {
					r = "Busy Here"
				}
				logger.Warn("sip invite rejected (inbound capacity)",
					zap.String("call_id", callID),
					zap.String("called_user", rawCalled),
					zap.Int("sip_status", code),
				)
				return s.makeResponse(msg, code, r, "", "")
			}
			capacityAcquired = true
		}
	}
	capacityEstablishOK := false
	defer func() {
		if capacityAcquired && !capacityEstablishOK {
			s.releaseInboundCapacity(callID)
		}
	}()

	toTagEarly := ensureToTag(msg.GetHeader("To"))
	s.registerPendingInvite(msg, addr, toTagEarly)

	remoteIP := net.ParseIP(offer.IP)
	if remoteIP == nil || offer.Port <= 0 {
		return s.makeResponse(msg, 400, "Bad Request", "", "")
	}

	remoteAddr := &net.UDPAddr{IP: remoteIP, Port: offer.Port}
	// NAT-friendly fallback: if SDP c= carries a private IPv4 but signaling source is public/reachable,
	// use signaling source IP with SDP media port as initial RTP destination.
	if addr != nil && isPrivateIPv4(remoteIP) && addr.IP != nil && addr.IP.To4() != nil {
		if !isPrivateIPv4(addr.IP) {
			remoteAddr = &net.UDPAddr{IP: addr.IP, Port: offer.Port}
			logger.Info("sip invite media target overridden (private SDP IP fallback)",
				zap.String("call_id", callID),
				zap.String("sdp_remote_ip", remoteIP.String()),
				zap.String("sip_source_ip", addr.IP.String()),
				zap.Int("media_port", offer.Port),
				zap.String("chosen_remote_rtp", remoteAddr.String()),
			)
		}
	}

	needAsync := s.inviteNeedsAsync(msg)
	var flight *inviteFlightState
	if needAsync && fk != "" {
		s.ensureSigCtx()
		flight = &inviteFlightState{
			flightKey: fk,
			callID:    callID,
			prackDone: make(chan struct{}, 1),
		}
		if v, loaded := s.inviteFlights.LoadOrStore(fk, flight); loaded {
			s.resendInviteProgress(v.(*inviteFlightState), addr)
			return nil
		}
	}

	// Allocate RTP session by env policy:
	// fixed port / range (for firewall-friendly deployments) or ephemeral fallback.
	rtpSess, err := newInboundRTPSession()
	if err != nil {
		if flight != nil && fk != "" {
			s.inviteFlights.Delete(fk)
		}
		return s.makeResponse(msg, 500, "Internal Server Error", "", "")
	}
	rtpSess.SetRemoteAddr(remoteAddr)

	var sdpExtras []string
	// SDES (RFC 4568) negotiation. PickSupportedSDESOffer prefers
	// AES_CM_128_HMAC_SHA1_80 (WebRTC default) but falls back to
	// _32 (Cisco/Avaya interop) when the peer didn't list _80.
	// Both suites use 16-byte key + 14-byte salt — only the auth
	// tag length differs — so the wire material is identical and
	// the suite is purely a pion-profile selection.
	if co, ok := sdp.PickSupportedSDESOffer(offer.CryptoOffers); ok && strings.Contains(strings.ToUpper(offer.Proto), "SAVP") && !sdp.IsDTLSTransport(offer.Proto) {
		prof, profOK := sdp.PionProfileForSuite(co.Suite)
		if !profOK {
			_ = rtpSess.Close()
			if flight != nil && fk != "" {
				s.inviteFlights.Delete(fk)
			}
			logger.Warn("sip invite rejected (unsupported SRTP suite)",
				zap.String("call_id", callID),
				zap.String("suite", co.Suite))
			return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
		}
		rk, rsalt, err := sdp.DecodeSDESInline(co.KeyParams)
		if err != nil {
			_ = rtpSess.Close()
			if flight != nil && fk != "" {
				s.inviteFlights.Delete(fk)
			}
			logger.Warn("sip invite rejected (invalid SRTP inline)", zap.String("call_id", callID), zap.Error(err))
			return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
		}
		lk := make([]byte, 16)
		lsalt := make([]byte, 14)
		if _, err := rand.Read(lk); err != nil {
			_ = rtpSess.Close()
			if flight != nil && fk != "" {
				s.inviteFlights.Delete(fk)
			}
			return s.makeResponse(msg, 500, "Internal Server Error", "", "")
		}
		if _, err := rand.Read(lsalt); err != nil {
			_ = rtpSess.Close()
			if flight != nil && fk != "" {
				s.inviteFlights.Delete(fk)
			}
			return s.makeResponse(msg, 500, "Internal Server Error", "", "")
		}
		// Echo the peer's chosen suite back. Mixing suites within
		// one m=audio block violates RFC 4568 §6.1.
		cryptoLine, err := sdp.FormatCryptoLine(co.Tag, co.Suite, lk, lsalt)
		if err != nil {
			_ = rtpSess.Close()
			if flight != nil && fk != "" {
				s.inviteFlights.Delete(fk)
			}
			logger.Warn("sip invite rejected (SRTP answer crypto)", zap.String("call_id", callID), zap.Error(err))
			return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
		}
		sdpExtras = append(sdpExtras, cryptoLine)
		if err := rtpSess.EnableSDESSRTPWithProfile(prof, rk, rsalt, lk, lsalt); err != nil {
			_ = rtpSess.Close()
			if flight != nil && fk != "" {
				s.inviteFlights.Delete(fk)
			}
			logger.Warn("sip invite SRTP enable failed", zap.String("call_id", callID), zap.Error(err))
			return s.makeResponse(msg, 500, "Internal Server Error", "", "")
		}
	}

	// RFC 5763/5764 DTLS-SRTP answer prep. Mints our cert, renders
	// `a=fingerprint:` + `a=setup:` extras, and stashes the per-call
	// material so handleAck can drive the handshake. No-op when the
	// offer isn't DTLS or inbound DTLS isn't enabled.
	if dtlsAns, derr := s.prepareDTLSAnswer(offer); derr != nil {
		_ = rtpSess.Close()
		if flight != nil && fk != "" {
			s.inviteFlights.Delete(fk)
		}
		logger.Warn("sip invite rejected (DTLS-SRTP prep failed)",
			zap.String("call_id", callID),
			zap.Error(derr))
		return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
	} else if dtlsAns != nil {
		sdpExtras = append(sdpExtras, dtlsAns.ExtraLines...)
		s.stashPendingDTLS(callID, dtlsAns.Pending)
	}

	// Store session for BYE.
	s.mu.Lock()
	// If a session already exists for this call, stop it first (idempotency-ish).
	if old := s.callStore[callID]; old != nil {
		old.Stop()
	}
	cs, err := sipSession.NewCallSession(callID, rtpSess, offer.Codecs)
	if err != nil {
		s.mu.Unlock()
		_ = rtpSess.Close()
		if flight != nil && fk != "" {
			s.inviteFlights.Delete(fk)
		}
		logger.Warn("sip invite rejected (no supported codec)",
			zap.String("call_id", callID),
			zap.Any("offered_codecs", offer.Codecs),
			zap.Error(err),
		)
		return s.makeResponse(msg, 488, "Not Acceptable Here", "", "")
	}
	s.callStore[callID] = cs
	s.mu.Unlock()

	remoteSig := ""
	if addr != nil {
		remoteSig = addr.String()
	}
	fromHdr := msg.GetHeader("From")
	s.storeInviteBrief(callID, fromHdr, msg.GetHeader("To"), remoteSig)
	// Hand the raw From over to the CallSession so transfer-to-agent can
	// display the original PSTN caller's number on the agent's phone
	// instead of the platform's trunk number (e.g. 400-xxx).
	cs.SetRemoteFromHeader(fromHdr)
	// Capture To + RFC 7044 History-Info + RFC 5806 Diversion off the
	// inbound INVITE so the transfer path can extend the retarget chain
	// when this call is B2BUA'd onward (see pkg/sip/conversation/transfer.go).
	cs.SetInboundRetargetHeaders(
		msg.GetHeader("To"),
		msg.GetHeader("History-Info"),
		msg.GetHeader("Diversion"),
	)
	// Arm RFC 4028 watchdog: if the peer doesn't refresh us within
	// ChosenSE seconds (re-INVITE / UPDATE), hang up the call per
	// §10. handleReInvite / handleUpdate calls cs.TouchSessionTimer()
	// to reset on every refresh.
	if stDecision.IsActive() {
		dur := time.Duration(stDecision.ChosenSE) * time.Second
		armedCallID := callID
		cs.ArmSessionTimerWatchdog(dur, func() {
			logger.Warn("sip session timer expired; hanging up",
				zap.String("call_id", armedCallID),
				zap.Int("se_seconds", stDecision.ChosenSE),
				zap.String("refresher", string(stDecision.Refresher)))
			s.HangupInboundCall(armedCallID)
		})
	}

	bind := s.resolveInboundDIDBinding(msg)
	cs.SetTenantID(bind.TenantID)
	cs.SetInboundUnboundTenant(bind.TenantID == 0)

	// IMPORTANT: Do not start media until ACK (call established).

	localPort := rtpSess.LocalAddr.Port

	if p := s.callPersistStore(); p != nil {
		neg := cs.NegotiatedCodec()
		p.OnInvite(context.Background(), InvitePersistParams{
			TenantID:             bind.TenantID,
			InboundTrunkNumberID: bind.TrunkNumberID,
			CallID:               callID,
			From:                 msg.GetHeader("From"),
			To:                   msg.GetHeader("To"),
			RemoteSig:            addr.String(),
			RemoteRTP:            remoteAddr.String(),
			LocalRTP:             fmt.Sprintf("%s:%d", s.localIP, localPort),
			Codec:                neg.Name,
			PayloadType:          neg.PayloadType,
			ClockRate:            neg.ClockRate,
			CSeqInvite:           msg.GetHeader("CSeq"),
			Direction:            "inbound",
		})
	}

	// Reply with negotiated audio codec; add telephone-event from offer so UAs can send RFC 2833 DTMF.
	neg := cs.NegotiatedCodec()
	codecs := []sdp.Codec{neg}
	if te, ok := sdp.PickTelephoneEventFromOffer(offer.Codecs, neg.ClockRate); ok {
		codecs = append(codecs, te)
	}
	respSDP := sdp.GenerateWithProtoExtras(s.localIP, localPort, offer.Proto, codecs, sdpExtras)

	// Use a single To-tag consistently across provisional/final responses.
	toWithTag := ensureToTag(msg.GetHeader("To"))

	respMsg := s.makeResponse(msg, 200, "OK", respSDP, toWithTag)
	respMsg.SetHeader("Content-Type", "application/sdp")
	respMsg.SetHeader("To", toWithTag)
	// For dialog establishment many clients expect a Contact header from UAS.
	// Use SDP local-ip as a reachable contact host.
	respMsg.SetHeader("Contact", fmt.Sprintf("<sip:server@%s:%d>", s.localIP, s.listenPort))
	respMsg.SetHeader("Allow", strings.Join([]string{
		stack.MethodInvite,
		stack.MethodAck,
		stack.MethodBye,
		stack.MethodRegister,
		stack.MethodOptions,
		stack.MethodCancel,
		stack.MethodInfo,
		stack.MethodPrack,
		stack.MethodSubscribe,
		stack.MethodNotify,
		stack.MethodPublish,
		stack.MethodRefer,
		stack.MethodMessage,
		stack.MethodUpdate,
	}, ", "))
	// RFC 4028 Session-Timer: echo Session-Expires + advertise Min-SE
	// + Supported: timer on the 200 OK so the peer knows we're playing
	// along and which side owns refresh. Watchdog is armed AFTER the
	// CallSession is registered (further down) so the BYE callback
	// can find it.
	if stDecision.IsActive() {
		respMsg.SetHeader("Session-Expires",
			session_timer.FormatSessionExpires(stDecision.ChosenSE, stDecision.Refresher))
		respMsg.SetHeader("Min-SE", session_timer.FormatMinSE(stDecision.MinSE))
		// Merge "timer" into Supported header (don't overwrite an existing one).
		mergeSupportedToken(respMsg, session_timer.SupportedTokenTimer)
		if stDecision.RequireTimer {
			respMsg.SetHeader("Require", session_timer.SupportedTokenTimer)
		}
	}
	respMsg.SetHeader("Content-Length", strconv.Itoa(stack.BodyBytesLen(respSDP)))

	logger.Info("sip invite negotiated",
		zap.String("call_id", callID),
		zap.String("remote_rtp", remoteAddr.String()),
		zap.String("answer_proto", offer.Proto),
		zap.Any("offered_codecs", offer.Codecs),
		zap.Any("negotiated_codec", neg),
	)
	if addr != nil {
		s.rememberUASDialog(callID, addr, msg, toWithTag)
	}

	if needAsync && flight != nil {
		reliable := inviteReliable(s.inviteEnv, msg)
		sdp183 := ""
		if reliable && s.inviteEnv.EarlyMediaSDP {
			sdp183 = respSDP
		}
		flight.inviteCSeq = stack.ParseCSeqNum(msg.GetHeader("CSeq"))
		if !reliable {
			flight.awaitRSeq = 0
		}
		s.inviteByCall.Store(callID, flight)
		capacityEstablishOK = true
		go s.runInviteAsync(msg, addr, flight, respMsg, reliable, sdp183, callID)
		return nil
	}

	// Provisional response: 180 Ringing (often expected by softphones).
	if s.ep != nil && addr != nil {
		ringing := s.makeResponse(msg, 180, "Ringing", "", toWithTag)
		ringing.SetHeader("To", toWithTag)
		ringing.SetHeader("Contact", fmt.Sprintf("<sip:server@%s:%d>", s.localIP, s.listenPort))
		ringing.SetHeader("Content-Length", "0")
		_ = s.ep.Send(ringing, addr)
	}

	if fk != "" {
		s.inviteFinal200Raw.Store(fk, respMsg.String())
		s.inviteFlightKeyByCall.Store(callID, fk)
	}

	capacityEstablishOK = true
	return respMsg
}

func (s *SIPServer) handleAck(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != stack.MethodAck {
		return nil
	}
	if s.txMgr != nil {
		_ = s.txMgr.HandleAck(msg, addr)
	}
	callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
	if callID == "" {
		return nil
	}

	s.inviteAsyncEnd(callID)

	s.mu.Lock()
	cs := s.callStore[callID]
	s.mu.Unlock()
	if cs != nil {
		// After transfer, media is bridged (raw RTP or PCM transcode); do not attach ASR/TTS again
		// (e.g. late or duplicate ACK / re-INVITE ACK would hit a cancelled MediaSession).
		tb := conversation.ActiveTransferBridgeForCallID(callID)
		wsSeat := conversation.ActiveWebSeatSession(callID)
		if tb || wsSeat {
			logger.Info("sip inbound ACK: skipping AI/voicedialog voice attach (transfer or web seat owns media)",
				zap.String("call_id", callID),
				zap.Bool("transfer_bridge_active", tb),
				zap.Bool("webseat_pending_or_active", wsSeat),
			)
			return nil
		}
		if cs.InboundUnboundTenant() {
			zl := logger.Lg
			if err := conversation.AttachInboundNotBoundPlayback(context.Background(), cs, zl); err != nil {
				logger.Warn("sip inbound not_bind playback attach failed",
					zap.String("call_id", callID),
					zap.Error(err),
				)
			}
			if p := s.callPersistStore(); p != nil {
				p.OnEstablished(context.Background(), callID)
			}
			return nil
		}
		fromH, toH, remSig := s.peekInviteBrief(callID)
		voiceURL := s.lookupVoiceDialogWS(callID)
		if err := voicedialog.AttachInboundVoiceDialog(context.Background(), cs, fromH, toH, remSig, voiceURL); err != nil {
			logger.Warn("sip inbound voicedialog attach failed; playing config_error then BYE",
				zap.String("call_id", callID),
				zap.Error(err),
			)
			if err2 := conversation.AttachInboundTenantAIConfigErrorPlayback(cs, logger.Lg); err2 != nil {
				logger.Warn("sip inbound config_error playback attach failed",
					zap.String("call_id", callID),
					zap.Error(err2),
				)
			}
		} else {
			logger.Info("sip inbound voice attached",
				zap.String("call_id", callID),
				zap.String("mode", "voicedialog_ws"),
			)
		}
		if p := s.callPersistStore(); p != nil {
			p.OnEstablished(context.Background(), callID)
		}
		// RFC 5764 DTLS-SRTP: if the INVITE answer negotiated DTLS,
		// kick off the handshake on a goroutine BEFORE StartOnACK so
		// the demux route is installed when the peer's ClientHello
		// arrives. Media-pipeline start is independent — the Session
		// will silently drop pre-handshake SRTP-looking bytes until
		// EnableDTLSSRTP installs the contexts.
		if pending := s.takePendingDTLS(callID); pending != nil {
			go s.runInboundDTLSHandshake(callID, cs.RTPSession(), pending)
		}
		// Active-calls gauge: bump on the first ACK we see for this
		// Call-ID. Idempotent against ACK retransmits.
		markInboundCallStarted(callID)
		// Inbound CDR: stamp the start moment so end-of-call emits a
		// JSON-Lines row into the configured sink. No-op when CDR is
		// disabled (sink unset). Codec is the negotiated one; from /
		// to are stored in inviteBrief.
		cdrFromH, cdrToH, _ := s.peekInviteBrief(callID)
		cdrCodec := strings.ToLower(cs.SourceCodec().Codec)
		trackInboundCallStart(callID, cdrCodec, cdrFromH, cdrToH, "", 0)
		cs.StartOnACK()
	}
	// ACK has no SIP response.
	return nil
}

func (s *SIPServer) handleBye(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != "BYE" {
		return nil
	}
	// Parse Reason header (RFC 3326) once; the bounded enum is
	// passed to the outbound cleanup callback so it can stamp the
	// CDR / metrics without re-parsing.
	byeReasonClass, _ := classifyBYEReason(msg)
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
	if callID == "" {
		return s.makeResponse(msg, 400, "Bad Request", "", "")
	}
	defer s.endVoiceDialogBridge(callID)
	defer s.inviteFinalRetransmitCleanup(callID)
	defer conversation.CleanupCallState(callID)

	if tb := conversation.HangupTransferBridgeIfAny(callID); tb != nil {
		s.forgetUASDialog(callID)
		s.releaseInboundCapacity(tb.InboundCallID)
		conversation.CleanupCallState(tb.InboundCallID)
		s.endVoiceDialogBridge(tb.InboundCallID)
		if s.outboundBYELegCleanup != nil && strings.TrimSpace(tb.OutboundCallID) != "" {
			s.outboundBYELegCleanup(tb.OutboundCallID, byeReasonClass)
		}
		if p := s.callPersistStore(); p != nil {
			go p.OnBye(context.Background(), ByePersistParams{
				CallID:             tb.InboundCallID,
				RawPayload:         tb.RawPayload,
				CodecName:          tb.CodecName,
				Initiator:          tb.Initiator,
				RecordSampleRate:   tb.RecordSampleRate,
				RecordOpusChannels: tb.RecordOpusChannels,
			})
		}
		return s.makeResponse(msg, 200, "OK", "", "")
	}
	if s.transferBridgeInboundFromOutbound != nil {
		if inbound, ok := s.transferBridgeInboundFromOutbound(callID); ok && inbound != "" {
			if logger.Lg != nil {
				logger.Lg.Info("sip transfer bridge: outbound remote BYE fallback (bridge map miss)",
					zap.String("outbound_call_id", callID),
					zap.String("inbound_call_id", inbound))
			}
			tb := conversation.TeardownTransferBridgeOnOutboundRemoteByeFallback(inbound, callID)
			s.forgetUASDialog(callID)
			s.releaseInboundCapacity(inbound)
			conversation.CleanupCallState(inbound)
			s.endVoiceDialogBridge(inbound)
			if s.outboundBYELegCleanup != nil {
				s.outboundBYELegCleanup(callID, byeReasonClass)
			}
			if tb != nil && s.callPersistStore() != nil {
				go s.callPersistStore().OnBye(context.Background(), ByePersistParams{
					CallID:             tb.InboundCallID,
					RawPayload:         tb.RawPayload,
					CodecName:          tb.CodecName,
					Initiator:          tb.Initiator,
					RecordSampleRate:   tb.RecordSampleRate,
					RecordOpusChannels: tb.RecordOpusChannels,
				})
			}
			return s.makeResponse(msg, 200, "OK", "", "")
		}
	}
	if conversation.HangupWebSeatBridgeIfAny(callID) {
		s.forgetUASDialog(callID)
		s.releaseInboundCapacity(callID)
		return s.makeResponse(msg, 200, "OK", "", "")
	}

	s.mu.Lock()
	cs := s.callStore[callID]
	delete(s.callStore, callID)
	s.mu.Unlock()

	var raw []byte
	var codec string
	var recSR, recOpusCh int
	var wavRec gateway.RecordingInfo
	var rtcpSnap rtp.RTCPStats
	if cs != nil {
		rtcpSnap = cs.RTCPStats()
		raw = cs.TakeRecording()
		codec = cs.NegotiatedCodec().Name
		src := cs.SourceCodec()
		recSR = src.SampleRate
		recOpusCh = src.OpusDecodeChannels
		if recOpusCh < 1 {
			recOpusCh = src.Channels
		}
		// Flush the new stereo PCM recorder before stopping the call
		// session — its goroutine relies on session-owned context being
		// alive while the final WAV is built and uploaded. Best-effort:
		// fall through to legacy SN3 path on any failure.
		if info, ok := cs.FlushRecorder(context.Background()); ok {
			wavRec = info
		}
		cs.Stop()
	}
	if p := s.callPersistStore(); p != nil {
		go p.OnBye(context.Background(), ByePersistParams{
			CallID:             callID,
			RawPayload:         raw,
			CodecName:          codec,
			Initiator:          "remote",
			RecordSampleRate:   recSR,
			RecordOpusChannels: recOpusCh,
			WAVRecording:       wavRec,
			RTCP:               rtcpSnap,
		})
	}
	s.forgetUASDialog(callID)
	s.releaseInboundCapacity(callID)
	if s.outboundBYELegCleanup != nil {
		s.outboundBYELegCleanup(callID, byeReasonClass)
	}
	return s.makeResponse(msg, 200, "OK", "", "")
}

func (s *SIPServer) handleOptions(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != stack.MethodOptions {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	resp := s.makeResponse(msg, 200, "OK", "", "")
	// Minimal Allow capability.
	resp.SetHeader("Allow", strings.Join([]string{
		stack.MethodInvite,
		stack.MethodAck,
		stack.MethodBye,
		stack.MethodRegister,
		stack.MethodOptions,
		stack.MethodCancel,
		stack.MethodInfo,
		stack.MethodPrack,
		stack.MethodSubscribe,
		stack.MethodNotify,
		stack.MethodPublish,
		stack.MethodRefer,
		stack.MethodMessage,
		stack.MethodUpdate,
	}, ", "))
	resp.SetHeader("Content-Length", "0")
	return resp
}

func (s *SIPServer) handleRegister(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != stack.MethodRegister {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	if !registerPasswordOK(msg) {
		if logger.Lg != nil {
			logger.Lg.Warn("sip register rejected (SIP_PASSWORD set but X-SIP-Register-Password missing or wrong)",
				zap.String("from", msg.GetHeader("From")),
				zap.String("remote", addrString(addr)),
			)
		}
		return s.makeResponse(msg, 403, "Forbidden", "", "")
	}
	s.upsertRegistration(msg, addr)
	// Minimal REGISTER: accept registration. Echo Contact if present and Expires if provided.
	resp := s.makeResponse(msg, 200, "OK", "", "")
	if c := msg.GetHeader("Contact"); c != "" {
		resp.SetHeader("Contact", c)
	}
	if exp := msg.GetHeader("Expires"); exp != "" {
		resp.SetHeader("Expires", exp)
	}
	resp.SetHeader("Content-Length", "0")
	return resp
}

func (s *SIPServer) handleInfo(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != stack.MethodInfo {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	callID := msg.GetHeader("Call-ID")
	var voiceLog *zap.Logger
	if logger.Lg != nil {
		voiceLog = logger.Lg.Named("sip-voice")
	}
	conversation.HandleSIPINFODTMF(callID, msg.GetHeader("Content-Type"), msg.Body, voiceLog)

	resp := s.makeResponse(msg, 200, "OK", "", "")
	resp.SetHeader("Content-Length", "0")
	return resp
}

func (s *SIPServer) handleCancel(msg *stack.Message, addr *net.UDPAddr) *stack.Message {
	if msg == nil || !msg.IsRequest || strings.ToUpper(msg.Method) != stack.MethodCancel {
		return nil
	}
	if s.absorbNonInviteRetransmit(msg, addr) {
		return nil
	}
	sendFn := func(m *stack.Message, a *net.UDPAddr) error {
		if s.ep == nil {
			return nil
		}
		return s.ep.Send(m, a)
	}
	if s.txMgr != nil && s.txMgr.HandleCancelRequest(msg, addr, sendFn) {
		callID := strings.TrimSpace(msg.GetHeader("Call-ID"))
		snap := s.takePendingInviteSnap(callID)
		if snap != nil && s.ep != nil {
			if inv, err := stack.Parse(snap.rawInvite); err == nil && inv != nil {
				r487 := s.makeResponse(inv, 487, "Request Terminated", "", snap.toTag)
				r487.SetHeader("To", snap.toTag)
				_ = s.ep.Send(r487, snap.addr)
				s.finalizeInviteServerTx(inv, r487, snap.addr)
			}
		}
		s.inviteAsyncEnd(callID)
		s.inviteFinalRetransmitCleanup(callID)
		if fkVal, ok := s.inviteFlightKeyByCall.Load(callID); ok {
			if fk, _ := fkVal.(string); fk != "" {
				s.inviteFlights.Delete(fk)
			}
		}
		s.stopCallSessionLocked(callID)
		return nil
	}
	resp := s.makeResponse(msg, 481, "Call/Transaction Does Not Exist", "", "")
	resp.SetHeader("Content-Length", "0")
	return resp
}

// makeResponse builds a response by copying dialog/transaction headers and allowing
// method-specific behavior. If toOverride is provided, it replaces the To header.
func (s *SIPServer) makeResponse(req *stack.Message, code int, text string, body string, toOverride string) *stack.Message {
	resp := &stack.Message{
		IsRequest:    false,
		Version:      "SIP/2.0",
		StatusCode:   code,
		StatusText:   text,
		Headers:      make(map[string]string),
		HeadersMulti: make(map[string][]string),
		Body:         body,
		Method:       "",
		RequestURI:   "",
	}

	if req != nil {
		// Via (multi-value) must be echoed back as-is.
		if vias := req.GetHeaders("Via"); len(vias) > 0 {
			resp.SetHeader("Via", vias[0])
			for i := 1; i < len(vias); i++ {
				resp.AddHeader("Via", vias[i])
			}
		}
		if v := req.GetHeader("From"); v != "" {
			resp.SetHeader("From", v)
		}
		if v := req.GetHeader("To"); v != "" {
			resp.SetHeader("To", v)
		}
		if v := req.GetHeader("Call-ID"); v != "" {
			resp.SetHeader("Call-ID", v)
		}
		if v := req.GetHeader("CSeq"); v != "" {
			resp.SetHeader("CSeq", v)
		}
	}
	if strings.TrimSpace(toOverride) != "" {
		resp.SetHeader("To", toOverride)
	}

	// Always emit explicit Content-Length (many clients expect it even for empty body).
	resp.SetHeader("Content-Length", strconv.Itoa(stack.BodyBytesLen(body)))
	return resp
}

func (s *SIPServer) String() string {
	return fmt.Sprintf("SIPServer{localIP=%s}", s.localIP)
}

// SendSIP sends a raw SIP request or response on the server's UDP socket.
// Used by the outbound module to send INVITE/ACK/BYE for UAC legs.
func (s *SIPServer) SendSIP(msg *stack.Message, addr *net.UDPAddr) error {
	if s == nil || s.ep == nil {
		return fmt.Errorf("sip: server not ready")
	}
	return s.ep.Send(msg, addr)
}

// ListenAddr returns the UDP listen address (host:port) for Contact/Via headers.
func (s *SIPServer) ListenAddr() (host string, port int) {
	if s == nil {
		return "", 0
	}
	return s.listenHost, s.listenPort
}

// GetCallSession returns the active CallSession for a Call-ID, or nil.
func (s *SIPServer) GetCallSession(callID string) *sipSession.CallSession {
	if s == nil {
		return nil
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callStore[callID]
}

// RemoveCallSession deletes a Call-ID from the store without stopping media (used when RTP was torn down elsewhere).
func (s *SIPServer) RemoveCallSession(callID string) {
	if s == nil {
		return
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	s.endVoiceDialogBridge(callID)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.callStore, callID)
}

// RegisterCallSession adds an established session (e.g. outbound UAC leg after ACK) so BYE and
// other in-dialog requests are handled the same as inbound calls.
func (s *SIPServer) RegisterCallSession(callID string, cs *sipSession.CallSession) {
	if s == nil || cs == nil {
		return
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.callStore[callID]; old != nil {
		old.Stop()
	}
	s.callStore[callID] = cs
}
