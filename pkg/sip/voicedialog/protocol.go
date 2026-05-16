package voicedialog

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	"github.com/LinByte/VoiceServer/pkg/utils"
)

func tsRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// event builds a normalized outbound JSON map with type, call_id, ts.
func event(typ, callID string, extra map[string]any) map[string]any {
	m := map[string]any{
		KeyType: typ,
		KeyTS:   tsRFC3339Nano(),
	}
	if strings.TrimSpace(callID) != "" {
		m[KeyCallID] = callID
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func errorWire(message string) map[string]any {
	return map[string]any{
		KeyType:    EvError,
		KeyMessage: message,
		KeyTS:      tsRFC3339Nano(),
	}
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func wsTokenExpected() string {
	return utils.GetEnv("VOICE_DIALOG_WS_TOKEN")
}

// transferLoadingAudioRef returns the transfer-loading WAV reference
// for the given inbound Call-ID. Resolution order matches the welcome
// audio path:
//
//  1. Per-DID TrunkNumber.TransferRingingURL via the resolver wired
//     in internal/sipserver/sipapp.go (always returns "" when no row /
//     no resolver).
//  2. SIP_TRANSFER_RINGING_WAV_PATH env (legacy operator override).
//  3. scripts/ringing.wav (project default).
//
// Callers pass the empty string for callID when no inbound is known
// (deliverConversationTransferPhase metadata enrichment) — the call
// then falls straight through to (2)/(3). Returns "" only when all
// three sources resolve to empty (which currently cannot happen
// because scripts/ringing.wav is hard-coded as fallback, but kept
// as the explicit empty-result contract for testability).
func transferLoadingAudioRef(callID string) string {
	if u := strings.TrimSpace(conversation.ResolveTransferRingingURL(callID)); u != "" {
		return u
	}
	if env := strings.TrimSpace(utils.GetEnv("SIP_TRANSFER_RINGING_WAV_PATH")); env != "" {
		return env
	}
	return "scripts/ringing.wav"
}

// ParseVoicedialogAudioRef returns source_kind, client-visible source, and filesystem path or URL for loading WAV PCM.
func ParseVoicedialogAudioRef(ref string) (kind string, sourceDisplay string, pathOrURL string) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "script:")
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", ""
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return SourceKindURL, ref, ref
	}
	if filepath.IsAbs(ref) {
		clean := filepath.Clean(ref)
		return SourceKindScript, filepath.Base(clean), clean
	}
	ref = strings.TrimPrefix(ref, "scripts/")
	ref = filepath.Base(ref)
	if ref == "" || ref == "." {
		return "", "", ""
	}
	path := filepath.Join("scripts", ref)
	return SourceKindScript, ref, path
}
