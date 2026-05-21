package constants

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Outbound / transfer / media env **key names** (for utils.GetEnv). Not a checklist of required .env
// entries—each caller has defaults when unset. Every name below is referenced from pkg/config/sip.go,
// pkg/sip/persist, internal/sipserver, or pkg/sip/session; do not remove without updating those sites.

const (
	EnvSIPCallerID          = "SIP_CALLER_ID"
	EnvSIPCallerDisplayName = "SIP_CALLER_DISPLAY_NAME"
	EnvSIPDefaultDomain     = "SIP_DEFAULT_DOMAIN"
	EnvSIPDefaultURIPort    = "SIP_DEFAULT_URI_PORT"
	EnvSIPTransferSigAddr   = "SIP_TRANSFER_SIGNALING_ADDR"
	EnvSIPTransferNumber    = "SIP_TRANSFER_NUMBER"
	EnvSIPTransferHost      = "SIP_TRANSFER_HOST"
	EnvSIPTransferPort      = "SIP_TRANSFER_PORT"
	EnvSIPRegisterPassword  = "SIP_PASSWORD"
)

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
	SIPScriptRunStarted     = "started"
	SIPScriptRunMatched     = "matched" // listen step: ASR text received (after OnListen succeeds)
	SIPScriptRunFailed      = "failed"
	SIPScriptRunTimeout     = "timeout"
	SIPScriptRunEnded       = "ended"
	SIPScriptRunRouteFailed = "route_failed" // listen: LLM branch routing failed or LLM not configured
)

// Env vars for campaign script listen latency (read via utils.GetEnv).
const (
	// EnvSIPScriptListenTailMSMax caps the computed tail (default 2000; was 6000).
	EnvSIPScriptListenTailMSMax = "SIP_SCRIPT_LISTEN_TAIL_MS_MAX"
	// EnvSIPScriptListenTailMSMin floor for non-zero tail (default 400).
	EnvSIPScriptListenTailMSMin = "SIP_SCRIPT_LISTEN_TAIL_MS_MIN"
	// EnvSIPScriptListenPollMS: DB poll interval while waiting for next user turn (default 120).
	EnvSIPScriptListenPollMS  = "SIP_SCRIPT_LISTEN_POLL_MS"
	EnvCHECKLLMProvider       = "CHECK_LLM_PROVIDER"
	EnvCHECKLLMBaseURL        = "CHECK_LLM_BASEURL"
	EnvCHECKLLMAPIKey         = "CHECK_LLM_APIKEY"
	EnvCHECKLLMModel          = "CHECK_LLM_MODEL"
	EnvCHECKLLMRouteTimeoutMS = "CHECK_LLM_ROUTE_TIMEOUT_MS"
	EnvCHECKLLMRouteDisabled  = "CHECK_LLM_ROUTE_DISABLED" // "1" or "true" disables LLM listen routing (then listen+transitions is invalid at runtime)
	// EnvCHECKLLMRouteLegacyJSON=1: model returns {"next_id":"..."}; default uses compact {"i":N} index (faster, fewer tokens).
	EnvCHECKLLMRouteLegacyJSON = "CHECK_LLM_ROUTE_LEGACY_JSON"
	// EnvCHECKLLMRouteMaxTokens: max completion tokens for route call (default 32, range 8–128).
	EnvCHECKLLMRouteMaxTokens = "CHECK_LLM_ROUTE_MAX_TOKENS"
	// EnvSIPScriptLLMFailPrompt: spoken when listen LLM routing is unavailable or fails (default below).
	EnvSIPScriptLLMFailPrompt = "SIP_SCRIPT_LLM_FAIL_PROMPT"
)
