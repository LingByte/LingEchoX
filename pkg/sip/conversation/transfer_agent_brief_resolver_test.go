package conversation

import "testing"

func TestResolveTransferBriefForCall_callerFallsBackToAgent(t *testing.T) {
	SetTransferAgentBriefTemplateResolver(func(callID string) string {
		if callID == "in-1" {
			return "agent {{Name}}"
		}
		return ""
	})
	SetTransferCallerBriefTemplateResolver(func(callID string) string {
		return ""
	})
	SetTransferAgentBriefVarsResolver(func(callID string) TransferAgentBriefVars {
		return TransferAgentBriefVars{
			Agent: TransferAgentBriefAgent{Name: "Alice"},
		}
	})
	t.Cleanup(func() {
		SetTransferAgentBriefTemplateResolver(nil)
		SetTransferCallerBriefTemplateResolver(nil)
		SetTransferAgentBriefVarsResolver(nil)
	})

	caller, agent, ok := resolveTransferBriefForCall("in-1")
	if !ok {
		t.Fatal("expected ok")
	}
	if caller != "agent Alice" || agent != "agent Alice" {
		t.Fatalf("caller=%q agent=%q", caller, agent)
	}
}

func TestResolveTransferBriefForCall_differentTexts(t *testing.T) {
	SetTransferAgentBriefTemplateResolver(func(callID string) string {
		return "agent line"
	})
	SetTransferCallerBriefTemplateResolver(func(callID string) string {
		return "caller line"
	})
	t.Cleanup(func() {
		SetTransferAgentBriefTemplateResolver(nil)
		SetTransferCallerBriefTemplateResolver(nil)
	})

	caller, agent, ok := resolveTransferBriefForCall("in-2")
	if !ok {
		t.Fatal("expected ok")
	}
	if caller != "caller line" || agent != "agent line" {
		t.Fatalf("caller=%q agent=%q", caller, agent)
	}
}

func TestHasTransferBriefConfigured(t *testing.T) {
	SetTransferAgentBriefTemplateResolver(func(callID string) string {
		return ""
	})
	SetTransferCallerBriefTemplateResolver(func(callID string) string {
		return "only caller"
	})
	t.Cleanup(func() {
		SetTransferAgentBriefTemplateResolver(nil)
		SetTransferCallerBriefTemplateResolver(nil)
	})
	if !hasTransferBriefConfigured("x") {
		t.Fatal("expected configured when caller template set")
	}
}
