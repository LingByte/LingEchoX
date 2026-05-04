package outbound

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LingByte/SoulNexus/pkg/sip/stack"
)

// inviteParams carries dialog fields needed for INVITE and later ACK.
type inviteParams struct {
	LocalIP         string
	SIPHost         string
	SIPPort         int
	RequestURI      string
	CallID          string
	FromTag         string
	Branch          string
	CSeq            int
	LocalRTPPort    int
	SDPBody         string
	FromUser        string // sip:FromUser@host:port
	FromDisplayName string // optional; quoted display-name in From
}

func sipEscapeQuotedDisplay(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\r', '\n':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// formatOutboundFromHeader builds the From header value (INVITE/ACK/BYE in-dialog).
func formatOutboundFromHeader(displayName, user, host string, port int, tag string) string {
	user = sanitizeSIPUser(user)
	host = nonEmpty(host, "127.0.0.1")
	port = nonZero(port, 6050)
	uri := fmt.Sprintf("<sip:%s@%s:%d>", user, host, port)
	dn := strings.TrimSpace(displayName)
	if dn == "" {
		return uri + ";tag=" + tag
	}
	return sipEscapeQuotedDisplay(dn) + " " + uri + ";tag=" + tag
}

func formatOutboundContact(user, host string, port int) string {
	user = sanitizeSIPUser(user)
	host = nonEmpty(host, "127.0.0.1")
	port = nonZero(port, 6050)
	return fmt.Sprintf("<sip:%s@%s:%d>", user, host, port)
}

func sanitizeSIPUser(user string) string {
	user = strings.TrimSpace(user)
	if user == "" {
		return "soulnexus"
	}
	var b strings.Builder
	for _, r := range user {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == '+' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	s := strings.Trim(strings.TrimSpace(b.String()), "._-+")
	if s == "" {
		return "soulnexus"
	}
	return s
}

func buildINVITE(p inviteParams) *stack.Message {
	via := fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bK%s;rport",
		nonEmpty(p.SIPHost, "127.0.0.1"), nonZero(p.SIPPort, 6050), p.Branch)

	from := formatOutboundFromHeader(p.FromDisplayName, p.FromUser, p.SIPHost, p.SIPPort, p.FromTag)
	to := formatToHeader(p.RequestURI)

	msg := &stack.Message{
		IsRequest:  true,
		Method:     stack.MethodInvite,
		RequestURI: p.RequestURI,
		Version:    "SIP/2.0",
		Body:       p.SDPBody,
	}
	msg.SetHeader("Via", via)
	msg.SetHeader("Max-Forwards", "70")
	msg.SetHeader("From", from)
	msg.SetHeader("To", to)
	msg.SetHeader("Call-ID", p.CallID)
	msg.SetHeader("CSeq", fmt.Sprintf("%d INVITE", p.CSeq))
	msg.SetHeader("Contact", formatOutboundContact(p.FromUser, p.SIPHost, p.SIPPort))
	msg.SetHeader("User-Agent", "SoulNexus-SIP/1.0")
	msg.SetHeader("Content-Type", "application/sdp")
	msg.SetHeader("Allow", "INVITE, ACK, BYE, CANCEL, OPTIONS")
	msg.SetHeader("Content-Length", strconv.Itoa(stack.BodyBytesLen(p.SDPBody)))
	return msg
}

func formatToHeader(requestURI string) string {
	u := strings.TrimSpace(requestURI)
	if u == "" {
		return "<sip:invalid@invalid>"
	}
	if !strings.HasPrefix(strings.ToLower(u), "sip:") {
		u = "sip:" + u
	}
	return "<" + u + ">"
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func nonZero(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

func newCallID(localIP string) string {
	return fmt.Sprintf("%d@%s", time.Now().UnixNano(), nonEmpty(localIP, "127.0.0.1"))
}
