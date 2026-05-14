package voicedialog

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/utils"
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

// transferLoadingAudioRef returns SIP_TRANSFER_RINGING_WAV_PATH when set (same clip family as SIP transfer ringback).
func transferLoadingAudioRef() string {
	return utils.GetEnv("SIP_TRANSFER_RINGING_WAV_PATH")
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
