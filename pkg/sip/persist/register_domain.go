package persist

import (
	"net"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
)

func isPrivateOrLocalHost(host string) bool {
	h := strings.TrimSpace(strings.Trim(host, "[]"))
	if h == "" {
		return false
	}
	hl := strings.ToLower(h)
	if hl == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// EffectiveDialDomain picks a reachable SIP domain for Request-URI host when dialing registered users.
// Preference: non-private preferredDomain > SIP_DEFAULT_DOMAIN > non-private signalingIP > fallback.
func EffectiveDialDomain(preferredDomain, signalingIP string) string {
	preferredDomain = strings.TrimSpace(preferredDomain)
	if preferredDomain != "" && !isPrivateOrLocalHost(preferredDomain) {
		return preferredDomain
	}
	if envDomain := utils.GetEnv(constants.EnvSIPDefaultDomain); envDomain != "" {
		return envDomain
	}
	signalingIP = strings.TrimSpace(signalingIP)
	if signalingIP != "" && !isPrivateOrLocalHost(signalingIP) {
		return signalingIP
	}
	if preferredDomain != "" {
		return preferredDomain
	}
	return "localhost"
}
