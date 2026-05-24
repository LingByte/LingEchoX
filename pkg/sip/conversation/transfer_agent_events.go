package conversation

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"go.uber.org/zap"
)

var (
	transferInviteTimers sync.Map // outbound Call-ID -> *time.Timer
	webSeatJoinTimers    sync.Map // inbound Call-ID -> *time.Timer
	transferLegAbandonMu sync.Mutex
	transferLegAbandonFn func(callID string) bool

	transferExcludeMu           sync.Mutex
	transferExcludeByInbound    = map[string][]uint{}
	transferLastACDRowByInbound sync.Map // inbound Call-ID -> uint (acd_pool_targets.id)

	// transferPendingOutbound 维护 inbound Call-ID -> outbound Call-ID
	// 的映射，仅在转接外呼 leg 已经发出 INVITE 但还没建立桥接（即坐席手机
	// 还在响铃 / 没接听）的窗口期内有效。CleanupCallState（入局 BYE）查
	// 这张表来决定要不要给那条还在响的坐席 leg 发 CANCEL。
	transferPendingOutbound sync.Map // inbound -> outbound

	transferLegCancelMu sync.Mutex
	transferLegCancelFn func(outboundCallID string) error
)

// SetTransferLegAbandoner wires outbound.Manager.AbandonEarlyTransferInvite for ring-timeout cleanup.
func SetTransferLegAbandoner(fn func(callID string) bool) {
	transferLegAbandonMu.Lock()
	defer transferLegAbandonMu.Unlock()
	transferLegAbandonFn = fn
}

// SetTransferLegCanceller wires outbound.Manager.SendCANCEL so that a
// PSTN caller hangup mid-ring tears down the agent-side INVITE
// instead of letting the agent's phone ring until carrier no-answer
// timeout (60-180s). Wired in cmd/sip after Manager construction.
func SetTransferLegCanceller(fn func(outboundCallID string) error) {
	transferLegCancelMu.Lock()
	defer transferLegCancelMu.Unlock()
	transferLegCancelFn = fn
}

// CancelPendingTransferLeg cancels an in-flight transfer-agent
// INVITE keyed by its inbound Call-ID. No-op when no transfer is
// pending for this call (campaign legs, already-bridged transfers,
// or never-transferred calls). Called from CleanupCallState on
// inbound hangup.
func CancelPendingTransferLeg(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	v, ok := transferPendingOutbound.LoadAndDelete(inboundCallID)
	if !ok || v == nil {
		return
	}
	outID, _ := v.(string)
	outID = strings.TrimSpace(outID)
	if outID == "" {
		return
	}
	transferLegCancelMu.Lock()
	fn := transferLegCancelFn
	transferLegCancelMu.Unlock()
	if fn == nil {
		return
	}
	cancelTransferInviteWatch(outID)
	if err := fn(outID); err != nil {
		lg := logger.Lg
		if lg == nil {
			lg = zap.NewNop()
		}
		lg.Warn("sip transfer: CANCEL pending agent leg failed",
			zap.String("inbound_call_id", inboundCallID),
			zap.String("outbound_call_id", outID),
			zap.Error(err))
		return
	}
	lg := logger.Lg
	if lg != nil {
		lg.Info("sip transfer: CANCEL sent for pending agent leg",
			zap.String("inbound_call_id", inboundCallID),
			zap.String("outbound_call_id", outID))
	}
}

func abandonTransferLeg(callID string) bool {
	transferLegAbandonMu.Lock()
	fn := transferLegAbandonFn
	transferLegAbandonMu.Unlock()
	if fn == nil {
		return false
	}
	return fn(callID)
}

func transferExcludeSnapshot(inbound string) []uint {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return nil
	}
	transferExcludeMu.Lock()
	defer transferExcludeMu.Unlock()
	s := transferExcludeByInbound[inbound]
	out := make([]uint, len(s))
	copy(out, s)
	return out
}

func transferExcludeAdd(inbound string, id uint) {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" || id == 0 {
		return
	}
	transferExcludeMu.Lock()
	defer transferExcludeMu.Unlock()
	for _, x := range transferExcludeByInbound[inbound] {
		if x == id {
			return
		}
	}
	transferExcludeByInbound[inbound] = append(transferExcludeByInbound[inbound], id)
}

func transferExcludeReset(inbound string) {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return
	}
	transferExcludeMu.Lock()
	delete(transferExcludeByInbound, inbound)
	transferExcludeMu.Unlock()
}

func transferAnswerTimeout() time.Duration {
	const def = 30 * time.Second
	raw := utils.GetEnv("SIP_TRANSFER_ANSWER_TIMEOUT_MS")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > 120000 {
		n = 120000
	}
	return time.Duration(n) * time.Millisecond
}

func scheduleTransferInviteWatch(inbound, outbound string) {
	inbound = strings.TrimSpace(inbound)
	outbound = strings.TrimSpace(outbound)
	if inbound == "" || outbound == "" {
		return
	}
	cancelTransferInviteWatch(outbound)
	d := transferAnswerTimeout()
	t := time.AfterFunc(d, func() {
		v, loaded := transferInviteTimers.LoadAndDelete(outbound)
		if !loaded || v == nil {
			return
		}
		if ActiveTransferBridgeForCallID(inbound) || ActiveWebSeatBridge(inbound) {
			return
		}
		_ = abandonTransferLeg(outbound)
	})
	transferInviteTimers.Store(outbound, t)
}

func cancelTransferInviteWatch(outbound string) {
	outbound = strings.TrimSpace(outbound)
	if outbound == "" {
		return
	}
	if v, ok := transferInviteTimers.LoadAndDelete(outbound); ok && v != nil {
		if t, ok := v.(*time.Timer); ok {
			t.Stop()
		}
	}
}

// MigrateTransferInviteOutboundCallID reschedules the ring-timeout timer when the outbound dialog
// Call-ID from 200 OK differs from the INVITE Call-ID. The timer callback captures the old id, so
// we stop the old timer and schedule a fresh watch keyed by newID.
func MigrateTransferInviteOutboundCallID(inbound, oldID, newID string) {
	inbound = strings.TrimSpace(inbound)
	oldID = strings.TrimSpace(oldID)
	newID = strings.TrimSpace(newID)
	if inbound == "" || oldID == "" || newID == "" || oldID == newID {
		return
	}
	v, ok := transferInviteTimers.LoadAndDelete(oldID)
	if !ok || v == nil {
		return
	}
	t, ok := v.(*time.Timer)
	if !ok {
		return
	}
	if t.Stop() {
		scheduleTransferInviteWatch(inbound, newID)
	}
}

func scheduleWebSeatJoinWatch(inbound string, acdTargetID uint) {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return
	}
	cancelWebSeatJoinWatch(inbound)
	d := transferAnswerTimeout()
	t := time.AfterFunc(d, func() {
		v, loaded := webSeatJoinTimers.LoadAndDelete(inbound)
		if !loaded || v == nil {
			return
		}
		if ActiveTransferBridgeForCallID(inbound) || ActiveWebSeatBridge(inbound) {
			return
		}
		if acdTargetID != 0 {
			transferExcludeAdd(inbound, acdTargetID)
		}
		webseat.ReleaseInboundWebACDOffer(inbound)
		transferStarted.Delete(inbound)
		transferLastACDRowByInbound.Delete(inbound)
		notifyTransferPhase(inbound, "retrying", map[string]any{"reason": "webseat_join_timeout"})
		lg := logger.Lg
		if lg == nil {
			lg = zap.NewNop()
		}
		logger.SafeGo("transfer-retry-after-fail", func() {
			time.Sleep(60 * time.Millisecond)
			TriggerTransferToAgent(context.Background(), inbound, lg)
		})
	})
	webSeatJoinTimers.Store(inbound, t)
}

func cancelWebSeatJoinWatch(inbound string) {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return
	}
	if v, ok := webSeatJoinTimers.LoadAndDelete(inbound); ok && v != nil {
		if t, ok := v.(*time.Timer); ok {
			t.Stop()
		}
	}
}

func transferFailureRetryable(code int, reason string) bool {
	r := strings.ToLower(strings.TrimSpace(reason))
	if strings.Contains(r, "transfer_invite_timeout") {
		return true
	}
	switch code {
	case 404, 410:
		return false
	default:
		if code == 0 {
			return true
		}
		return code >= 400
	}
}

// HandleTransferAgentDialEvent handles outbound lifecycle for blind transfer agent legs (retry next agent on busy / timeout).
func HandleTransferAgentDialEvent(evt outbound.DialEvent) {
	if evt.Scenario != outbound.ScenarioTransferAgent || evt.MediaProfile != outbound.MediaProfileTransferBridge {
		return
	}
	inbound := strings.TrimSpace(evt.CorrelationID)
	if inbound == "" {
		return
	}
	switch evt.State {
	case outbound.DialEventInvited:
		// 记录 inbound→outbound 映射，让入局 BYE 在桥接建立前能 CANCEL
		// 那条还在响铃的坐席 leg。Established / Failed 时清掉，避免给已
		// 经接通（用 BYE 处理）或已失败（无需 CANCEL）的 leg 再发 CANCEL。
		transferPendingOutbound.Store(inbound, evt.CallID)
		scheduleTransferInviteWatch(inbound, evt.CallID)
	case outbound.DialEventEstablished:
		transferPendingOutbound.Delete(inbound)
		cancelTransferInviteWatch(evt.CallID)
		// RFC 5589 §6: REFER 触发的外呼现在真正接通，给 transferor 发
		// "SIP/2.0 200 OK" sipfrag NOTIFY（如有挂载回调）。非 REFER 路径
		// 这里 LoadAndDelete 取不到东西，no-op。
		fireReferTerminalNotifyForEvent(evt)
	case outbound.DialEventFailed:
		transferPendingOutbound.Delete(inbound)
		cancelTransferInviteWatch(evt.CallID)
		// 失败也走 NOTIFY（同上），sipfrag 反映真实 SIP 状态码。
		fireReferTerminalNotifyForEvent(evt)
		onTransferAgentLegFailed(inbound, evt)
	default:
	}
}

func onTransferAgentLegFailed(inbound string, evt outbound.DialEvent) {
	if ActiveTransferBridgeForCallID(inbound) || ActiveWebSeatBridge(inbound) {
		return
	}
	lg := logger.Lg
	if lg == nil {
		lg = zap.NewNop()
	}
	transferMu.Lock()
	hasResolver := transferDialTarget != nil
	transferMu.Unlock()
	if !hasResolver {
		transferExcludeReset(inbound)
		transferStarted.Delete(inbound)
		notifyTransferPhase(inbound, "failed", map[string]any{"sip_code": evt.StatusCode, "reason": evt.Reason})
		startTransferRinging(context.Background(), inbound, lg)
		startNoAgentRetryLoop(inbound, lg)
		return
	}
	if !transferFailureRetryable(evt.StatusCode, evt.Reason) {
		transferExcludeReset(inbound)
		transferStarted.Delete(inbound)
		notifyTransferPhase(inbound, "failed", map[string]any{"sip_code": evt.StatusCode, "reason": evt.Reason})
		startTransferRinging(context.Background(), inbound, lg)
		startNoAgentRetryLoop(inbound, lg)
		return
	}
	var rowID uint
	if v, ok := transferLastACDRowByInbound.Load(inbound); ok {
		if id, ok := v.(uint); ok {
			rowID = id
		}
	}
	if rowID != 0 {
		transferExcludeAdd(inbound, rowID)
	}
	webseat.ReleaseInboundWebACDOffer(inbound)
	transferStarted.Delete(inbound)
	transferLastACDRowByInbound.Delete(inbound)

	notifyTransferPhase(inbound, "retrying", map[string]any{"sip_code": evt.StatusCode, "reason": evt.Reason})
	logger.SafeGo("transfer-retry-on-status", func() {
		time.Sleep(60 * time.Millisecond)
		TriggerTransferToAgent(context.Background(), inbound, lg)
	})
}

// ResetTransferRoutingState clears per-call transfer routing scratch state after a successful handoff.
//
// 注意：这里**不**清理 transferLastACDRowByInbound，因为 OnBye 阶段
// （pkg/sip/persist.CallStore.OnBye → TakeInboundTransferACDTargetID）
// 还要用它把 acd_pool_targets.id 落到 sip_calls.transfer_acd_target_id。
// 之前 OnWebSeatBridgeEstablished 一调本函数就把映射删了，结果 OnBye
// 拿到 0 → DB 不写 → 列表"转接"列就只能显示"未知坐席"。
// TakeInboundTransferACDTargetID 内部用 LoadAndDelete，BYE 之后会自动
// 清掉，不会泄漏。
func ResetTransferRoutingState(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	transferExcludeReset(inboundCallID)
	cancelWebSeatJoinWatch(inboundCallID)
	stopNoAgentRetryLoop(inboundCallID)
}

// OnWebSeatJoinTimeout is invoked after the browser seat misses the join deadline; tries the next ACD target.
func OnWebSeatJoinTimeout(inboundCallID string, acdTargetID uint) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	if acdTargetID != 0 {
		transferExcludeAdd(inboundCallID, acdTargetID)
	}
	cancelWebSeatJoinWatch(inboundCallID)
	transferStarted.Delete(inboundCallID)
	transferLastACDRowByInbound.Delete(inboundCallID)
	webseat.ReleaseInboundWebACDOffer(inboundCallID)
	lg := logger.Lg
	if lg == nil {
		lg = zap.NewNop()
	}
	logger.SafeGo("transfer-retry-join-timeout", func() {
		time.Sleep(60 * time.Millisecond)
		TriggerTransferToAgent(context.Background(), inboundCallID, lg)
	})
}
