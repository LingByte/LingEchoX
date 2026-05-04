package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/llm"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"go.uber.org/zap"
)

// VoiceEnvForVoicedialogLoopback returns env vars used by the SIP voice pipeline (LLM_* same as outbound).
func VoiceEnvForVoicedialogLoopback() VoiceEnv {
	return voiceEnvFromProcess()
}

// ReadyForVoicedialogLoopbackLLM is true when LLM_* env is sufficient for inbound voicedialog loopback (no ASR/TTS check).
func (e VoiceEnv) ReadyForVoicedialogLoopbackLLM() bool {
	prov := strings.TrimSpace(e.LLMProvider)
	if prov == "" {
		return false
	}
	llmReady := e.LLMAPIKey != "" && e.LLMBaseURL != ""
	if strings.EqualFold(prov, "alibaba") {
		llmReady = e.LLMAPIKey != "" && strings.TrimSpace(e.LLMAppID) != ""
	}
	return llmReady
}

// NewVoicedialogLoopbackLLMProvider builds one LLM session per inbound call (multi-turn history inside provider).
func NewVoicedialogLoopbackLLMProvider(ctx context.Context, callID string, lg *zap.Logger) (llm.LLMProvider, string, func(), error) {
	env := voiceEnvFromProcess()
	if !env.ReadyForVoicedialogLoopbackLLM() {
		return nil, "", nil, fmt.Errorf("voicedialog loopback LLM: incomplete LLM env (LLM_PROVIDER / LLM_APIKEY / LLM_BASEURL or Alibaba APP_ID)")
	}
	model := strings.TrimSpace(env.LLMModel)
	if model == "" {
		model = "qwen-plus"
	}
	systemPrompt := popSIPCallSystemPrompt(callID)
	endpoint := strings.TrimSpace(env.LLMBaseURL)
	if strings.EqualFold(env.LLMProvider, "alibaba") {
		endpoint = strings.TrimSpace(env.LLMAppID)
	}
	p, err := llm.NewLLMProvider(ctx, env.LLMProvider, env.LLMAPIKey, endpoint, systemPrompt)
	if err != nil {
		return nil, "", nil, err
	}
	if lg == nil && logger.Lg != nil {
		lg = logger.Lg
	}
	if lg != nil {
		registerSIPTransferTool(p, callID, lg.Named("voicedialog-loopback"))
	}
	cleanup := func() { p.Hangup() }
	return p, model, cleanup, nil
}

// VoicedialogLoopbackLLMQuery runs one user turn (provider keeps conversation history).
func VoicedialogLoopbackLLMQuery(_ context.Context, p llm.LLMProvider, model, userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" || p == nil {
		return "", nil
	}
	reply, err := p.Query(userText, model)
	if err != nil {
		return "", err
	}
	return normalizeTTSText(reply), nil
}

// VoicedialogShouldTriggerTransfer mirrors outbound SIP voice transfer gates after an assistant turn (tool / explicit phrase / pending flag).
func VoicedialogShouldTriggerTransfer(callID, userText string, prov llm.LLMProvider) bool {
	if prov == nil {
		return false
	}
	userText = strings.TrimSpace(userText)
	explicitXfer := sipIntentExplicitTransferRequest(userText)
	if ap, ok := prov.(*llm.AlibabaProvider); ok {
		if action := ap.ConsumePendingAction(); action == "transfer_to_agent" {
			return true
		}
		if explicitXfer {
			return true
		}
	} else if consumeSIPTransferPending(callID) || explicitXfer {
		return true
	}
	return false
}
