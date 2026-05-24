package outbound

import (
	"context"
	"crypto/rand"
	"crypto/tls"
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
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/sip/sdp"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/sip/session_timer"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
	"github.com/LinByte/VoiceServer/pkg/voice/cdr"
	"go.uber.org/zap"
)

var (
	outboundRTPPortAllocMu sync.Mutex
	outboundRTPPortNext    int
)

// applyOutboundAnswerSRTP enables SRTP on the RTP session when this
// UAC INVITE offered SDES crypto and the 200 OK SDP remains
// RTP/SAVP(F) with a supported AES_CM_128 suite. Plain RTP/AVP
// answers skip SRTP (interoperability downgrade).
//
// Accepts BOTH AES_CM_128_HMAC_SHA1_80 (our offer default) and
// AES_CM_128_HMAC_SHA1_32 — Cisco / Avaya peers commonly downgrade
// to _32 in the answer for bandwidth, and we'd previously fail the
// call with a misleading "missing a=crypto" error.
//
// DTLS-SRTP (UDP/TLS/RTP/SAVP) is handled by the dtlsPending branch
// in handleResponse; this function is a no-op for that proto.
func applyOutboundAnswerSRTP(sess *rtp.Session, offerKey, offerSalt []byte, answer *sdp.Info) error {
	if sess == nil || answer == nil || len(offerKey) == 0 {
		return nil
	}
	if !strings.Contains(strings.ToUpper(answer.Proto), "SAVP") {
		return nil
	}
	if sdp.IsDTLSTransport(answer.Proto) {
		// DTLS-SRTP doesn't carry SDES — caller's DTLS path handles it.
		return nil
	}
	co, ok := sdp.PickSupportedSDESOffer(answer.CryptoOffers)
	if !ok {
		return fmt.Errorf("sip/outbound: SRTP answer missing supported AES_CM_128 a=crypto")
	}
	prof, profOK := sdp.PionProfileForSuite(co.Suite)
	if !profOK {
		return fmt.Errorf("sip/outbound: unsupported SRTP suite in answer: %s", co.Suite)
	}
	rk, rs, err := sdp.DecodeSDESInline(co.KeyParams)
	if err != nil {
		return fmt.Errorf("sip/outbound: SRTP answer inline: %w", err)
	}
	return sess.EnableSDESSRTPWithProfile(prof, rk, rs, offerKey, offerSalt)
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

// newOutboundRTPSession allocates RTP UDP port based on env:
// - SIP_RTP_PORT: fixed single port
// - SIP_RTP_PORT_START/SIP_RTP_PORT_END: rotating range
// - fallback: ephemeral (port 0)
func newOutboundRTPSession() (*rtp.Session, error) {
	if fixed, ok := envInt("SIP_RTP_PORT"); ok && fixed > 0 {
		logger.Info("sip outbound rtp port policy: fixed", zap.Int("port", fixed))
		return rtp.NewSession(fixed)
	}
	start, hasStart := envInt("SIP_RTP_PORT_START")
	end, hasEnd := envInt("SIP_RTP_PORT_END")
	if hasStart && hasEnd && start > 0 && end >= start {
		logger.Info("sip outbound rtp port policy: range", zap.Int("start", start), zap.Int("end", end))
		return newOutboundRTPSessionFromRange(start, end)
	}
	logger.Info("sip outbound rtp port policy: ephemeral")
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

func newOutboundRTPSessionFromRange(start, end int) (*rtp.Session, error) {
	outboundRTPPortAllocMu.Lock()
	defer outboundRTPPortAllocMu.Unlock()
	span := end - start + 1
	if span <= 0 {
		return nil, fmt.Errorf("invalid outbound RTP port range: %d-%d", start, end)
	}
	if outboundRTPPortNext < start || outboundRTPPortNext > end {
		outboundRTPPortNext = start
	}
	var lastErr error
	for i := 0; i < span; i++ {
		p := outboundRTPPortNext
		outboundRTPPortNext++
		if outboundRTPPortNext > end {
			outboundRTPPortNext = start
		}
		sess, err := rtp.NewSession(p)
		if err == nil {
			return sess, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("outbound rtp range exhausted %d-%d: %w", start, end, lastErr)
}

// SignalingSender sends SIP on the shared UDP socket (typically *server.SIPServer).
type SignalingSender interface {
	SendSIP(msg *stack.Message, addr *net.UDPAddr) error
}

// ManagerConfig configures outbound legs.
type ManagerConfig struct {
	// LocalIP is used in SDP c= line (RTP advertised address).
	LocalIP string
	// SIPHost / SIPPort identify this UA in Via/Contact (usually listen addr).
	SIPHost string
	SIPPort int
	// FromUser is the local SIP user part for From/Contact (CLI / 外显号码；默认 soulnexus).
	FromUser string
	// FromDisplayName is optional quoted display-name in From (empty → no display-name).
	FromDisplayName string

	// MediaAttach is invoked after ACK for MediaProfileAI (e.g. conversation.AttachVoicePipeline).
	MediaAttach MediaAttachFunc

	// OnRegisterSession optionally registers the CallSession with the SIP server (BYE handling).
	OnRegisterSession func(callID string, cs *sipSession.CallSession)

	// OnEstablished is optional analytics hook after media hooks succeed.
	OnEstablished func(EstablishedLeg)

	// OnDialogCallIDAdopted runs when a 200 OK to INVITE uses a different Call-ID than our INVITE
	// (e.g. SBC rewrite). correlationID is req.CorrelationID (inbound Call-ID for transfers).
	OnDialogCallIDAdopted func(oldID, newID, correlationID string)

	// OnTransferBridge runs after 200 OK + ACK for MediaProfileTransferBridge.
	// CorrelationID on the request is the inbound Call-ID; cs is the outbound UAC leg.
	OnTransferBridge func(correlationID string, cs *sipSession.CallSession, outboundCallID string)

	// OnScript runs when MediaProfileScript is established.
	OnScript func(ctx context.Context, leg EstablishedLeg, scriptID string)

	// OnEvent reports dial lifecycle transitions for queue workers and metrics.
	OnEvent func(DialEvent)

	// TLSConfig is the *tls.Config used for outbound SIPS / TLS
	// signaling dials. nil → built lazily with system roots and
	// ServerName = dial host (strict verification by default per
	// product decision 2026-05-17). Callers that need skip-verify
	// for staging must populate this with InsecureSkipVerify=true;
	// no env-driven knob is exposed here on purpose so prod configs
	// don't accidentally inherit it.
	TLSConfig *tls.Config

	// STIRSigner is the RFC 8224 SHAKEN signer. nil = no Identity
	// header on outbound INVITEs (default; most deployments don't
	// have an STI-CA cert yet). When set, every outbound INVITE
	// where the FromUser and dest are E.164 numbers will carry a
	// signed PASSporT. Signing failures are soft (logged, then dial
	// without Identity) — STIR never blocks a legitimate call.
	STIRSigner *STIRSigner
}

// Manager owns outbound SIP legs keyed by Call-ID.
type Manager struct {
	cfg  ManagerConfig
	send func(*stack.Message, *net.UDPAddr) error

	// pool dispenses signalingPeers per (transport, host:port) for
	// connection-oriented transports (TCP/TLS). UDP short-circuits
	// to a fresh udpPeer wrapping `send`. Lazy-initialised on first
	// Dial because BindSender comes after NewManager.
	poolMu sync.Mutex
	pool   *signalingPool

	dialGateMu sync.RWMutex
	dialGate   func(context.Context, DialRequest, string) error

	outboundCapReleaseMu sync.RWMutex
	outboundCapRelease   func(callID string)

	mu       sync.Mutex
	legs     map[string]*outLeg // keyed by local outbound Call-ID
	legsByTx map[string]*outLeg // keyed by INVITE transaction (Via branch + CSeq)

	// cdrSinkMu protects the optional CDR writer pointer. Held as
	// a RWMutex so emitCDR's read on the hot cleanup path is cheap.
	// The pointer is set once from bootstrap (SetCDRSink) and never
	// races with itself, but tests inject and clear it freely.
	cdrSinkMu sync.RWMutex
	cdrSinkV  *cdr.Writer
}

// NewManager constructs a manager; call BindSender before Dial.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.FromUser == "" {
		cfg.FromUser = "soulnexus"
	}
	return &Manager{
		cfg:      cfg,
		legs:     make(map[string]*outLeg),
		legsByTx: make(map[string]*outLeg),
	}
}

// BindSender wires the UDP signaling path (required for Dial).
func (m *Manager) BindSender(s SignalingSender) {
	if m == nil || s == nil {
		return
	}
	m.send = func(msg *stack.Message, addr *net.UDPAddr) error {
		return s.SendSIP(msg, addr)
	}
}

// signalingPoolForDial returns the lazy-initialised pool; created on
// first call so we don't spin up a sweeper goroutine for managers
// that never dial TCP/TLS. tlsConfig is read off cfg.TLSConfig (set
// via SetTLSConfig before the first dial that needs it).
func (m *Manager) signalingPoolForDial() *signalingPool {
	if m == nil {
		return nil
	}
	m.poolMu.Lock()
	defer m.poolMu.Unlock()
	if m.pool != nil {
		return m.pool
	}
	m.pool = newSignalingPool(poolConfig{
		UDPSend:      m.send,
		ResponseSink: m.HandleSIPResponse,
		TLSConfig:    m.cfg.TLSConfig,
	})
	return m.pool
}

// ClosePool shuts the signaling pool (closes pooled TCP/TLS conns +
// stops the idle sweeper). Call from Manager teardown / process
// shutdown. Idempotent / nil-safe.
func (m *Manager) ClosePool() error {
	if m == nil {
		return nil
	}
	m.poolMu.Lock()
	pool := m.pool
	m.pool = nil
	m.poolMu.Unlock()
	if pool == nil {
		return nil
	}
	return pool.Close()
}

// SetDialGate runs after Call-ID allocation and before RTP allocation on Dial; non-nil error aborts the outbound INVITE.
func (m *Manager) SetDialGate(fn func(context.Context, DialRequest, string) error) {
	if m == nil {
		return
	}
	m.dialGateMu.Lock()
	defer m.dialGateMu.Unlock()
	m.dialGate = fn
}

// SetOutboundCapacityRelease is invoked from cleanupLeg (idempotent per Call-ID); optional.
func (m *Manager) SetOutboundCapacityRelease(fn func(callID string)) {
	if m == nil {
		return
	}
	m.outboundCapReleaseMu.Lock()
	defer m.outboundCapReleaseMu.Unlock()
	m.outboundCapRelease = fn
}

func (m *Manager) releaseOutboundCapacity(callID string) {
	if m == nil || strings.TrimSpace(callID) == "" {
		return
	}
	m.outboundCapReleaseMu.RLock()
	fn := m.outboundCapRelease
	m.outboundCapReleaseMu.RUnlock()
	if fn != nil {
		fn(strings.TrimSpace(callID))
	}
}

// HandleSIPResponse must be set on stack.EndpointConfig.OnSIPResponse / server.Config.OnSIPResponse.
func (m *Manager) HandleSIPResponse(resp *stack.Message, addr *net.UDPAddr) {
	if m == nil || resp == nil {
		return
	}
	txKey := txKeyFromResponse(resp)
	callID := strings.TrimSpace(resp.GetHeader("Call-ID"))
	m.mu.Lock()
	leg := (*outLeg)(nil)
	if txKey != "" {
		leg = m.legsByTx[txKey]
	}
	if leg == nil && callID != "" {
		leg = m.legs[callID]
	}
	m.mu.Unlock()
	if leg == nil {
		logger.Warn("sip outbound unmatched response",
			zap.String("call_id", callID),
			zap.String("tx_key", txKey),
			zap.String("cseq", strings.TrimSpace(resp.GetHeader("CSeq"))),
			zap.String("via", strings.TrimSpace(resp.GetHeader("Via"))),
			zap.Int("status", resp.StatusCode),
			zap.String("remote", udpAddrString(addr)),
		)
		return
	}
	// SBCs often rewrite the Call-ID host on 1xx/2xx; adopt before handleResponse so leg.params,
	// RegisterCallSession, transfer bridge maps, and in-dialog BYE use the dialog Call-ID.
	cseqHdr := strings.ToUpper(strings.TrimSpace(resp.GetHeader("CSeq")))
	if strings.Contains(cseqHdr, "INVITE") && resp.StatusCode < 300 {
		m.adoptOutboundDialogCallIDIfNeeded(leg, resp)
	}
	leg.handleResponse(context.Background(), resp, addr)
}

// Dial starts an outbound INVITE. Returns Call-ID on success.
func (m *Manager) Dial(ctx context.Context, req DialRequest) (callID string, err error) {
	if m == nil {
		return "", fmt.Errorf("sip/outbound: nil manager")
	}
	if m.send == nil {
		return "", ErrNoSignalingSender
	}
	if strings.TrimSpace(req.Target.RequestURI) == "" {
		return "", fmt.Errorf("sip/outbound: empty target request URI")
	}
	if strings.TrimSpace(req.Target.SignalingAddr) == "" {
		return "", fmt.Errorf("sip/outbound: empty signaling address")
	}

	sigHost := strings.TrimSpace(m.cfg.SIPHost)
	if sigHost == "" {
		sigHost = "127.0.0.1"
	}
	localSDP := strings.TrimSpace(m.cfg.LocalIP)
	if localSDP == "" {
		localSDP = "127.0.0.1"
	}
	callID = newCallID(sigHost)
	dialCommitted := false
	defer func() {
		if !dialCommitted {
			m.releaseOutboundCapacity(callID)
		}
	}()

	m.dialGateMu.RLock()
	dg := m.dialGate
	m.dialGateMu.RUnlock()
	if dg != nil {
		if err := dg(ctx, req, callID); err != nil {
			return "", err
		}
	}

	// Resolve via UDP-style addr regardless of transport: the
	// destination IP+Port is the same; only the wire transport
	// differs. We carry the *net.UDPAddr through outLeg / pool
	// because most downstream code (NAT detection, logging) reads
	// IP+Port off it; the actual TCP/TLS conn is held by the peer.
	addr, err := net.ResolveUDPAddr("udp", req.Target.SignalingAddr)
	if err != nil {
		return "", fmt.Errorf("sip/outbound: resolve signaling: %w", err)
	}
	transport := ResolveTransport(req.Target)

	localPort := m.cfg.SIPPort
	if localPort <= 0 {
		localPort = 6050
	}
	lh := strings.TrimSpace(m.cfg.SIPHost)
	if lh != "" && addr.IP != nil && addr.Port == localPort {
		if lip := net.ParseIP(lh); lip != nil && lip.Equal(addr.IP) {
			logger.Debug("sip outbound: signaling to same IP:port as listener (hairpin); REGISTERed users are proxied, others answered locally.",
				zap.String("dst", addr.String()),
				zap.String("local_listener", fmt.Sprintf("%s:%d", lh, localPort)),
				zap.String("request_uri", strings.TrimSpace(req.Target.RequestURI)))
		}
	}

	rtpSess, err := newOutboundRTPSession()
	if err != nil {
		return "", fmt.Errorf("sip/outbound: rtp session: %w", err)
	}
	localPort = rtpSess.LocalAddr.Port
	codecs := sdp.DefaultOutboundOfferCodecs()
	if req.MediaProfile == MediaProfileScript {
		// Script callbacks prioritize SIP endpoint compatibility over wideband quality.
		// Some UAs negotiate Opus but still produce garbled playout in this path.
		codecs = sdp.TransferAgentBridgeOfferCodecs()
	} else if req.Scenario == ScenarioTransferAgent && req.MediaProfile == MediaProfileTransferBridge {
		codecs = sdp.TransferAgentBridgeOfferCodecs()
	}

	var (
		mediaProto    string
		sdpExtras     []string
		srtpOfferKey  []byte
		srtpOfferSalt []byte
		dtlsPending   *outboundDTLSPending
	)
	if req.OfferDTLSSRTP && req.MediaProfile == MediaProfileTransferBridge {
		// DTLS-SRTP toward bridge profile peers is almost guaranteed
		// to 488 (most softphones don't speak it). Don't fail the
		// dial — just downgrade to RTP/AVP and log so the operator
		// knows their flag was ignored.
		logger.Warn("sip outbound: OfferDTLSSRTP ignored on MediaProfileTransferBridge (downgrading to RTP/AVP)",
			zap.String("scenario", string(req.Scenario)))
	}
	if req.Scenario == ScenarioTransferAgent && req.MediaProfile == MediaProfileTransferBridge {
		// Bridged agent leg targets desk phones / common softphones; many reject RTP/SAVPF+SDES with 488.
		// Plain RTP/AVP keeps SRTP on the customer inbound leg only.
		mediaProto = "RTP/AVP"
	} else if req.OfferDTLSSRTP {
		// RFC 5763/5764 DTLS-SRTP offer. We don't carry SDES extras
		// alongside — the proto value forbids interleaving SAVP and
		// SAVP-DTLS in the same m= block.
		proto, extras, pending, derr := prepareOutboundDTLSOffer()
		if derr != nil {
			_ = rtpSess.Close()
			return "", fmt.Errorf("sip/outbound: dtls offer prep: %w", derr)
		}
		mediaProto = proto
		sdpExtras = extras
		dtlsPending = pending
	} else {
		// Standard outbound offers RTP/SAVPF + SDES; plain RTP answers downgrade via applyOutboundAnswerSRTP.
		mediaProto = "RTP/SAVPF"
		mkey := make([]byte, 16)
		msalt := make([]byte, 14)
		if _, err := rand.Read(mkey); err != nil {
			_ = rtpSess.Close()
			return "", fmt.Errorf("sip/outbound: SRTP master key: %w", err)
		}
		if _, err := rand.Read(msalt); err != nil {
			_ = rtpSess.Close()
			return "", fmt.Errorf("sip/outbound: SRTP master salt: %w", err)
		}
		cryptoLine, cerr := sdp.FormatCryptoLine(1, sdp.SuiteAESCM128HMACSHA180, mkey, msalt)
		if cerr != nil {
			_ = rtpSess.Close()
			return "", cerr
		}
		sdpExtras = []string{cryptoLine}
		srtpOfferKey = append([]byte(nil), mkey...)
		srtpOfferSalt = append([]byte(nil), msalt...)
	}
	sdpBody := sdp.GenerateWithProtoExtras(localSDP, localPort, mediaProto, codecs, sdpExtras)

	ip := m.cfg.SIPHost
	if ip == "" {
		ip = "127.0.0.1"
	}
	port := m.cfg.SIPPort
	if port <= 0 {
		port = 6050
	}

	fromUser := m.cfg.FromUser
	fromDisp := m.cfg.FromDisplayName
	if u := strings.TrimSpace(req.CallerUser); u != "" {
		fromUser = u
		fromDisp = strings.TrimSpace(req.CallerDisplayName)
	} else if u := strings.TrimSpace(req.Target.CallerUser); u != "" {
		fromUser = u
		fromDisp = strings.TrimSpace(req.Target.CallerDisplayName)
	}

	params := inviteParams{
		LocalIP:                     localSDP,
		SIPHost:                     ip,
		SIPPort:                     port,
		RequestURI:                  strings.TrimSpace(req.Target.RequestURI),
		CallID:                      callID,
		FromTag:                     randomHex(8),
		Branch:                      randomHex(10),
		CSeq:                        1,
		LocalRTPPort:                localPort,
		SDPBody:                     sdpBody,
		FromUser:                    fromUser,
		FromDisplayName:             fromDisp,
		AssertedIdentityURI:         strings.TrimSpace(req.AssertedIdentityURI),
		AssertedIdentityDisplayName: strings.TrimSpace(req.AssertedIdentityDisplayName),
		PrivacyTokens:               req.PrivacyTokens,
		HistoryInfo:                 req.HistoryInfo,
		Diversion:                   req.Diversion,
		ViaTransport:                transport,
	}

	// RFC 8224 SHAKEN signing — opt-in via ManagerConfig.STIRSigner.
	// We pull TNs from the From user and Request-URI; both must be
	// E.164 for SHAKEN. Non-E.164 destinations (SIP URIs, alphanumeric)
	// silently skip signing. Failures are soft per outbound/stir.go.
	if m.cfg.STIRSigner != nil {
		destTN := extractTNFromRequestURI(params.RequestURI)
		if id, ok := signOutboundIdentity(m.cfg.STIRSigner, callID, fromUser, destTN); ok {
			params.IdentityHeader = id
		}
	}

	invite := buildINVITE(params)
	leg := &outLeg{
		m:             m,
		params:        params,
		req:           req,
		rtpSess:       rtpSess,
		dst:           addr,
		transport:     transport,
		txKey:         inviteTxKey(params.Branch, params.CSeq),
		srtpOfferKey:  srtpOfferKey,
		srtpOfferSalt: srtpOfferSalt,
		dtlsPending:   dtlsPending,
	}

	// Acquire signaling peer (UDP wraps shared listener;
	// TCP/TLS dials + pools per-target). Done BEFORE registering
	// the leg so a dial failure aborts cleanly without leaking
	// txKey state.
	pool := m.signalingPoolForDial()
	if pool == nil {
		_ = rtpSess.Close()
		return "", fmt.Errorf("sip/outbound: signaling pool unavailable")
	}
	peer, err := pool.Get(ctx, transport, addr)
	if err != nil {
		_ = rtpSess.Close()
		return "", fmt.Errorf("sip/outbound: dial %s: %w", transport, err)
	}
	leg.peerMu.Lock()
	leg.peer = peer
	leg.peerMu.Unlock()

	// Stamp the CDR start time before the INVITE hits the wire —
	// this is the closest moment to "user pressed dial". No-op when
	// no sink is configured.
	leg.beginCDR()

	m.mu.Lock()
	m.legs[callID] = leg
	if leg.txKey != "" {
		m.legsByTx[leg.txKey] = leg
	}
	m.mu.Unlock()

	if err := peer.Send(invite); err != nil {
		m.mu.Lock()
		delete(m.legs, callID)
		m.mu.Unlock()
		_ = rtpSess.Close()
		return "", fmt.Errorf("sip/outbound: send INVITE: %w", err)
	}

	logger.Info("sip outbound INVITE sent",
		zap.String("call_id", callID),
		zap.String("request_uri", strings.TrimSpace(req.Target.RequestURI)),
		zap.String("scenario", string(req.Scenario)),
		zap.String("media_profile", string(req.MediaProfile)),
		zap.String("correlation_id", strings.TrimSpace(req.CorrelationID)),
		zap.String("script_id", strings.TrimSpace(req.ScriptID)),
		zap.String("dst_udp", addr.String()),
		zap.String("from_user", fromUser),
		zap.String("signaling_addr_cfg", strings.TrimSpace(req.Target.SignalingAddr)),
		zap.String("pai", invite.GetHeader("P-Asserted-Identity")),
		zap.String("privacy", invite.GetHeader("Privacy")),
	)
	if m.cfg.OnEvent != nil {
		m.cfg.OnEvent(DialEvent{
			CallID:        callID,
			CorrelationID: strings.TrimSpace(req.CorrelationID),
			Scenario:      req.Scenario,
			MediaProfile:  req.MediaProfile,
			State:         DialEventInvited,
			At:            time.Now(),
			RequestURI:    strings.TrimSpace(req.Target.RequestURI),
			RemoteAddr:    addr.String(),
		})
	}
	dialCommitted = true
	return callID, nil
}

// AbandonEarlyTransferInvite tears down an outbound INVITE leg that never reached 200 OK (e.g. ring timeout)
// and emits DialEventFailed with status 408 so the transfer layer can fall through to the next agent.
func (m *Manager) AbandonEarlyTransferInvite(callID string) bool {
	if m == nil {
		return false
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	m.mu.Lock()
	leg := m.legs[callID]
	if leg == nil {
		m.mu.Unlock()
		return false
	}
	leg.mu.Lock()
	established := leg.established
	leg.mu.Unlock()
	if established {
		m.mu.Unlock()
		return false
	}
	req := leg.req
	remoteStr := ""
	if leg.dst != nil {
		remoteStr = udpAddrString(leg.dst)
	}
	requestURI := strings.TrimSpace(req.Target.RequestURI)
	m.mu.Unlock()

	// Send CANCEL on the wire BEFORE local cleanup — otherwise the
	// agent's phone keeps ringing until the carrier's own no-answer
	// timer (60-180s). Cleanup tears down leg.peer / leg.legs, so
	// once cleanupLeg returns we can't reach the remote anymore.
	// Failure here is logged but not fatal: cleanup must still run
	// or we leak per-call state forever.
	if err := buildAndSendCANCEL(leg); err != nil {
		logger.Warn("sip outbound CANCEL on abandon failed",
			zap.String("call_id", callID),
			zap.Error(err))
	}

	leg.cleanupLeg()

	if m.cfg.OnEvent != nil {
		m.cfg.OnEvent(DialEvent{
			CallID:        callID,
			CorrelationID: strings.TrimSpace(req.CorrelationID),
			Scenario:      req.Scenario,
			MediaProfile:  req.MediaProfile,
			State:         DialEventFailed,
			StatusCode:    408,
			Reason:        "transfer_invite_timeout",
			StatusText:    "Request Timeout",
			RequestURI:    requestURI,
			RemoteAddr:    remoteStr,
			At:            time.Now(),
		})
	}
	return true
}

// adoptOutboundDialogCallIDIfNeeded rekeys the outbound leg when a 2xx to INVITE echoes a different Call-ID
// (some SBCs normalize the host part). ACK/BYE and bridge maps must use the dialog Call-ID.
func (m *Manager) adoptOutboundDialogCallIDIfNeeded(leg *outLeg, resp *stack.Message) {
	if m == nil || leg == nil || resp == nil {
		return
	}
	newCID := strings.TrimSpace(resp.GetHeader("Call-ID"))
	oldCID := strings.TrimSpace(leg.params.CallID)
	if newCID == "" || newCID == oldCID {
		return
	}
	var adopCb func(string, string, string)
	m.mu.Lock()
	if m.legs[oldCID] != leg {
		m.mu.Unlock()
		return
	}
	if ex := m.legs[newCID]; ex != nil && ex != leg {
		m.mu.Unlock()
		logger.Warn("sip outbound: refuse dialog call-id adopt (collision)",
			zap.String("old", oldCID), zap.String("new", newCID))
		return
	}
	delete(m.legs, oldCID)
	leg.params.CallID = newCID
	m.legs[newCID] = leg
	adopCb = m.cfg.OnDialogCallIDAdopted
	m.mu.Unlock()

	if adopCb != nil {
		adopCb(oldCID, newCID, strings.TrimSpace(leg.req.CorrelationID))
	}
	logger.Info("sip outbound: dialog call-id adopted from 200 OK",
		zap.String("invite_call_id", oldCID),
		zap.String("dialog_call_id", newCID),
		zap.String("correlation_id", strings.TrimSpace(leg.req.CorrelationID)),
	)
}

type outLeg struct {
	m       *Manager
	params  inviteParams
	req     DialRequest
	rtpSess *rtp.Session
	dst     *net.UDPAddr

	// transport is the resolved wire transport for this leg's
	// signalling. Drives Via header rendering and ACK/BYE peer
	// reuse. Set in Dial via ResolveTransport(req.Target).
	transport Transport
	// peer is the live signalingPeer used to write request bytes.
	// For UDP it's a fresh udpPeer; for TCP/TLS it's a shared
	// pooled connPeer. nil after cleanupLeg.
	peerMu sync.Mutex
	peer   signalingPeer

	mu          sync.Mutex
	established bool
	callSession *sipSession.CallSession

	sigMu         sync.Mutex
	byeToHeader   string // To from 200 OK (remote tag)
	byeRequestURI string // in-dialog Request-URI (Contact)
	byeRemote     *net.UDPAddr
	byeCSeqNext   int
	txKey         string

	srtpOfferKey  []byte
	srtpOfferSalt []byte

	// dtlsPending is non-nil when the INVITE offered DTLS-SRTP.
	// Consumed by handleResponse once we have the answer SDP.
	dtlsPending *outboundDTLSPending

	// refreshMu guards refresher (start/stop is racy with cleanupLeg).
	refreshMu sync.Mutex
	refresher *outRefresher // RFC 4028 UAC-side refresh loop; nil unless armed by 2xx OK

	// cdr accumulates per-leg CDR state (start time, codec,
	// classification). Held by value so legs without a configured
	// CDR sink pay only the zero-value cost.
	cdr cdrState

	// gotProvisional is set true on the first 1xx received for the
	// INVITE. RFC 3261 §9.1 says CANCEL MUST NOT be sent before the
	// first provisional — some strict proxies will silently drop a
	// CANCEL that arrives before they have a transaction state for
	// the original INVITE (race with the INVITE itself in transit).
	gotProvisional atomic.Bool
	// pendingCancel is set true when SendCANCEL is invoked before
	// gotProvisional. The first 1xx hander then fires the CANCEL.
	// A 500ms fallback timer also fires the CANCEL if no 1xx arrives,
	// so a strictly silent carrier doesn't keep the agent ringing.
	pendingCancel atomic.Bool
	// cancelSent guards against duplicate CANCEL emissions when both
	// the inbound-BYE path and the ring-timeout path race.
	cancelSent atomic.Bool
	// cancelStop closes when the INVITE transaction reaches a final
	// response, telling the CANCEL retransmit goroutine to stop. Lazy
	// init in startCancelRetransmit.
	cancelStopMu sync.Mutex
	cancelStop   chan struct{}
}

func (leg *outLeg) handleResponse(ctx context.Context, resp *stack.Message, from *net.UDPAddr) {
	if leg == nil || resp == nil {
		return
	}
	st := resp.StatusCode
	cseqAll := strings.ToUpper(resp.GetHeader("CSeq"))
	if strings.Contains(cseqAll, "BYE") {
		if st >= 200 && st < 300 {
			// Local BYE acknowledged. Counted as "by=local" since we
			// initiated; remote-initiated BYEs come through the
			// inbound BYE handler (see bye.go / CleanupLegIfPresent).
			sipMetrics.Bye(sipMetrics.ByeByLocal, sipMetrics.ByeReasonNormal)
			leg.cdrSetHangup("local", "normal")
			leg.cleanupLeg()
		}
		return
	}
	// Bump the INVITE-result counter for any INVITE-CSeq response.
	// We do this at the response classifier rather than at every
	// branch below so 1xx / non-200 / 200 are all covered with a
	// single line.
	if strings.Contains(cseqAll, "INVITE") {
		sipMetrics.InviteResult(sipMetrics.DirectionOutbound, st)
	}
	// RFC 4028 / 3311: route responses to in-dialog UPDATE refreshes
	// into the refresher state machine BEFORE the generic INVITE
	// response handler (which would otherwise treat 422 as a hard
	// failure and tear the leg down).
	if strings.Contains(cseqAll, stack.MethodUpdate) {
		leg.refreshMu.Lock()
		r := leg.refresher
		leg.refreshMu.Unlock()
		if r != nil && !r.handleUPDATEResponse(resp) {
			leg.stopRefresher()
		}
		return
	}
	if st >= 100 && st < 200 {
		phrase := strings.TrimSpace(resp.StatusText)
		// RFC 3261 §9.1: a CANCEL queued before the first provisional
		// is held back to avoid racing the proxy's transaction state.
		// The first 1xx unblocks it. We use CompareAndSwap so only the
		// FIRST 1xx fires the deferred CANCEL (subsequent 18x are no-op).
		if leg.gotProvisional.CompareAndSwap(false, true) {
			if leg.pendingCancel.Load() {
				go leg.fireDeferredCANCEL()
			}
		}
		logger.Info("sip outbound provisional response",
			zap.String("call_id", leg.params.CallID),
			zap.Int("status", st),
			zap.String("reason_phrase", phrase),
			zap.String("remote_udp", udpAddrString(from)),
			zap.String("scenario", string(leg.req.Scenario)),
			zap.String("correlation_id", strings.TrimSpace(leg.req.CorrelationID)),
		)
		if leg.m.cfg.OnEvent != nil {
			leg.m.cfg.OnEvent(DialEvent{
				CallID:        leg.params.CallID,
				CorrelationID: strings.TrimSpace(leg.req.CorrelationID),
				Scenario:      leg.req.Scenario,
				MediaProfile:  leg.req.MediaProfile,
				State:         DialEventProvisional,
				StatusCode:    st,
				StatusText:    phrase,
				RemoteAddr:    udpAddrString(from),
				RequestURI:    strings.TrimSpace(leg.req.Target.RequestURI),
				At:            time.Now(),
			})
		}
		return
	}
	if st != 200 {
		reason := strings.TrimSpace(resp.StatusText)
		if reason == "" {
			reason = "non_200"
		}
		// Final INVITE response → stop any CANCEL retransmit goroutine.
		// 487 Request Terminated is the textbook reply to a successful
		// CANCEL; other 4xx/5xx/6xx mean the transaction died on its
		// own and any in-flight CANCEL is moot.
		leg.stopCANCELRetransmit()
		// Record the failure into the CDR before logging / event
		// emission so even an Emit during cleanup carries the cause.
		leg.cdrSetError(st, reason)
		logger.Warn("sip outbound non-200 response",
			zap.String("call_id", leg.params.CallID),
			zap.Int("status", st),
			zap.String("reason_phrase", reason),
			zap.String("remote_udp", udpAddrString(from)),
			zap.String("request_uri", strings.TrimSpace(leg.req.Target.RequestURI)),
			zap.String("correlation_id", strings.TrimSpace(leg.req.CorrelationID)),
			zap.String("content_type", strings.TrimSpace(resp.GetHeader("Content-Type"))),
			zap.Int("content_length", len(resp.Body)),
			zap.String("body_preview", previewBody(resp.Body, 500)),
		)
		if leg.m.cfg.OnEvent != nil {
			leg.m.cfg.OnEvent(DialEvent{
				CallID:        leg.params.CallID,
				CorrelationID: strings.TrimSpace(leg.req.CorrelationID),
				Scenario:      leg.req.Scenario,
				MediaProfile:  leg.req.MediaProfile,
				State:         DialEventFailed,
				StatusCode:    st,
				Reason:        reason,
				StatusText:    strings.TrimSpace(resp.StatusText),
				RemoteAddr:    udpAddrString(from),
				RequestURI:    strings.TrimSpace(leg.req.Target.RequestURI),
				At:            time.Now(),
			})
		}
		leg.cleanupLeg()
		return
	}
	cseq := resp.GetHeader("CSeq")
	if !strings.Contains(strings.ToUpper(cseq), "INVITE") {
		return
	}

	leg.mu.Lock()
	if leg.established {
		leg.mu.Unlock()
		return
	}
	leg.mu.Unlock()

	if strings.TrimSpace(resp.Body) == "" {
		logger.Warn("sip outbound 200 OK without SDP", zap.String("call_id", leg.params.CallID))
		leg.cleanupLeg()
		return
	}

	answer, err := sdp.Parse(resp.Body)
	if err != nil {
		logger.Warn("sip outbound bad answer SDP", zap.String("call_id", leg.params.CallID), zap.Error(err))
		leg.cleanupLeg()
		return
	}
	remoteIP := net.ParseIP(answer.IP)
	if remoteIP == nil || answer.Port <= 0 {
		logger.Warn("sip outbound invalid RTP in answer", zap.String("call_id", leg.params.CallID))
		leg.cleanupLeg()
		return
	}
	remoteRTP := &net.UDPAddr{IP: remoteIP, Port: answer.Port}
	// NAT fallback for outbound UAC legs: when answer SDP has a private media IP but
	// response source is public/reachable, use response source IP + SDP port.
	if from != nil && isPrivateIPv4(remoteIP) && from.IP != nil && from.IP.To4() != nil && !isPrivateIPv4(from.IP) {
		remoteRTP = &net.UDPAddr{IP: from.IP, Port: answer.Port}
		logger.Info("sip outbound media target overridden (private SDP IP fallback)",
			zap.String("call_id", leg.params.CallID),
			zap.String("sdp_remote_ip", remoteIP.String()),
			zap.String("sip_source_ip", from.IP.String()),
			zap.Int("media_port", answer.Port),
			zap.String("chosen_remote_rtp", remoteRTP.String()),
		)
	}
	leg.rtpSess.SetRemoteAddr(remoteRTP)

	leg.m.adoptOutboundDialogCallIDIfNeeded(leg, resp)

	// RFC 5763/5764: if we offered DTLS-SRTP, run the handshake on
	// the RTP socket and install the derived contexts. Otherwise
	// fall through to the SDES negotiator. The two paths are
	// mutually exclusive (proto string can't be both
	// UDP/TLS/RTP/SAVP and RTP/SAVPF in the same m= block).
	if leg.dtlsPending != nil {
		if !sdp.IsDTLSTransport(answer.Proto) {
			logger.Warn("sip outbound dtls offer answered with non-DTLS proto (488 expected, got 2xx)",
				zap.String("call_id", leg.params.CallID),
				zap.String("answer_proto", answer.Proto))
			leg.cleanupLeg()
			return
		}
		startOutboundDTLSHandshake(leg, leg.dtlsPending, answer)
	} else if err := applyOutboundAnswerSRTP(leg.rtpSess, leg.srtpOfferKey, leg.srtpOfferSalt, answer); err != nil {
		logger.Warn("sip outbound SRTP negotiation failed", zap.String("call_id", leg.params.CallID), zap.Error(err))
		leg.cleanupLeg()
		return
	}

	cs, err := sipSession.NewCallSession(leg.params.CallID, leg.rtpSess, answer.Codecs)
	if err != nil {
		logger.Warn("sip outbound CallSession", zap.String("call_id", leg.params.CallID), zap.Error(err))
		leg.cleanupLeg()
		return
	}
	cs.SetTenantID(leg.req.DialTenantID)

	ackURI := ackRequestURI(resp, leg.params.RequestURI)
	ack := buildACK(leg.params, resp, ackURI)
	if ack == nil {
		leg.cleanupLeg()
		return
	}
	// ACK travels on the same transport as the INVITE — for TCP/TLS
	// the peer holds the live conn; for UDP it wraps the shared
	// listener and delivers to `from` (the actual response source,
	// in case of NAT rebinding). Falling back to leg.dst is fine
	// when the peer abstraction handles the address itself.
	if err := leg.sendOnPeer(ack, from); err != nil {
		logger.Warn("sip outbound ACK failed", zap.String("call_id", leg.params.CallID), zap.Error(err))
		cs.Stop()
		leg.cleanupLeg()
		return
	}

	leg.sigMu.Lock()
	leg.byeToHeader = resp.GetHeader("To")
	leg.byeRequestURI = ackRequestURI(resp, leg.params.RequestURI)
	if from != nil {
		leg.byeRemote = cloneUDPAddr(from)
	} else {
		leg.byeRemote = cloneUDPAddr(leg.dst)
	}
	leg.byeCSeqNext = leg.params.CSeq + 1
	leg.sigMu.Unlock()

	leg.mu.Lock()
	leg.established = true
	leg.callSession = cs
	leg.mu.Unlock()

	// CDR: mark the call as answered with the negotiated codec. The
	// first codec in the answer is the one we'll actually use (RFC
	// 3264 §6.1 — accepted offer's first format is preferred).
	codecName := ""
	if len(answer.Codecs) > 0 {
		codecName = answer.Codecs[0].Name
	}
	leg.cdrSetAnswered(codecName)

	// RFC 4028 §10: arm UAC-side refresher IFF peer's 200 OK assigned
	// us the refresher role. Peer-as-refresher is handled implicitly:
	// we have no watchdog (see refresher.go scope notes), so a silent
	// dialog will eventually be torn down by the peer's own timer.
	if peerSE, peerRefresher, _ := session_timer.ParseSessionExpires(resp.GetHeader("Session-Expires")); peerSE > 0 {
		leg.startRefresherIfUAC(peerSE, peerRefresher)
	}

	if leg.m.cfg.OnRegisterSession != nil {
		leg.m.cfg.OnRegisterSession(leg.params.CallID, cs)
	}

	// Bridge profile owns RTP via conversation.StartTransferBridge (raw relay or PCM transcode fallback).
	// Starting the default MediaSession here would race ReadFromUDP on the same socket and cause noise.
	startDefaultMedia := true
	switch leg.req.MediaProfile {
	case MediaProfileAI:
		if leg.m.cfg.MediaAttach != nil {
			if err := leg.m.cfg.MediaAttach(ctx, cs); err != nil {
				logger.Warn("sip outbound media attach", zap.String("call_id", leg.params.CallID), zap.Error(err))
			}
		}
	case MediaProfileScript:
		if leg.m.cfg.MediaAttach != nil {
			if err := leg.m.cfg.MediaAttach(ctx, cs); err != nil {
				logger.Warn("sip outbound script media attach", zap.String("call_id", leg.params.CallID), zap.Error(err))
			}
		}
		if leg.m.cfg.OnScript != nil {
			fromH := formatOutboundFromHeader(leg.params.FromDisplayName, leg.params.FromUser,
				leg.params.SIPHost, leg.params.SIPPort, leg.params.FromTag)
			leg.m.cfg.OnScript(ctx, EstablishedLeg{
				CallID:              leg.params.CallID,
				Scenario:            leg.req.Scenario,
				CorrelationID:       leg.req.CorrelationID,
				Session:             cs,
				CreatedAt:           time.Now(),
				FromHeader:          fromH,
				ToHeader:            leg.params.RequestURI,
				RemoteSignalingAddr: leg.dst.String(),
				CSeqInvite:          fmt.Sprintf("%d INVITE", leg.params.CSeq),
			}, strings.TrimSpace(leg.req.ScriptID))
		}
	case MediaProfileTransferBridge:
		startDefaultMedia = false
		cid := strings.TrimSpace(leg.req.CorrelationID)
		if cid == "" {
			logger.Warn("sip outbound bridge: empty correlation id (inbound Call-ID)",
				zap.String("call_id", leg.params.CallID))
			leg.cleanupLeg()
			return
		}
		if leg.m.cfg.OnTransferBridge != nil {
			leg.m.cfg.OnTransferBridge(cid, cs, leg.params.CallID)
		} else {
			logger.Warn("sip outbound bridge: OnTransferBridge not configured",
				zap.String("call_id", leg.params.CallID))
		}
	default:
		// MediaProfileNone
	}

	if startDefaultMedia {
		cs.StartOnACK()
	}

	if leg.m.cfg.OnEstablished != nil {
		fromH := formatOutboundFromHeader(leg.params.FromDisplayName, leg.params.FromUser,
			leg.params.SIPHost, leg.params.SIPPort, leg.params.FromTag)
		leg.m.cfg.OnEstablished(EstablishedLeg{
			CallID:              leg.params.CallID,
			Scenario:            leg.req.Scenario,
			CorrelationID:       leg.req.CorrelationID,
			Session:             cs,
			CreatedAt:           time.Now(),
			FromHeader:          fromH,
			ToHeader:            leg.params.RequestURI,
			RemoteSignalingAddr: leg.dst.String(),
			CSeqInvite:          fmt.Sprintf("%d INVITE", leg.params.CSeq),
		})
	}
	if leg.m.cfg.OnEvent != nil {
		leg.m.cfg.OnEvent(DialEvent{
			CallID:        leg.params.CallID,
			CorrelationID: strings.TrimSpace(leg.req.CorrelationID),
			Scenario:      leg.req.Scenario,
			MediaProfile:  leg.req.MediaProfile,
			State:         DialEventEstablished,
			StatusCode:    200,
			StatusText:    strings.TrimSpace(resp.StatusText),
			RemoteAddr:    udpAddrString(from),
			RequestURI:    strings.TrimSpace(leg.req.Target.RequestURI),
			At:            time.Now(),
		})
	}

	logger.Info("sip outbound established",
		zap.String("call_id", leg.params.CallID),
		zap.String("correlation_id", strings.TrimSpace(leg.req.CorrelationID)),
		zap.String("scenario", string(leg.req.Scenario)),
		zap.String("media_profile", string(leg.req.MediaProfile)),
		zap.String("negotiated_codec", cs.NegotiatedCodec().Name),
		zap.Int("negotiated_clock_rate", cs.NegotiatedCodec().ClockRate),
		zap.Int("negotiated_channels", cs.NegotiatedCodec().Channels),
	)
}

func previewBody(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func udpAddrString(a *net.UDPAddr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

func (leg *outLeg) cleanupLeg() {
	if leg == nil || leg.m == nil {
		return
	}
	// Stop the session-timer refresher BEFORE we drop the peer ref —
	// otherwise a tick fired between the two could try to send on a
	// nil peer and log a misleading "send failed" warning.
	leg.stopRefresher()
	// Stop any CANCEL retransmit goroutine for the same reason —
	// once we drop the peer it would write to a nil socket.
	leg.stopCANCELRetransmit()
	// Flush per-call QoS into the metrics histograms BEFORE we close
	// the RTP session. After Close() the snapshot would be empty.
	// This is the once-per-call observation point — one RTCP read,
	// no hot-path cost.
	flushOutboundCallQoS(leg)
	// Emit the CDR record (no-op if no sink is configured). Reads
	// RTCP one more time internally so it captures the same QoS
	// numbers we just flushed to the histograms.
	leg.emitCDR()
	// Always-on accounting (independent of the CDR sink): decrement
	// active-calls gauge + bump calls_total by end status. Safe on
	// legs that never reached "answered" — internal idempotent gate.
	leg.endCDRActiveCount()
	callID := leg.params.CallID
	m := leg.m
	m.mu.Lock()
	delete(m.legs, callID)
	if leg.txKey != "" {
		delete(m.legsByTx, leg.txKey)
	}
	m.mu.Unlock()
	if leg.rtpSess != nil {
		_ = leg.rtpSess.Close()
	}
	// Drop the peer reference but DON'T close it — the connection
	// is pool-owned and may be in use by another call to the same
	// target. The pool's idle sweeper closes it when truly unused.
	leg.peerMu.Lock()
	leg.peer = nil
	leg.peerMu.Unlock()
	m.releaseOutboundCapacity(callID)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", nBytes)
	}
	return hex.EncodeToString(b)
}

func inviteTxKey(branch string, cseq int) string {
	branch = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(branch), "z9hG4bK"))
	if branch == "" || cseq <= 0 {
		return ""
	}
	return fmt.Sprintf("%s|%d", branch, cseq)
}

func txKeyFromResponse(resp *stack.Message) string {
	if resp == nil {
		return ""
	}
	cseqNum := stack.ParseCSeqNum(strings.TrimSpace(resp.GetHeader("CSeq")))
	if cseqNum <= 0 {
		return ""
	}
	via := strings.TrimSpace(resp.GetHeader("Via"))
	if via == "" {
		return ""
	}
	lower := strings.ToLower(via)
	idx := strings.Index(lower, "branch=")
	if idx < 0 {
		return ""
	}
	val := via[idx+len("branch="):]
	if semi := strings.Index(val, ";"); semi >= 0 {
		val = val[:semi]
	}
	val = strings.TrimSpace(strings.Trim(val, "\""))
	return inviteTxKey(val, cseqNum)
}

// callIDLocalPart returns the substring before the last '@' for SIP Call-ID local@host matching.
func callIDLocalPart(callID string) (local string, ok bool) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return "", false
	}
	i := strings.LastIndex(callID, "@")
	if i <= 0 || i >= len(callID)-1 {
		return "", false
	}
	return callID[:i], true
}

// legByCallIDOrHostRewrite finds an outbound leg by exact Call-ID or by matching the local part
// before '@' when an SBC rewrites only the host portion (responses / BYE vs INVITE).
func (m *Manager) legByCallIDOrHostRewrite(callID string) *outLeg {
	callID = strings.TrimSpace(callID)
	if m == nil || callID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if lg := m.legs[callID]; lg != nil {
		return lg
	}
	local, ok := callIDLocalPart(callID)
	if !ok {
		return nil
	}
	for _, lg := range m.legs {
		if lg == nil {
			continue
		}
		if l2, ok2 := callIDLocalPart(lg.params.CallID); ok2 && l2 == local {
			return lg
		}
	}
	return nil
}
