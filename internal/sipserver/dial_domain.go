package sipserver

import (
	"net"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/sip/outbound"
	"github.com/LingByte/SoulNexus/pkg/utils"
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

// effectiveDialDomain picks a reachable SIP domain for Request-URI host.
// Preference: non-private preferredDomain > SIP_DEFAULT_DOMAIN > non-private signalingIP > fallback.
func effectiveDialDomain(preferredDomain, signalingIP string) string {
	preferredDomain = strings.TrimSpace(preferredDomain)
	if preferredDomain != "" && !isPrivateOrLocalHost(preferredDomain) {
		return preferredDomain
	}
	if envDomain := strings.TrimSpace(utils.GetEnv(outbound.EnvSIPDefaultDomain)); envDomain != "" {
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

