package conversation

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
	"go.uber.org/zap"
)

// referTerminalNotifyCallbacks 跟踪每条 REFER 触发的外呼 leg 对应的
// sipfrag NOTIFY 回调。在 Established / Failed 事件触发时取出并调用，
// 确保 transferor PBX 收到的 NOTIFY 反映真实的被叫终态。
var referTerminalNotifyCallbacks sync.Map // outbound Call-ID -> func(sipfrag, subState)

func registerReferTerminalNotify(outboundCallID string, fn func(sipfragLine, subscriptionState string)) {
	outboundCallID = strings.TrimSpace(outboundCallID)
	if outboundCallID == "" || fn == nil {
		return
	}
	referTerminalNotifyCallbacks.Store(outboundCallID, fn)
}

// fireReferTerminalNotifyForEvent 是 HandleTransferAgentDialEvent 在
// Established / Failed 事件命中时的钩子：拉取回调（如有）并按 RFC 5589
// 的语义构造 sipfrag。Established → "SIP/2.0 200 OK"；Failed →
// "SIP/2.0 <code> <reason>"；subscription-state 一律 terminated。
func fireReferTerminalNotifyForEvent(evt outbound.DialEvent) {
	cid := strings.TrimSpace(evt.CallID)
	if cid == "" {
		return
	}
	v, ok := referTerminalNotifyCallbacks.LoadAndDelete(cid)
	if !ok || v == nil {
		return
	}
	fn, _ := v.(func(sipfragLine, subscriptionState string))
	if fn == nil {
		return
	}
	switch evt.State {
	case outbound.DialEventEstablished:
		fn("SIP/2.0 200 OK", "terminated;reason=noresource")
	case outbound.DialEventFailed:
		code := evt.StatusCode
		reason := strings.TrimSpace(evt.StatusText)
		if code <= 0 {
			code = 503
		}
		if reason == "" {
			reason = "Service Unavailable"
		}
		fn(fmt.Sprintf("SIP/2.0 %d %s", code, reason), "terminated;reason=giveup")
	}
}

// TriggerTransferFromReferTo starts the same outbound transfer bridge as TriggerTransferToAgent,
// but the dial target is taken from a SIP Refer-To header (sip:user@host[:port]).
// onTerminalNotify is optional: invoked once after the outbound INVITE dispatch returns (sipfrag status line + Subscription-State value).
func TriggerTransferFromReferTo(ctx context.Context, inboundCallID string, referToHeader string, lg *zap.Logger, onTerminalNotify func(sipfragLine, subscriptionState string)) {
	referToHeader = strings.TrimSpace(referToHeader)
	if inboundCallID == "" || referToHeader == "" {
		return
	}
	tgt, err := outbound.DialTargetFromReferTo(referToHeader)
	if err != nil {
		if lg != nil {
			lg.Warn("sip refer: bad Refer-To", zap.String("call_id", inboundCallID), zap.Error(err))
		}
		return
	}
	transferMu.Lock()
	d := transferDialer
	transferMu.Unlock()

	if _, loaded := transferStarted.LoadOrStore(inboundCallID, true); loaded {
		if lg != nil {
			lg.Info("sip refer: transfer already started", zap.String("call_id", inboundCallID))
		}
		return
	}

	notifyTransferPhase(inboundCallID, "requested", map[string]any{"refer_to": referToHeader})

	if d == nil {
		if lg != nil {
			lg.Warn("sip refer: no TransferDialer (SetTransferDialer not called)", zap.String("call_id", inboundCallID))
		}
		notifyTransferPhase(inboundCallID, "failed", map[string]any{"reason": "no_transfer_dialer"})
		transferStarted.Delete(inboundCallID)
		return
	}

	if lg != nil {
		lg.Info("sip refer: dialing target", zap.String("inbound_call_id", inboundCallID), zap.String("request_uri", tgt.RequestURI))
	} else if logger.Lg != nil {
		logger.Lg.Info("sip refer: dialing target", zap.String("inbound_call_id", inboundCallID), zap.String("request_uri", tgt.RequestURI))
	}

	notifyTransferPhase(inboundCallID, "loading", nil)
	startTransferRinging(ctx, inboundCallID, lg)
	notifyTransferPhase(inboundCallID, "ringing", nil)

	// Pull inbound retarget headers so we extend any upstream chain
	// rather than synthesise a fresh one. For REFER specifically the
	// Refer-To often originates inside the trust domain (the calling
	// PBX explicitly asking us to retarget), so the Diversion reason
	// is "deflection" per RFC 5806 §4.1.1.
	var inboundTo, inboundHistory, inboundDiversion string
	if inSess := lookupInboundSession(inboundCallID); inSess != nil {
		inboundTo, inboundHistory, inboundDiversion = inSess.InboundRetargetHeaders()
	}

	go func() {
		req := outbound.DialRequest{
			Scenario:      outbound.ScenarioTransferAgent,
			Target:        tgt,
			CorrelationID: inboundCallID,
			MediaProfile:  outbound.MediaProfileTransferBridge,
		}
		applyRetargetHeaders(&req, inboundTo, inboundHistory, inboundDiversion,
			`SIP;cause=302;text="REFER"`,
			historyinfo.DiversionDeflection,
		)
		cid, err := d.Dial(ctx, req)
		// RFC 5589 §6 / RFC 3515: REFER 的 NOTIFY sipfrag 应该反映
		// 被 refer 出去的那条新 dialog 的【真实】最终状态——200 表示
		// 被叫摘机、4xx/5xx/6xx 表示拒接 / 超时 / 不可达。早期版本在
		// `d.Dial` 同步返回后立即发 NOTIFY，等于只反映"INVITE 发没发出"，
		// 严格的转移发起方（Cisco/Avaya PBX）会以为转接成功而提前挂掉
		// transferor leg，结果用户在静音中等被叫超时。
		//
		// 现在改成：Dial 同步失败（连 INVITE 都没发出）→ 立即 NOTIFY；
		// Dial 成功 → 把 onTerminalNotify 挂到 outbound CallID 上，由
		// HandleTransferAgentDialEvent 在 Established / Failed 事件触发。
		if err != nil {
			if onTerminalNotify != nil {
				onTerminalNotify("SIP/2.0 503 Service Unavailable", "terminated;reason=giveup")
			}
			stopTransferRinging(inboundCallID)
			transferStarted.Delete(inboundCallID)
			notifyTransferPhase(inboundCallID, "failed", map[string]any{"error": err.Error()})
			if lg != nil {
				lg.Warn("sip refer: dial failed", zap.String("inbound_call_id", inboundCallID), zap.Error(err))
			}
			return
		}
		if onTerminalNotify != nil {
			registerReferTerminalNotify(cid, onTerminalNotify)
		}
		if lg != nil {
			lg.Info("sip refer: outbound INVITE sent", zap.String("inbound_call_id", inboundCallID), zap.String("outbound_call_id", cid))
		}
	}()
}
