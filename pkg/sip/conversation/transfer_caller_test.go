// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package conversation

import "testing"

// TestParseSIPUserPart 覆盖入站 From header 的常见三种形态，确保转接
// display-name override 能拿到正确的主叫手机号。
func TestParseSIPUserPart(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"name-addr with display", `"张三" <sip:13800138000@10.0.4.12:5060>;tag=abc`, "13800138000"},
		{"name-addr no display", `<sip:13800138000@10.0.4.12>;tag=xyz`, "13800138000"},
		{"bare sip URI", `sip:13800138000@gw.example`, "13800138000"},
		{"sips scheme", `<sips:agent42@pbx.example.com:5061;transport=tls>`, "agent42"},
		{"user with params", `<sip:13800138000;npi=4@gw>;tag=t`, "13800138000"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"anonymous", `<sip:anonymous@anonymous.invalid>`, "anonymous"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseSIPUserPart(c.in)
			if got != c.want {
				t.Errorf("in=%q got=%q want=%q", c.in, got, c.want)
			}
		})
	}
}

// TestExtractInboundCallerNumber_NilSafe 验证查不到 inbound session 或
// session 上没有 From header 时安全返回 ""，转接流程不会因此崩溃，只是
// 维持中继默认 display-name 不动。
func TestExtractInboundCallerNumber_NilSafe(t *testing.T) {
	// 没有 SetInboundSessionLookup wiring 时 lookupInboundSession 返回 nil。
	if got := extractInboundCallerNumber("nonexistent-call-id"); got != "" {
		t.Fatalf("expected empty for missing inbound, got %q", got)
	}
	if got := extractInboundCallerNumber(""); got != "" {
		t.Fatalf("expected empty for blank call-id, got %q", got)
	}
}
