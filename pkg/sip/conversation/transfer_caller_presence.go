package conversation

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/sip/sipagentpoll"
	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
	"go.uber.org/zap"
)

// transferCallerHungUp marks inbound legs whose PSTN caller has already hung up.
// Retries scheduled before CleanupCallState finishes use this to avoid dialing the next agent.
var transferCallerHungUp sync.Map // inbound Call-ID -> struct{}

func inboundCallerStillPresentForTransfer(inboundCallID string) bool {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return false
	}
	if _, hungUp := transferCallerHungUp.Load(inboundCallID); hungUp {
		return false
	}
	inbound := lookupInboundSession(inboundCallID)
	if inbound == nil {
		return false
	}
	return inbound.MediaSession() != nil
}

func markTransferCallerHungUp(callID string) {
	inbound := ResolveInboundCallIDForTransfer(callID)
	if inbound == "" {
		inbound = strings.TrimSpace(callID)
	}
	if inbound == "" {
		return
	}
	transferCallerHungUp.Store(inbound, struct{}{})
}

func clearTransferCallerHungUp(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	transferCallerHungUp.Delete(inboundCallID)
}

// abandonTransferBecauseCallerGone stops retry/ring state when the PSTN caller is already gone.
func abandonTransferBecauseCallerGone(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	stopTransferRinging(inboundCallID)
	cancelWebSeatJoinWatch(inboundCallID)
	stopNoAgentRetryLoop(inboundCallID)
	sipagentpoll.ClearByInbound(inboundCallID)
	releaseTransferACDWorkState(inboundCallID)
	transferStarted.Delete(inboundCallID)
}

func scheduleTransferRetryToNextAgent(inboundCallID string, lg *zap.Logger) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	if !inboundCallerStillPresentForTransfer(inboundCallID) {
		abandonTransferBecauseCallerGone(inboundCallID)
		return
	}
	if lg == nil {
		lg = logger.Lg
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	logger.SafeGo("transfer-retry-next-agent", func() {
		time.Sleep(60 * time.Millisecond)
		if !inboundCallerStillPresentForTransfer(inboundCallID) {
			abandonTransferBecauseCallerGone(inboundCallID)
			return
		}
		TriggerTransferToAgent(context.Background(), inboundCallID, lg)
	})
}

// transferFailureAgentRejected reports explicit agent-side reject (SIP 486/603 etc.), not no-answer timeout.
func transferFailureAgentRejected(code int, reason string) bool {
	switch code {
	case 486, 600, 603:
		return true
	}
	r := strings.ToLower(strings.TrimSpace(reason))
	return strings.Contains(r, "decline") ||
		strings.Contains(r, "busy here") ||
		strings.Contains(r, "call rejected")
}

// AbortTransferOnAgentReject clears in-flight transfer state when an agent explicitly rejects.
// Call before hanging up the PSTN leg (web HangupFull or SIP RequestSIPHangup).
func AbortTransferOnAgentReject(inboundCallID string) {
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return
	}
	markTransferCallerHungUp(inboundCallID)
	stopTransferRinging(inboundCallID)
	cancelWebSeatJoinWatch(inboundCallID)
	stopNoAgentRetryLoop(inboundCallID)
	sipagentpoll.ClearByInbound(inboundCallID)
	releaseTransferACDWorkState(inboundCallID)
	webseat.ReleaseInboundWebACDOffer(inboundCallID)
	transferStarted.Delete(inboundCallID)
	transferLastACDRowByInbound.Delete(inboundCallID)
	transferExcludeReset(inboundCallID)
}

func terminateTransferBecauseAgentRejected(inbound string, sipCode int, reason string) {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return
	}
	if id, ok := PeekInboundTransferACDTargetID(inbound); ok && id > 0 {
		RecordTransferRejected(inbound, id)
	}
	AbortTransferOnAgentReject(inbound)
	notifyTransferPhase(inbound, "agent_rejected", map[string]any{
		"sip_code": sipCode,
		"reason":   strings.TrimSpace(reason),
	})
	RequestSIPHangup(inbound)
}
