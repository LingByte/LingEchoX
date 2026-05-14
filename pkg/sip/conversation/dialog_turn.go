package conversation

import "time"

// DialogTurn is persisted to sip_calls.turns (JSON) via SetSIPTurnPersist → persist.CallStore.SaveConversationTurn
// or conversation.RecordDialogTurn from voicedialog gateway.
type DialogTurn struct {
	ASRText     string
	LLMText     string
	ASRProvider string
	TTSProvider string
	LLMModel    string
	// Trigger is how ASR fired this turn: final | partial | partial-timeout (script / partial ASR).
	Trigger string
	// ScriptStepID / RouteIntent are optional (script routing, future hooks).
	ScriptStepID string
	RouteIntent  string
	LLMFirstMs   int       // first LLM token latency (ms) when streaming; for non-stream ≈ LLMWallMs
	LLMWallMs    int       // LLM Query / QueryStream wall time (ms); streaming includes time while TTS flush runs inside callbacks
	TTSMs        int       // cumulative ttsPipe.Speak wall time (ms)
	PipelineMs   int       // wall time for the whole ASR→LLM→TTS goroutine slice (caller-filled)
	At           time.Time // zero → persistence uses time.Now()
	// TurnGroupID 同一逻辑轮次（同一 ASR final → 同一 LLM 流式回复的所有 TTS 段）共用的标识。
	// 持久化层遇到与上一条相同的非空 TurnGroupID 时会把 LLMText 追加到上一条而不是新建条目，
	// 从而把流式分段合成的多条记录折叠成一行（避免同一用户问句重复出现）。
	// 单段（非流式）回复留空字符串即可——空值不参与折叠。
	TurnGroupID string
}

// StreamTurnTimings is measured inside streamLLMToTTS (LLM + TTS only, not ASR).
type StreamTurnTimings struct {
	LLMFirstMs int
	LLMWallMs  int
	TTSMs      int
}
