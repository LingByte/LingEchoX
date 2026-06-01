package conversation

import (
	"context"
	"strings"
	"sync"
)

var (
	acdPoolWorkStateMu sync.RWMutex
	acdPoolWorkStateFn func(ctx context.Context, targetID uint, workState string) error
)

// SetACDPoolTargetWorkStateUpdater wires acd_pool_targets.work_state updates from the transfer layer.
func SetACDPoolTargetWorkStateUpdater(fn func(ctx context.Context, targetID uint, workState string) error) {
	acdPoolWorkStateMu.Lock()
	acdPoolWorkStateFn = fn
	acdPoolWorkStateMu.Unlock()
}

func markTransferACDWorkState(targetID uint, workState string) {
	targetID = uint(targetID)
	workState = strings.TrimSpace(workState)
	if targetID == 0 || workState == "" {
		return
	}
	acdPoolWorkStateMu.RLock()
	fn := acdPoolWorkStateFn
	acdPoolWorkStateMu.RUnlock()
	if fn == nil {
		return
	}
	_ = fn(context.Background(), targetID, workState)
}

func markTransferACDWorkStateForCall(callID string, workState string) {
	inbound := ResolveInboundCallIDForTransfer(callID)
	if inbound == "" {
		return
	}
	if id, ok := PeekInboundTransferACDTargetID(inbound); ok && id > 0 {
		markTransferACDWorkState(id, workState)
	}
}

// releaseTransferACDWorkState sets the routed seat back to available (post-call / retry / cancel).
func releaseTransferACDWorkState(callID string) {
	markTransferACDWorkStateForCall(callID, "available")
}
