package conversation

import (
	"strings"
	"sync"
)

var (
	transferAgentBriefTemplateMu sync.RWMutex
	transferAgentBriefTemplate   func(callID string) string

	transferACDAgentNameMu sync.RWMutex
	transferACDAgentName     func(callID string) string
)

// SetTransferAgentBriefTemplateResolver returns the per-DID template from
// TrunkNumber.transfer_agent_brief_text. Empty → skip agent brief.
func SetTransferAgentBriefTemplateResolver(fn func(callID string) string) {
	transferAgentBriefTemplateMu.Lock()
	transferAgentBriefTemplate = fn
	transferAgentBriefTemplateMu.Unlock()
}

// SetTransferACDAgentNameResolver returns the connected agent display name
// (acd_pool_targets.name, else target_value) for template {{Name}}.
func SetTransferACDAgentNameResolver(fn func(callID string) string) {
	transferACDAgentNameMu.Lock()
	transferACDAgentName = fn
	transferACDAgentNameMu.Unlock()
}

func resolveTransferAgentBriefTemplate(callID string) string {
	transferAgentBriefTemplateMu.RLock()
	fn := transferAgentBriefTemplate
	transferAgentBriefTemplateMu.RUnlock()
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(fn(callID))
}

func resolveTransferACDAgentName(callID string) string {
	transferACDAgentNameMu.RLock()
	fn := transferACDAgentName
	transferACDAgentNameMu.RUnlock()
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(fn(callID))
}

// PeekInboundTransferACDTargetID returns the last routed acd_pool_targets.id without clearing.
func PeekInboundTransferACDTargetID(callID string) (uint, bool) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return 0, false
	}
	if v, ok := transferLastACDRowByInbound.Load(callID); ok {
		if id, ok := v.(uint); ok && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func buildTransferAgentBriefVars(inboundCallID string) TransferAgentBriefVars {
	return TransferAgentBriefVars{
		CallerNumber: extractInboundCallerNumber(inboundCallID),
		AgentName:    resolveTransferACDAgentName(inboundCallID),
	}
}
