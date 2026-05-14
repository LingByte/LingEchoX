package persist

import "sync"

var (
	registerOutboundReqURIPortMu sync.RWMutex
	registerOutboundReqURIPort   int
)

// SetRegisterOutboundRequestURIServerPort sets the SIP UDP listen port embedded in sip:user@host:PORT when
// dialing registered extensions. Call with the same port as the embedded SIP stack (e.g. sipapp cfg.Port).
func SetRegisterOutboundRequestURIServerPort(port int) {
	registerOutboundReqURIPortMu.Lock()
	defer registerOutboundReqURIPortMu.Unlock()
	if port > 0 {
		registerOutboundReqURIPort = port
	}
}

// EffectiveRegisterDialRequestURIPort returns the configured listen port if set, otherwise fallback.
func EffectiveRegisterDialRequestURIPort(fallback int) int {
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
