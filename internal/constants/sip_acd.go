package constants

import "time"

// ACD dispatch mode (sip_trunk_numbers.acd_dispatch_mode).
const (
	ACDDispatchModeWeight      = "weight"
	ACDDispatchModeRoundRobin  = "round_robin"
)

// ACD pool route types.
const (
	ACDPoolRouteTypeSIP = "sip"
	ACDPoolRouteTypeWeb = "web"
)

// ACD SIP row source (when route type is SIP).
const (
	ACDSipSourceInternal = "internal"
	ACDSipSourceTrunk    = "trunk"
)

// ACD seat work states (real-time eligibility).
const (
	ACDWorkStateOffline   = "offline"
	ACDWorkStateAvailable = "available"
	ACDWorkStateRinging   = "ringing"
	ACDWorkStateBusy      = "busy"
	ACDWorkStateACW       = "acw"
	ACDWorkStateBreak     = "break"
)

// WebSeatStaleAfter is the max age of web-seat heartbeat for "online" eligibility.
const WebSeatStaleAfter = 90 * time.Second
