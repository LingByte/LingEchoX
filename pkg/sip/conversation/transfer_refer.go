package conversation

import (
	"context"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/sip/outbound"
	"go.uber.org/zap"
)

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

	go func() {
		cid, err := d.Dial(ctx, outbound.DialRequest{
			Scenario:      outbound.ScenarioTransferAgent,
			Target:        tgt,
			CorrelationID: inboundCallID,
			MediaProfile:  outbound.MediaProfileTransferBridge,
		})
		if onTerminalNotify != nil {
			if err != nil {
				onTerminalNotify("SIP/2.0 603 Decline", "terminated;reason=giveup")
			} else {
				onTerminalNotify("SIP/2.0 200 OK", "terminated;reason=noresource")
			}
		}
		if err != nil {
			stopTransferRinging(inboundCallID)
			transferStarted.Delete(inboundCallID)
			notifyTransferPhase(inboundCallID, "failed", map[string]any{"error": err.Error()})
			if lg != nil {
				lg.Warn("sip refer: dial failed", zap.String("inbound_call_id", inboundCallID), zap.Error(err))
			}
			return
		}
		if lg != nil {
			lg.Info("sip refer: outbound INVITE sent", zap.String("inbound_call_id", inboundCallID), zap.String("outbound_call_id", cid))
		}
	}()
}
