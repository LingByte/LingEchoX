package conversation

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Realtime-mode intent detection.
//
// Why this file exists:
// ---------------------
// The classic SIP voice path uses LLM tool calls (`transfer_to_agent`) to
// trigger transfers — see `tools.go`. Qwen3.5-Omni-Realtime supports
// Function Calling on the WS protocol (see `realtime_tools.go` and
// pkg/realtime/aliyunomni). When tools are disabled we drive transfer
// detection from text:
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
	"strconv"
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
	"人工",
	"人工，", // e.g.「人工，人工」
	"人工客服",
	"真人客服",
	"找客服",
	"接客服",
	"接线员",
	"我要人工",
	"我这边要人工",
	"换个人工",
	"转接人工",
	"可以转接人工吗",
	"可以转接人工",
}

// transferAckPromisePhrases are active transfer-in-progress wording only.
// Capability lines like「可以转接人工客服」must not match.
var transferAckPromisePhrases = []string{
	"正在为您转接",
	"正在转接",
	"马上为您转接",
	"马上转接",
	"已为您转接",
	"这就为您转接",
	"开始为您转接",
	"帮您转接人工",
	"为您转接人工",
}

// realtimeMatchTransferAckPhrase reports the assistant is promising an active
// transfer (not merely mentioning the transfer capability).
func realtimeMatchTransferAckPhrase(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	for _, p := range transferAckPromisePhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	// e.g.「请稍候，正在转接」— require both cues, not「请稍候」alone.
	if strings.Contains(t, "稍候") && strings.Contains(t, "转接") {
		return true
	}
	return false
}

// realtimeNoProactiveTransferRule keeps transfer capability internal until the user asks.
const realtimeNoProactiveTransferRule = "话术约束：自我介绍、问候、产品解答时禁止主动提及转人工、人工客服、真人客服、" +
	"「可转人工」「如需转接」等表述；不要向用户罗列或暗示转人工选项。仅当用户明确说要转人工/找人工/接线员时，才进入转人工流程。"

func realtimeTransferToolPromptRule(confirmRequired int) string {
	confirmRequired = clampTransferConfirmCount(confirmRequired)
	tools := "后台工具（勿向用户宣读）：get_current_time、is_business_hours、calculate；" +
		"transfer_to_agent 仅用户明确要求转人工时调用，平时勿提起。"
	if confirmRequired <= 1 {
		return realtimeNoProactiveTransferRule + "\n" + tools +
			" 问时间请调用 get_current_time。用户明确要转人工时调用 transfer_to_agent；对用户说「" + transferConfirmExecuteReplyZH + "」，勿说其它转接措辞。"
	}
	return realtimeNoProactiveTransferRule + "\n" + tools +
		" 问时间请调用 get_current_time，不要编造。" +
		" 转人工由后台累计用户 " + strconv.Itoa(confirmRequired) + " 次明确表达后才可调用 transfer_to_agent（勿向用户透露累计几次或还剩几次）。" +
		"未满次数时勿调用该工具；对用户只能说「" + transferConfirmNormalReplyZH + "」，严禁「正在为您转接」「请稍候」「马上转接」，不要追问「再说一次转人工」。" +
		"满足次数后调用 transfer_to_agent；对用户只说「" + transferConfirmExecuteReplyZH + "」。"
}

// realtimeAugmentSystemPrompt appends transfer/tool rules after operatorCore.
// Pass empty operatorCore to get rules-only (for merging with tenant instructions).
func realtimeAugmentSystemPrompt(operatorCore string, useTransferTool bool, transferConfirmRequired int) string {
	var rule string
	if useTransferTool {
		rule = realtimeTransferToolPromptRule(transferConfirmRequired)
	} else {
		rule = "重要规则：当用户明确要求转接人工客服 / 真人客服时，请在你回复的最后单独追加一行" +
			" `" + transferAgentMarker + "` 标记（这是系统识别用的指令，对用户不可见，请勿读出来）。" +
			"如果用户没有要求转人工，绝对不要输出该标记。"
	}
	user := strings.TrimSpace(operatorCore)
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
			if p == "人工" && strings.Contains(t, "人工智能") {
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
