// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestSIPFormatDisplayName_ASCII 验证 ASCII display name 走 quoted-string
// 路径，引号 / 反斜杠按 RFC 3261 §25.1 转义。
func TestSIPFormatDisplayName_ASCII(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"NiuNiu Tech", `"NiuNiu Tech"`},
		{`Say "hi"`, `"Say \"hi\""`},
		{`a\b`, `"a\\b"`},
		{"line1\r\nline2", `"line1  line2"`}, // CR/LF folded to spaces
		{"   ", ""},                          // pure whitespace → empty
	}
	for _, c := range cases {
		got := sipFormatDisplayName(c.in)
		if got != c.want {
			t.Errorf("in=%q got=%q want=%q", c.in, got, c.want)
		}
	}
}

// TestSIPFormatDisplayName_UTF8 验证含中文 / emoji 的 display name 走
// RFC 2047 MIME encoded-word 路径——国内运营商 SBC 普遍要求这种形式才能
// 把中文主叫名透传到被叫话机。
func TestSIPFormatDisplayName_UTF8(t *testing.T) {
	in := "牛牛科技无限公司"
	got := sipFormatDisplayName(in)
	if !strings.HasPrefix(got, "=?UTF-8?B?") || !strings.HasSuffix(got, "?=") {
		t.Fatalf("expected MIME encoded-word, got %q", got)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(got, "=?UTF-8?B?"), "?=")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if string(decoded) != in {
		t.Fatalf("round-trip mismatch: got %q want %q", string(decoded), in)
	}
}

// TestFormatOutboundFromHeader_UTF8DisplayName 把 UTF-8 display name 灌进
// 完整 From header，确认 SIP URI 与 ;tag= 拼接正确（既不会和 base64 串
// 混淆，也不会把整个 UTF-8 quoted-string 当作 URI 的一部分）。
func TestFormatOutboundFromHeader_UTF8DisplayName(t *testing.T) {
	got := formatOutboundFromHeader("牛牛科技无限公司", "1000", "10.0.0.1", 5060, "tag-abc")
	if !strings.HasPrefix(got, "=?UTF-8?B?") {
		t.Fatalf("From should start with MIME encoded-word, got: %s", got)
	}
	if !strings.Contains(got, "<sip:1000@10.0.0.1:5060>") {
		t.Fatalf("From missing URI: %s", got)
	}
	if !strings.HasSuffix(got, ";tag=tag-abc") {
		t.Fatalf("From missing tag: %s", got)
	}
}

// TestFormatOutboundFromHeader_EmptyDisplayName 显示名为空（含全空格）时
// From 不应出现 display-name token，直接 <sip:...>;tag=。
func TestFormatOutboundFromHeader_EmptyDisplayName(t *testing.T) {
	got := formatOutboundFromHeader("   ", "1000", "10.0.0.1", 5060, "tag-abc")
	if strings.HasPrefix(got, `"`) || strings.HasPrefix(got, "=?") {
		t.Fatalf("expected no display-name, got: %s", got)
	}
	if got != "<sip:1000@10.0.0.1:5060>;tag=tag-abc" {
		t.Fatalf("unexpected From: %s", got)
	}
}
