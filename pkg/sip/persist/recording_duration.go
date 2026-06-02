package persist

import (
	"strings"

	"github.com/LinByte/VoiceServer/pkg/voice/gateway"
)

// ResolveRecordingDurationSec returns whole seconds from WAV bytes (header parse),
// else from stored object key, else from recorder sample-derived DurationMs.
// Never uses wall-clock BYE timestamps.
func ResolveRecordingDurationSec(info gateway.RecordingInfo, wav []byte) int {
	if sec := WAVDurationSec(wav); sec > 0 {
		return sec
	}
	key := strings.TrimSpace(info.Key)
	if key != "" {
		if b, err := readFromStore(key); err == nil && len(b) > 0 {
			if sec := WAVDurationSec(b); sec > 0 {
				return sec
			}
		}
	}
	if info.DurationMs > 0 {
		sec := int((info.DurationMs + 500) / 1000)
		if sec > 0 {
			return sec
		}
	}
	return 0
}
