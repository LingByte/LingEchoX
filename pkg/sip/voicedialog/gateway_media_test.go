package voicedialog

import "testing"

func TestGatewayTTSCloudSR(t *testing.T) {
	if n := gatewayTTSCloudSR(24000, 8000); n != 24000 {
		t.Fatal(n)
	}
	if n := gatewayTTSCloudSR(0, 8000); n != 8000 {
		t.Fatal(n)
	}
	if n := gatewayTTSCloudSR(0, 0); n != 16000 {
		t.Fatal(n)
	}
}

func TestPcmBridgeHz(t *testing.T) {
	if n := pcmBridgeHz(nil); n != 16000 {
		t.Fatal(n)
	}
}
