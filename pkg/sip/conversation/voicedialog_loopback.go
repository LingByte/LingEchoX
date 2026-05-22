package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/llm"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"go.uber.org/zap"
)

// ReadyForVoicedialogLoopbackLLM is true when tenant LLM JSON is sufficient for inbound voicedialog loopback.
func (e VoiceEnv) ReadyForVoicedialogLoopbackLLM() bool {
	return voiceEnvLLMReady(e)
}

// NewVoicedialogLoopbackLLMProvider builds one LLM session per inbound call (multi-turn history inside provider).
func NewVoicedialogLoopbackLLMProvider(ctx context.Context, callID string, lg *zap.Logger) (llm.LLMProvider, string, func(), error) {
	cs := LookupInboundCallSession(callID)
	if cs == nil {
		return nil, "", nil, fmt.Errorf("voicedialog loopback LLM: no inbound CallSession for %s", callID)
	}
	env, loaded, err := ResolveTenantVoiceEnv(ctx, cs)
	if err != nil {
		return nil, "", nil, err
	}
	if !loaded || !env.ReadyForVoicedialogLoopbackLLM() {
		return nil, "", nil, fmt.Errorf("voicedialog loopback LLM: incomplete tenant llmConfig")
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
		registerSIPTransferTool(p, callID, TransferConfirmRequired(VoiceEnv{}), lg.Named("voicedialog-loopback"))
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

// VoicedialogLoopbackLLMStream runs one user turn in streaming mode and emits "message" deltas
// so the caller can split by sentence punctuation and dispatch tts.speak segments early.
//
// All providers (OpenAI/Coze/Ollama/Alibaba) share the same delta semantics via the
// LLMProvider.QueryStream contract: callback(segment, isComplete) where segment is the new
// text increment, and isComplete=true is sent exactly once at end-of-stream with segment="".
//
// onDelta is invoked with (deltaText, isFinal); the ctx parameter is retained for future
// cancellation hooks but currently providers manage their own internal context.
func VoicedialogLoopbackLLMStream(
	_ context.Context,
	p llm.LLMProvider,
	model, userText string,
	onDelta func(delta string, isFinal bool) error,
) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" || p == nil {
		return "", nil
	}
	full, err := p.QueryStream(userText, llm.QueryOptions{Model: model, Stream: true},
		func(seg string, complete bool) error {
			if onDelta == nil {
				return nil
			}
			if !complete && seg != "" {
				return onDelta(seg, false)
			}
			if complete {
				return onDelta("", true)
			}
			return nil
		})
	if err != nil {
		return full, err
	}
	return normalizeTTSText(full), nil
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
