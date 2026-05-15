package voicedialog

import "testing"

func TestGatewayTTSCloudSR(t *testing.T) {
	// Explicit override always wins.
	if n := gatewayTTSCloudSR(24000, 8000); n != 24000 {
		t.Fatal(n)
	}
	// Default: cloud == bridge to avoid local resampling. We don't have a quality
	// anti-alias LPF, so any 16k→8k decimation aliases 4-8 kHz content into 0-4 kHz
	// as audible hiss. Confirmed via SIP_TTS_RAW_DUMP_DIR diagnostic on 2026-05-16:
	// 16k upstream PCM is clean → noise was introduced by our downsampler.
	if n := gatewayTTSCloudSR(0, 8000); n != 8000 {
		t.Fatalf("G.711 bridge should request 8k cloud (no resample), got %d", n)
	}
	if n := gatewayTTSCloudSR(0, 16000); n != 16000 {
		t.Fatal(n)
	}
	if n := gatewayTTSCloudSR(0, 48000); n != 48000 {
		t.Fatal(n)
	}
	// No bridge info → 16k default.
	if n := gatewayTTSCloudSR(0, 0); n != 16000 {
		t.Fatal(n)
	}
}

func TestPcmBridgeHz(t *testing.T) {
	if n := pcmBridgeHz(nil); n != 16000 {
		t.Fatal(n)
	}
}
