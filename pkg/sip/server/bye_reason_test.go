package server

// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"testing"

	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

func makeBYEWithReason(reasonHeader string) *stack.Message {
	msg := &stack.Message{
		IsRequest: true,
		Method:    "BYE",
	}
	if reasonHeader != "" {
		msg.SetHeader("Reason", reasonHeader)
	}
	return msg
}

func TestClassifyBYEReason_MissingHeader(t *testing.T) {
	rc, raw := classifyBYEReason(makeBYEWithReason(""))
	if rc != sipMetrics.ByeReasonNormal {
		t.Errorf("missing header → expected normal, got %q", rc)
	}
	if raw != "" {
		t.Errorf("missing header should yield empty text; got %q", raw)
	}
}

func TestClassifyBYEReason_Q850Cases(t *testing.T) {
	cases := []struct {
		header    string
		wantClass string
		wantText  string
	}{
		{`Q.850 ;cause=16 ;text="Normal call clearing"`, sipMetrics.ByeReasonNormal, "Normal call clearing"},
		{`Q.850 ;cause=17 ;text="User busy"`, sipMetrics.ByeReasonNormal, "User busy"},
		{`Q.850 ;cause=21 ;text="Call rejected"`, sipMetrics.ByeReasonUserHangup, "Call rejected"},
		{`Q.850 ;cause=102 ;text="Recovery on timer expiry"`, sipMetrics.ByeReasonTimerExpired, "Recovery on timer expiry"},
		{`Q.850 ;cause=34 ;text="No circuit"`, sipMetrics.ByeReasonError, "No circuit"},
		// Unknown Q.850 cause → normal (conservative; an unknown
		// code from a new peer shouldn't paint the dashboard red).
		{`Q.850 ;cause=999 ;text="weird"`, sipMetrics.ByeReasonNormal, "weird"},
	}
	for _, tc := range cases {
		rc, raw := classifyBYEReason(makeBYEWithReason(tc.header))
		if rc != tc.wantClass {
			t.Errorf("header=%q class=%q want %q", tc.header, rc, tc.wantClass)
		}
		if raw != tc.wantText {
			t.Errorf("header=%q text=%q want %q", tc.header, raw, tc.wantText)
		}
	}
}

func TestClassifyBYEReason_SIPCases(t *testing.T) {
	cases := []struct {
		header    string
		wantClass string
	}{
		{`SIP ;cause=200 ;text="OK"`, sipMetrics.ByeReasonNormal},
		{`SIP ;cause=408 ;text="Request Timeout"`, sipMetrics.ByeReasonTimerExpired},
		{`SIP ;cause=503 ;text="Service Unavailable"`, sipMetrics.ByeReasonError},
		{`SIP ;cause=486 ;text="Busy Here"`, sipMetrics.ByeReasonError},
	}
	for _, tc := range cases {
		rc, _ := classifyBYEReason(makeBYEWithReason(tc.header))
		if rc != tc.wantClass {
			t.Errorf("header=%q got=%q want=%q", tc.header, rc, tc.wantClass)
		}
	}
}

func TestParseRFC3326Reason_LenientFormat(t *testing.T) {
	// Param order shouldn't matter; whitespace tolerated; quotes optional.
	cases := []struct {
		header    string
		wantProto string
		wantCause int
		wantText  string
	}{
		{`Q.850;cause=16;text="Normal"`, "Q.850", 16, "Normal"},
		{`Q.850 ;text=NoQuotes ;cause=17`, "Q.850", 17, "NoQuotes"},
		{`SIP;cause=486`, "SIP", 486, ""},
		{`SIP`, "SIP", 0, ""},
		{`Q.850;cause="16"`, "Q.850", 16, ""}, // quoted cause (some impls)
	}
	for _, tc := range cases {
		p, c, txt := parseRFC3326Reason(tc.header)
		if p != tc.wantProto || c != tc.wantCause || txt != tc.wantText {
			t.Errorf("parse(%q) = (%q,%d,%q), want (%q,%d,%q)",
				tc.header, p, c, txt, tc.wantProto, tc.wantCause, tc.wantText)
		}
	}
}

func TestClassifyBYEReason_UnknownProtocolFallsBackToNormal(t *testing.T) {
	rc, _ := classifyBYEReason(makeBYEWithReason(`X-Vendor ;cause=42`))
	if rc != sipMetrics.ByeReasonNormal {
		t.Errorf("unknown protocol → expected normal, got %q", rc)
	}
}
