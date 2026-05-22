package conversation

import (
	"encoding/json"
	"testing"
)

func TestRunGetCurrentTime(t *testing.T) {
	out := runGetCurrentTime(map[string]any{"timezone": "Asia/Shanghai"})
	if out["ok"] != true {
		t.Fatalf("expected ok: %v", out)
	}
	if out["spoken_zh"] == "" {
		t.Fatal("spoken_zh empty")
	}
}

func TestRunIsBusinessHours(t *testing.T) {
	out := runIsBusinessHours(map[string]any{})
	if out["ok"] != true {
		t.Fatalf("expected ok: %v", out)
	}
	if _, ok := out["in_business_hours"].(bool); !ok {
		t.Fatal("missing in_business_hours")
	}
}

func TestEvalSimpleArithmetic(t *testing.T) {
	cases := map[string]float64{
		"1+2":       3,
		"10-3":      7,
		"4*5":       20,
		"20/4":      5,
		"(2+3)*4":   20,
		"-5+10":     5,
		"100+20*3":  160,
	}
	for expr, want := range cases {
		got, err := evalSimpleArithmetic(expr)
		if err != nil {
			t.Fatalf("%q: %v", expr, err)
		}
		if got != want {
			t.Fatalf("%q: got %v want %v", expr, got, want)
		}
	}
}

func TestSIPRealtimeToolHandler_Unknown(t *testing.T) {
	h := newSIPRealtimeToolHandler("c1", 3, nil, func(string) {})
	out := h("no_such_tool", nil)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil || m["ok"] != false {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSIPRealtimeTools_Count(t *testing.T) {
	if len(SIPRealtimeTools()) < 4 {
		t.Fatalf("expected at least 4 tools, got %d", len(SIPRealtimeTools()))
	}
}
