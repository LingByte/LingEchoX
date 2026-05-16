// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/welcomeaudio"
)

// validWAVBytes builds an 8 kHz / 16-bit / mono 50 ms PCM WAV (≈800
// data bytes). Used by tests that exercise the runtime fetch + decode
// path — must be decodeable by LoadWAVAsPCM16FromBytes, not just pass
// the magic check.
func validWAVBytes(t *testing.T) []byte {
	t.Helper()
	const sampleRate = 8000
	const numSamples = 400 // 50 ms of silence
	data := make([]byte, numSamples*2)

	// Header (44 bytes) then data. Use a single buffer + PutUintXX so
	// every offset is known and bounds-safe (the earlier append-based
	// version crashed when computing a placeholder size offset).
	out := make([]byte, 44+len(data))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+len(data))) // RIFF chunk size
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(out[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(out[22:24], 1)  // channels
	binary.LittleEndian.PutUint32(out[24:28], sampleRate)
	binary.LittleEndian.PutUint32(out[28:32], sampleRate*2) // byte rate
	binary.LittleEndian.PutUint16(out[32:34], 2)            // block align
	binary.LittleEndian.PutUint16(out[34:36], 16)           // bits per sample
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(data)))
	copy(out[44:], data)
	return out
}

// TestLoadWelcomePCM_SkipsWhenNothingConfigured: no resolver wired AND
// no local file → returns (nil, skip, nil), allowing AttachVoicePipeline
// to silently skip the welcome stage.
func TestLoadWelcomePCM_SkipsWhenNothingConfigured(t *testing.T) {
	SetWelcomeAudioResolver(nil)
	t.Setenv("SIP_WELCOME_WAV_PATH", filepath.Join(t.TempDir(), "does-not-exist.wav"))

	pcm, src, err := loadWelcomePCM(context.Background(), "call-1", 16000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src != welcomeSourceSkip {
		t.Errorf("source: got %q want %q", src, welcomeSourceSkip)
	}
	if pcm != nil {
		t.Errorf("pcm should be nil for skip path, got %d bytes", len(pcm))
	}
}

// TestLoadWelcomePCM_LocalFileFallback: no resolver, but a real WAV
// exists at the resolved path → loads it and tags source=local_wav.
func TestLoadWelcomePCM_LocalFileFallback(t *testing.T) {
	SetWelcomeAudioResolver(nil)
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "welcome.wav")
	if err := os.WriteFile(wavPath, validWAVBytes(t), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIP_WELCOME_WAV_PATH", wavPath)

	pcm, src, err := loadWelcomePCM(context.Background(), "call-2", 16000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src != welcomeSourceLocal {
		t.Errorf("source: got %q want %q", src, welcomeSourceLocal)
	}
	if len(pcm) == 0 {
		t.Error("expected decoded PCM, got 0 bytes")
	}
}

// TestLoadWelcomePCM_TrunkURLWins: when the resolver returns a URL,
// it is preferred over the local file — and local file presence is
// irrelevant. Source must be trunk_url. We also verify the second
// call hits the in-package cache (no extra HTTP request observed).
func TestLoadWelcomePCM_TrunkURLWins(t *testing.T) {
	welcomeaudio.PurgeCache()
	defer welcomeaudio.PurgeCache()

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(validWAVBytes(t))
	}))
	defer srv.Close()

	SetWelcomeAudioResolver(func(callID string) string { return srv.URL + "/welcome.wav" })
	t.Cleanup(func() { SetWelcomeAudioResolver(nil) })

	// Even if a local file exists, URL wins.
	dir := t.TempDir()
	t.Setenv("SIP_WELCOME_WAV_PATH", filepath.Join(dir, "fallback.wav"))
	_ = os.WriteFile(filepath.Join(dir, "fallback.wav"), validWAVBytes(t), 0644)

	pcm, src, err := loadWelcomePCM(context.Background(), "call-3", 16000, nil)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if src != welcomeSourceURL {
		t.Errorf("source: got %q want %q", src, welcomeSourceURL)
	}
	if len(pcm) == 0 {
		t.Error("expected non-empty PCM")
	}
	// Second call → cache hit, no extra HTTP request.
	if _, _, err := loadWelcomePCM(context.Background(), "call-3", 16000, nil); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected exactly 1 HTTP hit (second served from cache), got %d", hits)
	}
}

// TestLoadWelcomePCM_TrunkURLFailureDoesNotFallback: when the operator
// configured a URL but it's unreachable, we must NOT silently fall
// back to the local file — that would mask the misconfiguration.
// loadWelcomePCM returns (nil, trunk_url, ErrUnreachable).
func TestLoadWelcomePCM_TrunkURLFailureDoesNotFallback(t *testing.T) {
	welcomeaudio.PurgeCache()
	defer welcomeaudio.PurgeCache()

	// Local file IS present and valid.
	dir := t.TempDir()
	localPath := filepath.Join(dir, "fallback.wav")
	if err := os.WriteFile(localPath, validWAVBytes(t), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIP_WELCOME_WAV_PATH", localPath)

	// Resolver returns an unreachable URL.
	SetWelcomeAudioResolver(func(callID string) string { return "http://127.0.0.1:1/no.wav" })
	t.Cleanup(func() { SetWelcomeAudioResolver(nil) })

	pcm, src, err := loadWelcomePCM(context.Background(), "call-4", 16000, nil)
	if !errors.Is(err, welcomeaudio.ErrUnreachable) {
		t.Errorf("err: got %v want ErrUnreachable", err)
	}
	if src != welcomeSourceURL {
		t.Errorf("source: got %q want %q (must surface URL failure, not silently fall back)", src, welcomeSourceURL)
	}
	if pcm != nil {
		t.Errorf("pcm should be nil on URL failure, got %d bytes", len(pcm))
	}
}
