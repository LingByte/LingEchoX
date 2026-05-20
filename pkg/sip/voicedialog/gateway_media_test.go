package voicedialog

import "testing"

func TestGatewayTTSCloudSR(t *testing.T) {
	// Explicit tenant override (ttsConfig.sampleRate) always wins.
	if n := gatewayTTSCloudSR(24000, 16000, 8000); n != 24000 {
		t.Fatalf("tenant override should win, got %d", n)
	}
	// No tenant override → trust the synthesizer's reported rate. Aliyun
	// Qwen-TTS realtime emits 24 kHz natively; we should request that and
	// resample downstream rather than asking the cloud for 8 kHz it can't
	// produce.
	if n := gatewayTTSCloudSR(0, 24000, 8000); n != 24000 {
		t.Fatalf("synth-native should win when tenant unset, got %d", n)
	}
	// No tenant + no synth hint: cloud == bridge to avoid local resampling.
	// We don't ship a quality anti-alias LPF, so any 16k→8k decimation
	// aliases 4-8 kHz content into 0-4 kHz as audible hiss. QCloud's 8 kHz
	// model is trained for the telephony band and avoids this entirely.
	if n := gatewayTTSCloudSR(0, 0, 8000); n != 8000 {
		t.Fatalf("G.711 bridge should request 8k cloud (no resample), got %d", n)
	}
	if n := gatewayTTSCloudSR(0, 0, 16000); n != 16000 {
		t.Fatal(n)
	}
	if n := gatewayTTSCloudSR(0, 0, 48000); n != 48000 {
		t.Fatal(n)
	}
	// No info at all → 16k default.
	if n := gatewayTTSCloudSR(0, 0, 0); n != 16000 {
		t.Fatal(n)
	}
}

func TestPcmBridgeHz(t *testing.T) {
	if n := pcmBridgeHz(nil); n != 16000 {
		t.Fatal(n)
	}
}
