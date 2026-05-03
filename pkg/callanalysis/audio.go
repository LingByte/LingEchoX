package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const pcmChannels = 1
const pcmBits = 16

// PCMSampleRateFromASRModel returns the PCM sample rate implied by Tencent engine id.
// If the model is 8k_* but audio is decoded at 16kHz (or the reverse), recognition degrades badly.
func PCMSampleRateFromASRModel(modelType string) int {
	m := strings.ToLower(strings.TrimSpace(modelType))
	if m == "" {
		return 16000
	}
	if strings.Contains(m, "8k") {
		return 8000
	}
	return 16000
}

// DecodeToPCMSMono converts arbitrary audio (via ffmpeg) to little-endian s16 mono at sampleRateHz.
func DecodeToPCMSMono(audioPath string, sampleRateHz int) ([]byte, error) {
	if sampleRateHz <= 0 {
		sampleRateHz = 16000
	}
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", audioPath,
		"-f", "s16le", "-ac", strconv.Itoa(pcmChannels), "-ar", strconv.Itoa(sampleRateHz),
		"pipe:1",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(6 * time.Minute):
		_ = cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("ffmpeg: timeout")
	case err := <-done:
		if err != nil {
			msg := stderr.String()
			if len(msg) > 800 {
				msg = msg[:800] + "...(truncated)"
			}
			if msg != "" {
				return nil, fmt.Errorf("ffmpeg: %w: %s", err, msg)
			}
			return nil, fmt.Errorf("ffmpeg: %w", err)
		}
	}
	raw := out.Bytes()
	if len(raw) == 0 {
		return nil, fmt.Errorf("ffmpeg: empty pcm output (unsupported or corrupt audio?)")
	}
	return raw, nil
}

// PCMDurationSeconds returns duration of s16le mono pcm at sampleRateHz.
func PCMDurationSeconds(pcm []byte, sampleRateHz int) float64 {
	if len(pcm) == 0 || sampleRateHz <= 0 {
		return 0
	}
	bytesPerSec := sampleRateHz * (pcmBits / 8) * pcmChannels
	return float64(len(pcm)) / float64(bytesPerSec)
}

// MaxAudioBytes limits download / processing size (before decode).
const MaxAudioBytes = 80 << 20 // 80 MiB

// ReadAllLimited reads r until EOF or max+1 bytes (returns error if over max).
func ReadAllLimited(r io.Reader, max int) ([]byte, error) {
	limited := io.LimitReader(r, int64(max)+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("audio larger than %d bytes", max)
	}
	return b, nil
}
