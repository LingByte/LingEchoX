// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/voice/cdr"
)

// newTestCDRWriter spins up a Writer rooted at a t.TempDir so each
// test gets isolated artifact paths.
func newTestCDRWriter(t *testing.T) (*cdr.Writer, string) {
	t.Helper()
	dir := t.TempDir()
	w := cdr.NewWriter(cdr.Config{
		Dir:            dir,
		BaseName:       "cdr",
		BufSize:        32,
		MaxFileBytes:   1 << 20,
		RotateInterval: time.Hour,
	})
	if err := w.Start(); err != nil {
		t.Fatalf("Writer.Start: %v", err)
	}
	t.Cleanup(func() { w.Stop() })
	return w, dir
}

// waitForCDRLines polls the live JSONL until at least `n` lines
// have been flushed, or the timeout expires. Returns the parsed
// records.
func waitForCDRLines(t *testing.T, dir string, n int, timeout time.Duration) []cdr.CallRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	path := filepath.Join(dir, "cdr.current.jsonl")
	for time.Now().Before(deadline) {
		recs := readCDRFile(t, path)
		if len(recs) >= n {
			return recs
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d CDR lines in %s", n, path)
	return nil
}

func readCDRFile(t *testing.T, path string) []cdr.CallRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []cdr.CallRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r cdr.CallRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("malformed CDR line: %v\n%s", err, sc.Text())
		}
		out = append(out, r)
	}
	return out
}

// TestEmitCDR_NoSinkIsNoOp confirms the default zero-config path
// doesn't allocate or hit the disk. A leg without SetCDRSink should
// produce zero records anywhere.
func TestEmitCDR_NoSinkIsNoOp(t *testing.T) {
	m := &Manager{}
	leg := &outLeg{m: m}
	leg.beginCDR()
	leg.cdrSetHangup("local", "normal")
	leg.emitCDR() // must not panic, must not allocate to disk
}

// TestEmitCDR_HappyPath_Local seeds a leg with the bare minimum
// state and confirms the record makes it onto disk with the right
// classification.
func TestEmitCDR_HappyPath_Local(t *testing.T) {
	w, dir := newTestCDRWriter(t)
	m := &Manager{}
	m.SetCDRSink(w)

	leg := &outLeg{m: m}
	leg.params.CallID = "call-happy-001"
	leg.req.Scenario = "outbound-test"
	leg.req.CorrelationID = "corr-001"

	leg.beginCDR()
	time.Sleep(10 * time.Millisecond) // make duration non-zero
	leg.cdrSetAnswered("PCMU")
	leg.cdrSetHangup("local", "normal")
	leg.emitCDR()

	recs := waitForCDRLines(t, dir, 1, 2*time.Second)
	got := recs[0]
	if got.CallID != "call-happy-001" {
		t.Errorf("CallID round-trip: %q", got.CallID)
	}
	if got.Transport != "sip" {
		t.Errorf("Transport: %q", got.Transport)
	}
	if got.Codec != "pcmu" {
		t.Errorf("Codec must be lower-cased: %q", got.Codec)
	}
	if got.EndStatus != "ok" {
		t.Errorf("EndStatus: %q", got.EndStatus)
	}
	if got.HangupBy != "local" {
		t.Errorf("HangupBy: %q", got.HangupBy)
	}
	if got.CorrelationID != "corr-001" {
		t.Errorf("CorrelationID: %q", got.CorrelationID)
	}
	if got.Scenario != "outbound-test" {
		t.Errorf("Scenario: %q", got.Scenario)
	}
	if got.DurationMs <= 0 {
		t.Errorf("DurationMs not computed: %d", got.DurationMs)
	}
	if got.SchemaVersion != cdr.CurrentSchemaVersion {
		t.Errorf("SchemaVersion: %d", got.SchemaVersion)
	}
}

// TestEmitCDR_ErrorBeforeAnswer captures the case "INVITE got 404,
// no media flow ever happened". EndStatus must reflect the signaling
// failure with the actual SIP code so dashboards can group reasons.
func TestEmitCDR_ErrorBeforeAnswer(t *testing.T) {
	w, dir := newTestCDRWriter(t)
	m := &Manager{}
	m.SetCDRSink(w)

	leg := &outLeg{m: m}
	leg.params.CallID = "call-err-001"

	leg.beginCDR()
	leg.cdrSetError(404, "Not Found")
	leg.emitCDR()

	recs := waitForCDRLines(t, dir, 1, 2*time.Second)
	got := recs[0]
	if got.EndStatus != "signaling-error" {
		t.Errorf("EndStatus: %q", got.EndStatus)
	}
	if got.SIPFinalCode != 404 {
		t.Errorf("SIPFinalCode: %d", got.SIPFinalCode)
	}
	if len(got.Errors) == 0 || got.Errors[0] != "Not Found" {
		t.Errorf("Errors not captured: %v", got.Errors)
	}
}

// TestCDRSetHangup_FirstWriteWins protects against the race where a
// remote BYE and our own local cleanup both try to label the leg.
// Whichever ran first should determine the hangup_by tag.
func TestCDRSetHangup_FirstWriteWins(t *testing.T) {
	leg := &outLeg{}
	leg.cdrSetHangup("remote", "normal")
	leg.cdrSetHangup("local", "normal") // should NOT overwrite
	snap := leg.cdr.snapshot()
	if snap.hangupBy != "remote" {
		t.Errorf("first hangup wins; got %q", snap.hangupBy)
	}
}

// TestSetCDRSink_NilDisablesEmit ensures we can wire / un-wire the
// sink at runtime without panic and without leaking emit calls
// across a nil swap.
func TestSetCDRSink_NilDisablesEmit(t *testing.T) {
	w, dir := newTestCDRWriter(t)
	m := &Manager{}
	m.SetCDRSink(w)
	m.SetCDRSink(nil) // disable

	leg := &outLeg{m: m}
	leg.params.CallID = "call-disabled"
	leg.beginCDR()
	leg.cdrSetHangup("local", "normal")
	leg.emitCDR()

	// Give the (possibly already running) drain a beat to flush
	// anything queued before the nil. Then assert nothing landed.
	time.Sleep(50 * time.Millisecond)
	path := filepath.Join(dir, "cdr.current.jsonl")
	recs := readCDRFile(t, path)
	if len(recs) != 0 {
		t.Errorf("nil sink must suppress emits; got %d records", len(recs))
	}
}
