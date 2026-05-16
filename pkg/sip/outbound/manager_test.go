package outbound

import (
	"context"
	"testing"
)

func TestManager_Dial_RequiresBind(t *testing.T) {
	m := NewManager(ManagerConfig{LocalIP: "127.0.0.1", SIPHost: "127.0.0.1", SIPPort: 6050})
	_, err := m.Dial(context.Background(), DialRequest{
		Scenario: ScenarioCampaign,
		Target: DialTarget{
			RequestURI:    "sip:bob@example.com",
			SignalingAddr: "127.0.0.1:6050",
		},
		MediaProfile: MediaProfileNone,
	})
	if err != ErrNoSignalingSender {
		t.Fatalf("expected ErrNoSignalingSender, got %v", err)
	}
}

func TestFormatOutboundFromHeader_CallerID(t *testing.T) {
	const tag = "t1"
	noDisp := formatOutboundFromHeader("", "4001880771", "192.0.2.1", 6050, tag)
	wantNo := "<sip:4001880771@192.0.2.1:6050>;tag=t1"
	if noDisp != wantNo {
		t.Fatalf("no display: got %q want %q", noDisp, wantNo)
	}
	// 中文 display name 走 RFC 2047 MIME encoded-word（国内运营商 SBC 兼容）。
	// "客服热线" 的 UTF-8 base64 = "5a6i5pyN54Ot57q/"。
	withDisp := formatOutboundFromHeader("客服热线", "4001880771", "192.0.2.1", 6050, tag)
	want := "=?UTF-8?B?5a6i5pyN54Ot57q/?= <sip:4001880771@192.0.2.1:6050>;tag=" + tag
	if withDisp != want {
		t.Fatalf("with display: got %q want %q", withDisp, want)
	}
}

func TestBuildINVITE_ContainsMethod(t *testing.T) {
	p := inviteParams{
		LocalIP:      "127.0.0.1",
		SIPHost:      "127.0.0.1",
		SIPPort:      6050,
		RequestURI:   "sip:bob@example.com",
		CallID:       "test@127.0.0.1",
		FromTag:      "abc",
		Branch:       "branch1",
		CSeq:         1,
		LocalRTPPort: 10000,
		SDPBody:      "v=0\r\n",
		FromUser:     "alice",
	}
	msg := buildINVITE(p)
	if !msg.IsRequest || msg.Method != "INVITE" {
		t.Fatalf("expected INVITE request")
	}
	if msg.GetHeader("Call-ID") != p.CallID {
		t.Fatalf("call-id")
	}
}
