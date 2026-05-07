package persist

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"
)

func turnsBytesFromUpdate(v any) []byte {
	switch x := v.(type) {
	case []byte:
		return x
	case datatypes.JSON:
		return []byte(x)
	case json.RawMessage:
		return []byte(x)
	default:
		return nil
	}
}

func TestMergeSIPCall(t *testing.T) {
	dst := &SIPCall{CallID: "c1", State: SIPCallStateRinging}
	patch := &SIPCall{
		CallID:         "c1",
		State:          SIPCallStateEstablished,
		FromHeader:     "f",
		Codec:          "pcmu",
		Direction:      DirectionInbound,
		HadSIPTransfer: true,
	}
	MergeSIPCall(dst, patch)
	if dst.State != SIPCallStateEstablished || dst.FromHeader != "f" || dst.Codec != "pcmu" || !dst.HadSIPTransfer {
		t.Fatalf("merged: %+v", dst)
	}
	MergeSIPCall(dst, &SIPCall{CallID: "c1"})
	if dst.FromHeader != "f" {
		t.Fatal("empty patch cleared fields")
	}
}

func TestSIPCallAppendTurnUpdateMap(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	t1 := SIPCallDialogTurn{ASRText: "hi", LLMText: "hello", At: now}
	row := SIPCall{CallID: "x"}
	upd, n, err := SIPCallAppendTurnUpdateMap(row, t1, now)
	if err != nil || n != 1 {
		t.Fatalf("append empty: err=%v n=%d", err, n)
	}
	if upd["turn_count"] != 1 {
		t.Fatal(upd["turn_count"])
	}
	raw := turnsBytesFromUpdate(upd["turns"])
	var decoded []SIPCallDialogTurn
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded) != 1 || decoded[0].ASRText != "hi" {
		t.Fatalf("turns json: %v err=%v", decoded, err)
	}

	row2 := SIPCall{CallID: "x", Turns: datatypes.JSON(raw), TurnCount: 1, FirstTurnAt: &now}
	t2 := SIPCallDialogTurn{ASRText: "bye", LLMText: "ok", At: now}
	upd2, n2, err := SIPCallAppendTurnUpdateMap(row2, t2, now.Add(time.Minute))
	if err != nil || n2 != 2 {
		t.Fatalf("append second: err=%v n=%d", err, n2)
	}
	raw2 := turnsBytesFromUpdate(upd2["turns"])
	var decoded2 []SIPCallDialogTurn
	if err := json.Unmarshal(raw2, &decoded2); err != nil || len(decoded2) != 2 {
		t.Fatalf("two turns: %v err=%v", decoded2, err)
	}
	if _, ok := upd2["first_turn_at"]; ok {
		t.Fatal("first_turn_at should not overwrite when already set")
	}
}

func TestUnmarshalSIPCallTurns_InvalidJSON(t *testing.T) {
	_, err := UnmarshalSIPCallTurns(datatypes.JSON(`not-json`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSIPCallEndStatusForBye(t *testing.T) {
	if SIPCallEndStatusForBye("local", false, false) != SIPCallEndCompletedLocal {
		t.Fatal()
	}
	if SIPCallEndStatusForBye("remote", false, false) != SIPCallEndCompletedRemote {
		t.Fatal()
	}
	if SIPCallEndStatusForBye("local", true, false) != SIPCallEndAfterTransferLocal {
		t.Fatal()
	}
	if SIPCallEndStatusForBye("remote", false, true) != SIPCallEndAfterTransferRemote {
		t.Fatal()
	}
}

func TestSIPCallDurationSince(t *testing.T) {
	ack := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := ack.Add(90 * time.Second)
	if n := SIPCallDurationSince(&ack, nil, end); n != 90 {
		t.Fatalf("got %d", n)
	}
	inv := ack.Add(-time.Minute)
	if n := SIPCallDurationSince(nil, &inv, end); n != int(end.Sub(inv).Seconds()) {
		t.Fatalf("invite fallback: %d", n)
	}
}

func TestRedactSIPCallForAPI(t *testing.T) {
	c := &SIPCall{
		FromHeader:     `"Alice" <sip:1001@10.0.0.5>;tag=abc`,
		ToHeader:       `<sip:400@192.168.1.10>;tag=def`,
		CSeqInvite:     "1 INVITE",
		RemoteAddr:     "203.0.113.9:5060",
		RemoteRTPAddr:  "203.0.113.9:12000",
		LocalRTPAddr:   "10.0.0.1:8000",
		FailureReason:  "timeout to 198.51.100.2:5060",
		FromNumber:     "1001",
		ToNumber:       "400",
	}
	RedactSIPCallForAPI(c)
	if c.FromHeader != "" || c.ToHeader != "" || c.CSeqInvite != "" || c.RemoteAddr != "" || c.RemoteRTPAddr != "" || c.LocalRTPAddr != "" {
		t.Fatalf("expected cleared topology/raw headers, got %#v", c)
	}
	if c.FromNumber != "1001" || c.ToNumber != "400" {
		t.Fatal("numbers should remain")
	}
	if !strings.Contains(c.FailureReason, "[redacted]") || strings.Contains(c.FailureReason, "198.51.100.2") {
		t.Fatalf("failure reason not redacted: %q", c.FailureReason)
	}
}

func TestComputeCallDurationSec_Enrich(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(125 * time.Second)
	c := &SIPCall{
		AckAt:       &start,
		EndedAt:     &end,
		DurationSec: 0,
	}
	if n := ComputeCallDurationSec(c); n != 125 {
		t.Fatalf("computed %d", n)
	}
	c2 := &SIPCall{TurnCount: 0, Turns: datatypes.JSON(`[{"asrText":"a","llmText":"b","at":"2026-01-01T00:00:00Z"}]`)}
	EnrichSIPCallResponse(c2)
	if c2.TurnCount != 1 {
		t.Fatalf("derive turns: %d", c2.TurnCount)
	}
	c3 := &SIPCall{EndStatus: "", State: SIPCallStateEnded}
	EnrichSIPCallResponse(c3)
	if c3.EndStatus != SIPCallEndUnknown {
		t.Fatal(c3.EndStatus)
	}
}

func TestApplyRTPMediaToSIPCall(t *testing.T) {
	var c SIPCall
	ApplyRTPMediaToSIPCall(&c, "192.0.2.1", 1234, "10.0.0.1", 5678, "PCMU", 0, 8000)
	if c.RemoteRTPAddr != "192.0.2.1:1234" || c.LocalRTPAddr != "10.0.0.1:5678" {
		t.Fatalf("rtp %q %q", c.RemoteRTPAddr, c.LocalRTPAddr)
	}
	if c.Codec != "pcmu" || c.ClockRate != 8000 {
		t.Fatal(c.Codec, c.ClockRate)
	}
}

func TestNewSIPCallRinging_DefaultDirection(t *testing.T) {
	now := time.Now()
	c := NewSIPCallRinging("id", "", "", "", "", "", "", "", 0, "", 0, now, 0, 0)
	if c.Direction != DirectionInbound {
		t.Fatal(c.Direction)
	}
}
