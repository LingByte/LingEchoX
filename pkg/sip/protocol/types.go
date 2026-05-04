package protocol

import "github.com/LingByte/SoulNexus/pkg/sip/stack"

// SIP method constants (upper-case).
const (
	MethodInvite   = "INVITE"
	MethodAck      = "ACK"
	MethodBye      = "BYE"
	MethodRegister = "REGISTER"
	MethodOptions  = "OPTIONS"
	MethodCancel   = "CANCEL"
	MethodInfo     = "INFO"
	MethodPublish  = "PUBLISH"
	MethodPrack    = "PRACK"
)

// Message is the canonical SIP wire parse model (delegates to pkg/sip/stack).
type Message = stack.Message

// Parse parses a raw SIP message.
func Parse(raw string) (*Message, error) {
	return stack.Parse(raw)
}

// BodyBytesLen returns the byte length of the SDP/message body after CRLF normalization.
func BodyBytesLen(body string) int {
	return stack.BodyBytesLen(body)
}
