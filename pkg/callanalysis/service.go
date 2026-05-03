package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BuildExportDoc assembles the persisted/export JSON document (shared by HTTP Run and WebSocket stream).
func BuildExportDoc(id string, env Env, in Input, pcmRate int, dur float64, asrModel, transcript string, llmJSON []byte, llmRaw string) *ExportDoc {
	return &ExportDoc{
		Version:   "1",
		ID:        id,
		CreatedAt: time.Now().UTC(),
		Meta: ExportMeta{
			Source:          in.Source,
			Filename:        in.Filename,
			AudioURL:        in.AudioURL,
			PCMDurationS:    dur,
			PCMSampleRateHz: pcmRate,
			ASRModel:        asrModel,
			ASRProvider:     "qcloud",
			LLMProvider:     env.LLMProvider,
			LLMModel:        env.llmModelOrDefault(),
		},
		ASR: ExportASR{
			Provider:   "qcloud",
			Transcript: transcript,
		},
		LLM:    json.RawMessage(llmJSON),
		LLMRaw: llmRaw,
	}
}

// Run decodes audio → ASR → LLM and builds an export document (caller supplies a temp audio file path).
func Run(ctx context.Context, env Env, in Input) (*ExportDoc, error) {
	if err := env.Validate(); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(in.LocalPath)
	if path == "" {
		return nil, fmt.Errorf("call analysis: missing local audio path")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("call analysis: stat audio: %w", err)
	}
	if st.Size() == 0 {
		return nil, fmt.Errorf("call analysis: empty audio file")
	}

	asrModel := strings.TrimSpace(env.ASRModelType)
	if asrModel == "" {
		asrModel = "16k_zh"
	}
	pcmRate := PCMSampleRateFromASRModel(asrModel)
	pcm, err := DecodeToPCMSMono(path, pcmRate)
	if err != nil {
		return nil, err
	}
	dur := PCMDurationSeconds(pcm, pcmRate)

	transcript, err := TranscribePCM(ctx, pcm, env, pcmRate)
	if err != nil {
		return nil, err
	}

	llmJSON, llmRaw, lerr := AnalyzeTranscript(ctx, env, transcript)
	if lerr != nil {
		return nil, lerr
	}

	id := uuid.New().String()
	return BuildExportDoc(id, env, in, pcmRate, dur, asrModel, transcript, llmJSON, llmRaw), nil
}
