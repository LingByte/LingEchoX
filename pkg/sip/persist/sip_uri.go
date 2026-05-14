package persist

import "strings"

// ExtractSIPUserPart 解析 SIP From/To 头里的「号码」部分（URI 的 user 部分）。
//
// 兼容以下常见形态：
//
//	"Bob" <sip:bob@10.0.4.12>;tag=xyz                    → "bob"
//	<sip:13800138000@example.com:5060>;tag=...           → "13800138000"
//	sip:alice@host                                       → "alice"
//	tel:+8613800138000                                   → "+8613800138000"
//	"4001608853" <sip:4001608853@gw.example>             → "4001608853"
//
// 解析失败（找不到 user@host 或 sip:user）时返回去除引号 / 空白后的原值。
func ExtractSIPUserPart(header string) string {
	s := strings.TrimSpace(header)
	if s == "" {
		return ""
	}

	// 1) 优先取 <...> 包围的 SIP-URI。
	if l := strings.IndexByte(s, '<'); l >= 0 {
		if r := strings.IndexByte(s[l:], '>'); r > 0 {
			s = s[l+1 : l+r]
		}
	}

	// 2) 去掉常见 scheme 前缀。tel: 直接返回数字部分；sip:/sips: 继续向下找 @。
	if u, ok := stripPrefixCI(s, "tel:"); ok {
		return trimURIParams(u)
	}
	if u, ok := stripPrefixCI(s, "sip:"); ok {
		s = u
	} else if u, ok := stripPrefixCI(s, "sips:"); ok {
		s = u
	}

	// 3) 取 @ 之前的 user 部分，并去掉 ;params。
	if at := strings.IndexByte(s, '@'); at >= 0 {
		s = s[:at]
	}
	s = trimURIParams(s)
	s = strings.Trim(s, "\"' ")
	return s
}

func stripPrefixCI(s, prefix string) (string, bool) {
	if len(s) < len(prefix) {
		return s, false
	}
	if strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// trimURIParams 去掉 ;tag=... / ;user=phone 之类的参数与 ?query。
func trimURIParams(s string) string {
	if i := strings.IndexAny(s, ";?"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
