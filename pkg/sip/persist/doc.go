// Package persist holds SIP persistence types and helpers for sip_users and sip_calls (GORM).
//
// Models and CRUD: sip_call.go, sip_user.go, types.go.
// Wiring for server/outbound: CallStore (call_store.go), GormStore REGISTER bindings (register_store.go),
// dial-domain / freshness helpers (register_domain.go, register_fresh.go), listen-port for Request-URI (register_listen_port.go).
package persist
