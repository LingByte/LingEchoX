package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/llm"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"go.uber.org/zap"
)

// ReadyForVoicedialogLoopbackLLM is true when LLM_* env is sufficient for inbound voicedialog loopback (no ASR/TTS check).
func (e VoiceEnv) ReadyForVoicedialogLoopbackLLM() bool {
	if strings.TrimSpace(e.LLMProvider) == "" || e.LLMAPIKey == "" {
		return false
	}
	if strings.EqualFold(e.LLMProvider, "alibaba") {
		return e.LLMAppID != ""
	}
	return e.LLMBaseURL != ""
}

// NewVoicedialogLoopbackLLMProvider builds one LLM session per inbound call (multi-turn history inside provider).
func NewVoicedialogLoopbackLLMProvider(ctx context.Context, callID string, lg *zap.Logger) (llm.LLMProvider, string, func(), error) {
	env := VoiceEnvFromProcess()
	if !env.ReadyForVoicedialogLoopbackLLM() {
		return nil, "", nil, fmt.Errorf("voicedialog loopback LLM: incomplete LLM env (LLM_PROVIDER / LLM_APIKEY / LLM_BASEURL or Alibaba APP_ID)")
	}
	systemPrompt := popSIPCallSystemPrompt(callID)
	p, err := llm.NewLLMProvider(ctx, env.LLMProvider, env.LLMAPIKey, llmAPIURLForProvider(env), systemPrompt)
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
	return p, env.LLMModel, cleanup, nil
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

// VoicedialogShouldTriggerTransfer mirrors outbound SIP voice transfer gates after an assistant turn (LLM tool / pending flag).
func VoicedialogShouldTriggerTransfer(callID string, prov llm.LLMProvider) bool {
	if prov == nil {
		return false
	}
	if ap, ok := prov.(*llm.AlibabaProvider); ok {
		return ap.ConsumePendingAction() == "transfer_to_agent"
	}
	return consumeSIPTransferPending(callID)
}
