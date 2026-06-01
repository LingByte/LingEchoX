package conversation

import (
	"testing"
)

func TestInboundCallerStillPresentForTransfer(t *testing.T) {
	const inbound = "in-hungup"
	markTransferCallerHungUp(inbound)
	t.Cleanup(func() { clearTransferCallerHungUp(inbound) })

	if inboundCallerStillPresentForTransfer(inbound) {
		t.Fatal("expected caller marked hung up")
	}
}

func TestScheduleTransferRetrySkipsWhenCallerHungUp(t *testing.T) {
	const inbound = "in-retry-skip"
	markTransferCallerHungUp(inbound)
	t.Cleanup(func() {
		clearTransferCallerHungUp(inbound)
		transferStarted.Delete(inbound)
	})

	scheduleTransferRetryToNextAgent(inbound, nil)

	if _, ok := transferStarted.Load(inbound); ok {
		t.Fatal("transfer should not start when caller already hung up")
	}
}

func TestTransferFailureAgentRejected(t *testing.T) {
	if !transferFailureAgentRejected(603, "Decline") {
		t.Fatal("603 Decline should be agent reject")
	}
	if !transferFailureAgentRejected(486, "Busy Here") {
		t.Fatal("486 Busy Here should be agent reject")
	}
	if transferFailureAgentRejected(408, "transfer_invite_timeout") {
		t.Fatal("408 timeout should retry next agent, not agent reject")
	}
}

func TestAbortTransferOnAgentRejectBlocksRetry(t *testing.T) {
	const inbound = "in-agent-reject"
	transferStarted.Store(inbound, true)
	t.Cleanup(func() {
		clearTransferCallerHungUp(inbound)
		transferStarted.Delete(inbound)
	})

	AbortTransferOnAgentReject(inbound)
	scheduleTransferRetryToNextAgent(inbound, nil)

	if _, ok := transferStarted.Load(inbound); ok {
		t.Fatal("retry should not restart transfer after agent reject abort")
	}
}
