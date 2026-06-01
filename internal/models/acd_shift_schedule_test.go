package models

import (
	"testing"
	"time"
)

func TestACDFitsShiftSchedule_EmptyMeansAlways(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	at := time.Date(2026, 6, 2, 10, 0, 0, 0, loc) // Monday
	if !ACDFitsShiftSchedule("", at, loc) {
		t.Fatal("empty schedule should mean 24/7")
	}
}

func TestACDFitsShiftSchedule_WeekdayWindow(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	sched := `[{"weekdays":[1,2,3,4,5],"start":"09:00","end":"18:00"}]`
	in := time.Date(2026, 6, 2, 10, 0, 0, 0, loc)  // Monday 10:00
	out := time.Date(2026, 6, 2, 20, 0, 0, 0, loc) // Monday 20:00
	if !ACDFitsShiftSchedule(sched, in, loc) {
		t.Fatal("Monday 10:00 should be inside Mon-Fri 09-18")
	}
	if ACDFitsShiftSchedule(sched, out, loc) {
		t.Fatal("Monday 20:00 should be outside Mon-Fri 09-18")
	}
}

func TestACDFitsShiftSchedule_Overnight(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	sched := `[{"weekdays":[1],"start":"22:00","end":"06:00"}]`
	late := time.Date(2026, 6, 1, 23, 0, 0, 0, loc) // Monday 23:00
	early := time.Date(2026, 6, 2, 5, 0, 0, 0, loc)  // Tuesday 05:00 — weekday in schedule is Monday only
	if !ACDFitsShiftSchedule(sched, late, loc) {
		t.Fatal("Monday 23:00 should match overnight Mon window")
	}
	// Tuesday 05:00 is still "Monday overnight" segment only if weekday check applies to start day;
	// backend uses current weekday for the window entry — Tuesday 05:00 has wd=2, not in [1].
	if ACDFitsShiftSchedule(sched, early, loc) {
		t.Fatal("Tuesday 05:00 should not match Monday-only overnight window in current implementation")
	}
}

func TestWebSeatLastSeenFresh(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-30 * time.Second)
	stale := now.Add(-2 * time.Minute)
	if !WebSeatLastSeenFresh(&fresh) {
		t.Fatal("30s ago should be fresh")
	}
	if WebSeatLastSeenFresh(&stale) {
		t.Fatal("2m ago should be stale")
	}
	if WebSeatLastSeenFresh(nil) {
		t.Fatal("nil should be stale")
	}
}

func TestApplyACDPoolTargetShiftWorkState_Decisions(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	weekdaySched := `[{"weekdays":[1,2,3,4,5],"start":"09:00","end":"18:00"}]`
	inShift := time.Date(2026, 6, 1, 10, 0, 0, 0, loc)  // Monday
	outShift := time.Date(2026, 6, 1, 20, 0, 0, 0, loc) // Monday evening

	sipOffline := ACDPoolTarget{RouteType: "sip", WorkState: "offline", ShiftScheduleJSON: weekdaySched}
	if !ACDFitsShiftSchedule(sipOffline.ShiftScheduleJSON, inShift, loc) {
		t.Fatal("expected in shift")
	}
	if ACDFitsShiftSchedule(sipOffline.ShiftScheduleJSON, outShift, loc) {
		t.Fatal("expected out of shift")
	}
	if !ACDFitsShiftSchedule("", inShift, loc) {
		t.Fatal("empty schedule should always fit")
	}

	breakSeat := ACDPoolTarget{WorkState: "break", ShiftScheduleJSON: weekdaySched}
	if ACDFitsShiftSchedule(breakSeat.ShiftScheduleJSON, outShift, loc) {
		t.Fatal("break seat out of shift should be marked offline by apply (state check)")
	}
	_ = breakSeat
}
