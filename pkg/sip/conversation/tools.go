package conversation

import (
	"encoding/json"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/llm"
	"go.uber.org/zap"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

func registerSIPTransferTool(provider llm.LLMProvider, callID string, confirmRequired int, lg *zap.Logger) {
	if provider == nil || strings.TrimSpace(callID) == "" {
		return
	}
	params := json.RawMessage(`{
		"type":"object",
		"properties":{
			"reason":{"type":"string","description":"用户请求转人工的简短原因"},
			"confidence":{"type":"number","description":"0到1，当前意图置信度"}
		},
		"required":[],
		"additionalProperties":true
	}`)
	provider.RegisterFunctionTool(
		"transfer_to_agent",
		"当用户明确要求转人工且后台确认次数已满足时调用；未满次数勿调用，对用户照常解答、勿透露还剩几次。",
		params,
		func(args map[string]interface{}, _ interface{}) (string, error) {
			allowed, count := sipTransferMayExecute(callID, confirmRequired)
			if !allowed {
				if lg != nil {
					lg.Info("sip voice: transfer tool blocked (confirm count)",
						zap.String("call_id", callID),
						zap.Int("count", count),
						zap.Int("required", confirmRequired),
					)
				}
				return transferConfirmToolMessage(count, confirmRequired), nil
			}
			if lg != nil {
				lg.Info("sip voice: transfer tool invoked",
					zap.String("call_id", callID),
					zap.Any("args", args),
					zap.Int("confirm_count", count),
				)
			}
			markSIPTransferPending(callID)
			return "transfer_requested", nil
		},
	)
}
