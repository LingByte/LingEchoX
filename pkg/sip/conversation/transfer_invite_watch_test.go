package conversation

import (
	"context"
	"sync"
	"testing"
)

func TestReleaseTransferRingingSeatForRetry(t *testing.T) {
	var mu sync.Mutex
	got := map[uint]string{}
	var excluded []uint

	SetACDPoolTargetWorkStateUpdater(func(_ context.Context, targetID uint, workState string) error {
		mu.Lock()
		got[targetID] = workState
		mu.Unlock()
		return nil
	})
	t.Cleanup(func() {
		SetACDPoolTargetWorkStateUpdater(nil)
		transferLastACDRowByInbound.Delete("in-timeout")
		transferExcludeReset("in-timeout")
	})

	transferLastACDRowByInbound.Store("in-timeout", uint(99))
	id := releaseTransferRingingSeatForRetry("in-timeout")
	if id != 99 {
		t.Fatalf("rowID = %d, want 99", id)
	}

	mu.Lock()
	if got[99] != "available" {
		t.Fatalf("work_state = %q, want available", got[99])
	}
	mu.Unlock()

	excluded = transferExcludeSnapshot("in-timeout")
	if len(excluded) != 1 || excluded[0] != 99 {
		t.Fatalf("exclude = %v, want [99]", excluded)
	}
}
