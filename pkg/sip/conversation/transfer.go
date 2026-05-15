package conversation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/scriptlisten"
	sipdtmf "github.com/LinByte/VoiceServer/pkg/sip/dtmf"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"go.uber.org/zap"
)

// TransferDialer is implemented by outbound.Manager (Dial).
type TransferDialer interface {
	Dial(ctx context.Context, req outbound.DialRequest) (callID string, err error)
}

var (
	transferMu     sync.Mutex
	transferDialer TransferDialer
	// Optional: DB-backed dial target (acd_pool_targets); env fallback only when this resolver is nil.
	// inboundCallID is the PSTN inbound Call-ID (used to bind Web ACD rows to this call).
	// excludeIDs lists acd_pool_targets ids already attempted for this inbound leg (SIP busy / no-answer chain).
	transferDialTarget func(context.Context, string, []uint) (outbound.DialTarget, bool)
	// WebSeatTransfer starts inbound ↔ browser WebRTC bridging when DialTarget.WebSeat (pool route_type web).
	// If nil and WebSeat is requested, transfer logs a warning and releases the dedupe slot.
	webSeatTransfer      func(inboundCallID string, lg *zap.Logger)
	transferStarted      sync.Map // inbound Call-ID -> bool (dedupe)
	transferRingMu       sync.Mutex
	transferRingStop     map[string]context.CancelFunc
	transferNoAgentRetry sync.Map // inbound Call-ID -> context.CancelFunc
)

// SetTransferDialer wires the outbound module (call from cmd/sip after creating outbound.Manager).
func SetTransferDialer(d TransferDialer) {
	transferMu.Lock()
	defer transferMu.Unlock()
	transferDialer = d
}

// SetTransferDialTargetResolver sets an optional resolver (e.g. acd_pool_targets).
// When it returns ok=false and the resolver is nil, outbound.TransferDialTargetFromEnv is used (standalone binaries).
func SetTransferDialTargetResolver(fn func(context.Context, string, []uint) (outbound.DialTarget, bool)) {
	transferMu.Lock()
	defer transferMu.Unlock()
	transferDialTarget = fn
}

// SetWebSeatTransfer registers the handler for WebSeat pool targets (browser agent). Optional until WebRTC gateway ships.
func SetWebSeatTransfer(fn func(inboundCallID string, lg *zap.Logger)) {
	transferMu.Lock()
	defer transferMu.Unlock()
	webSeatTransfer = fn
}

// HandleSIPINFODTMF parses SIP INFO (application/dtmf-relay). In script mode, digits wake listen waiters.
func HandleSIPINFODTMF(inboundCallID string, contentType, body string, lg *zap.Logger) {
	if lg == nil && logger.Lg != nil {
		lg = logger.Lg
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	if d, ok := sipdtmf.DigitFromSIPINFO(contentType, body); ok && isSIPScriptMode(inboundCallID) {
		scriptlisten.PublishDTMF(inboundCallID, d)
		lg.Info("sip info dtmf (script)",
			zap.String("call_id", inboundCallID),
			zap.String("digit", d),
		)
		return
	}
	lg.Info("sip info received (dtmf transfer disabled)",
		zap.String("call_id", inboundCallID),
		zap.String("content_type", strings.TrimSpace(contentType)),
		zap.Int("body_len", len(strings.TrimSpace(body))),
	)
}

// TriggerTransferToAgent starts transfer for an inbound call (AI/tool/fallback text).
func TriggerTransferToAgent(ctx context.Context, inboundCallID string, lg *zap.Logger) {
	transferMu.Lock()
	d := transferDialer
	resolveTgt := transferDialTarget
	webFn := webSeatTransfer
	transferMu.Unlock()

	var tgt outbound.DialTarget
	var ok bool
	if resolveTgt != nil {
		tgt, ok = resolveTgt(ctx, inboundCallID, transferExcludeSnapshot(inboundCallID))
	}
	// When cmd/sip wires a DB resolver, targets come only from acd_pool_targets — do not fall back to SIP_TRANSFER_* env.
	if !ok && resolveTgt == nil {
		tgt, ok = outbound.TransferDialTargetFromEnv()
	}
	if !ok {
		if resolveTgt != nil {
			lg.Warn("sip transfer: no eligible acd_pool_targets row (need weight>0, work_state=available, route sip|web; configure SIP fields on pool rows + trunks; web seat needs fresh heartbeat)")
		} else {
			lg.Warn("sip transfer: configure SIP_TRANSFER_* env for standalone mode, or wire SetTransferDialTargetResolver (ACD pool) in cmd/sip")
		}
		notifyTransferPhase(inboundCallID, "no_agent", map[string]any{"reason": "no_dial_target"})
		startTransferRinging(context.Background(), inboundCallID, lg)
		startNoAgentRetryLoop(inboundCallID, lg)
		return
	}

	if _, loaded := transferStarted.LoadOrStore(inboundCallID, true); loaded {
		lg.Info("sip transfer: already started for this call", zap.String("call_id", inboundCallID))
		return
	}
	stopNoAgentRetryLoop(inboundCallID)

	if tgt.ACDPoolTargetID != 0 {
		transferLastACDRowByInbound.Store(inboundCallID, tgt.ACDPoolTargetID)
	}

	notifyTransferPhase(inboundCallID, "requested", map[string]any{
		"web_seat": tgt.WebSeat,
	})

	if tgt.WebSeat {
		if webFn == nil {
			lg.Warn("sip transfer: WebSeat target but SetWebSeatTransfer not configured")
			notifyTransferPhase(inboundCallID, "failed", map[string]any{"reason": "webseat_not_configured"})
			webseat.ReleaseInboundWebACDOffer(inboundCallID)
			transferStarted.Delete(inboundCallID)
			return
		}
		lg.Info("sip transfer: web seat — handing off to WebRTC bridge", zap.String("inbound_call_id", inboundCallID))
		notifyTransferPhase(inboundCallID, "loading", nil)
		startTransferRinging(ctx, inboundCallID, lg)
		notifyTransferPhase(inboundCallID, "ringing", nil)
		scheduleWebSeatJoinWatch(inboundCallID, tgt.ACDPoolTargetID)
		go func() { webFn(inboundCallID, lg) }()
		return
	}

	if d == nil {
		lg.Warn("sip transfer: no TransferDialer (SetTransferDialer not called)")
		notifyTransferPhase(inboundCallID, "failed", map[string]any{"reason": "no_transfer_dialer"})
		transferStarted.Delete(inboundCallID)
		return
	}

	lg.Info("sip transfer: dialing agent leg", zap.String("inbound_call_id", inboundCallID), zap.String("agent_uri", tgt.RequestURI))
	notifyTransferPhase(inboundCallID, "loading", nil)
	startTransferRinging(ctx, inboundCallID, lg)
	notifyTransferPhase(inboundCallID, "ringing", nil)

	go func() {
		cid, err := d.Dial(ctx, outbound.DialRequest{
			Scenario:      outbound.ScenarioTransferAgent,
			Target:        tgt,
			CorrelationID: inboundCallID,
			MediaProfile:  outbound.MediaProfileTransferBridge,
		})
		if err != nil {
			stopTransferRinging(inboundCallID)
			transferStarted.Delete(inboundCallID)
			notifyTransferPhase(inboundCallID, "failed", map[string]any{"error": err.Error()})
			lg.Warn("sip transfer: outbound dial failed", zap.String("inbound_call_id", inboundCallID), zap.Error(err))
			return
		}
		lg.Info("sip transfer: agent leg INVITE sent", zap.String("inbound_call_id", inboundCallID), zap.String("outbound_call_id", cid))
	}()
}

func startTransferRinging(ctx context.Context, inboundCallID string, lg *zap.Logger) {
	inbound := lookupInboundSession(inboundCallID)
	if inbound == nil {
		return
	}
	transferRingMu.Lock()
	if transferRingStop == nil {
		transferRingStop = make(map[string]context.CancelFunc)
	}
	if _, exists := transferRingStop[inboundCallID]; exists {
		transferRingMu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	transferRingStop[inboundCallID] = cancel
	transferRingMu.Unlock()

	go func() {
		defer stopTransferRinging(inboundCallID)
		if err := playTransferRingingLoop(runCtx, inbound, lg); err != nil && !errorsIsCtxDone(err) {
			lg.Warn("sip transfer ring playback failed", zap.String("inbound_call_id", inboundCallID), zap.Error(err))
		}
	}()
}

func stopTransferRinging(inboundCallID string) {
	transferRingMu.Lock()
	cancel := transferRingStop[inboundCallID]
	delete(transferRingStop, inboundCallID)
	transferRingMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func lookupInboundSession(callID string) *sipSession.CallSession {
	if lookupInbound == nil {
		return nil
	}
	return lookupInbound(callID)
}

func playTransferRingingLoop(ctx context.Context, inbound *sipSession.CallSession, lg *zap.Logger) error {
	if inbound == nil {
		return fmt.Errorf("nil inbound session")
	}
	ms := inbound.MediaSession()
	if ms == nil {
		return fmt.Errorf("nil inbound media session")
	}
	path := utils.GetEnv("SIP_TRANSFER_RINGING_WAV_PATH")
	if path == "" {
		path = "scripts/ringing.wav"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Clean(path)
	}
	pcmSR := inbound.PCMSampleRate()
	if pcmSR <= 0 {
		pcmSR = 16000
	}
	pcm, err := LoadWAVAsPCM16Mono(path, pcmSR)
	if err != nil {
		return fmt.Errorf("load transfer ringing wav: %w", err)
	}
	bytesPerFrame := pcmSR * 2 * 20 / 1000
	if bytesPerFrame <= 0 {
		bytesPerFrame = 640
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	if lg != nil {
		lg.Info("sip transfer ring playback started", zap.Int("bytes", len(pcm)))
	}
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ms.GetContext().Done():
			return ms.GetContext().Err()
		case <-ticker.C:
		}
		if ActiveTransferBridgeForCallID(inbound.CallID) || ActiveWebSeatBridge(inbound.CallID) {
			return nil
		}
		end := offset + bytesPerFrame
		if end > len(pcm) {
			end = len(pcm)
		}
		frame := pcm[offset:end]
		if len(frame) > 0 {
			ms.SendToOutput("sip-transfer-ringing", &media.AudioPacket{
				Payload:       frame,
				IsSynthesized: true,
			})
		}
		offset = end
		if offset >= len(pcm) {
			offset = 0
		}
	}
}

func errorsIsCtxDone(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func startNoAgentRetryLoop(inboundCallID string, lg *zap.Logger) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	if _, exists := transferNoAgentRetry.Load(inboundCallID); exists {
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	if _, loaded := transferNoAgentRetry.LoadOrStore(inboundCallID, cancel); loaded {
		// Another goroutine raced us; cancel our context to avoid leak, keep the existing loop.
		cancel()
		return
	}
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		defer transferNoAgentRetry.Delete(inboundCallID)
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
			if ActiveTransferBridgeForCallID(inboundCallID) || ActiveWebSeatBridge(inboundCallID) {
				return
			}
			if _, active := transferStarted.Load(inboundCallID); active {
				return
			}
			inbound := lookupInboundSession(inboundCallID)
			if inbound == nil || inbound.MediaSession() == nil {
				return
			}
			TriggerTransferToAgent(context.Background(), inboundCallID, lg)
		}
	}()
}

func stopNoAgentRetryLoop(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	if v, ok := transferNoAgentRetry.LoadAndDelete(inboundCallID); ok && v != nil {
		if cancel, ok := v.(context.CancelFunc); ok && cancel != nil {
			cancel()
		}
	}
}

// IsTransferInProgress 表示该呼叫已进入「转人工」流程（候选/振铃/桥接任意阶段），
// 期间应停止 ASR/LLM 对话:此时主叫已切到 hold 音乐或坐席通话,继续跑 AI 会"AI 跟坐席抢话"。
//
// 返回 true 的条件（任一）：
//  1. transferStarted 标记位已置（TriggerTransferToAgent 已成功进入派单阶段，含 ringing/loading）。
//  2. 已建立 SIP 转接桥接（PSTN ↔ 坐席 RTP 桥）。
//  3. 已建立 Web 坐席桥接（PSTN ↔ 浏览器 WebRTC）。
func IsTransferInProgress(callID string) bool {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	if _, ok := transferStarted.Load(callID); ok {
		return true
	}
	if ActiveTransferBridgeForCallID(callID) {
		return true
	}
	if ActiveWebSeatBridge(callID) {
		return true
	}
	return false
}

// CleanupCallState releases all per-call transfer / script-mode state.
// Must be called on every call termination path (BYE, abort, hangup).
func CleanupCallState(callID string) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	transferStarted.Delete(callID)
	stopTransferRinging(callID)
	stopNoAgentRetryLoop(callID)
	ResetTransferRoutingState(callID)
	ClearSIPScriptMode(callID)
}
