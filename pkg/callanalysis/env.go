package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"fmt"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/utils"
)

// EnvFromProcess reads the same env keys used by the SIP voice stack (ASR / LLM only).
func EnvFromProcess() Env {
	provider := strings.TrimSpace(utils.GetEnv("LLM_PROVIDER"))
	appID := strings.TrimSpace(utils.GetEnv("LLM_APP_ID"))
	if appID == "" {
		appID = strings.TrimSpace(utils.GetEnv("ALIBABA_AI_APP_ID"))
	}
	apiKey := strings.TrimSpace(utils.GetEnv("LLM_APIKEY"))
	if apiKey == "" && strings.EqualFold(provider, "alibaba") {
		apiKey = strings.TrimSpace(utils.GetEnv("ALIBABA_AI_API_KEY"))
	}
	return Env{
		LLMProvider: provider,
		LLMBaseURL:  strings.TrimSpace(utils.GetEnv("LLM_BASEURL")),
		LLMAppID:    appID,
		LLMAPIKey:   apiKey,
		LLMModel:    strings.TrimSpace(utils.GetEnv("LLM_MODEL")),

		ASRAppID:     strings.TrimSpace(utils.GetEnv("ASR_APPID")),
		ASRSecretID:  strings.TrimSpace(utils.GetEnv("ASR_SECRET_ID")),
		ASRSecretKey: strings.TrimSpace(utils.GetEnv("ASR_SECRET_KEY")),
		ASRModelType: strings.TrimSpace(utils.GetEnv("ASR_MODEL_TYPE")),
	}
}

func (e Env) Validate() error {
	if e.ASRAppID == "" || e.ASRSecretID == "" || e.ASRSecretKey == "" {
		return fmt.Errorf("call analysis: missing ASR env (ASR_APPID, ASR_SECRET_ID, ASR_SECRET_KEY)")
	}
	llmOK := e.LLMAPIKey != "" && strings.TrimSpace(e.LLMBaseURL) != ""
	if strings.EqualFold(e.LLMProvider, "alibaba") {
		llmOK = e.LLMAPIKey != "" && strings.TrimSpace(e.LLMAppID) != ""
	}
	if !llmOK {
		return fmt.Errorf("call analysis: missing LLM env (LLM_APIKEY + LLM_BASEURL, or alibaba: LLM_APIKEY + LLM_APP_ID)")
	}
	return nil
}

func (e Env) llmModelOrDefault() string {
	if strings.TrimSpace(e.LLMModel) != "" {
		return strings.TrimSpace(e.LLMModel)
	}
	return "qwen-plus"
}
