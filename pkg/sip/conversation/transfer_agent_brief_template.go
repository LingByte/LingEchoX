package conversation

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// MaxTransferAgentBriefTextLen is the DB/API cap for transfer_agent_brief_text (one short TTS line).
const MaxTransferAgentBriefTextLen = 256

// MaxTransferAgentBriefRenderedLen caps expanded TTS after placeholder substitution.
const MaxTransferAgentBriefRenderedLen = 512

// TransferAgentBriefVars supplies {{N}} / {{Name}} template fields.
type TransferAgentBriefVars struct {
	CallerNumber string // full digits or display as received
	AgentName    string // ACD seat label (Name, else TargetValue)
}

var transferAgentBriefPlaceholderRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

// RenderTransferAgentBriefTemplate expands {{N}}, {{NTail}}, {{NTail4}}, {{Name}}, etc.
// Unknown placeholders are removed (empty string).
func RenderTransferAgentBriefTemplate(tmpl string, vars TransferAgentBriefVars) string {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		return ""
	}
	caller := strings.TrimSpace(vars.CallerNumber)
	digits := digitsOnly(caller)
	agentName := strings.TrimSpace(vars.AgentName)

	out := transferAgentBriefPlaceholderRE.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := transferAgentBriefPlaceholderRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		key := strings.ToLower(strings.TrimSpace(sub[1]))
		switch key {
		case "n", "number", "caller":
			if caller != "" {
				return caller
			}
			return digits
		case "name", "agentname", "agent":
			return agentName
		case "ntail":
			return tailDigits(digits, 4)
		}
		if strings.HasPrefix(key, "ntail") {
			n := 4
			if suffix := strings.TrimPrefix(key, "ntail"); suffix != "" {
				if v, err := strconv.Atoi(suffix); err == nil && v > 0 && v <= 11 {
					n = v
				}
			}
			return tailDigits(digits, n)
		}
		return ""
	})
	out = strings.TrimSpace(out)
	if len([]rune(out)) > MaxTransferAgentBriefRenderedLen {
		r := []rune(out)
		out = string(r[:MaxTransferAgentBriefRenderedLen])
	}
	return out
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func tailDigits(digits string, n int) string {
	if n <= 0 || digits == "" {
		return ""
	}
	runes := []rune(digits)
	if len(runes) <= n {
		return digits
	}
	return string(runes[len(runes)-n:])
}
