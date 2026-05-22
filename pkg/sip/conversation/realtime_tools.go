package conversation

import (
	"encoding/json"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/realtime"
)

var (
	sipRealtimeTransferToolParams = json.RawMessage(`{
		"type":"object",
		"properties":{
			"reason":{"type":"string","description":"用户请求转人工的简短原因"},
			"confidence":{"type":"number","description":"0到1，当前意图置信度"}
		},
		"required":[],
		"additionalProperties":true
	}`)

	sipRealtimeGetCurrentTimeParams = json.RawMessage(`{
		"type":"object",
		"properties":{
			"timezone":{"type":"string","description":"IANA 时区，如 Asia/Shanghai；默认 Asia/Shanghai"}
		},
		"required":[],
		"additionalProperties":false
	}`)

	sipRealtimeIsBusinessHoursParams = json.RawMessage(`{
		"type":"object",
		"properties":{
			"timezone":{"type":"string","description":"IANA 时区，默认 Asia/Shanghai"}
		},
		"required":[],
		"additionalProperties":false
	}`)

	sipRealtimeCalculateParams = json.RawMessage(`{
		"type":"object",
		"properties":{
			"expression":{"type":"string","description":"简单算术表达式，仅支持数字与 + - * / 和括号，如 100+20*3"}
		},
		"required":["expression"],
		"additionalProperties":false
	}`)
)

// SIPRealtimeTools returns tools registered on Qwen-Omni-Realtime session.update.
func SIPRealtimeTools() []realtime.Tool {
	return []realtime.Tool{
		{
			Name:        "transfer_to_agent",
			Description: "仅当用户明确要求转人工且后台确认次数已满足时调用；未满次数勿调用。勿向用户透露确认次数；未满时照常客服应答即可。",
			Parameters:  sipRealtimeTransferToolParams,
		},
		{
			Name:        "get_current_time",
			Description: "获取当前日期、时间与星期。用户问「几点了」「今天几号」「星期几」时调用。",
			Parameters:  sipRealtimeGetCurrentTimeParams,
		},
		{
			Name:        "is_business_hours",
			Description: "判断当前是否在工作时间（周一至周五 9:00-18:00，指定时区）。用户问是否在营业时间、能否转人工时调用。",
			Parameters:  sipRealtimeIsBusinessHoursParams,
		},
		{
			Name:        "calculate",
			Description: "计算简单算术表达式（仅 + - * / 与括号）。用户问心算类金额、数量时调用。",
			Parameters:  sipRealtimeCalculateParams,
		},
	}
}

// SIPRealtimeTransferTools is deprecated; use SIPRealtimeTools.
func SIPRealtimeTransferTools() []realtime.Tool {
	return SIPRealtimeTools()
}

// realtimeSupportsTransferTools reports whether the configured realtime provider
// uses the DashScope WS Function Calling path (Qwen3.5-Omni-Realtime).
func realtimeSupportsTransferTools(env VoiceEnv) bool {
	p := strings.ToLower(strings.TrimSpace(env.RealtimeProvider))
	switch p {
	case "", "aliyun_omni", "qwen_omni", "dashscope_omni":
		return true
	default:
		return false
	}
}
