package models

import "testing"

func TestLookupACDMetaPath(t *testing.T) {
	meta := map[string]any{
		"FactoryNumber": "F-1001",
		"nested":        map[string]any{"city": "上海"},
	}
	if got := LookupACDMetaPath(meta, "FactoryNumber"); got != "F-1001" {
		t.Fatalf("FactoryNumber: %q", got)
	}
	if got := LookupACDMetaPath(meta, "nested.city"); got != "上海" {
		t.Fatalf("nested.city: %q", got)
	}
}

func TestNormalizeACDMetaDataJSON(t *testing.T) {
	s, err := NormalizeACDMetaDataJSON(map[string]any{"a": 1})
	if err != nil || s == "" {
		t.Fatalf("err=%v s=%q", err, s)
	}
	if _, err := NormalizeACDMetaDataJSON("not json"); err == nil {
		t.Fatal("expected error")
	}
}
