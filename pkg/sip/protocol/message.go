package protocol

import "github.com/LingByte/SoulNexus/pkg/sip/stack"

// Message is the canonical SIP wire parse model; kept under protocol for backward-compatible imports.
type Message = stack.Message

// Parse parses a raw SIP message (delegates to pkg/sip/stack).
func Parse(raw string) (*Message, error) {
	return stack.Parse(raw)
}

// BodyBytesLen returns the byte length of the SDP/message body after CRLF normalization.
func BodyBytesLen(body string) int {
	return stack.BodyBytesLen(body)
}
