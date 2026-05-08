package models

import "strings"

// ACD dispatch mode values stored on sip_trunk_numbers.acd_dispatch_mode.
const (
	ACDDispatchModeWeight     = "weight"
	ACDDispatchModeRoundRobin = "round_robin"
)

func NormalizeACDDispatchMode(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case ACDDispatchModeRoundRobin, "rr":
		return ACDDispatchModeRoundRobin
	case "", ACDDispatchModeWeight:
		return ACDDispatchModeWeight
	default:
		return ACDDispatchModeWeight
	}
}
