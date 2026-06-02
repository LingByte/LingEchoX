package models

import (
	"testing"
	"time"
)

func TestACDFitsShiftSchedule_HolidayCalendar(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	sched := `[{"weekdays":[],"start":"09:00","end":"18:00","calendar":"holiday"}]`
	national := time.Date(2026, 10, 1, 10, 0, 0, 0, loc)
	regular := time.Date(2026, 6, 2, 10, 0, 0, 0, loc)
	if !ACDFitsShiftSchedule(sched, national, loc) {
		t.Fatal("Oct 1 2026 should match holiday window")
	}
	if ACDFitsShiftSchedule(sched, regular, loc) {
		t.Fatal("regular Tuesday should not match holiday-only window")
	}
}

func TestACDFitsShiftSchedule_WorkdayCalendar(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	sched := `[{"weekdays":[1,2,3,4,5],"start":"09:00","end":"18:00","calendar":"workday"}]`
	in := time.Date(2026, 6, 2, 10, 0, 0, 0, loc) // Tuesday
	if !ACDFitsShiftSchedule(sched, in, loc) {
		t.Fatal("Tuesday workday should match")
	}
}
