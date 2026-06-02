package conversation

import "testing"

func TestTransferTraceLifecycle(t *testing.T) {
	const inbound = "in-trace-1"
	t.Cleanup(func() {
		TakeInboundTransferTrace(inbound)
	})

	RecordTransferNoAnswer(inbound, 10)
	RecordTransferNoAnswer(inbound, 10)
	RecordTransferAnswered(inbound, 20)

	trace := TakeInboundTransferTrace(inbound)
	if len(trace) != 2 {
		t.Fatalf("len=%d", len(trace))
	}
	if trace[0].ACDTargetID != 10 || trace[0].Outcome != TransferTraceNoAnswer {
		t.Fatalf("first=%#v", trace[0])
	}
	if trace[1].ACDTargetID != 20 || trace[1].Outcome != TransferTraceAnswered {
		t.Fatalf("second=%#v", trace[1])
	}
	if again := TakeInboundTransferTrace(inbound); again != nil {
		t.Fatalf("expected cleared trace")
	}
}

func TestTransferTraceNoAnswerAfterAnsweredBlocked(t *testing.T) {
	const inbound = "in-trace-answered-then-noanswer"
	const acdID uint = 15780195150487093760
	t.Cleanup(func() {
		TakeInboundTransferTrace(inbound)
	})

	RecordTransferAnswered(inbound, acdID)
	RecordTransferNoAnswer(inbound, acdID)

	trace := TakeInboundTransferTrace(inbound)
	if len(trace) != 1 {
		t.Fatalf("len=%d trace=%#v", len(trace), trace)
	}
	if trace[0].Outcome != TransferTraceAnswered || trace[0].ACDTargetID != acdID {
		t.Fatalf("entry=%#v", trace[0])
	}
}
