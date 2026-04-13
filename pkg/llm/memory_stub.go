package llm

import (
	"context"

	"go.uber.org/zap"
)

// memorySummarizeFunc kept for compatibility.
type memorySummarizeFunc func(ctx context.Context, model string, transcript string, previousSummary string) (string, error)

// asyncTurnMemory is intentionally a no-op compatibility stub.
type asyncTurnMemory struct{}

func newAsyncTurnMemory(_ context.Context, _ *zap.Logger) *asyncTurnMemory { return &asyncTurnMemory{} }
func (m *asyncTurnMemory) reset()                                          {}
func (m *asyncTurnMemory) setMaxMemoryMessages(_ int)                      {}
func (m *asyncTurnMemory) getMaxMemoryMessages() int                       { return defaultMaxMemoryMessages }
func (m *asyncTurnMemory) mergedSystemPrompt(base string) string           { return base }
func (m *asyncTurnMemory) buildChatCompletionMessages(systemPrompt string, userText string) []map[string]string {
	msgs := make([]map[string]string, 0, 2)
	if systemPrompt != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": systemPrompt})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userText})
	return msgs
}
func (m *asyncTurnMemory) snapshotTurns() []llmMemoryMessage { return nil }
func (m *asyncTurnMemory) summaryText() string               { return "" }
func (m *asyncTurnMemory) appendPairAndMaybeSummarize(_ context.Context, _, _, _ string, _ memorySummarizeFunc) {
}
func (m *asyncTurnMemory) runSummarize(_ context.Context, _ string, _ []llmMemoryMessage, _ string, _ uint64, _ memorySummarizeFunc) {
}
func (m *asyncTurnMemory) summarizeMemorySync(_ context.Context, _ string, _ memorySummarizeFunc) (string, error) {
	return "", nil
}
