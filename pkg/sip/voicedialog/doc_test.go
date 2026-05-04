package voicedialog

import "testing"

// Smoke test so doc.go remains wired to the same package as implementation files.
func TestPackageDocConstants(t *testing.T) {
	if ProtocolVersion < 1 {
		t.Fatal(ProtocolVersion)
	}
}
