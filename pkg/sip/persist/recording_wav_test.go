package persist

import (
	"testing"
)

func TestWAVDurationSec_Stereo8k(t *testing.T) {
	// Minimal valid stereo 8kHz 16-bit PCM WAV: 1 second of silence.
	// byteRate = 32000, dataSize = 32000
	wav := buildTestWAV(32000, 32000)
	if got := WAVDurationSec(wav); got != 1 {
		t.Fatalf("want 1 sec got %d", got)
	}
}

func TestWAVDurationSec_Invalid(t *testing.T) {
	if got := WAVDurationSec([]byte("not wav")); got != 0 {
		t.Fatalf("want 0 got %d", got)
	}
}

func TestRecordingStorageKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://cdn.example.com/sip/recordings/foo.wav", "sip/recordings/foo.wav"},
		{"/uploads/sip/recordings/foo.wav", "sip/recordings/foo.wav"},
		{"sip/recordings/foo.wav", "sip/recordings/foo.wav"},
	}
	for _, c := range cases {
		if got := recordingStorageKey(c.in); got != c.want {
			t.Fatalf("recordingStorageKey(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func buildTestWAV(byteRate, dataSize uint32) []byte {
	wav := make([]byte, 44+int(dataSize))
	copy(wav[0:4], "RIFF")
	// file size - 8
	fileSize := uint32(36 + dataSize)
	wav[4] = byte(fileSize)
	wav[5] = byte(fileSize >> 8)
	wav[6] = byte(fileSize >> 16)
	wav[7] = byte(fileSize >> 24)
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	wav[16] = 16 // chunk size
	wav[20] = 1  // PCM
	wav[22] = 2  // stereo
	wav[24] = 0x40
	wav[25] = 0x1f // 8000 Hz
	wav[28] = byte(byteRate)
	wav[29] = byte(byteRate >> 8)
	wav[30] = byte(byteRate >> 16)
	wav[31] = byte(byteRate >> 24)
	copy(wav[36:40], "data")
	wav[40] = byte(dataSize)
	wav[41] = byte(dataSize >> 8)
	wav[42] = byte(dataSize >> 16)
	wav[43] = byte(dataSize >> 24)
	return wav
}
