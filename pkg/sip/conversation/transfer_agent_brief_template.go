package conversation

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// MaxTransferAgentBriefTextLen is the DB/API cap for transfer_agent_brief_text (one short TTS line).
const MaxTransferAgentBriefTextLen = 256

// MaxTransferAgentBriefRenderedLen caps expanded TTS after placeholder substitution.
const MaxTransferAgentBriefRenderedLen = 512

// TransferAgentBriefAgent carries ACD seat fields for template expansion.
type TransferAgentBriefAgent struct {
	Name                 string
	TargetValue          string
	RouteType            string
	WorkState            string
	SipCallerID          string
	SipCallerDisplayName string
	Remark               string // BaseModel plain-text remark (admin note)
}

// TransferAgentBriefVars supplies caller + agent + metadata for brief templates.
type TransferAgentBriefVars struct {
	CallerNumber string
	Agent        TransferAgentBriefAgent
	Meta         map[string]any // parsed acd_pool_targets.meta_data JSON
}

var transferAgentBriefPlaceholderRE = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9]*(?:\.[A-Za-z][A-Za-z0-9]*)*)\}\}`)

// RenderTransferAgentBriefTemplate expands placeholders such as:
//   - {{N}} {{NTail4}} — caller
//   - {{Name}} {{TargetValue}} {{SipCallerId}} — agent columns
//   - {{MetaData.FactoryNumber}} — acd_pool_targets.meta_data JSON keys (alias: {{Meta.FactoryNumber}})
//   - {{Note}} — plain BaseModel remark text
func RenderTransferAgentBriefTemplate(tmpl string, vars TransferAgentBriefVars) string {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		return ""
	}
	caller := strings.TrimSpace(vars.CallerNumber)
	digits := digitsOnly(caller)
	agent := vars.Agent
	meta := vars.Meta
	if meta == nil {
		meta = map[string]any{}
	}

	out := transferAgentBriefPlaceholderRE.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := transferAgentBriefPlaceholderRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		path := strings.TrimSpace(sub[1])
		if path == "" {
			return ""
		}
		parts := strings.Split(path, ".")
		root := strings.ToLower(parts[0])
		if len(parts) == 1 {
			return resolveTransferBriefSingle(root, caller, digits, agent, meta)
		}
		subPath := strings.Join(parts[1:], ".")
		switch root {
		case "metadata", "meta":
			return lookupMetaPath(meta, subPath)
		case "agent":
			return resolveTransferBriefAgentField(subPath, agent, meta)
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

func resolveTransferBriefSingle(root, caller, digits string, agent TransferAgentBriefAgent, meta map[string]any) string {
	switch root {
	case "n", "number", "caller":
		if caller != "" {
			return caller
		}
		return digits
	case "name", "agentname":
		return agentDisplayName(agent)
	case "targetvalue", "target":
		return strings.TrimSpace(agent.TargetValue)
	case "routetype", "route":
		return strings.TrimSpace(agent.RouteType)
	case "workstate", "state":
		return strings.TrimSpace(agent.WorkState)
	case "sipcallerid", "callerid":
		return strings.TrimSpace(agent.SipCallerID)
	case "sipcallerdisplayname", "callerdisplayname", "displayname":
		return strings.TrimSpace(agent.SipCallerDisplayName)
	case "note", "baseremark", "agentremark":
		return strings.TrimSpace(agent.Remark)
	case "ntail":
		return tailDigits(digits, 4)
	}
	if strings.HasPrefix(root, "ntail") {
		n := 4
		if suffix := strings.TrimPrefix(root, "ntail"); suffix != "" {
			if v, err := strconv.Atoi(suffix); err == nil && v > 0 && v <= 11 {
				n = v
			}
		}
		return tailDigits(digits, n)
	}
	return ""
}

func resolveTransferBriefAgentField(subPath string, agent TransferAgentBriefAgent, meta map[string]any) string {
	parts := strings.Split(subPath, ".")
	if len(parts) == 0 {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	if len(parts) == 1 {
		switch key {
		case "name":
			return agentDisplayName(agent)
		case "targetvalue", "target":
			return strings.TrimSpace(agent.TargetValue)
		case "routetype":
			return strings.TrimSpace(agent.RouteType)
		case "workstate":
			return strings.TrimSpace(agent.WorkState)
		case "sipcallerid":
			return strings.TrimSpace(agent.SipCallerID)
		case "sipcallerdisplayname":
			return strings.TrimSpace(agent.SipCallerDisplayName)
		case "remark", "note":
			return strings.TrimSpace(agent.Remark)
		}
		return ""
	}
	if key == "metadata" || key == "meta" {
		return lookupMetaPath(meta, strings.Join(parts[1:], "."))
	}
	return lookupMetaPath(meta, subPath)
}

func agentDisplayName(agent TransferAgentBriefAgent) string {
	if n := strings.TrimSpace(agent.Name); n != "" {
		return n
	}
	return strings.TrimSpace(agent.TargetValue)
}

func lookupMetaPath(meta map[string]any, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || len(meta) == 0 {
		return ""
	}
	parts := strings.Split(path, ".")
	var cur any = meta
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return ""
		}
		switch node := cur.(type) {
		case map[string]any:
			var ok bool
			cur, ok = node[p]
			if !ok {
				lp := strings.ToLower(p)
				for k, v := range node {
					if strings.ToLower(k) == lp {
						cur = v
						ok = true
						break
					}
				}
				if !ok {
					return ""
				}
			}
		default:
			return ""
		}
	}
	return metaValueToString(cur)
}

func metaValueToString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "是"
		}
		return "否"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		s := strings.TrimSpace(string(b))
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
		return s
	}
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
