package sipagentpoll

import "testing"

func TestSetClearRinging(t *testing.T) {
	SetSIPAgentRinging(7, "inbound-1", "13800138000")
	s := SnapshotByACDTarget(7)
	if !s.Incoming || s.InboundCallID != "inbound-1" || s.CallerNumber != "13800138000" {
		t.Fatalf("snapshot: %+v", s)
	}
	ClearByInbound("inbound-1")
	if SnapshotByACDTarget(7).Incoming {
		t.Fatal("expected cleared")
	}
}

func TestRetargetClearsPreviousACD(t *testing.T) {
	SetSIPAgentRinging(1, "call-a", "100")
	SetSIPAgentRinging(2, "call-a", "100")
	if SnapshotByACDTarget(1).Incoming {
		t.Fatal("seat 1 should be cleared after retarget")
	}
	if !SnapshotByACDTarget(2).Incoming {
		t.Fatal("seat 2 should be ringing")
	}
	ClearByInbound("call-a")
}

func TestResolveSnapshotClearsStaleMemory(t *testing.T) {
	SetSIPAgentRinging(9, "stale-inbound", "13800000000")
	s := ResolveSnapshot(9, func(inbound string) bool {
		return inbound == "stale-inbound"
	})
	if !s.Incoming {
		t.Fatal("expected incoming while live")
	}
	s = ResolveSnapshot(9, func(string) bool { return false })
	if s.Incoming {
		t.Fatal("expected cleared when transfer no longer live")
	}
	if SnapshotByACDTarget(9).Incoming {
		t.Fatal("memory should be cleared")
	}
}
