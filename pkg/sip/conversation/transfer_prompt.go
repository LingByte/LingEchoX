// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/utils"
	"go.uber.org/zap"
)

// Transfer prompt: a fixed announcement (e.g. "您好，这边为您转接人工，请稍等")
// played to the caller immediately before TriggerTransferToAgent fires
// the dial-leg / hold music. This exists primarily for realtime
// (multimodal) mode where the model's mid-sentence acknowledgement
// gets cancelled on transfer detection — without an announcement the
// caller would hear silence → ringing, which feels abrupt.
//
// Pipeline mode does not need this hook because the LLM's reply is
// synthesised by TTS before transfer fires (see voice.go transfer-
// after-AI-tts-confirmation gate); the natural reply already plays
// the equivalent phrase.
//
// Source resolution (highest priority first):
//
//  1. SIP_TRANSFER_PROMPT_WAV_PATH env (ops override).
//  2. scripts/transfer_prompt.wav (project default; not shipped — drop
//     a recording or TTS-rendered WAV here to enable the announcement).
//
// When neither source resolves to an existing file, we treat the
// announcement stage as a *legitimate skip*: log Info once and proceed
// directly to TriggerTransferToAgent. This preserves backwards
// compatibility for deployments that haven't recorded a prompt yet.

// resolveTransferPromptWavPath returns the cleaned filesystem path that
// the transfer prompt WAV would be loaded from. Mirrors resolveWelcomeWavPath.
func resolveTransferPromptWavPath() string {
	path := utils.GetEnv("SIP_TRANSFER_PROMPT_WAV_PATH")
	if strings.TrimSpace(path) == "" {
		path = "scripts/transfer_prompt.wav"
	}
	return filepath.Clean(path)
}

// loadTransferPromptPCM resolves and decodes the transfer announcement
// WAV at sampleRate. Returns:
//
//   - (pcm, nil) when a configured file exists and decodes cleanly.
//   - (nil, nil) when no file is configured or it does not exist on
//     disk — caller treats this as "skip announcement, transfer now".
//     This is a legitimate operational state, not an error.
//   - (nil, err) when a configured file exists but failed to decode
//     (truncated, wrong format, permission). Caller logs and falls
//     through to immediate transfer rather than blocking the call.
func loadTransferPromptPCM(_ context.Context, sampleRate int, lg *zap.Logger) ([]byte, error) {
	if lg == nil {
		lg = zap.NewNop()
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	path := resolveTransferPromptWavPath()
	ok, _ := welcomeWavExists(path)
	if !ok {
		lg.Info("sip voice: transfer prompt wav not configured, skipping announcement (drop a WAV at this path or set SIP_TRANSFER_PROMPT_WAV_PATH)",
			zap.String("local_path", path),
		)
		return nil, nil
	}
	pcm, err := LoadWAVAsPCM16Mono(path, sampleRate)
	if err != nil {
		return nil, err
	}
	return pcm, nil
}
