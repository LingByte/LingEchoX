package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// StreamEmitter sends one JSON event map to the client (e.g. WebSocket). Implementations must be concurrency-safe.
type StreamEmitter func(event map[string]interface{}) error

// StreamFullAnalysis runs ASR (with per-sentence events) then LLM, emitting progress via emit.
// Returns the same ExportDoc shape as Run for store/export.
func StreamFullAnalysis(ctx context.Context, env Env, pcm []byte, pcmRate int, in Input, emit StreamEmitter) (*ExportDoc, error) {
	if err := env.Validate(); err != nil {
		return nil, err
	}
	if emit == nil {
		emit = func(map[string]interface{}) error { return nil }
	}
	if err := emit(map[string]interface{}{"type": "stage", "stage": "asr"}); err != nil {
		return nil, err
	}

	asrModel := strings.TrimSpace(env.ASRModelType)
	if asrModel == "" {
		asrModel = "16k_zh"
	}

	transcript, err := TranscribePCMWithSentenceNotify(ctx, pcm, env, pcmRate, func(frag, cum string) {
		_ = emit(map[string]interface{}{
			"type":       "asr_sentence",
			"fragment":   frag,
			"cumulative": cum,
		})
	})
	if err != nil {
		_ = emit(map[string]interface{}{"type": "error", "stage": "asr", "message": err.Error()})
		return nil, err
	}
	if err := emit(map[string]interface{}{"type": "asr_done", "transcript": transcript}); err != nil {
		return nil, err
	}

	if err := emit(map[string]interface{}{"type": "stage", "stage": "llm"}); err != nil {
		return nil, err
	}
	llmJSON, llmRaw, lerr := AnalyzeTranscript(ctx, env, transcript)
	if lerr != nil {
		_ = emit(map[string]interface{}{"type": "error", "stage": "llm", "message": lerr.Error()})
		return nil, lerr
	}
	var llmObj interface{}
	if err := json.Unmarshal(llmJSON, &llmObj); err != nil {
		llmObj = string(llmJSON)
	}
	if err := emit(map[string]interface{}{"type": "llm_done", "analysis": llmObj, "llm_raw": llmRaw}); err != nil {
		return nil, err
	}

	dur := PCMDurationSeconds(pcm, pcmRate)
	id := uuid.New().String()
	doc := BuildExportDoc(id, env, in, pcmRate, dur, asrModel, transcript, llmJSON, llmRaw)
	if err := emit(map[string]interface{}{"type": "complete", "id": doc.ID, "export_doc": doc}); err != nil {
		return nil, err
	}
	return doc, nil
}
