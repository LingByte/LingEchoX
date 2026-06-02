package models

import (
	"strings"
	"time"

	"github.com/bastengao/chinese-holidays-go/holidays"
)

// acdShiftCalendar scopes a shift window beyond weekday matching.
// Empty / "weekday" = use weekdays only (legacy).
// "holiday" = statutory holidays & adjusted rest days (CN calendar, incl. 调休).
// "workday" = working days per CN calendar (Mon–Fri + 调休补班, excludes holidays).
// "weekend" = Saturday and Sunday (calendar weekend, not 调休补班).
func acdCalendarMatches(cal string, t time.Time, loc *time.Location) bool {
	cal = strings.ToLower(strings.TrimSpace(cal))
	switch cal {
	case "", "weekday", "any":
		return true
	case "holiday":
		if loc == nil {
			loc = time.Local
		}
		ok, err := holidays.IsHoliday(t.In(loc))
		return err == nil && ok
	case "workday":
		if loc == nil {
			loc = time.Local
		}
		ok, err := holidays.IsWorkingday(t.In(loc))
		return err == nil && ok
	case "weekend":
		if loc == nil {
			loc = time.Local
		}
		wd := int(t.In(loc).Weekday())
		return wd == 0 || wd == 6
	default:
		return true
	}
}
