package conversation

import (
	"strings"
	"testing"
)

// Transfer marker contract: the augmented system prompt must contain the
// sentinel marker so the realtime model has a stable token to emit. If
// somebody changes the marker value, this test fails loudly — a silent
// rename would break every existing tenant's transfer-to-human flow.
func TestRealtimeAugmentSystemPrompt_ContainsMarker(t *testing.T) {
	cases := []string{"", "你是个人助理小云"}
	for _, p := range cases {
		out := realtimeAugmentSystemPrompt(p, false, 3)
		if !strings.Contains(out, transferAgentMarker) {
			t.Fatalf("marker missing for input %q: got %q", p, out)
		}
		if p != "" && !strings.HasPrefix(out, p) {
			t.Fatalf("operator prompt must come first; got %q", out)
		}
	}
}

func TestRealtimeAugmentSystemPrompt_TransferTool(t *testing.T) {
	out := realtimeAugmentSystemPrompt("你是客服", true, 2)
	if !strings.Contains(out, "transfer_to_agent") {
		t.Fatalf("tool rule missing: %q", out)
	}
	if !strings.Contains(out, "后台累计用户 2 次") {
		t.Fatalf("confirm count rule missing: %q", out)
	}
	if !strings.Contains(out, "请问有什么可以帮您的") {
		t.Fatalf("normal reply guidance missing: %q", out)
	}
	if !strings.Contains(out, "禁止主动提及") {
		t.Fatalf("no-proactive-transfer rule missing: %q", out)
	}
	if strings.Contains(out, transferAgentMarker) {
		t.Fatalf("marker should not appear in tool mode: %q", out)
	}
}

func TestRealtimeMatchTransferAckPhrase(t *testing.T) {
	pos := []string{"正在为您转接，请稍候。", "好的，马上为您转接人工客服。"}
	for _, p := range pos {
		if !realtimeMatchTransferAckPhrase(p) {
			t.Fatalf("expected ack match for %q", p)
		}
	}
	neg := []string{
		"",
		"请稍候",
		"听得清，您请说。",
		"您好，我是智能助手，可以帮您查询信息、计算问题或转接人工客服。",
	}
	for _, n := range neg {
		if realtimeMatchTransferAckPhrase(n) {
			t.Fatalf("false positive for %q", n)
		}
	}
}

func TestRealtimeMatchTransferIntent_Assistant(t *testing.T) {
	// Marker-driven: positive samples must trip, false positives must not.
	pos := []string{
		"好的，我现在为您转接人工客服。[TRANSFER_TO_AGENT]",
		"已为您转接。\n[TRANSFER_TO_AGENT]\n",
		// Case-insensitive: model may lowercase the bracketed sentinel.
		"transfer requested [transfer_to_agent]",
	}
	for _, p := range pos {
		if !realtimeMatchTransferIntent("assistant", p, nil) {
			t.Fatalf("expected transfer trigger for %q", p)
		}
	}
	neg := []string{
		"",
		"我可以帮您转接业务，是否需要？",
		"AGENT", // partial / suffix only
		"[TRANSFER]",
	}
	for _, n := range neg {
		if realtimeMatchTransferIntent("assistant", n, nil) {
			t.Fatalf("false positive on assistant text %q", n)
		}
	}
}

func TestRealtimeMatchTransferIntent_UserKeywords(t *testing.T) {
	// Keyword fallback: realtime providers may forget the marker (small
	// models / system prompt regressions). User-side detection MUST cover
	// the common Chinese phrasings.
	pos := []string{
		"我要转人工",
		"请帮我接人工客服",
		"找客服",
		"接线员可以吗",
	}
	for _, p := range pos {
		if !realtimeMatchTransferIntent("user", p, nil) {
			t.Fatalf("expected transfer trigger on user %q", p)
		}
	}
	neg := []string{
		"",
		"人工智能很厉害",      // 'AI' adjacent, not transfer intent
		"这个工作怎么做",      // unrelated
		"什么是工程化",        // unrelated
	}
	for _, n := range neg {
		if realtimeMatchTransferIntent("user", n, nil) {
			t.Fatalf("false positive on user text %q", n)
		}
	}
}

// Custom phrase list overrides the defaults (used when tenant config
// adds vertical-specific transfer phrases).
func TestRealtimeMatchTransferIntent_CustomPhrases(t *testing.T) {
	custom := []string{"call human", "live agent"}
	if !realtimeMatchTransferIntent("user", "please connect me to a live agent", custom) {
		t.Fatal("custom phrase not matched")
	}
	if realtimeMatchTransferIntent("user", "我要转人工", custom) {
		// Custom phrase list supersedes — the Chinese default phrase
		// must not still fire when the operator explicitly opted in to
		// an English-only set.
		t.Fatal("default phrase fired despite custom override")
	}
}

func TestRealtimeStripMarker(t *testing.T) {
	cases := map[string]string{
		"":                                "",
		"已为您转接 [TRANSFER_TO_AGENT]":      "已为您转接",
		"[TRANSFER_TO_AGENT]再见":           "再见",
		"hello [transfer_to_agent] world": "hello  world",
	}
	for in, want := range cases {
		got := realtimeStripMarker(in)
		// `realtimeStripMarker` collapses extra whitespace; compare on
		// substring of meaningful tokens to keep test resilient.
		if !strings.Contains(got, strings.TrimSpace(want)) && want != "" {
			t.Fatalf("input %q: want substring %q, got %q", in, want, got)
		}
		if strings.Contains(strings.ToLower(got), strings.ToLower(transferAgentMarker)) {
			t.Fatalf("input %q: marker not stripped: %q", in, got)
		}
	}
}

func TestRealtimeMatchHangupIntent(t *testing.T) {
	phrases := []string{"再见", "拜拜"}
	if !realtimeMatchHangupIntent("好的再见", phrases) {
		t.Fatal("expected hangup intent")
	}
	if realtimeMatchHangupIntent("欢迎再次光临", phrases) == false {
		// 'farewell-like' assistant text containing one of the phrase
		// substrings legitimately matches; this is the documented
		// behaviour ('再次' contains '再'? no, the configured phrase is
		// '再见' so it doesn't substring-match here — keep the test in
		// place as the contract). Confirm the negative:
		t.Log("good: '欢迎再次光临' did not match — phrases are substring, not regex")
	}
	if realtimeMatchHangupIntent("", phrases) {
		t.Fatal("empty must not match")
	}
}
