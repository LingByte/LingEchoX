package persist

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordingPathForCall(t *testing.T) {
	dir := "/data/rec"
	p := RecordingPathForCall(dir, "abc@def")
	if filepath.Dir(p) != dir {
		t.Fatalf("dir: got %q", filepath.Dir(p))
	}
	if !strings.HasSuffix(p, ".wav") {
		t.Fatalf("suffix: %q", p)
	}
	if RecordingPathForCall("", "x") != "" {
		t.Fatal("empty base")
	}
	if RecordingPathForCall("/tmp", "") != "" {
		t.Fatal("empty call-id")
	}
}
