package outbound

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LinByte/VoiceServer/pkg/sip/stack"
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

// sipFormatDisplayName renders a SIP From display-name in a wire format
// every reasonable SBC / carrier will accept.
//
// Two encodings:
//
//   - 纯 ASCII (token / quoted-string)：原样 quoted-string，反斜杠转义引号/反斜杠，
//     回车换行折叠为空格。
//   - 含任意非 ASCII（中文 / emoji 等）：RFC 2047 §5 MIME encoded-word
//     `=?UTF-8?B?<base64>?=` 形式，整个 token 不再加引号。
//
// 为什么不直接把 UTF-8 quoted-string 塞进去？—— RFC 3261 §25.1 BNF 仍然
// 严格遵守 RFC 2822 的 quoted-string ASCII 范围，国内运营商 SBC（移动 /
// 联通 / 电信 NGN 网关）经常按字面执行，发现 `"牛牛科技无限公司"` 这种
// 高位字节直接 strip 整个 From display-name 或 400 Bad Request 退回。
// MIME encoded-word 是这些设备公认能透传的中文显示名编码。
func sipFormatDisplayName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if isASCIIOnly(s) {
		return sipQuotedASCIIDisplay(s)
	}
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

// isASCIIOnly 仅检测是否含非 ASCII（即 UTF-8 多字节）字符；ASCII 控制字符
// （\r\n\t 等）由 quoted-string 路径自行折叠，不必跳到 MIME 编码。
func isASCIIOnly(s string) bool {
	for _, r := range s {
		if r > 0x7E {
			return false
		}
	}
	return true
}

func sipQuotedASCIIDisplay(s string) string {
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
	dn := sipFormatDisplayName(displayName)
	if dn == "" {
		return uri + ";tag=" + tag
	}
	return dn + " " + uri + ";tag=" + tag
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
	// Host part should match Via/Contact identity (SIPHost) so carriers do not rewrite Call-ID.
	return fmt.Sprintf("%d@%s", time.Now().UnixNano(), nonEmpty(localIP, "127.0.0.1"))
}
