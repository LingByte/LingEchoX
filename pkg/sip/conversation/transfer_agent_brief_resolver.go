package conversation

import (
	"strings"
	"sync"
)

var (
	transferAgentBriefTemplateMu sync.RWMutex
	transferAgentBriefTemplate   func(callID string) string

	transferAgentBriefVarsMu sync.RWMutex
	transferAgentBriefVarsFn func(callID string) TransferAgentBriefVars
)

// SetTransferAgentBriefTemplateResolver returns the per-DID template from
// TrunkNumber.transfer_agent_brief_text. Empty → skip agent brief.
func SetTransferAgentBriefTemplateResolver(fn func(callID string) string) {
	transferAgentBriefTemplateMu.Lock()
	transferAgentBriefTemplate = fn
	transferAgentBriefTemplateMu.Unlock()
}

// SetTransferAgentBriefVarsResolver supplies caller + ACD seat fields for template rendering.
func SetTransferAgentBriefVarsResolver(fn func(callID string) TransferAgentBriefVars) {
	transferAgentBriefVarsMu.Lock()
	transferAgentBriefVarsFn = fn
	transferAgentBriefVarsMu.Unlock()
}

// SetTransferACDAgentNameResolver is kept for compatibility; prefer SetTransferAgentBriefVarsResolver.
func SetTransferACDAgentNameResolver(fn func(callID string) string) {
	SetTransferAgentBriefVarsResolver(func(callID string) TransferAgentBriefVars {
		return TransferAgentBriefVars{
			CallerNumber: extractInboundCallerNumber(callID),
			Agent: TransferAgentBriefAgent{
				Name: fn(callID),
			},
		}
	})
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

func buildTransferAgentBriefVars(inboundCallID string) TransferAgentBriefVars {
	transferAgentBriefVarsMu.RLock()
	fn := transferAgentBriefVarsFn
	transferAgentBriefVarsMu.RUnlock()
	if fn != nil {
		v := fn(inboundCallID)
		if strings.TrimSpace(v.CallerNumber) == "" {
			v.CallerNumber = extractInboundCallerNumber(inboundCallID)
		}
		return v
	}
	return TransferAgentBriefVars{
		CallerNumber: extractInboundCallerNumber(inboundCallID),
		Agent:        TransferAgentBriefAgent{},
		Meta:         map[string]any{},
	}
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
