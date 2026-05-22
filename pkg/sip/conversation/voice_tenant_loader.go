package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// TenantVoiceJSONLoader loads raw JSON blobs for one tenant.
//
// `voiceMode` is the tenant's `voice_mode` column ("pipeline" or
// "realtime"; empty defaults to pipeline). `realtime` is the
// `realtime_config` JSON, only consulted when voiceMode == "realtime".
//
// Wired from internal/sipserver (DB). Return ok=false when tenant row is
// missing — the call leg then falls back to env-driven config / playback.
type TenantVoiceJSONLoader func(ctx context.Context, tenantID uint) (
	asr, tts, llm, realtime []byte, voiceMode string, ok bool)

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

// VoiceEnvFromTenantJSON builds VoiceEnv from tenant JSON blobs (no
// process env). `realtimeRaw` and `voiceMode` are honored only when
// voiceMode == "realtime"; otherwise they're recorded but the SIP attach
// path runs the legacy 3-layer pipeline.
func VoiceEnvFromTenantJSON(asrRaw, ttsRaw, llmRaw, realtimeRaw []byte, voiceMode string) (VoiceEnv, error) {
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
	rm, err := parseObj(realtimeRaw)
	if err != nil {
		return out, err
	}
	out.VoiceMode = strings.ToLower(strings.TrimSpace(voiceMode))
	if len(rm) > 0 {
		raw := make(map[string]any, len(rm))
		for k, v := range rm {
			raw[k] = v
		}
		out.RealtimeProvider = strings.ToLower(strings.TrimSpace(strFromMap(rm, "provider")))
		if _, ok := raw["provider"]; !ok && out.RealtimeProvider != "" {
			raw["provider"] = out.RealtimeProvider
		}
		out.RealtimeConfigRaw = raw
	}
	// VoiceMode inference. Three cases:
	//
	//  1. Explicit "realtime" / "pipeline" from DB column → trust it.
	//  2. Empty AND realtime tab populated with credentials → infer
	//     "realtime". This is the migration-resilient path: when the
	//     `voice_mode` column hasn't been added yet (fresh deploy w/o
	//     AutoMigrate restart, legacy row, manual DB import…) but the
	//     operator already filled the realtime credentials, we must NOT
	//     bounce the call to config_error.wav.
	//  3. Empty AND realtime tab empty → "pipeline" (legacy default).
	//
	// Step 2 covers the user-reported regression: "选了多模态还是判到 ASR"
	// when the column-default trip lands on empty string.
	if out.VoiceMode == "" {
		if TenantRealtimeReady(out) {
			out.VoiceMode = "realtime"
		} else {
			out.VoiceMode = "pipeline"
		}
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
	// Pass-through copy of the parsed TTS JSON. Downstream call sites feed
	// this directly to synthesizer.NewStreamingFromCredential so any
	// provider in TENANT_TTS_PROVIDER_RULES works without per-provider Go
	// plumbing here.
	if len(tm) > 0 {
		raw := make(map[string]any, len(tm))
		for k, v := range tm {
			raw[k] = v
		}
		if _, ok := raw["provider"]; !ok && out.TTSProvider != "" {
			raw["provider"] = out.TTSProvider
		}
		out.TTSConfigRaw = raw
	}

	out.LLMProvider = strings.ToLower(strings.TrimSpace(strFromMap(lm, "provider")))
	out.LLMAPIKey = strFromMap(lm, "apiKey", "api_key")
	out.LLMBaseURL = strFromMap(lm, "baseUrl", "base_url")
	out.LLMAppID = strFromMap(lm, "appId", "app_id")
	out.LLMModel = strFromMap(lm, "model")

	n := parseTransferConfirmCount(rm)
	if n == 0 {
		n = parseTransferConfirmCount(lm)
	}
	out.TransferConfirmCount = n

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
	asr, tts, llm, realtime, voiceMode, ok := tenantVoiceJSONLoader(ctx, tid)
	if !ok {
		return VoiceEnv{}, false, nil
	}
	env, err := VoiceEnvFromTenantJSON(asr, tts, llm, realtime, voiceMode)
	return env, true, err
}

// TenantVoiceReady reports whether env has the minimum fields for the
// configured voice attach path:
//
//   - VoiceMode == "realtime": only RealtimeConfigRaw is consulted; the
//     ASR/TTS/LLM tabs are intentionally bypassed because the realtime
//     model collapses all three.
//   - VoiceMode == "pipeline" (default): the legacy ASR + TTS + LLM
//     readiness check.
func TenantVoiceReady(env VoiceEnv) bool {
	if strings.EqualFold(env.VoiceMode, "realtime") {
		return TenantRealtimeReady(env)
	}
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
	// Per-provider readiness: any provider whose minimum credential fields
	// are present in TTSConfigRaw qualifies. We intentionally avoid calling
	// synthesizer.NewSynthesisServiceFromCredential here (it would open
	// network handles); cheap field presence is enough as a gate, the
	// factory will surface a typed error later if config is malformed.
	if !ttsCredentialsReady(tsp, env) {
		return false
	}
	return voiceEnvLLMReady(env)
}

// ttsCredentialsReady is a low-cost feasibility check for the configured
// TTS provider. The exhaustive validation lives inside
// synthesizer.NewSynthesisServiceFromCredential; this just rules out
// obviously-empty configs so the call leg can fall back to the
// `config_error.wav` playback instead of attempting and failing mid-call.
func ttsCredentialsReady(provider string, env VoiceEnv) bool {
	switch provider {
	case "qcloud", "tencent":
		return env.TTSAppID != "" && env.TTSSecretID != "" && env.TTSSecretKey != ""
	case "aliyun", "dashscope", "qwen",
		"qiniu", "openai", "elevenlabs", "fishspeech", "fishaudio",
		"google", "minimax", "volcengine", "volcengine_stream", "volcengine_llm", "volcengine_clone":
		// API-key-driven providers: require at least an apiKey field on the
		// raw JSON (covers both camelCase / snake_case keys).
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
		// Baidu needs either `token` or `apiKey`+`secretKey` pair.
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
				// try snake_case alt
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
		// Unknown providers: defer to the credential factory at call time.
		// Returning true here lets the factory produce a precise error and
		// the call leg fall back to config_error.wav playback.
		return env.TTSConfigRaw != nil
	}
}

func snakeCaseAlt(camel string) string {
	var b strings.Builder
	for i, r := range camel {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
			b.WriteRune(r + 32)
			continue
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pipelineCredsUsable mirrors the body of the legacy TenantVoiceReady
// pipeline branch but stops short of the LLM check. We split it out so
// the SIP attach gate can ask "would pipeline have worked?" without
// also triggering the realtime branch, used by the auto-fallback in
// AttachVoicePipeline (legacy rows with column default "pipeline" plus
// an only-realtime credential set should still attach via realtime).
func pipelineCredsUsable(env VoiceEnv) bool {
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
	return voiceEnvLLMReady(env)
}

// TenantRealtimeReady is the realtime-mode counterpart of TenantVoiceReady.
// We only need a credential blob and a known provider here — the WS
// factory in pkg/realtime is responsible for vendor-specific field
// validation. Requiring at minimum apiKey is enough to rule out the
// "tenant left the form blank" case so the call leg falls back to
// config_error.wav rather than ringing into a broken session.
func TenantRealtimeReady(env VoiceEnv) bool {
	raw := env.RealtimeConfigRaw
	if len(raw) == 0 {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(env.RealtimeProvider))
	if provider == "" {
		// provider may live only in the raw map.
		provider = strings.ToLower(strings.TrimSpace(strFromMap(raw, "provider")))
	}
	if provider == "" {
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
