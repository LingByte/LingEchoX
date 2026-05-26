package conversation

import "testing"

func TestTransferConfirmRequired_Default(t *testing.T) {
	if got := TransferConfirmRequired(VoiceEnv{}); got != 3 {
		t.Fatalf("default want 3 got %d", got)
	}
}

func TestTransferConfirmRequired_Clamp(t *testing.T) {
	if got := TransferConfirmRequired(VoiceEnv{TransferConfirmCount: 99}); got != 10 {
		t.Fatalf("max clamp want 10 got %d", got)
	}
	if got := TransferConfirmRequired(VoiceEnv{TransferConfirmCount: 1}); got != 1 {
		t.Fatalf("min want 1 got %d", got)
	}
}

func TestRecordSIPTransferIntent_PerTurn(t *testing.T) {
	const id = "test-call-confirm"
	defer cleanupSIPTransferConfirm(id)

	if c := recordSIPTransferIntent(id, "你好"); c != 0 {
		t.Fatalf("non-transfer should be 0, got %d", c)
	}
	if c := recordSIPTransferIntent(id, "转人工"); c != 1 {
		t.Fatalf("first want 1 got %d", c)
	}
	// One breath with repeated phrase still counts as one turn.
	if c := recordSIPTransferIntent(id, "转人工转人工转人工"); c != 2 {
		t.Fatalf("same-turn repeat want 2 got %d", c)
	}
	if c := recordSIPTransferIntent(id, "转人工"); c != 3 {
		t.Fatalf("third want 3 got %d", c)
	}
}

func TestSipTransferMayExecute_DefaultTwo(t *testing.T) {
	const id = "test-call-exec-2"
	defer cleanupSIPTransferConfirm(id)

	recordSIPTransferIntent(id, "转人工")
	allowed, _ := sipTransferMayExecute(id, 2)
	if allowed {
		t.Fatal("should block at count 1 when required=2")
	}
	recordSIPTransferIntent(id, "转人工")
	allowed, count := sipTransferMayExecute(id, 2)
	if !allowed || count != 2 {
		t.Fatalf("want allowed at 2, got allowed=%v count=%d", allowed, count)
	}
}

func TestSipTransferMayExecute(t *testing.T) {
	const id = "test-call-exec"
	defer cleanupSIPTransferConfirm(id)

	recordSIPTransferIntent(id, "转人工")
	allowed, _ := sipTransferMayExecute(id, 3)
	if allowed {
		t.Fatal("should block at count 1")
	}
	recordSIPTransferIntent(id, "转人工")
	recordSIPTransferIntent(id, "转人工")
	allowed, count := sipTransferMayExecute(id, 3)
	if !allowed || count != 3 {
		t.Fatalf("want allowed at 3, got allowed=%v count=%d", allowed, count)
	}
}

func TestSipTransferMayExecute_ImmediateWhenOne(t *testing.T) {
	const id = "test-call-immediate"
	defer cleanupSIPTransferConfirm(id)

	allowed, _ := sipTransferMayExecute(id, 1)
	if !allowed {
		t.Fatal("required=1 should allow without any intent")
	}
}
