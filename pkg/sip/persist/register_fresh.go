package persist

import (
	"time"

	"github.com/LingByte/SoulNexus/pkg/utils"
)

// EnvRegisterFreshSeconds is the process env key for REGISTER binding staleness (seconds).
const EnvRegisterFreshSeconds = "SIP_REGISTER_FRESH_SECONDS"

func registerFreshWindow() time.Duration {
	sec := int(utils.GetIntEnv(EnvRegisterFreshSeconds))
	if sec <= 0 {
		sec = 60
	}
	return time.Duration(sec) * time.Second
}

// IsRegisterFresh reports whether LastSeenAt is within the freshness window (default 60s).
func IsRegisterFresh(lastSeenAt *time.Time) bool {
	if lastSeenAt == nil || lastSeenAt.IsZero() {
		return false
	}
	return time.Since(*lastSeenAt) <= registerFreshWindow()
}
