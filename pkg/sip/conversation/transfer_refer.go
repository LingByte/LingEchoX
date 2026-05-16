package conversation

import (
	"context"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
	"github.com/LinByte/VoiceServer/pkg/sip/outbound"
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
