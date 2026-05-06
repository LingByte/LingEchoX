package models

import (
	"testing"
	"time"
)

func TestACDFitsShiftSchedule_EmptyAlwaysOn(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	tm := time.Date(2026, 5, 6, 10, 0, 0, 0, loc)
	if !ACDFitsShiftSchedule("", tm, loc) {
		t.Fatal("empty schedule should allow")
	}
	if !ACDFitsShiftSchedule("not-json", tm, loc) {
		t.Fatal("invalid json should allow (lenient)")
	}
}

func TestACDFitsShiftSchedule_WeekdayWindow(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	// 2026-05-06 is Wednesday => weekday 3
	schedule := `[{"weekdays":[3],"start":"09:00","end":"18:00"}]`
	tm := time.Date(2026, 5, 6, 10, 0, 0, 0, loc)
	if !ACDFitsShiftSchedule(schedule, tm, loc) {
		t.Fatal("wed 10:00 should fit")
	}
	tm2 := time.Date(2026, 5, 6, 19, 0, 0, 0, loc)
	if ACDFitsShiftSchedule(schedule, tm2, loc) {
		t.Fatal("wed 19:00 should not fit")
	}
}
