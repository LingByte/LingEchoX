package constants

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Environment variable keys for outbound dialing (.env, read via utils.GetEnv / LookupEnv).
const (
	EnvSIPTargetNumber     = "SIP_TARGET_NUMBER"        // user part, e.g. extension or E.164 local part
	EnvSIPOutboundHost     = "SIP_OUTBOUND_HOST"        // domain or IP for Request-URI host
	EnvSIPOutboundPort     = "SIP_OUTBOUND_PORT"        // port in Request-URI (default 5060)
	EnvSIPSignalingAddr    = "SIP_SIGNALING_ADDR"       // UDP host:port where INVITE is sent (default host:port from above)
	EnvSIPOutboundReqURI   = "SIP_OUTBOUND_REQUEST_URI" // optional full override, e.g. sip:user@domain:5060;user=phone
	EnvSIPOutboundAutoDial = "SIP_OUTBOUND_AUTO_DIAL"   // "true"/"1" to Dial once at sip process startup

	// Outbound From / Contact user part (CLI). Empty → default user in outbound.NewManager.
	EnvSIPCallerID = "SIP_CALLER_ID"
	// Optional quoted display-name in From (RFC 3261 display-name).
	EnvSIPCallerDisplayName = "SIP_CALLER_DISPLAY_NAME"

	// DB-backed dial (sip_users): optional domain filter and Request-URI port when building sip:user@domain:port.
	EnvSIPDefaultDomain  = "SIP_DEFAULT_DOMAIN"
	EnvSIPDefaultURIPort = "SIP_DEFAULT_URI_PORT"

	// Transfer-to-agent route config: separate from campaign outbound.
	EnvSIPTransferReqURI  = "SIP_TRANSFER_REQUEST_URI"
	EnvSIPTransferSigAddr = "SIP_TRANSFER_SIGNALING_ADDR"
	// SIP_TRANSFER_NUMBER: extension for sip:user@SIP_TRANSFER_HOST, or literal "web" → browser WebRTC agent.
	EnvSIPTransferNumber = "SIP_TRANSFER_NUMBER"
	EnvSIPTransferHost   = "SIP_TRANSFER_HOST"
	EnvSIPTransferPort   = "SIP_TRANSFER_PORT"
)
