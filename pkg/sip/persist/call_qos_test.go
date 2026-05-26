package persist

import (
	"testing"

	siprtp "github.com/LinByte/VoiceServer/pkg/sip/rtp"
)

func TestQoSDBUpdatesFromRTCP_empty(t *testing.T) {
	if got := QoSDBUpdatesFromRTCP("pcmu", 8000, siprtp.RTCPStats{}); got != nil {
		t.Fatalf("expected nil updates, got %v", got)
	}
}

func TestQoSDBUpdatesFromRTCP_withRTT(t *testing.T) {
	up := QoSDBUpdatesFromRTCP("pcmu", 8000, siprtp.RTCPStats{
		PeerSeenRR:       true,
		RTTMs:            42,
		PeerLossFraction: 10,
		LocalJitter:      8,
	})
	if up == nil {
		t.Fatal("expected updates")
	}
	if up["qos_rtt_ms"] != uint32(42) {
		t.Fatalf("rtt: %v", up["qos_rtt_ms"])
	}
}
