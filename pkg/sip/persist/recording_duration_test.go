package persist

import (
	"testing"

	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
)

func TestResolveRecordingDurationSec_FromWAVBytes(t *testing.T) {
	wav := buildTestWAV(32000, 32000)
	sec := ResolveRecordingDurationSec(gateway.RecordingInfo{}, wav)
	if sec != 1 {
		t.Fatalf("duration = %d, want 1", sec)
	}
}

func TestResolveRecordingDurationSec_FromDurationMs(t *testing.T) {
	sec := ResolveRecordingDurationSec(gateway.RecordingInfo{DurationMs: 5100}, nil)
	if sec != 5 {
		t.Fatalf("duration = %d, want 5", sec)
	}
}
