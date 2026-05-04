package persist

import (
	"context"
	"testing"
	"time"
)

func TestJSONStoresRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := NewJSONStores(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now()
	c := &SIPCall{
		CallID:    "cid-1",
		Direction: DirectionInbound,
		State:     SIPCallStateRinging,
		InviteAt:  &now,
		EndStatus: SIPCallEndUnknown,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := st.Calls.CreateSIPCall(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := st.Calls.UpdateSIPCall(ctx, &SIPCall{
		CallID: c.CallID,
		State:  SIPCallStateEstablished,
		AckAt:  &now,
	}); err != nil {
		t.Fatal(err)
	}
	u := &SIPUser{
		Username: "alice",
		Domain:   "example.com",
		Online:   true,
	}
	if err := st.Users.UpsertUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, err := st.Users.GetUser(ctx, "alice", "example.com")
	if err != nil || got == nil || got.Username != "alice" {
		t.Fatalf("user: err=%v got=%v", err, got)
	}
}
