package sipserver

import (
	"time"

	"github.com/LingByte/SoulNexus/pkg/utils"
)

const EnvSIPRegisterFreshSeconds = "SIP_REGISTER_FRESH_SECONDS"

func sipRegisterFreshWindow() time.Duration {
	sec := int(utils.GetIntEnv(EnvSIPRegisterFreshSeconds))
	if sec <= 0 {
		sec = 60
	}
	return time.Duration(sec) * time.Second
}

func isSIPRegisterFresh(lastSeenAt *time.Time) bool {
	if lastSeenAt == nil || lastSeenAt.IsZero() {
		return false
	}
	return time.Since(*lastSeenAt) <= sipRegisterFreshWindow()
}

