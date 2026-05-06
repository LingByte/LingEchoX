package models

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/utils"
)

type acdShiftWindow struct {
	Weekdays []int  `json:"weekdays"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

// ACDFitsShiftSchedule reports whether t falls into any configured window. Empty or invalid JSON means 24/7 (eligible).
func ACDFitsShiftSchedule(scheduleJSON string, t time.Time, loc *time.Location) bool {
	scheduleJSON = strings.TrimSpace(scheduleJSON)
	if scheduleJSON == "" {
		return true
	}
	var windows []acdShiftWindow
	if err := json.Unmarshal([]byte(scheduleJSON), &windows); err != nil || len(windows) == 0 {
		return true
	}
	if loc == nil {
		loc = time.Local
	}
	tt := t.In(loc)
	wd := int(tt.Weekday())
	minutes := tt.Hour()*60 + tt.Minute()

	for _, w := range windows {
		if !acdWeekdayListed(w.Weekdays, wd) {
			continue
		}
		startM, endM, ok := acdParseHHMMRange(w.Start, w.End)
		if !ok {
			continue
		}
		if startM <= endM {
			if minutes >= startM && minutes < endM {
				return true
			}
		} else {
			if minutes >= startM || minutes < endM {
				return true
			}
		}
	}
	return false
}

func acdWeekdayListed(days []int, wd int) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == wd {
			return true
		}
	}
	return false
}

func acdParseHHMMRange(start, end string) (startMin, endMin int, ok bool) {
	a, ok1 := acdParseHHMM(strings.TrimSpace(start))
	b, ok2 := acdParseHHMM(strings.TrimSpace(end))
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return a, b, true
}

func acdParseHHMM(s string) (minutes int, ok bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// ACDShiftTimeLocation loads IANA zone from ACD_SHIFT_TIMEZONE or falls back to Local.
func ACDShiftTimeLocation() *time.Location {
	name := strings.TrimSpace(utils.GetEnv("ACD_SHIFT_TIMEZONE"))
	if name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}
