package stack

import "testing"

func TestParseCSeqNum(t *testing.T) {
	if n := ParseCSeqNum("42 INVITE"); n != 42 {
		t.Fatalf("got %d", n)
	}
	if n := ParseCSeqNum(""); n != 0 {
		t.Fatalf("empty: got %d", n)
	}
}

func TestWithCSeqACK(t *testing.T) {
	if s := WithCSeqACK(3); s != "3 ACK" {
		t.Fatalf("got %q", s)
	}
	if s := WithCSeqACK(0); s != "1 ACK" {
		t.Fatalf("got %q", s)
	}
}
