package constants

// SIP hybrid script step types.
const (
	SIPScriptStepSay       = "say"
	SIPScriptStepListen    = "listen"
	SIPScriptStepLLMReply  = "llm_reply"
	SIPScriptStepCondition = "condition"
	SIPScriptStepEnd       = "end"
)

// SIP hybrid script runtime event results.
const (
	SIPScriptRunStarted = "started"
	SIPScriptRunMatched = "matched" // listen step: ASR text received (after OnListen succeeds)
	SIPScriptRunFailed  = "failed"
	SIPScriptRunTimeout = "timeout"
	SIPScriptRunEnded   = "ended"
)

// Env vars for campaign script listen latency (read via utils.GetEnv).
const (
	// SIP_SCRIPT_LISTEN_AFTER_TTS_TAIL: extra ms after say before listen timeout fully applies.
	// "0", "false", "off", "no" disables the tail (only script listen_timeout_ms + silence_timeout_ms apply).
	EnvSIPScriptListenAfterTTSTail = "SIP_SCRIPT_LISTEN_AFTER_TTS_TAIL"
	// SIP_SCRIPT_LISTEN_TAIL_MS_MAX caps the computed tail (default 2000; was 6000).
	EnvSIPScriptListenTailMSMax = "SIP_SCRIPT_LISTEN_TAIL_MS_MAX"
	// SIP_SCRIPT_LISTEN_TAIL_MS_MIN floor for non-zero tail (default 400).
	EnvSIPScriptListenTailMSMin = "SIP_SCRIPT_LISTEN_TAIL_MS_MIN"
	// SIP_SCRIPT_LISTEN_POLL_MS: DB poll interval while waiting for next user turn (default 120).
	EnvSIPScriptListenPollMS = "SIP_SCRIPT_LISTEN_POLL_MS"
)
