package persist

import (
	"testing"

	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

func TestParseRegistrationAOR(t *testing.T) {
	u, d := ParseRegistrationAOR("sip:alice@example.com")
	if u != "alice" || d != "example.com" {
		t.Fatalf("got %q %q", u, d)
	}
}

func TestRegisterExpiresSeconds(t *testing.T) {
	raw := "REGISTER sip:bob@pbx SIP/2.0\r\n" +
		"Contact: <sip:bob@192.0.2.1:5060>;expires=120\r\n" +
		"\r\n"
	msg, err := stack.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if n := RegisterExpiresSeconds(msg); n != 120 {
		t.Fatalf("contact expires: %d", n)
	}
	raw2 := "REGISTER sip:bob@pbx SIP/2.0\r\nExpires: 60\r\n\r\n"
	msg2, err := stack.Parse(raw2)
	if err != nil {
		t.Fatal(err)
	}
	if n := RegisterExpiresSeconds(msg2); n != 60 {
		t.Fatalf("expires header: %d", n)
	}
}
