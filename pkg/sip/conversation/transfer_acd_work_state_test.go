package conversation

import (
	"context"
	"sync"
	"testing"
)

func TestTransferACDWorkStateLifecycle(t *testing.T) {
	var mu sync.Mutex
	got := map[uint]string{}

	SetACDPoolTargetWorkStateUpdater(func(_ context.Context, targetID uint, workState string) error {
		mu.Lock()
		got[targetID] = workState
		mu.Unlock()
		return nil
	})
	t.Cleanup(func() {
		SetACDPoolTargetWorkStateUpdater(nil)
		transferLastACDRowByInbound.Delete("in-1")
	})

	transferLastACDRowByInbound.Store("in-1", uint(42))

	markTransferACDWorkState(42, "ringing")
	markTransferACDWorkStateForCall("in-1", "busy")
	releaseTransferACDWorkState("in-1")

	mu.Lock()
	defer mu.Unlock()
	if got[42] != "available" {
		t.Fatalf("final work_state = %q, want available", got[42])
	}
}

func TestResolveInboundCallIDForTransfer(t *testing.T) {
	transferLastACDRowByInbound.Store("in-2", uint(7))
	t.Cleanup(func() { transferLastACDRowByInbound.Delete("in-2") })

	if got := ResolveInboundCallIDForTransfer("in-2"); got != "in-2" {
		t.Fatalf("inbound resolve = %q", got)
	}
}
