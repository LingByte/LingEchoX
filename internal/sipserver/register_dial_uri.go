package sipserver

import "sync"

// Request-URI host port when dialing registered extensions (must match embedded SIP listen port).
var (
	registerOutboundReqURIPortMu sync.RWMutex
	registerOutboundReqURIPort int
)

// SetRegisterOutboundRequestURIServerPort sets the server UDP port embedded in sip:user@host:PORT for
// register-resolved outbound dials. Call from sipapp with the same cfg.Port as the embedded SIP stack.
func SetRegisterOutboundRequestURIServerPort(port int) {
	registerOutboundReqURIPortMu.Lock()
	defer registerOutboundReqURIPortMu.Unlock()
	if port > 0 {
		registerOutboundReqURIPort = port
	}
}

func effectiveRegisterDialRequestURIPort(fallback int) int {
	if fallback <= 0 {
		fallback = 6050
	}
	registerOutboundReqURIPortMu.RLock()
	p := registerOutboundReqURIPort
	registerOutboundReqURIPortMu.RUnlock()
	if p > 0 {
		return p
	}
	return fallback
}
