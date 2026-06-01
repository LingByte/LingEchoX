package sipagentpoll

import (
	"testing"
	"time"
)

func TestSubscribeACDChangesNotifies(t *testing.T) {
	ch, cancel := SubscribeACDChanges([]uint{42})
	defer cancel()

	done := make(chan uint, 1)
	go func() {
		select {
		case id := <-ch:
			done <- id
		case <-time.After(2 * time.Second):
		}
	}()

	SetSIPAgentRinging(42, "call-sse-1", "13900000001")

	select {
	case id := <-done:
		if id != 42 {
			t.Fatalf("got acd %d", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notify")
	}

	ClearByInbound("call-sse-1")
}
