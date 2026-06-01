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
	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
	"github.com/LinByte/VoiceServer/pkg/sip/sipagentpoll"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/welcomeaudio"
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
		logger.SafeGo("webseat-handoff", func() { webFn(inboundCallID, lg) })
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
	if tgt.ACDPoolTargetID != 0 {
		sipagentpoll.SetSIPAgentRinging(tgt.ACDPoolTargetID, inboundCallID, extractInboundCallerNumber(inboundCallID))
	}

	// 转接坐席时，From 的 SIP URI user 部分仍走中继配置的主叫号（避免运营商
	// ANI 校验拒接），但把 display-name 改成真实主叫（PSTN 用户）的手机号
	// —— 坐席话机屏幕上看到的是【谁打来的】，而不是 400 中继。
	// 出局 / 主动 dial（campaign）的 CallSession remoteFromHeader 是空，
	// 这种情况维持 Target.CallerDisplayName 默认值不动。
	agentDisplayOverride := extractInboundCallerNumber(inboundCallID)
	// PAI (RFC 3325) 透传真实主叫，From 维持中继外显号通过运营商 ANI 校验，
	// 同时让坐席话机 / 软电话从 P-Asserted-Identity 取出真实手机号显示。
	// URI 形态优先使用入局 From 的完整 sip:user@host（运营商原样回传，最贴近
	// "operator-validated" 的语义），否则回落 tel:<number>。
	agentAssertedURI := extractInboundCallerURI(inboundCallID)
	paiSource := "inbound_from"
	if agentAssertedURI == "" && agentDisplayOverride != "" {
		agentAssertedURI = "tel:" + agentDisplayOverride
		paiSource = "tel_fallback"
	}
	if agentAssertedURI == "" {
		paiSource = "none"
	}
	lg.Info("sip transfer: P-Asserted-Identity prepared",
		zap.String("inbound_call_id", inboundCallID),
		zap.String("from_user", strings.TrimSpace(tgt.CallerUser)),
		zap.String("from_display", agentDisplayOverride),
		zap.String("pai_uri", agentAssertedURI),
		zap.String("pai_display", agentDisplayOverride),
		zap.String("pai_source", paiSource),
	)
	// Pull the inbound INVITE's To + History-Info + Diversion headers so
	// we can extend the retarget chain on the outbound leg (RFC 7044 /
	// RFC 5806). Look up via the same session helper used elsewhere in
	// this file; nil session → empty headers → no chain emitted, which
	// is correct for outbound-originated CallSessions.
	var inboundTo, inboundHistory, inboundDiversion string
	if inSess := lookupInboundSession(inboundCallID); inSess != nil {
		inboundTo, inboundHistory, inboundDiversion = inSess.InboundRetargetHeaders()
	}

	logger.SafeGo("transfer-outbound-dial", func() {
		// 入局 BYE 可能在 SafeGo 调度之前就到达（PSTN 抖动 / 用户在
		// "正在为您转接"播报中挂断）。此时 transferStarted 已被
		// CleanupCallState 清掉，但本 goroutine 还没运行；如果再发
		// INVITE 给坐席，会出现"坐席响铃后无人对接"的孤儿 leg。
		// 这里 Dial 前再核一次 inbound session 还在不在，省一通无效
		// 外呼 + 后续 30s 的 ring-timeout 兜底窗口。
		if lookupInboundSession(inboundCallID) == nil {
			lg.Info("sip transfer: inbound gone before agent dial — abort",
				zap.String("inbound_call_id", inboundCallID))
			stopTransferRinging(inboundCallID)
			sipagentpoll.ClearByInbound(inboundCallID)
			transferStarted.Delete(inboundCallID)
			notifyTransferPhase(inboundCallID, "aborted", map[string]any{"reason": "inbound_gone"})
			return
		}
		req := outbound.DialRequest{
			Scenario:      outbound.ScenarioTransferAgent,
			Target:        tgt,
			CorrelationID: inboundCallID,
			MediaProfile:  outbound.MediaProfileTransferBridge,
		}
		if agentDisplayOverride != "" {
			// outbound.manager 的优先级是「DialRequest.CallerUser 非空 →
			// 用 DialRequest 的 user+display；否则 fallback 到 Target.*」，
			// 单独设 CallerDisplayName 会被丢弃。所以这里把 user 部分显式
			// 写成 Target.CallerUser（保留中继外显号通过 ANI 校验），同时
			// 把 display 改成主叫真实手机号。真正的"号码透传"靠 PAI 头
			// （见下面的 AssertedIdentity 设置），需中继支持。
			req.CallerUser = strings.TrimSpace(tgt.CallerUser)
			req.CallerDisplayName = agentDisplayOverride
		}
		if agentAssertedURI != "" {
			// PAI 走 RFC 3325 的"运营商已验证主叫"语义，display-name 取
			// 真实手机号方便坐席话机直接读出来。Privacy 头不设（默认 none），
			// 让中继 SBC / 坐席侧自由消费这条 PAI。
			req.AssertedIdentityURI = agentAssertedURI
			if agentDisplayOverride != "" {
				req.AssertedIdentityDisplayName = agentDisplayOverride
			}
		}
		// AI / ACD-driven retarget: cause=302 (Moved Temporarily) is the
		// closest fit and the Diversion reason is "unconditional"
		// because nothing about the original target signalled a
		// busy/no-answer/etc — we platform-decided to transfer.
		applyRetargetHeaders(&req, inboundTo, inboundHistory, inboundDiversion,
			`SIP;cause=302;text="Transfer"`,
			historyinfo.DiversionUnconditional,
		)
		cid, err := d.Dial(ctx, req)
		if err != nil {
			stopTransferRinging(inboundCallID)
			sipagentpoll.ClearByInbound(inboundCallID)
			transferStarted.Delete(inboundCallID)
			notifyTransferPhase(inboundCallID, "failed", map[string]any{"error": err.Error()})
			lg.Warn("sip transfer: outbound dial failed", zap.String("inbound_call_id", inboundCallID), zap.Error(err))
			return
		}
		lg.Info("sip transfer: agent leg INVITE sent", zap.String("inbound_call_id", inboundCallID), zap.String("outbound_call_id", cid))
		// 处理"d.Dial 期间用户挂断"的深层 race：BYE 在 dial 中途到达，
		// CleanupCallState → CancelPendingTransferLeg 跑时 transferPendingOutbound
		// 还没被 DialEventInvited 写入（同步事件比 BYE 处理慢几微秒），LoadAndDelete
		// 拿不到东西，结果坐席响铃直到 30s ring-timeout。dial 后再核一次 inbound
		// 存活，如果已挂就主动 CANCEL（requestCANCEL 是 idempotent，与上游路径并发安全）。
		if lookupInboundSession(inboundCallID) == nil {
			lg.Info("sip transfer: inbound went away during dial — cancelling agent leg",
				zap.String("inbound_call_id", inboundCallID),
				zap.String("outbound_call_id", cid))
			transferPendingOutbound.Delete(inboundCallID)
			transferLegCancelMu.Lock()
			fn := transferLegCancelFn
			transferLegCancelMu.Unlock()
			if fn != nil {
				if err := fn(cid); err != nil {
					lg.Warn("sip transfer: post-dial CANCEL failed",
						zap.String("outbound_call_id", cid), zap.Error(err))
				}
			}
			sipagentpoll.ClearByInbound(inboundCallID)
			transferStarted.Delete(inboundCallID)
		}
	})
}

// extractInboundCallerNumber 取该入站通话的 SIP From URI 的 user 部分（即
// 真实主叫的手机号），用于转接坐席时覆盖外呼 display-name。流程：
//
//  1. 从 CallSession.RemoteFromHeader() 拿到 INVITE 原始 From（含 display +
//     URI），sip/server 在收到 INVITE 时已经注入。
//  2. 解析出 `<sip:USER@host>` 或 `sip:USER@host` 里的 USER。
//
// 任一步失败返回 ""，调用方维持中继默认 CallerDisplayName 不动。
func extractInboundCallerNumber(inboundCallID string) string {
	inbound := lookupInboundSession(inboundCallID)
	if inbound == nil {
		return ""
	}
	hdr := inbound.RemoteFromHeader()
	if hdr == "" {
		return ""
	}
	return parseSIPUserPart(hdr)
}

// extractInboundCallerURI 取入站 INVITE From 头里完整的 sip:user@host
// （去掉 display-name / 角括号 / ;params），用作 PAI URI。运营商落地的
// From 通常带着它自己的 host，作为 PAI 透传给坐席侧最贴近 RFC 3325
// "operator-validated identity" 的语义。无法解析时返回 ""，调用方会
// 回落到 tel:<number> 形态。
func extractInboundCallerURI(inboundCallID string) string {
	inbound := lookupInboundSession(inboundCallID)
	if inbound == nil {
		return ""
	}
	hdr := inbound.RemoteFromHeader()
	if hdr == "" {
		return ""
	}
	return parseSIPNameAddrURI(hdr)
}

// parseSIPNameAddrURI 从 name-addr / addr-spec 形态的 SIP header 里提取
// 纯 URI（含 scheme，但不含 <>、display-name、header params）。
//
//	"Bob" <sip:bob@host;user=phone>;tag=xxx → sip:bob@host;user=phone
//	<sip:bob@host>                          → sip:bob@host
//	sip:bob@host;tag=xxx                    → sip:bob@host
func parseSIPNameAddrURI(header string) string {
	s := strings.TrimSpace(header)
	if s == "" {
		return ""
	}
	if lt := strings.Index(s, "<"); lt >= 0 {
		if gt := strings.Index(s[lt:], ">"); gt > 0 {
			return strings.TrimSpace(s[lt+1 : lt+gt])
		}
	}
	// addr-spec 形态：直接到第一个 header param 分隔符（;）就停。注意
	// 不能用 URI 内部的 ;user=phone 来截断 —— 但 addr-spec 形态下 SIP
	// 规范只允许 URI 没有外层包裹时把整个剩余串当 URI，header param 必须
	// 写在 <> 外，否则解析歧义。这里保守处理：取首个 ';' 之前。
	if i := strings.Index(s, ";"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseSIPUserPart 是 pkg/sip/persist.ExtractSIPUserPart 的精简同款实现，
// 内联以避免 conversation → persist 反向 import 依赖。支持常见三种形态：
//
//	"Bob" <sip:bob@host>;tag=...
//	<sip:bob@host:5060>
//	sip:bob@host
func parseSIPUserPart(header string) string {
	s := strings.TrimSpace(header)
	if s == "" {
		return ""
	}
	// 截掉 name-addr 外层的 <...>;params。
	if lt := strings.Index(s, "<"); lt >= 0 {
		if gt := strings.Index(s[lt:], ">"); gt > 0 {
			s = s[lt+1 : lt+gt]
		}
	}
	// 去掉 sip:/sips: scheme。
	if i := strings.Index(s, ":"); i >= 0 && (strings.EqualFold(s[:i], "sip") || strings.EqualFold(s[:i], "sips")) {
		s = s[i+1:]
	}
	// 截 user@host 的 user。
	if at := strings.Index(s, "@"); at >= 0 {
		s = s[:at]
	}
	// 去掉 ;params 等尾巴。
	if i := strings.IndexAny(s, ";?"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
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

	logger.SafeGo("transfer-ringing-loop", func() {
		defer stopTransferRinging(inboundCallID)
		if err := playTransferRingingLoop(runCtx, inbound, lg); err != nil && !errorsIsCtxDone(err) {
			lg.Warn("sip transfer ring playback failed", zap.String("inbound_call_id", inboundCallID), zap.Error(err))
		}
	})
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

// StopTransferRingingForCall cancels transfer hold/ring playback on the inbound leg (e.g. before WebSeat bridge).
func StopTransferRingingForCall(inboundCallID string) {
	stopTransferRinging(inboundCallID)
}

func lookupInboundSession(callID string) *sipSession.CallSession {
	if lookupInbound == nil {
		return nil
	}
	return lookupInbound(callID)
}

// LookupInboundCallSession exposes inbound CallSession lookup (e.g. voicedialog loopback LLM tenant wiring).
func LookupInboundCallSession(callID string) *sipSession.CallSession {
	return lookupInboundSession(callID)
}

func playTransferRingingLoop(ctx context.Context, inbound *sipSession.CallSession, lg *zap.Logger) error {
	if inbound == nil {
		return fmt.Errorf("nil inbound session")
	}
	ms := inbound.MediaSession()
	if ms == nil {
		return fmt.Errorf("nil inbound media session")
	}
	// Source resolution order, mirroring loadWelcomePCM:
	//   1) Per-DID TrunkNumber.TransferRingingURL via SetTransferRingingResolver.
	//      URL failures (unreachable / non-WAV) DO NOT fall back to local —
	//      surfacing the misconfiguration is more valuable than silently
	//      replaying a generic ringback.
	//   2) SIP_TRANSFER_RINGING_WAV_PATH env / scripts/ringing.wav (legacy).
	pcmSR := inbound.PCMSampleRate()
	if pcmSR <= 0 {
		pcmSR = 16000
	}
	var (
		pcm    []byte
		err    error
		source string
	)
	if u := strings.TrimSpace(ResolveTransferRingingURL(inbound.CallID)); u != "" {
		source = u
		pcm, err = welcomeaudio.FetchPCM(ctx, u, pcmSR, LoadWAVAsPCM16FromBytes)
		if err != nil {
			return fmt.Errorf("load transfer ringing url %q: %w", u, err)
		}
	} else {
		path := utils.GetEnv("SIP_TRANSFER_RINGING_WAV_PATH")
		if path == "" {
			path = "scripts/ringing.wav"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Clean(path)
		}
		source = path
		pcm, err = LoadWAVAsPCM16Mono(path, pcmSR)
		if err != nil {
			return fmt.Errorf("load transfer ringing wav %q: %w", path, err)
		}
	}
	bytesPerFrame := pcmSR * 2 * 20 / 1000
	if bytesPerFrame <= 0 {
		bytesPerFrame = 640
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	if lg != nil {
		lg.Info("sip transfer ring playback started",
			zap.Int("bytes", len(pcm)),
			zap.String("source", source))
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
			// 让转接振铃音乐进入立体声录音的 AI 通道，否则最终 WAV 里
			// 等待坐席的这段时间会是静音 —— 用户回放时无法判断到底是
			// AI 卡死还是确实在等坐席。recorder 在 inbound.PCMSampleRate()
			// 上工作，pcmSR 选取也以此为准，采样率一致无需重采样。
			inbound.WriteAIPCM(frame)
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
	logger.SafeGo("transfer-no-agent-retry", func() {
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
	})
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

// IsTransferRingingActive is true while hold/ring WAV is playing on the inbound leg.
func IsTransferRingingActive(callID string) bool {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	transferRingMu.Lock()
	defer transferRingMu.Unlock()
	if transferRingStop == nil {
		return false
	}
	_, ok := transferRingStop[callID]
	return ok
}

// IsTransferInProgress 表示该呼叫已进入「转人工」流程（候选/振铃/桥接任意阶段），
// 期间应停止 ASR/LLM 对话:此时主叫已切到 hold 音乐或坐席通话,继续跑 AI 会"AI 跟坐席抢话"。
//
// 返回 true 的条件（任一）：
//  1. transferStarted 标记位已置（TriggerTransferToAgent 已成功进入派单阶段，含 ringing/loading）。
//  2. 入局正在播放转接等待音（即使 Web 坐席 join 超时后清掉了 transferStarted，铃音仍可能循环）。
//  3. 已建立 SIP 转接桥接（PSTN ↔ 坐席 RTP 桥）。
//  4. 已建立 Web 坐席桥接（PSTN ↔ 浏览器 WebRTC）。
func IsTransferInProgress(callID string) bool {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	if _, ok := transferStarted.Load(callID); ok {
		return true
	}
	if IsTransferRingingActive(callID) {
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
	// 入局 BYE 到达时，桥接还没建立的转接 leg 仍在响铃 —— 必须给它
	// 发 CANCEL，否则坐席手机会一直响到运营商 no-answer 超时（60-180s）。
	// 已建立桥接的 leg 走 HangupTransferBridgeFull → bridgeSendOutboundBYE
	// 路径，这里 LoadAndDelete 取不到东西就 no-op，互不冲突。
	CancelPendingTransferLeg(callID)
	sipagentpoll.ClearByInbound(callID)
	transferStarted.Delete(callID)
	stopTransferRinging(callID)
	stopNoAgentRetryLoop(callID)
	ResetTransferRoutingState(callID)
	ClearSIPScriptMode(callID)
	cleanupSIPTransferConfirm(callID)
}
