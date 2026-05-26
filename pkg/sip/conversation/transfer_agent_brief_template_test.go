package conversation

import "testing"

func TestRenderTransferAgentBriefTemplate(t *testing.T) {
	vars := TransferAgentBriefVars{
		CallerNumber: "13800138000",
		Agent: TransferAgentBriefAgent{
			Name:   "张三",
			Remark: "人工备注",
		},
		Meta: map[string]any{
			"FactoryNumber": "F-1001",
		},
	}
	got := RenderTransferAgentBriefTemplate("您好{{Name}}，工厂{{MetaData.FactoryNumber}}，尾号{{NTail4}}。", vars)
	want := "您好张三，工厂F-1001，尾号8000。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRenderTransferAgentBriefTemplate_AgentFields(t *testing.T) {
	vars := TransferAgentBriefVars{
		CallerNumber: "01088886666",
		Agent: TransferAgentBriefAgent{
			TargetValue:          "10086",
			SipCallerDisplayName: "客服",
		},
	}
	got := RenderTransferAgentBriefTemplate("坐席{{TargetValue}}，主叫{{N}}。", vars)
	if got != "坐席10086，主叫01088886666。" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderTransferAgentBriefTemplate_UnknownPlaceholder(t *testing.T) {
	got := RenderTransferAgentBriefTemplate("测试{{Foo}}结束", TransferAgentBriefVars{})
	if got != "测试结束" {
		t.Fatalf("got %q", got)
	}
}
