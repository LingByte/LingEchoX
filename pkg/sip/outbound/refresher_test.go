// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import (
	"strings"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/session_timer"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

func makeRefreshLeg() *outLeg {
	return &outLeg{
		params: inviteParams{
			CallID:     "leg-1@example.com",
			FromUser:   "alice",
			FromTag:    "a-tag",
			SIPHost:    "127.0.0.1",
			SIPPort:    5060,
			RequestURI: "sip:bob@127.0.0.1",
			CSeq:       1,
		},
		byeToHeader:   "<sip:bob@127.0.0.1>;tag=b-tag",
		byeRequestURI: "sip:bob@127.0.0.1",
		byeCSeqNext:   2,
	}
}

func TestBuildUPDATE_HeadersAndShape(t *testing.T) {
	leg := makeRefreshLeg()
	msg := buildUPDATE(leg.params, leg.byeToHeader, leg.byeRequestURI,
		leg.byeCSeqNext, "deadbeef", 1800, 90)
	if !msg.IsRequest || msg.Method != stack.MethodUpdate {
		t.Fatalf("not an UPDATE request: %+v", msg)
	}
	if got := msg.GetHeader("CSeq"); got != "2 UPDATE" {
		t.Errorf("CSeq=%q want %q", got, "2 UPDATE")
	}
	if got := msg.GetHeader("To"); got != leg.byeToHeader {
		t.Errorf("To header should echo 200 OK To (with remote tag): %q", got)
	}
	se := msg.GetHeader("Session-Expires")
	if !strings.Contains(se, "1800") || !strings.Contains(se, "refresher=uac") {
		t.Errorf("Session-Expires=%q must carry SE + refresher=uac", se)
	}
	if got := msg.GetHeader("Min-SE"); got != "90" {
		t.Errorf("Min-SE=%q want 90", got)
	}
	if !strings.Contains(msg.GetHeader("Supported"), "timer") {
		t.Errorf("Supported must include 'timer': %q", msg.GetHeader("Supported"))
	}
	if msg.GetHeader("Content-Length") != "0" {
		t.Errorf("UPDATE for timer refresh MUST be body-less")
	}
	if msg.Body != "" {
		t.Errorf("body must be empty, got: %q", msg.Body)
	}
}

func TestStartRefresherIfUAC_OnlyArmsWhenAssignedUAC(t *testing.T) {
	cases := []struct {
		name      string
		se        int
		role      session_timer.Refresher
		wantArmed bool
	}{
		{"refresher=uac arms", 1800, session_timer.RefresherUAC, true},
		{"refresher=uas no-op", 1800, session_timer.RefresherUAS, false},
		{"unset no-op", 1800, session_timer.RefresherUnset, false},
		{"sub-90s SE rejected", 30, session_timer.RefresherUAC, false},
		{"zero SE no-op", 0, session_timer.RefresherUAC, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			leg := makeRefreshLeg()
			leg.startRefresherIfUAC(tc.se, tc.role)
			leg.refreshMu.Lock()
			armed := leg.refresher != nil
			r := leg.refresher
			leg.refreshMu.Unlock()
			if armed != tc.wantArmed {
				t.Fatalf("armed=%v want=%v", armed, tc.wantArmed)
			}
			if r != nil {
				r.stop() // tear down the goroutine
			}
		})
	}
}

func TestStartRefresherIfUAC_IsIdempotent(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(1800, session_timer.RefresherUAC)
	first := leg.refresher
	leg.startRefresherIfUAC(900, session_timer.RefresherUAC)
	second := leg.refresher
	if first == nil || first != second {
		t.Errorf("second call should not replace the first refresher; first=%p second=%p", first, second)
	}
	if first != nil {
		first.stop()
	}
}

func TestHandleUPDATEResponse_200OK_AdoptsShorterSE(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(1800, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	resp := &stack.Message{IsRequest: false, StatusCode: 200, StatusText: "OK"}
	resp.SetHeader("Session-Expires", "900;refresher=uac")
	if !r.handleUPDATEResponse(resp) {
		t.Fatal("200 OK should keep refresher armed")
	}
	r.mu.Lock()
	got := r.se
	r.mu.Unlock()
	if got != 900 {
		t.Errorf("SE should be adopted-down to 900, got %d", got)
	}
}

func TestHandleUPDATEResponse_200OK_LongerSEIgnored(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(900, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	resp := &stack.Message{IsRequest: false, StatusCode: 200, StatusText: "OK"}
	resp.SetHeader("Session-Expires", "1800;refresher=uac")
	r.handleUPDATEResponse(resp)
	r.mu.Lock()
	got := r.se
	r.mu.Unlock()
	if got != 900 {
		// RFC 4028 §7.1: response SE MUST be ≤ request SE; if peer
		// breaks that we keep our smaller value.
		t.Errorf("SE should NOT be lengthened by 200 OK; got %d want 900", got)
	}
}

func TestHandleUPDATEResponse_200OK_RoleSwapToUASStopsRefresher(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(1800, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	resp := &stack.Message{IsRequest: false, StatusCode: 200, StatusText: "OK"}
	resp.SetHeader("Session-Expires", "1800;refresher=uas")
	if r.handleUPDATEResponse(resp) {
		t.Fatal("role swap to uas MUST stop the refresher")
	}
}

func TestHandleUPDATEResponse_422_BumpsSEAndRetries(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(120, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	resp := &stack.Message{IsRequest: false, StatusCode: 422, StatusText: "Session Interval Too Small"}
	resp.SetHeader("Min-SE", "1800")
	if !r.handleUPDATEResponse(resp) {
		t.Fatal("recoverable 422 should keep refresher armed")
	}
	r.mu.Lock()
	gotSE := r.se
	gotMin := r.minSE
	retried := r.retried422
	r.mu.Unlock()
	if gotSE < 1800 {
		t.Errorf("SE should be bumped to peer Min-SE; got %d", gotSE)
	}
	if gotMin < 1800 {
		t.Errorf("minSE should be raised; got %d", gotMin)
	}
	if !retried {
		t.Errorf("retried422 flag must be set")
	}
}

func TestHandleUPDATEResponse_422_SecondTimeGivesUp(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(120, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	resp := &stack.Message{IsRequest: false, StatusCode: 422, StatusText: "Session Interval Too Small"}
	resp.SetHeader("Min-SE", "1800")
	r.handleUPDATEResponse(resp) // first 422 → bump + retry
	if r.handleUPDATEResponse(resp) {
		t.Fatal("second 422 must stop the refresher to break a 422 loop")
	}
}

func TestHandleUPDATEResponse_422_NoMinSEGivesUp(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(120, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	// Buggy peer returns 422 without Min-SE — we can't compute a new
	// floor, so stop rather than guess.
	resp := &stack.Message{IsRequest: false, StatusCode: 422}
	if r.handleUPDATEResponse(resp) {
		t.Fatal("422 without Min-SE must stop refresher (we can't pick a sane new SE)")
	}
}

func TestHandleUPDATEResponse_481StopsRefresher(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(1800, session_timer.RefresherUAC)
	r := leg.refresher
	defer r.stop()

	resp := &stack.Message{IsRequest: false, StatusCode: 481, StatusText: "Call/Transaction Does Not Exist"}
	if r.handleUPDATEResponse(resp) {
		t.Fatal("481 means dialog is gone — refresher must stop")
	}
}

func TestHandleUPDATEResponse_OtherErrorsKeepArmed(t *testing.T) {
	// Transient SBC errors (408 / 500 / 503) shouldn't kill keepalive.
	for _, code := range []int{408, 500, 503} {
		leg := makeRefreshLeg()
		leg.startRefresherIfUAC(1800, session_timer.RefresherUAC)
		r := leg.refresher
		resp := &stack.Message{IsRequest: false, StatusCode: code, StatusText: "transient"}
		if !r.handleUPDATEResponse(resp) {
			t.Errorf("status %d should NOT tear down the refresher", code)
		}
		r.stop()
	}
}

func TestStopRefresher_IsIdempotent(t *testing.T) {
	leg := makeRefreshLeg()
	leg.startRefresherIfUAC(1800, session_timer.RefresherUAC)
	leg.stopRefresher()
	leg.stopRefresher() // must not panic on double-close
	if leg.refresher != nil {
		t.Error("refresher should be cleared after stop")
	}
}
