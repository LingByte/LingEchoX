package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"encoding/json"
	"time"
)

// Env holds ASR + LLM credentials from process environment (same variables as SIP voice pipeline).
type Env struct {
	LLMProvider string
	LLMBaseURL  string
	LLMAppID    string
	LLMAPIKey   string
	LLMModel    string

	ASRAppID     string
	ASRSecretID  string
	ASRSecretKey string
	ASRModelType string
}

// Input is one analysis job: either a local audio file path or a remote URL (already downloaded to a temp path).
type Input struct {
	LocalPath string
	Source    string // "upload" | "url"
	Filename  string
	AudioURL  string
}

// ExportDoc is persisted and returned as JSON download; it includes transcript and LLM output.
type ExportDoc struct {
	Version   string          `json:"version"`
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Meta      ExportMeta      `json:"meta"`
	ASR       ExportASR       `json:"asr"`
	LLM       json.RawMessage `json:"llm_analysis"`
	LLMRaw    string          `json:"llm_raw,omitempty"`
}

// ExportMeta describes how the job was produced.
type ExportMeta struct {
	Source          string  `json:"source"`
	Filename        string  `json:"filename,omitempty"`
	AudioURL        string  `json:"audio_url,omitempty"`
	PCMDurationS    float64 `json:"pcm_duration_sec"`
	PCMSampleRateHz int     `json:"pcm_sample_rate_hz"`
	ASRModel        string  `json:"asr_model"`
	ASRProvider     string  `json:"asr_provider"`
	LLMProvider     string  `json:"llm_provider"`
	LLMModel        string  `json:"llm_model"`
}

// ExportASR holds transcription (通话原文 / 转写).
type ExportASR struct {
	Provider   string `json:"provider"`
	Transcript string `json:"transcript"`
}
