// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package tenantcfg

import (
	"context"
	"sync"
)

// JSONLoader fetches the four raw JSON blobs + voice_mode for one tenant.
//
// Wired from the storage layer (internal/sipserver, tests). Return ok=false
// when the tenant row is missing — the call leg then falls back to env-driven
// config or scripts/config_error.wav playback.
type JSONLoader func(ctx context.Context, tenantID uint) (
	asr, tts, llm, realtime []byte, voiceMode string, ok bool)

var (
	loaderMu sync.RWMutex
	loader   JSONLoader
)

// SetLoader installs the DB-backed tenant voice JSON loader. Safe to
// call from any goroutine; the loader pointer is protected by an
// RWMutex (read paths are lock-free on the cache-coherent fast read).
//
// Idempotent: nil is allowed to clear the loader (used by tests).
func SetLoader(fn JSONLoader) {
	loaderMu.Lock()
	loader = fn
	loaderMu.Unlock()
}

// Loader returns the currently installed JSONLoader (or nil).
// Exposed for tests / introspection.
func Loader() JSONLoader {
	loaderMu.RLock()
	fn := loader
	loaderMu.RUnlock()
	return fn
}

// Resolve loads the tenant's voice JSON via the installed loader and
// parses it into a VoiceEnv. Returns:
//
//   - (env, true, nil)  — happy path
//   - (zero, false, nil) — no loader installed, or tenantID == 0, or
//                          loader returned ok=false. Caller should
//                          play config_error.wav.
//   - (zero, false, err) — JSON parse error. Caller should play
//                          config_error.wav AND log the error.
func Resolve(ctx context.Context, tenantID uint) (VoiceEnv, bool, error) {
	if tenantID == 0 {
		return VoiceEnv{}, false, nil
	}
	fn := Loader()
	if fn == nil {
		return VoiceEnv{}, false, nil
	}
	asr, tts, llm, realtime, voiceMode, ok := fn(ctx, tenantID)
	if !ok {
		return VoiceEnv{}, false, nil
	}
	env, err := VoiceEnvFromJSON(asr, tts, llm, realtime, voiceMode)
	if err != nil {
		return VoiceEnv{}, false, err
	}
	return env, true, nil
}
