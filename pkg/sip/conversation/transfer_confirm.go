package conversation

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/realtime"
)

const (
	defaultTransferConfirmCount = 2
	minTransferConfirmCount     = 1
	maxTransferConfirmCount     = 10
	transferConfirmIdleReset    = 2 * time.Minute
	// First (required-1) transfer-intent replies — deterministic, not model-generated.
	transferConfirmNormalReplyZH = "您好，有什么可以帮您的？"
	// Final reply before dial + hold music.
	transferConfirmExecuteReplyZH = "您好，这边马上为您转接人工，请稍后"
)

var (
	sipTransferConfirmMu    sync.Mutex
	sipTransferConfirmByID  = map[string]*sipTransferConfirmState{}
)

type sipTransferConfirmState struct {
	count    int
	lastBump time.Time
}

// TransferConfirmRequired returns how many distinct user transfer intents are
// required before dial is allowed (clamped 1–10, default 2).
func TransferConfirmRequired(env VoiceEnv) int {
	n := env.TransferConfirmCount
	if n <= 0 {
		n = defaultTransferConfirmCount
	}
	return clampTransferConfirmCount(n)
}

func clampTransferConfirmCount(n int) int {
	if n < minTransferConfirmCount {
		return minTransferConfirmCount
	}
	if n > maxTransferConfirmCount {
		return maxTransferConfirmCount
	}
	return n
}

func parseTransferConfirmCount(maps ...map[string]any) int {
	for _, m := range maps {
		if len(m) == 0 {
			continue
		}
		n := intFromMap(m, "transferConfirmCount", "transfer_confirm_count")
		if n > 0 {
			return n
		}
	}
	return 0
}

// recordSIPTransferIntent increments the per-call counter when userText
// expresses transfer intent. Each final user transcript can add at most one
// (prevents a single breath of "转人工×N" from satisfying N confirmations).
func recordSIPTransferIntent(callID, userText string) int {
	callID = strings.TrimSpace(callID)
	if callID == "" || !realtimeMatchTransferIntent("user", userText, nil) {
		return sipTransferConfirmCount(callID)
	}
	now := time.Now()
	sipTransferConfirmMu.Lock()
	defer sipTransferConfirmMu.Unlock()
	st := sipTransferConfirmByID[callID]
	if st == nil {
		st = &sipTransferConfirmState{}
		sipTransferConfirmByID[callID] = st
	}
	if !st.lastBump.IsZero() && now.Sub(st.lastBump) > transferConfirmIdleReset {
		st.count = 0
	}
	st.count++
	st.lastBump = now
	return st.count
}

func sipTransferConfirmCount(callID string) int {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return 0
	}
	sipTransferConfirmMu.Lock()
	defer sipTransferConfirmMu.Unlock()
	if st := sipTransferConfirmByID[callID]; st != nil {
		return st.count
	}
	return 0
}

// sipTransferMayExecute reports whether transfer dial is allowed for this call.
// Server-side gate: model/tool cannot bypass by ignoring tool output.
func sipTransferMayExecute(callID string, required int) (allowed bool, count int) {
	required = clampTransferConfirmCount(required)
	if required <= 1 {
		return true, sipTransferConfirmCount(callID)
	}
	count = sipTransferConfirmCount(callID)
	return count >= required, count
}

func cleanupSIPTransferConfirm(callID string) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	sipTransferConfirmMu.Lock()
	delete(sipTransferConfirmByID, callID)
	sipTransferConfirmMu.Unlock()
}

// transferConfirmToolMessage is returned to the LLM when the tool is blocked (pipeline path).
func transferConfirmToolMessage(count, required int) string {
	remaining := required - count
	if remaining < 1 {
		remaining = 1
	}
	b, _ := json.Marshal(transferConfirmToolPayload(count, required, remaining))
	return string(b)
}

func transferConfirmToolPayload(count, required, remaining int) map[string]any {
	return map[string]any{
		"ok":        false,
		"action":    "need_more_confirmations",
		"count":     count,
		"required":  required,
		"remaining": remaining,
		"spoken_zh": transferConfirmSpokenZH(count, required, remaining),
	}
}

func transferConfirmSpokenZH(count, required, remaining int) string {
	_ = remaining
	if count >= required && required > 0 {
		return transferConfirmExecuteReplyZH
	}
	_ = count
	return transferConfirmNormalReplyZH
}

// transferConfirmSessionHint is appended to realtime instructions after each user
// turn so the model does not promise transfer before the server allows dial.
func transferConfirmSessionHint(callID string, confirmRequired int) string {
	confirmRequired = clampTransferConfirmCount(confirmRequired)
	if confirmRequired <= 1 {
		return ""
	}
	count := sipTransferConfirmCount(callID)
	allowed, _ := sipTransferMayExecute(callID, confirmRequired)
	if allowed {
		return "【系统状态·仅模型可见】转人工确认已满足（" + strconv.Itoa(count) + "/" + strconv.Itoa(confirmRequired) +
			"）。请调用 transfer_to_agent；对用户仅说「" + transferConfirmExecuteReplyZH + "」。" +
			"禁止其它措辞；禁止「正在为您转接」「请稍候」等变体——系统会在你播报后自动拨号。"
	}
	return "【系统状态·仅模型可见】用户转人工意图累计 " + strconv.Itoa(count) + "/" + strconv.Itoa(confirmRequired) +
		"（后台计数，勿对用户透露次数、剩余次数，勿要求「再说一次转人工」或追问是否要转人工）。" +
		"严禁对用户说「正在为您转接」「正在转接」「请稍候」「马上转接」等任何转接进行中的表述。" +
		"本轮对用户只能说：「" + transferConfirmNormalReplyZH + "」。"
}

func mergeRealtimeInstructions(base, hint string) string {
	base = strings.TrimSpace(base)
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return base
	}
	if base == "" {
		return hint
	}
	return base + "\n\n" + hint
}

// syncRealtimeTransferInstructions pushes the latest confirm counter into the
// realtime session so the next model reply matches server gate state.
func syncRealtimeTransferInstructions(agent realtime.Agent, baseInstructions, callID string, confirmRequired int) {
	if agent == nil {
		return
	}
	hint := transferConfirmSessionHint(callID, confirmRequired)
	full := mergeRealtimeInstructions(baseInstructions, hint)
	if full == "" {
		return
	}
	_ = agent.UpdateInstructions(full)
}
