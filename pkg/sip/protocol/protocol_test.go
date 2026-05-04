package protocol

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LingByte/SoulNexus/pkg/sip/stack"
)

func TestParse_SIPRequest(t *testing.T) {
	raw := strings.Join([]string{
		"INVITE sip:user@domain.com SIP/2.0",
		"Via: SIP/2.0/UDP a.example.com:6050;branch=z9hG4bK1",
		"Via: SIP/2.0/UDP b.example.com:6050;branch=z9hG4bK2",
		"Call-Id: abc123",
		"Content-Type: application/sdp",
		"Content-Length: 0",
		"",
		"",
	}, "\r\n")

	msg, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if msg == nil {
		t.Fatalf("expected non-nil message")
	}
	if !msg.IsRequest {
		t.Fatalf("expected request message")
	}
	if msg.Method != "INVITE" {
		t.Fatalf("method mismatch: got=%s", msg.Method)
	}
	if msg.RequestURI != "sip:user@domain.com" {
		t.Fatalf("uri mismatch: got=%s", msg.RequestURI)
	}
	if msg.GetHeader("Via") == "" {
		t.Fatalf("expected Via header")
	}
	if len(msg.GetHeaders("Via")) != 2 {
		t.Fatalf("expected 2 Via values, got=%d", len(msg.GetHeaders("Via")))
	}
	if msg.GetHeader("Call-ID") != "abc123" {
		t.Fatalf("Call-ID mismatch: got=%q", msg.GetHeader("Call-ID"))
	}
}

func TestParse_Invalid(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestIsSIPSignalingNoiseDatagram(t *testing.T) {
	if !stack.IsSignalingNoiseDatagram([]byte("\r\n\r\n")) {
		t.Fatal("expected CRLFCRLF keepalive")
	}
	if !stack.IsSignalingNoiseDatagram([]byte("\r\n")) {
		t.Fatal("expected CRLF")
	}
	if stack.IsSignalingNoiseDatagram([]byte("INVITE sip:a SIP/2.0\r\n")) {
		t.Fatal("invite is not noise")
	}
	if stack.IsSignalingNoiseDatagram(nil) || stack.IsSignalingNoiseDatagram([]byte{}) {
		t.Fatal("empty is not noise (handled elsewhere)")
	}
}

func TestServer_OnSIPResponse(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = s.Stop() }()

	var saw atomic.Bool
	s.OnSIPResponse = func(resp *Message, _ *net.UDPAddr) {
		if resp != nil && resp.StatusCode == 200 {
			saw.Store(true)
		}
	}

	raw := "SIP/2.0 200 OK\r\n" +
		"Via: SIP/2.0/UDP 127.0.0.1:9;branch=z9hG4bKtest\r\n" +
		"From: <sip:a@b>;tag=1\r\n" +
		"To: <sip:a@b>;tag=2\r\n" +
		"Call-ID: cid-1\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Length: 0\r\n\r\n"

	addr, _ := net.ResolveUDPAddr("udp", s.Conn.LocalAddr().String())
	_, _ = s.Conn.WriteToUDP([]byte(raw), addr)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if saw.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("OnSIPResponse not invoked for 200 OK")
}
