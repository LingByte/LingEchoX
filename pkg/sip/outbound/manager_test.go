package outbound

import (
	"context"
	"strings"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/historyinfo"
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
	if msg.GetHeader("P-Asserted-Identity") != "" {
		t.Fatalf("PAI should be absent when AssertedIdentityURI is empty")
	}
	if msg.GetHeader("Privacy") != "" {
		t.Fatalf("Privacy should be absent when PrivacyTokens is nil")
	}
}

// TestBuildINVITE_PAI_Privacy verifies that populating the RFC 3325 /
// RFC 3323 fields on inviteParams produces correctly-formatted headers
// on the outbound INVITE.
func TestBuildINVITE_PAI_Privacy(t *testing.T) {
	p := inviteParams{
		LocalIP:                     "127.0.0.1",
		SIPHost:                     "127.0.0.1",
		SIPPort:                     6050,
		RequestURI:                  "sip:bob@example.com",
		CallID:                      "test@127.0.0.1",
		FromTag:                     "abc",
		Branch:                      "branch1",
		CSeq:                        1,
		LocalRTPPort:                10000,
		SDPBody:                     "v=0\r\n",
		FromUser:                    "alice",
		AssertedIdentityURI:         "sip:+8613800138000@trust.example",
		AssertedIdentityDisplayName: "Customer Service",
		PrivacyTokens:               []string{"id"},
	}
	msg := buildINVITE(p)

	pai := msg.GetHeader("P-Asserted-Identity")
	want := `"Customer Service" <sip:+8613800138000@trust.example>`
	if pai != want {
		t.Fatalf("PAI = %q, want %q", pai, want)
	}
	if pr := msg.GetHeader("Privacy"); pr != "id" {
		t.Fatalf("Privacy = %q, want %q", pr, "id")
	}
}

// TestBuildINVITE_PAI_OmittedWhenURIEmpty ensures a non-nil but empty
// AssertedIdentityURI doesn't accidentally emit a malformed PAI header.
func TestBuildINVITE_PAI_OmittedWhenURIEmpty(t *testing.T) {
	p := inviteParams{
		LocalIP:             "127.0.0.1",
		SIPHost:             "127.0.0.1",
		SIPPort:             6050,
		RequestURI:          "sip:bob@example.com",
		CallID:              "test@127.0.0.1",
		FromTag:             "abc",
		Branch:              "branch1",
		CSeq:                1,
		LocalRTPPort:        10000,
		SDPBody:             "v=0\r\n",
		FromUser:            "alice",
		AssertedIdentityURI: "   ", // whitespace-only must be treated as absent
		PrivacyTokens:       []string{"id"},
	}
	msg := buildINVITE(p)
	if msg.GetHeader("P-Asserted-Identity") != "" {
		t.Fatalf("PAI should be absent for whitespace-only URI")
	}
	// Privacy: id without PAI is valid (RFC 3323 standalone "id" → request
	// anonymous; downstream can drop PAI it would otherwise *insert*).
	if pr := msg.GetHeader("Privacy"); pr != "id" {
		t.Fatalf("Privacy = %q, want %q", pr, "id")
	}
}

// TestBuildINVITE_HistoryInfoAndDiversion verifies that the RFC 7044 /
// RFC 5806 chains populated on inviteParams reach the wire as
// History-Info and Diversion headers, in the order they appear in the
// chain. Critical for B2BUA transfer legs — downstream PBX rendering
// of "originally called T, now ringing here" depends on this.
func TestBuildINVITE_HistoryInfoAndDiversion(t *testing.T) {
	p := inviteParams{
		LocalIP:      "127.0.0.1",
		SIPHost:      "127.0.0.1",
		SIPPort:      6050,
		RequestURI:   "sip:agent42@pool.example",
		CallID:       "test@127.0.0.1",
		FromTag:      "abc",
		Branch:       "branch1",
		CSeq:         1,
		LocalRTPPort: 10000,
		SDPBody:      "v=0\r\n",
		FromUser:     "trunk-cli",
		HistoryInfo: []historyinfo.Entry{
			{URI: "sip:+8613800138000@trunk.example", Index: "1"},
			{URI: "sip:agent42@pool.example", Index: "2", ReasonHeader: `SIP;cause=302;text="Transfer"`},
		},
		Diversion: []historyinfo.Diversion{
			{URI: "sip:+8613800138000@trunk.example", Reason: historyinfo.DiversionUnconditional, Counter: 1},
		},
	}
	msg := buildINVITE(p)

	hi := msg.GetHeader("History-Info")
	if !strings.Contains(hi, "<sip:+8613800138000@trunk.example>;index=1") {
		t.Errorf("History-Info missing root entry: %q", hi)
	}
	if !strings.Contains(hi, "index=2") || !strings.Contains(hi, "Reason=SIP%3Bcause%3D302") {
		t.Errorf("History-Info missing percent-encoded reason on retarget entry: %q", hi)
	}

	dv := msg.GetHeader("Diversion")
	want := "<sip:+8613800138000@trunk.example>;reason=unconditional;counter=1"
	if dv != want {
		t.Errorf("Diversion = %q, want %q", dv, want)
	}
}

// TestBuildINVITE_HistoryInfoOmittedWhenEmpty: nil/empty chains must
// not emit a malformed empty header.
func TestBuildINVITE_HistoryInfoOmittedWhenEmpty(t *testing.T) {
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
	if h := msg.GetHeader("History-Info"); h != "" {
		t.Errorf("History-Info should be absent when chain empty; got %q", h)
	}
	if d := msg.GetHeader("Diversion"); d != "" {
		t.Errorf("Diversion should be absent when chain empty; got %q", d)
	}
}

// TestBuildINVITE_ViaTransport verifies the Via header transport
// token follows inviteParams.ViaTransport: UDP default for backward
// compat, TCP/TLS when explicitly set. This is what RFC 3261 §17.1.1.3
// pins responses to the right transaction layer.
func TestBuildINVITE_ViaTransport(t *testing.T) {
	base := inviteParams{
		LocalIP: "127.0.0.1", SIPHost: "127.0.0.1", SIPPort: 6050,
		RequestURI: "sip:bob@example.com", CallID: "c@h", FromTag: "t",
		Branch: "br", CSeq: 1, FromUser: "alice",
	}
	cases := map[Transport]string{
		TransportUnset: "SIP/2.0/UDP", // backward compat default
		TransportUDP:   "SIP/2.0/UDP",
		TransportTCP:   "SIP/2.0/TCP",
		TransportTLS:   "SIP/2.0/TLS",
	}
	for tr, wantToken := range cases {
		p := base
		p.ViaTransport = tr
		via := buildINVITE(p).GetHeader("Via")
		if !strings.HasPrefix(via, wantToken+" ") {
			t.Errorf("ViaTransport=%q: Via=%q, want prefix %q", tr, via, wantToken)
		}
	}
}
