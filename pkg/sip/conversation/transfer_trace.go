package conversation

import (
	"strings"
	"sync"
)

const (
	TransferTraceNoAnswer  = "no_answer"
	TransferTraceAnswered  = "answered"
	TransferTraceRejected  = "rejected"
)

// SIPCallTransferTraceEntry is one ACD routing step persisted on sip_calls.transfer_trace_json.
type SIPCallTransferTraceEntry struct {
	ACDTargetID uint   `json:"acdTargetId"`
	Outcome     string `json:"outcome"`
}

var (
	transferTraceMu        sync.Mutex
	transferTraceByInbound = map[string][]SIPCallTransferTraceEntry{}
)

func recordTransferTraceOutcome(inbound string, acdID uint, outcome string) {
	inbound = strings.TrimSpace(inbound)
	outcome = strings.TrimSpace(outcome)
	if inbound == "" || acdID == 0 || outcome == "" {
		return
	}
	transferTraceMu.Lock()
	defer transferTraceMu.Unlock()
	trace := transferTraceByInbound[inbound]
	if len(trace) > 0 {
		last := trace[len(trace)-1]
		if last.ACDTargetID == acdID && last.Outcome == outcome {
			return
		}
	}
	transferTraceByInbound[inbound] = append(trace, SIPCallTransferTraceEntry{
		ACDTargetID: acdID,
		Outcome:     outcome,
	})
}

// RecordTransferNoAnswer appends a no-answer / timeout step for an ACD seat during transfer retries.
func RecordTransferNoAnswer(inbound string, acdID uint) {
	recordTransferTraceOutcome(inbound, acdID, TransferTraceNoAnswer)
}

// RecordTransferAnswered appends the seat that picked up (SIP bridge or Web seat join).
func RecordTransferAnswered(inbound string, acdID uint) {
	recordTransferTraceOutcome(inbound, acdID, TransferTraceAnswered)
}

// RecordTransferRejected appends an explicit agent reject (486/603 etc.).
func RecordTransferRejected(inbound string, acdID uint) {
	recordTransferTraceOutcome(inbound, acdID, TransferTraceRejected)
}

// TakeInboundTransferTrace returns the per-call transfer trace and clears it (OnBye persist).
func TakeInboundTransferTrace(inbound string) []SIPCallTransferTraceEntry {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return nil
	}
	transferTraceMu.Lock()
	defer transferTraceMu.Unlock()
	trace := transferTraceByInbound[inbound]
	if len(trace) == 0 {
		delete(transferTraceByInbound, inbound)
		return nil
	}
	out := make([]SIPCallTransferTraceEntry, len(trace))
	copy(out, trace)
	delete(transferTraceByInbound, inbound)
	return out
}

func recordTransferNoAnswerForCurrentTarget(inbound string) {
	inbound = strings.TrimSpace(inbound)
	if inbound == "" {
		return
	}
	if id, ok := PeekInboundTransferACDTargetID(inbound); ok && id > 0 {
		RecordTransferNoAnswer(inbound, id)
	}
}
