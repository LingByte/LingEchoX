package voicedialog

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("ab", 10); got != "ab" {
		t.Fatalf("short: %q", got)
	}
	if got := truncateRunes("αβγδ", 2); got != "αβ…" {
		t.Fatalf("runes: %q", got)
	}
}

func TestEventAndErrorWire(t *testing.T) {
	ev := event(EvPong, "cid-1", map[string]any{KeyOK: true})
	if ev[KeyType] != EvPong || ev[KeyCallID] != "cid-1" {
		t.Fatalf("event: %#v", ev)
	}
	ts, _ := ev[KeyTS].(string)
	if ts == "" {
		t.Fatal("missing ts")
	}
	errM := errorWire("nope")
	if errM[KeyType] != EvError || errM[KeyMessage] != "nope" {
		t.Fatalf("errorWire: %#v", errM)
	}
}

func TestParseVoicedialogAudioRef(t *testing.T) {
	t.Run("url", func(t *testing.T) {
		k, d, p := ParseVoicedialogAudioRef("https://x.example/a.wav")
		if k != SourceKindURL || d != "https://x.example/a.wav" || p != "https://x.example/a.wav" {
			t.Fatalf("got %q %q %q", k, d, p)
		}
	})
	t.Run("script_prefix_stripped", func(t *testing.T) {
		k, d, p := ParseVoicedialogAudioRef("script:welcome.wav")
		if k != SourceKindScript || d != "welcome.wav" {
			t.Fatalf("display: %q %q", k, d)
		}
		if !strings.HasSuffix(p, filepath.Join("scripts", "welcome.wav")) {
			t.Fatalf("path: %q", p)
		}
	})
	t.Run("empty", func(t *testing.T) {
		k, _, _ := ParseVoicedialogAudioRef("   ")
		if k != "" {
			t.Fatal(k)
		}
	})
}

func TestTransferLoadingAudioRef(t *testing.T) {
	// Env override wins over the scripts/ringing.wav fallback when no
	// per-DID resolver match exists for the supplied Call-ID.
	t.Setenv("SIP_TRANSFER_RINGING_WAV_PATH", "/tmp/x.wav")
	if got := transferLoadingAudioRef(""); got != "/tmp/x.wav" {
		t.Fatalf("env override: got %q", got)
	}

	// With env unset and no resolver wired, we expect the project
	// default scripts/ringing.wav (caller decides whether that file
	// exists; this function is responsible for the path resolution only).
	t.Setenv("SIP_TRANSFER_RINGING_WAV_PATH", "")
	if got := transferLoadingAudioRef(""); got != "scripts/ringing.wav" {
		t.Fatalf("default fallback: got %q", got)
	}
}
