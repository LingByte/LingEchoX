package persist

import (
	"strings"

	siprtp "github.com/LinByte/VoiceServer/pkg/sip/rtp"
	"github.com/LinByte/VoiceServer/pkg/voice/qos"
)

// QoSDBUpdatesFromRTCP builds sip_calls column updates from a one-shot RTCP
// snapshot at BYE time. O(1), no extra RTCP traffic — reads counters only.
func QoSDBUpdatesFromRTCP(codecName string, clockRate int, snap siprtp.RTCPStats) map[string]any {
	if snap.RTTMs == 0 && !snap.PeerSeenRR && snap.LocalPacketsRecv == 0 && snap.LocalJitter == 0 {
		return nil
	}
	if clockRate <= 0 {
		clockRate = qosClockRateForCodec(codecName)
	}
	lossFraction := float64(snap.PeerLossFraction) / 256.0
	jitterMs := float32(float64(snap.LocalJitter) * 1000.0 / float64(clockRate))
	lossPct := float32(lossFraction * 100.0)
	mos := qos.Estimate(qos.MOSInput{
		RTTMs:            snap.RTTMs,
		JitterRTPUnits:   snap.LocalJitter,
		JitterClockRate:  uint32(clockRate),
		PeerLossFraction: lossFraction,
		Codec:            codecName,
	})
	out := map[string]any{
		"qos_jitter_ms":         jitterMs,
		"qos_packet_loss_pct":   lossPct,
		"qos_mos_estimate":      float32(mos.MOS),
	}
	if snap.RTTMs > 0 {
		out["qos_rtt_ms"] = snap.RTTMs
	}
	return out
}

func qosClockRateForCodec(codecName string) int {
	c := strings.ToLower(strings.TrimSpace(codecName))
	switch {
	case strings.Contains(c, "opus"):
		return 48000
	case strings.Contains(c, "g722"):
		return 8000 // RTP clock for G.722
	default:
		return 8000
	}
}
