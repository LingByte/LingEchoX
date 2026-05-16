package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// TenantVoiceJSONLoader loads raw JSON blobs for one tenant (asr / tts / llm columns).
// Wired from internal/sipserver (DB). Return ok=false when tenant row is missing.
type TenantVoiceJSONLoader func(ctx context.Context, tenantID uint) (asr, tts, llm []byte, ok bool)

var tenantVoiceJSONLoader TenantVoiceJSONLoader

// SetTenantVoiceJSONLoader installs DB-backed tenant AI JSON loading (required for tenant-scoped voice).
func SetTenantVoiceJSONLoader(fn TenantVoiceJSONLoader) {
	tenantVoiceJSONLoader = fn
}

func strFromMap(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				s := strings.TrimSpace(t)
				if s != "" {
					return s
				}
			case float64:
				return strings.TrimSpace(fmt.Sprint(int64(t)))
			}
		}
	}
	return ""
}

func intFromMap(m map[string]any, keys ...string) int {
	s := strFromMap(m, keys...)
	if s == "" {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

func int64FromMap(m map[string]any, keys ...string) int64 {
	s := strFromMap(m, keys...)
	if s == "" {
		if v, ok := m[keys[0]]; ok {
			if f, ok := v.(float64); ok {
				return int64(f)
			}
		}
		return 0
	}
	var n int64
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

func parseObj(raw []byte) (map[string]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}

// VoiceEnvFromTenantJSON builds VoiceEnv from three tenant JSON blobs (no process env).
func VoiceEnvFromTenantJSON(asrRaw, ttsRaw, llmRaw []byte) (VoiceEnv, error) {
	var out VoiceEnv
	am, err := parseObj(asrRaw)
	if err != nil {
		return out, err
	}
	tm, err := parseObj(ttsRaw)
	if err != nil {
		return out, err
	}
	lm, err := parseObj(llmRaw)
	if err != nil {
		return out, err
	}

	out.ASRProvider = strings.ToLower(strings.TrimSpace(strFromMap(am, "provider")))
	out.ASRAppID = strFromMap(am, "appId", "app_id")
	out.ASRSecretID = strFromMap(am, "secretId", "secret_id")
	out.ASRSecretKey = strFromMap(am, "secretKey", "secret_key", "secret")
	out.ASRModelType = strFromMap(am, "modelType", "model_type")

	out.TTSProvider = strings.ToLower(strings.TrimSpace(strFromMap(tm, "provider")))
	out.TTSAppID = strFromMap(tm, "appId", "app_id")
	out.TTSSecretID = strFromMap(tm, "secretId", "secret_id")
	out.TTSSecretKey = strFromMap(tm, "secretKey", "secret_key", "secret")
	if v := int64FromMap(tm, "voiceType", "voice_type"); v != 0 {
		out.TTSVoiceType = v
	}
	if v := int64FromMap(tm, "speed"); v != 0 || strFromMap(tm, "speed") == "0" {
		out.TTSSpeed = int64FromMap(tm, "speed")
	}
	out.TTSSampleRate = intFromMap(tm, "sampleRate", "sample_rate")

	out.LLMProvider = strings.ToLower(strings.TrimSpace(strFromMap(lm, "provider")))
	out.LLMAPIKey = strFromMap(lm, "apiKey", "api_key")
	out.LLMBaseURL = strFromMap(lm, "baseUrl", "base_url")
	out.LLMAppID = strFromMap(lm, "appId", "app_id")
	out.LLMModel = strFromMap(lm, "model")

	if strings.EqualFold(out.LLMProvider, "coze") {
		botID := strFromMap(lm, "botId", "bot_id")
		userID := strFromMap(lm, "userId", "user_id")
		baseURL := strFromMap(lm, "baseUrl", "base_url")
		if botID != "" || userID != "" || baseURL != "" {
			type cozeWire struct {
				BotID   string `json:"botId"`
				UserID  string `json:"userId"`
				BaseURL string `json:"baseUrl"`
			}
			b, _ := json.Marshal(cozeWire{BotID: botID, UserID: userID, BaseURL: baseURL})
			out.LLMBaseURL = string(b)
		}
	}
	return out, nil
}

// ResolveTenantVoiceEnv loads tenant JSON and merges into VoiceEnv (tenant-only credentials).
func ResolveTenantVoiceEnv(ctx context.Context, cs *sipSession.CallSession) (VoiceEnv, bool, error) {
	if cs == nil || tenantVoiceJSONLoader == nil {
		return VoiceEnv{}, false, nil
	}
	tid := cs.TenantID()
	if tid == 0 {
		return VoiceEnv{}, false, nil
	}
	asr, tts, llm, ok := tenantVoiceJSONLoader(ctx, tid)
	if !ok {
		return VoiceEnv{}, false, nil
	}
	env, err := VoiceEnvFromTenantJSON(asr, tts, llm)
	return env, true, err
}

// TenantVoiceReady reports whether env has the minimum fields for the **wired** SIP pipeline
// (QCloud ASR + QCloud TTS + configured LLM). Other ASR/TTS providers in JSON are not executed here yet.
func TenantVoiceReady(env VoiceEnv) bool {
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
	if tsp != "qcloud" {
		return false
	}
	if env.TTSAppID == "" || env.TTSSecretID == "" || env.TTSSecretKey == "" {
		return false
	}
	return voiceEnvLLMReady(env)
}

func voiceEnvLLMReady(e VoiceEnv) bool {
	if strings.EqualFold(e.LLMProvider, "alibaba") {
		return e.LLMAPIKey != "" && e.LLMAppID != ""
	}
	if strings.EqualFold(e.LLMProvider, "coze") {
		return e.LLMAPIKey != "" && e.LLMBaseURL != ""
	}
	if strings.EqualFold(e.LLMProvider, "ollama") {
		return strings.TrimSpace(e.LLMBaseURL) != ""
	}
	// openai-compatible default
	return e.LLMAPIKey != ""
}

// AttachInboundNotBoundPlayback plays scripts/not_bind.wav then server BYE (no AI / no voicedialog).
func AttachInboundNotBoundPlayback(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	return cs.AttachVoiceConversation(func() error {
		return playLocalAnnounceThenHangup(cs, "scripts/not_bind.wav", lg)
	})
}

// AttachInboundTenantAIConfigErrorPlayback plays scripts/config_error.wav then server BYE when tenant
// ASR/TTS/LLM is incomplete or voicedialog gateway attach failed (same audio as AttachVoicePipeline gate).
func AttachInboundTenantAIConfigErrorPlayback(cs *sipSession.CallSession, lg *zap.Logger) error {
	return attachTenantConfigErrorPlayback(cs, lg)
}

// attachTenantConfigErrorPlayback registers playback of scripts/config_error.wav then hangup.
func attachTenantConfigErrorPlayback(cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	return cs.AttachVoiceConversation(func() error {
		return playLocalAnnounceThenHangup(cs, "scripts/config_error.wav", lg)
	})
}

func playLocalAnnounceThenHangup(cs *sipSession.CallSession, wavPath string, lg *zap.Logger) error {
	cs.StartOnACK()
	pcmSR := cs.PCMSampleRate()
	if pcmSR <= 0 {
		pcmSR = 8000
	}
	pcm, err := LoadWAVAsPCM16Mono(wavPath, pcmSR)
	if err != nil {
		lg.Warn("sip announce wav load failed", zap.String("call_id", cs.CallID), zap.String("path", wavPath), zap.Error(err))
		RequestSIPHangup(cs.CallID)
		return err
	}
	ms := cs.MediaSession()
	if ms == nil {
		RequestSIPHangup(cs.CallID)
		return fmt.Errorf("nil media session")
	}
	playCtx := ms.GetContext()
	if playCtx == nil {
		playCtx = context.Background()
	}
	_ = playWelcomePCM(playCtx, pcm, ms, lg, pcmSR, cs.WriteAIPCM)
	RequestSIPHangup(cs.CallID)
	return nil
}
