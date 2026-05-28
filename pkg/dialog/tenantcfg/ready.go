// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package tenantcfg

import "strings"

// VoiceReady reports whether env has the minimum fields for the
// configured voice attach path. Dispatches by VoiceMode:
//
//   - "realtime": only RealtimeConfigRaw is consulted (multimodal
//     collapses ASR/TTS/LLM into one provider).
//   - "pipeline" (default): legacy ASR + TTS + LLM readiness check.
func VoiceReady(env VoiceEnv) bool {
	if strings.EqualFold(env.VoiceMode, "realtime") {
		return RealtimeReady(env)
	}
	return PipelineUsable(env)
}

// PipelineUsable is the cascaded-mode credential gate. Returns true
// when ASR / TTS / LLM all have at least the minimum field set for
// their resolved provider. Mirrors the legacy
// conversation.pipelineCredsUsable.
//
// We intentionally avoid calling provider factories here (they'd
// open network handles); cheap field presence is enough as a gate —
// the factory will surface a typed error later if config is malformed.
func PipelineUsable(env VoiceEnv) bool {
	asp := strings.ToLower(strings.TrimSpace(env.ASRProvider))
	if asp == "" {
		asp = "qcloud"
	}
	if asp != "qcloud" {
		return false
	}
	if env.ASRAppID == "" || env.ASRSecretID == "" || env.ASRSecretKey == "" {
		return false
	}
	tsp := strings.ToLower(strings.TrimSpace(env.TTSProvider))
	if tsp == "" {
		tsp = "qcloud"
	}
	if !ttsCredentialsReady(tsp, env) {
		return false
	}
	return llmReady(env)
}

// RealtimeReady is the realtime-mode credential gate. Requires a
// provider string + at least one apiKey field on RealtimeConfigRaw.
// Provider-specific validation lives in pkg/realtime; this rules out
// the "tenant left the form blank" case so the call leg can fall
// back to config_error.wav rather than ringing into a broken session.
func RealtimeReady(env VoiceEnv) bool {
	raw := env.RealtimeConfigRaw
	if len(raw) == 0 {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(env.RealtimeProvider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(strFromMap(raw, "provider")))
	}
	if provider == "" {
		return false
	}
	switch provider {
	case "volcengine_dialogue", "volc_realtime", "doubao_realtime", "volcengine_realtime":
		appID := strFromMap(raw, "appId", "app_id")
		accessKey := strFromMap(raw, "accessKey", "access_key", "access_token", "token")
		return appID != "" && accessKey != ""
	default:
		for _, k := range []string{"apiKey", "api_key"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return true
				}
			}
		}
		return false
	}
}

// llmReady (private) is the LLM credential check for cascaded mode.
// Per-provider rules:
//
//   - "alibaba"     → API key + App ID
//   - "coze"        → API key + base URL (which carries botId/userId)
//   - "ollama"      → base URL (self-hosted, no key)
//   - default       → API key (OpenAI-compatible)
func llmReady(e VoiceEnv) bool {
	if strings.EqualFold(e.LLMProvider, "alibaba") {
		return e.LLMAPIKey != "" && e.LLMAppID != ""
	}
	if strings.EqualFold(e.LLMProvider, "coze") {
		return e.LLMAPIKey != "" && e.LLMBaseURL != ""
	}
	if strings.EqualFold(e.LLMProvider, "ollama") {
		return strings.TrimSpace(e.LLMBaseURL) != ""
	}
	return e.LLMAPIKey != ""
}

// ttsCredentialsReady (private) is a low-cost feasibility check for
// the configured TTS provider. The exhaustive validation lives in
// synthesizer.NewSynthesisServiceFromCredential; this rules out
// obviously-empty configs.
func ttsCredentialsReady(provider string, env VoiceEnv) bool {
	switch provider {
	case "qcloud", "tencent":
		return env.TTSAppID != "" && env.TTSSecretID != "" && env.TTSSecretKey != ""
	case "aliyun", "dashscope", "qwen",
		"qiniu", "openai", "elevenlabs", "fishspeech", "fishaudio",
		"google", "minimax", "volcengine", "volcengine_stream", "volcengine_llm", "volcengine_clone":
		raw := env.TTSConfigRaw
		if raw == nil {
			return false
		}
		for _, k := range []string{"apiKey", "api_key"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return true
				}
			}
		}
		return false
	case "azure":
		raw := env.TTSConfigRaw
		if raw == nil {
			return false
		}
		hasKey := false
		hasRegion := false
		for _, k := range []string{"subscriptionKey", "subscription_key", "apiKey", "api_key"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					hasKey = true
				}
			}
		}
		if v, ok := raw["region"]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				hasRegion = true
			}
		}
		return hasKey && hasRegion
	case "baidu":
		raw := env.TTSConfigRaw
		if raw == nil {
			return false
		}
		if v, ok := raw["token"]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
		hasKey := false
		hasSecret := false
		for _, k := range []string{"apiKey", "api_key"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					hasKey = true
				}
			}
		}
		for _, k := range []string{"secretKey", "secret_key"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					hasSecret = true
				}
			}
		}
		return hasKey && hasSecret
	case "xunfei":
		raw := env.TTSConfigRaw
		if raw == nil {
			return false
		}
		needed := []string{"appId", "apiKey", "apiSecret"}
		for _, k := range needed {
			v, ok := raw[k]
			if !ok {
				v, ok = raw[snakeCaseAlt(k)]
				if !ok {
					return false
				}
			}
			s, ok := v.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return false
			}
		}
		return true
	case "aws":
		raw := env.TTSConfigRaw
		if raw == nil {
			return false
		}
		hasID := false
		hasSecret := false
		for _, k := range []string{"accessKeyId", "access_key_id"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					hasID = true
				}
			}
		}
		for _, k := range []string{"secretAccessKey", "secret_access_key"} {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					hasSecret = true
				}
			}
		}
		return hasID && hasSecret
	default:
		// Unknown providers: defer to the credential factory at call
		// time. Returning true on a non-nil raw lets the factory
		// produce a precise error and the call leg fall back to
		// config_error.wav playback.
		return env.TTSConfigRaw != nil
	}
}
