package callanalysis

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/llm"
)

// analysisSystemPrompt 只做轻量结构化：摘要、要点、关联知识点，降低漏键/错 JSON 概率。
// 值用中文为主；键名固定便于导出解析。
const analysisSystemPrompt = `你是通话/对话摘要助手。用户输入为语音识别转写文本，可能有错字、无说话人标签或语句不完整。

要求：
- 在合理范围内理解内容，不要编造转写里未出现的具体事实（金额、单号、承诺等）；不确定的可写在 key_points 里用「待核实：」开头短句。
- 若文本无效、极短或无法分析：summary 说明原因，两个数组填空数组 []。

只输出一段合法 JSON（从 { 到 }），禁止 markdown 围栏、禁止多余说明。必须且只能包含这 3 个键：
{
  "summary": "",
  "key_points": [],
  "related_topics": []
}

字段说明：
- summary：2～6 句中文，概括对话在谈什么、结果大致如何（若看不出结果也写明「未体现明确结论」）。
- key_points：3～8 条短句，每条一句，抓核心信息（诉求、分歧、约定、数字/时间若出现可写进某条）。
- related_topics：2～8 条短语，表示可能涉及的业务/产品/流程/法规等「知识点或主题标签」，与内容弱相关也要少写，避免空泛堆砌。无则 []。`

// AnalyzeTranscript runs the configured LLM once over the transcript; returns structured JSON bytes when possible.
func AnalyzeTranscript(ctx context.Context, env Env, transcript string) (analysisJSON []byte, raw string, err error) {
	model := env.llmModelOrDefault()
	endpoint := strings.TrimSpace(env.LLMBaseURL)
	if strings.EqualFold(env.LLMProvider, "alibaba") {
		endpoint = strings.TrimSpace(env.LLMAppID)
	}
	prov, err := llm.NewLLMProvider(ctx, env.LLMProvider, env.LLMAPIKey, endpoint, analysisSystemPrompt)
	if err != nil {
		return nil, "", fmt.Errorf("llm init: %w", err)
	}
	defer prov.Hangup()

	user := "以下是需要分析的通话/对话语音识别全文。请严格按系统说明只输出 JSON，不要输出任何其它文字：\n\n---\n" + transcript + "\n---"
	raw, err = prov.Query(user, model)
	if err != nil {
		return nil, raw, fmt.Errorf("llm query: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		fallback, _ := json.Marshal(map[string]string{"error": "empty llm response"})
		return fallback, raw, nil
	}
	// Strip accidental ```json fences
	s := raw
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```JSON")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
	}
	var anyJSON json.RawMessage
	if err := json.Unmarshal([]byte(s), &anyJSON); err == nil && len(anyJSON) > 0 {
		return anyJSON, raw, nil
	}
	wrapped, werr := json.Marshal(map[string]string{"parse_error": "llm output was not valid JSON", "raw": raw})
	if werr != nil {
		return []byte(`{"parse_error":"marshal failed"}`), raw, nil
	}
	return wrapped, raw, nil
}
