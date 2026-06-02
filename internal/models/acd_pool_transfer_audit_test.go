package models

import (
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
)

func TestACDPoolTransferRejectReasons(t *testing.T) {
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	fresh := now.Add(-constants.WebSeatStaleAfter)
	loc := time.UTC
	p := ACDPoolTransferAuditParams{TenantID: 1, InboundTrunkNumberID: 5}
	exclude := map[uint]struct{}{99: {}}

	row := ACDPoolTarget{
		BaseModel:     BaseModel{ID: 99},
		TrunkNumberID: 5,
		RouteType:     constants.ACDPoolRouteTypeSIP,
		TargetValue:   "1001",
		Weight:        10,
		WorkState:     constants.ACDWorkStateAvailable,
	}
	if r := ACDPoolTransferRejectReasons(row, p, exclude, fresh, now, loc); len(r) != 1 || r[0] != "already_attempted_this_call" {
		t.Fatalf("exclude: %#v", r)
	}

	row.ID = 1
	if r := ACDPoolTransferRejectReasons(row, p, exclude, fresh, now, loc); len(r) != 0 {
		t.Fatalf("eligible sip: %#v", r)
	}

	row.WorkState = constants.ACDWorkStateOffline
	if r := ACDPoolTransferRejectReasons(row, p, exclude, fresh, now, loc); len(r) != 1 || r[0] != "work_state=offline" {
		t.Fatalf("offline: %#v", r)
	}

	row.WorkState = constants.ACDWorkStateAvailable
	row.RouteType = constants.ACDPoolRouteTypeWeb
	if r := ACDPoolTransferRejectReasons(row, p, exclude, fresh, now, loc); len(r) != 1 || r[0] != "web_seat_never_seen" {
		t.Fatalf("web never seen: %#v", r)
	}

	seen := now.Add(-2 * time.Minute)
	row.WebSeatLastSeenAt = &seen
	if r := ACDPoolTransferRejectReasons(row, p, exclude, fresh, now, loc); len(r) != 1 {
		t.Fatalf("web stale: %#v", r)
	}

	row.TrunkNumberID = 9
	row.RouteType = constants.ACDPoolRouteTypeSIP
	row.WebSeatLastSeenAt = nil
	if r := ACDPoolTransferRejectReasons(row, p, exclude, fresh, now, loc); len(r) != 1 {
		t.Fatalf("trunk mismatch: %#v", r)
	}
}
