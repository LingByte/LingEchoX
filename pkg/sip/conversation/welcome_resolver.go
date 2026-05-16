// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"context"
	"errors"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/welcomeaudio"
	"go.uber.org/zap"
)

// welcomeAudioURLResolver, when set, returns the per-DID welcome WAV
// URL for a given inbound Call-ID. Wiring lives in
// internal/sipserver/sipapp.go (alongside SetVoiceDialogWSLookup) so
// this package keeps zero direct dependency on internal/models.
//
// Contract:
//   - "" (empty) → no per-DID override; AttachVoicePipeline falls back
//     to scripts/welcome.wav (or whatever resolveWelcomeWavPath says).
//   - non-empty → MUST be a previously write-validated URL (admin
//     write path runs welcomeaudio.ValidateURL). We still re-validate
//     RIFF/WAVE magic on fetch as a defence-in-depth measure.
var (
	welcomeAudioResolverMu sync.RWMutex
	welcomeAudioResolver   func(callID string) string

	transferRingingResolverMu sync.RWMutex
	transferRingingResolver   func(callID string) string
)

// SetWelcomeAudioResolver installs the per-DID welcome WAV URL lookup.
// Pass nil to clear (tests). Safe for concurrent calls.
func SetWelcomeAudioResolver(fn func(callID string) string) {
	welcomeAudioResolverMu.Lock()
	welcomeAudioResolver = fn
	welcomeAudioResolverMu.Unlock()
}

// SetTransferRingingResolver installs the per-DID transfer-ringback
// WAV URL lookup. Pass nil to clear (tests). Same contract as
// SetWelcomeAudioResolver — empty string means "fall through to the
// SIP_TRANSFER_RINGING_WAV_PATH env / scripts/ringing.wav default".
func SetTransferRingingResolver(fn func(callID string) string) {
	transferRingingResolverMu.Lock()
	transferRingingResolver = fn
	transferRingingResolverMu.Unlock()
}

// ResolveWelcomeAudioURL exposes the resolver to sibling packages
// (pkg/sip/voicedialog) that drive their own welcome playback path
// outside AttachVoicePipeline. Returns "" when no resolver is wired
// OR when the resolver itself returns empty for this call.
func ResolveWelcomeAudioURL(callID string) string {
	return resolveWelcomeAudioURL(callID)
}

// ResolveTransferRingingURL is the analogue for transfer ringback,
// consumed by both playTransferRingingLoop (this package) and
// pkg/sip/voicedialog's transfer-loading playback loop.
func ResolveTransferRingingURL(callID string) string {
	transferRingingResolverMu.RLock()
	fn := transferRingingResolver
	transferRingingResolverMu.RUnlock()
	if fn == nil {
		return ""
	}
	return fn(callID)
}

// resolveWelcomeAudioURL is the read side, nil-safe.
func resolveWelcomeAudioURL(callID string) string {
	welcomeAudioResolverMu.RLock()
	fn := welcomeAudioResolver
	welcomeAudioResolverMu.RUnlock()
	if fn == nil {
		return ""
	}
	return fn(callID)
}

// welcomeSource is a short tag used purely for log fields so operators
// can tell which path served the audio for a given call.
type welcomeSource string

const (
	welcomeSourceURL   welcomeSource = "trunk_url" // TrunkNumber.WelcomeAudioURL
	welcomeSourceLocal welcomeSource = "local_wav" // scripts/welcome.wav (or SIP_WELCOME_WAV_PATH)
	welcomeSourceSkip  welcomeSource = "skip"      // no URL + no local file → caller skips welcome stage
)

// loadWelcomePCM resolves and decodes the welcome WAV for an inbound
// call to s16le mono PCM at sampleRate. Resolution order:
//
//  1. Per-DID URL via welcomeAudioResolver. If the trunk number has a
//     configured WelcomeAudioURL, fetch (+cache) via pkg/welcomeaudio.
//     URL failures (unreachable, non-WAV) do NOT fall back to local —
//     misconfigured operators deserve a visible error in the log, not
//     silent fallback that masks the bug.
//  2. Local file via resolveWelcomeWavPath() + LoadWAVAsPCM16Mono.
//
// Returns (nil, welcomeSourceSkip, nil) when neither source is
// configured/available; callers treat that as "skip welcome stage".
// Returns an error only when a *configured* source failed — the
// caller's goroutine should log and bail without playing audio.
func loadWelcomePCM(ctx context.Context, callID string, sampleRate int, lg *zap.Logger) ([]byte, welcomeSource, error) {
	if lg == nil {
		lg = zap.NewNop()
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}

	// 1) Per-DID URL override.
	if u := resolveWelcomeAudioURL(callID); u != "" {
		pcm, err := welcomeaudio.FetchPCM(ctx, u, sampleRate, LoadWAVAsPCM16FromBytes)
		if err != nil {
			// Surface the kind of failure for operator triage. We do
			// NOT fall back to the local file here — silent fallback
			// would hide a real misconfiguration (the operator put a
			// URL on the trunk number and expects it to play).
			switch {
			case errors.Is(err, welcomeaudio.ErrUnreachable):
				lg.Warn("sip voice: welcome audio url unreachable",
					zap.String("call_id", callID),
					zap.String("url", u),
					zap.Error(err))
			case errors.Is(err, welcomeaudio.ErrNotAudio):
				lg.Warn("sip voice: welcome audio url is not WAV",
					zap.String("call_id", callID),
					zap.String("url", u),
					zap.Error(err))
			case errors.Is(err, welcomeaudio.ErrUnsupportedScheme):
				lg.Warn("sip voice: welcome audio url scheme invalid",
					zap.String("call_id", callID),
					zap.String("url", u),
					zap.Error(err))
			default:
				lg.Warn("sip voice: welcome audio url load failed",
					zap.String("call_id", callID),
					zap.String("url", u),
					zap.Error(err))
			}
			return nil, welcomeSourceURL, err
		}
		return pcm, welcomeSourceURL, nil
	}

	// 2) Local file fallback (legacy / operator default).
	path := resolveWelcomeWavPath()
	if ok, _ := welcomeWavExists(path); !ok {
		lg.Info("sip voice: welcome wav not configured, skipping welcome phase",
			zap.String("call_id", callID),
			zap.String("local_path", path))
		return nil, welcomeSourceSkip, nil
	}
	pcm, err := LoadWAVAsPCM16Mono(path, sampleRate)
	if err != nil {
		return nil, welcomeSourceLocal, err
	}
	return pcm, welcomeSourceLocal, nil
}
