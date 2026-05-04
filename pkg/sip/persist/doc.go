// Package persist holds SIP persistence for sip_users and sip_calls: types and stores in types.go,
// user helpers in sip_user.go, SIP call row + turns JSON in sip_call.go, DB queries in sip_call_repo.go,
// invite/bye/RTP lifecycle helpers in sip_call_lifecycle.go, plus GORM and optional JSON backends.
//
// JSON file mode: set SIP_PERSIST=json. Writes under SIP_DATA_DIR/sip/: calls.json, users.json
// (atomic rename, per-file mutexes, MergeSIPCall patch semantics).
package persist
