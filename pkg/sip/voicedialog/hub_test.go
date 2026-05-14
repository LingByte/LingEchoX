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

	t.Run("empty_expected_allows", func(t *testing.T) {
		t.Setenv("VOICE_DIALOG_WS_TOKEN", "")
		defaultHub = nil
		r := httptest.NewRequest("GET", "/?token=nothing", nil)
		if !WebSocketTokenOK(r) {
			t.Fatal()
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
