package conversation

import "testing"

func TestRenderTransferAgentBriefTemplate(t *testing.T) {
	vars := TransferAgentBriefVars{
		CallerNumber: "13800138000",
		AgentName:    "张三",
	}
	got := RenderTransferAgentBriefTemplate("您好{{Name}}，尾号{{NTail4}}的来电。", vars)
	want := "您好张三，尾号8000的来电。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRenderTransferAgentBriefTemplate_FullNumber(t *testing.T) {
	vars := TransferAgentBriefVars{CallerNumber: "01088886666", AgentName: "小李"}
	got := RenderTransferAgentBriefTemplate("来电号码{{N}}，坐席{{Name}}。", vars)
	if got != "来电号码01088886666，坐席小李。" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderTransferAgentBriefTemplate_UnknownPlaceholder(t *testing.T) {
	got := RenderTransferAgentBriefTemplate("测试{{Foo}}结束", TransferAgentBriefVars{})
	if got != "测试结束" {
		t.Fatalf("got %q", got)
	}
}
