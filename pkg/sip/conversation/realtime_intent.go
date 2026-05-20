package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Realtime-mode intent detection.
//
// Why this file exists:
// ---------------------
// The classic SIP voice path uses LLM tool calls (`transfer_to_agent`) to
// trigger transfers — see `tools.go`. Realtime providers like Qwen-Omni
// realtime do **not** support function/tool calling on the WS protocol;
// the assistant only emits text + audio. So in realtime mode we drive
// transfer detection entirely from text:
//
//  1. The system prompt is augmented (see realtimeAugmentSystemPrompt) so
//     the model is *instructed* to append a sentinel marker
//     (`[TRANSFER_TO_AGENT]`) when the user asks for a human agent. We
//     scan assistant text for that marker.
//
//  2. Belt-and-braces: we also keyword-match the user transcript against
//     a Chinese phrase list. If the model misbehaves and forgets the
//     marker, the user-side match still triggers transfer.
//
// Output is the same `markSIPTransferPending` flag the tool-call path
// sets — the existing `consumeSIPTransferPending` consumer fires the
// real `TriggerTransferToAgent` after the in-flight reply finishes (or
// immediately, in realtime mode where there's nothing to drain).
//
// Hangup phrase detection ("再见 / 拜拜 / 挂了") follows the same shape
// and reuses `sipHangupPhrasesFromEnv()` from voice.go.

import (
	"strings"
)

// transferAgentMarker is the sentinel the realtime model is asked to
// append to its reply when the user asks for a human agent. Chosen to
// be distinct from any natural language so false positives are unlikely.
const transferAgentMarker = "[TRANSFER_TO_AGENT]"

// transferUserPhrasesDefault is the keyword list for user-side detection.
// Order doesn't matter; first hit wins. Keep this conservative — false
// positives cause unnecessary transfers, which are user-visible.
var transferUserPhrasesDefault = []string{
	"转人工",
	"人工客服",
	"真人客服",
	"找客服",
	"接客服",
	"接线员",
	"我要人工",
	"换个人工",
	"转接人工",
}

// realtimeAugmentSystemPrompt merges the operator-supplied system prompt
// with the marker contract. We always append (never prepend) so the
// caller's intent dominates style/persona while the marker rule sits at
// the bottom as a hard constraint the model is unlikely to drop.
//
// Empty `userPrompt` is fine — the marker rule alone is a valid prompt.
func realtimeAugmentSystemPrompt(userPrompt string) string {
	const rule = "重要规则：当用户明确要求转接人工客服 / 真人客服时，请在你回复的最后单独追加一行" +
		" `" + transferAgentMarker + "` 标记（这是系统识别用的指令，对用户不可见，请勿读出来）。" +
		"如果用户没有要求转人工，绝对不要输出该标记。"
	user := strings.TrimSpace(userPrompt)
	if user == "" {
		return rule
	}
	return user + "\n\n" + rule
}

// realtimeMatchTransferIntent returns true when either:
//   - the assistant text contains the sentinel marker (model-driven path), or
//   - the user transcript contains any phrase from `userPhrases`
//     (keyword fallback). When userPhrases is nil, the default Chinese
//     list is used.
//
// `which` is "user" or "assistant" — controls which detector runs.
func realtimeMatchTransferIntent(which, text string, userPhrases []string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	switch which {
	case "assistant":
		return strings.Contains(t, strings.ToLower(transferAgentMarker))
	case "user":
		phrases := userPhrases
		if phrases == nil {
			phrases = transferUserPhrasesDefault
		}
		for _, p := range phrases {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == "" {
				continue
			}
			if strings.Contains(t, p) {
				return true
			}
		}
	}
	return false
}

// realtimeStripMarker drops the transfer marker (and any blank trailing
// line introduced by the augmentation rule) from `text` before the
// caller logs / forwards the assistant reply. We never want the marker
// to leak into transcripts, dialog turn records, or downstream UIs.
func realtimeStripMarker(text string) string {
	if text == "" {
		return text
	}
	out := strings.ReplaceAll(text, transferAgentMarker, "")
	// Some models indent / wrap; do a case-insensitive sweep too.
	if i := strings.Index(strings.ToLower(out), strings.ToLower(transferAgentMarker)); i != -1 {
		out = out[:i] + out[i+len(transferAgentMarker):]
	}
	return strings.TrimSpace(out)
}

// realtimeMatchHangupIntent runs the env-configured hangup phrase set
// against an *assistant* text. We don't auto-hang on user farewells —
// the model must explicitly say goodbye for the AI to release.
func realtimeMatchHangupIntent(text string, phrases []string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	for _, p := range phrases {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}
