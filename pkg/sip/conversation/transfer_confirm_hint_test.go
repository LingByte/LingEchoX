package conversation

import (
	"strings"
	"testing"
)

func TestTransferConfirmSessionHint_NotReady(t *testing.T) {
	const id = "hint-not-ready"
	defer cleanupSIPTransferConfirm(id)
	recordSIPTransferIntent(id, "转人工")
	h := transferConfirmSessionHint(id, 3)
	if strings.Contains(h, "禁止说") == false {
		t.Fatalf("expected forbid transfer promise: %q", h)
	}
	if !strings.Contains(h, "本轮请仅对用户说") {
		t.Fatalf("expected explicit say-this line: %q", h)
	}
	if strings.Contains(h, "再说一次") == false {
		t.Fatalf("expected guide phrase: %q", h)
	}
}

func TestMergeRealtimeInstructions(t *testing.T) {
	got := mergeRealtimeInstructions("base", "hint")
	if !strings.HasPrefix(got, "base") || !strings.Contains(got, "hint") {
		t.Fatalf("merge failed: %q", got)
	}
}
