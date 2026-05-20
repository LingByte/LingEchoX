package voicedialog

import (
	"net/http/httptest"
	"testing"
)

func TestCmdKeysSorted(t *testing.T) {
	if cmdKeysSorted(nil) != nil {
		t.Fatal()
	}
	got := cmdKeysSorted(map[string]any{"b": 1, "a": 2})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%v", got)
	}
}

func TestWsTokenOK(t *testing.T) {
	t.Cleanup(func() { defaultHub = nil })

	// Strict-by-default: empty env now rejects. Old "empty allows"
	// behaviour is gated behind VOICE_DIALOG_ALLOW_EMPTY_TOKEN=true.
	t.Run("empty_expected_rejects", func(t *testing.T) {
		t.Setenv("VOICE_DIALOG_WS_TOKEN", "")
		t.Setenv("VOICE_DIALOG_ALLOW_EMPTY_TOKEN", "")
		defaultHub = nil
		r := httptest.NewRequest("GET", "/?token=nothing", nil)
		if WebSocketTokenOK(r) {
			t.Fatal("expected reject when both envs unset")
		}
	})

	t.Run("empty_expected_allows_with_dev_opt_in", func(t *testing.T) {
		t.Setenv("VOICE_DIALOG_WS_TOKEN", "")
		t.Setenv("VOICE_DIALOG_ALLOW_EMPTY_TOKEN", "true")
		defaultHub = nil
		r := httptest.NewRequest("GET", "/?token=nothing", nil)
		if !WebSocketTokenOK(r) {
			t.Fatal("expected allow when ALLOW_EMPTY_TOKEN=true")
		}
	})

	t.Run("match", func(t *testing.T) {
		t.Setenv("VOICE_DIALOG_WS_TOKEN", "secret-token")
		InitDefault(Config{})
		r := httptest.NewRequest("GET", "/?token=secret-token", nil)
		if !WebSocketTokenOK(r) {
			t.Fatal()
		}
	})

	t.Run("wrong_len", func(t *testing.T) {
		t.Setenv("VOICE_DIALOG_WS_TOKEN", "secret-token")
		InitDefault(Config{})
		r := httptest.NewRequest("GET", "/?token=wrong", nil)
		if WebSocketTokenOK(r) {
			t.Fatal()
		}
	})

	t.Run("wrong_value_same_len", func(t *testing.T) {
		t.Setenv("VOICE_DIALOG_WS_TOKEN", "aaaaaaaaaa")
		InitDefault(Config{})
		r := httptest.NewRequest("GET", "/?token=bbbbbbbbbb", nil)
		if WebSocketTokenOK(r) {
			t.Fatal()
		}
	})
}
