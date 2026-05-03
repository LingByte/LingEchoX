package vad

import (
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/utils"
)

const DefaultThreshold = 3200.0

// BargeInEnabled is false when SIP_VAD_BARGE_IN is 0/false/off/no (unset → enabled).
func BargeInEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(utils.GetEnv("SIP_VAD_BARGE_IN")))
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// ThresholdFromEnv returns SIP_VAD_THRESHOLD or DefaultThreshold when unset/invalid.
func ThresholdFromEnv() float64 {
	s := strings.TrimSpace(utils.GetEnv("SIP_VAD_THRESHOLD"))
	if s == "" {
		return DefaultThreshold
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil || x <= 0 {
		return DefaultThreshold
	}
	return x
}

// ConsecutiveFramesFromEnv returns SIP_VAD_CONSEC_FRAMES or default 3 (~60 ms @ 20 ms/frame).
func ConsecutiveFramesFromEnv() int {
	s := strings.TrimSpace(utils.GetEnv("SIP_VAD_CONSEC_FRAMES"))
	if s == "" {
		return 3
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 3
	}
	if n > 50 {
		return 50
	}
	return n
}
