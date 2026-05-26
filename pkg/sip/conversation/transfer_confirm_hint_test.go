package conversation

import (
	"strings"
	"testing"
)

func TestTransferConfirmSessionHint_NotReady(t *testing.T) {
	const id = "hint-not-ready"
	defer cleanupSIPTransferConfirm(id)
	recordSIPTransferIntent(id, "转人工")
	h := transferConfirmSessionHint(id, 2)
	if !strings.Contains(h, "严禁") && !strings.Contains(h, "禁止") {
		t.Fatalf("expected forbid transfer promise: %q", h)
	}
	if !strings.Contains(h, transferConfirmNormalReplyZH) {
		t.Fatalf("expected normal reply example: %q", h)
	}
	for _, bad := range []string{"请再确认一次", "若您仍需人工", "已记录您的需求，请继续说明是否要"} {
		if strings.Contains(h, bad) {
			t.Fatalf("must not use old coach-to-repeat script %q in: %s", bad, h)
		}
	}
}

func TestTransferConfirmSpokenZH_NotReady(t *testing.T) {
	got := transferConfirmSpokenZH(1, 3, 2)
	if got != transferConfirmNormalReplyZH {
		t.Fatalf("want %q got %q", transferConfirmNormalReplyZH, got)
	}
}

func TestTransferConfirmSpokenZH_Ready(t *testing.T) {
	got := transferConfirmSpokenZH(3, 3, 0)
	if got != transferConfirmExecuteReplyZH {
		t.Fatalf("want %q got %q", transferConfirmExecuteReplyZH, got)
	}
}

func TestMergeRealtimeInstructions(t *testing.T) {
	got := mergeRealtimeInstructions("base", "hint")
	if !strings.HasPrefix(got, "base") || !strings.Contains(got, "hint") {
		t.Fatalf("merge failed: %q", got)
	}
}
