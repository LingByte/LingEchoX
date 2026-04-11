package outbound

import "testing"

func TestParseRouteLLMJSON(t *testing.T) {
	id, err := parseRouteLLMJSON(`{"next_id":"collect_contact"}`)
	if err != nil || id != "collect_contact" {
		t.Fatalf("plain: id=%q err=%v", id, err)
	}
	id, err = parseRouteLLMJSON("```json\n{\"next_id\":\"close_soft\"}\n```")
	if err != nil || id != "close_soft" {
		t.Fatalf("fenced: id=%q err=%v", id, err)
	}
	id, err = parseRouteLLMJSON(`here is {"next_id":"x"}`)
	if err != nil || id != "x" {
		t.Fatalf("embedded: id=%q err=%v", id, err)
	}
	if _, err := parseRouteLLMJSON(""); err == nil {
		t.Fatal("expected error on empty")
	}
}
