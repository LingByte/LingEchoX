package utils

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// SN2 stereo must preserve RTP silence between AI bursts (timestamp gaps), not concatenate PCM back-to-back.
func TestG711TaggedRecordingToStereoWav_RTPGapBetweenAIUtterances(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("SN2")
	pcm160 := bytes.Repeat([]byte{0x7f}, 160)
	appendPkt := func(dir byte, seq uint16, ts uint32, payload []byte) {
		buf.WriteByte(dir)
		binary.Write(&buf, binary.LittleEndian, seq)
		binary.Write(&buf, binary.LittleEndian, ts)
		binary.Write(&buf, binary.LittleEndian, uint16(len(payload)))
		buf.Write(payload)
	}
	// User speaks once at ts=0
	appendPkt(recTagUserLeg, 1, 0, pcm160)
	// AI first phrase ts=0..160
	appendPkt(recTagAILeg, 10, 0, pcm160)
	// 200ms gap at 8kHz => +1600 ticks before next AI packet
	appendPkt(recTagAILeg, 11, 1600, pcm160)

	wav := G711TaggedRecordingToStereoWav(buf.Bytes(), "pcmu")
	if len(wav) == 0 {
		t.Fatal("expected wav")
	}
	// PCM data chunk holds stereo int16 samples; gap should insert ~1600 zeros on R channel between bursts.
	// Approximate: at least 160 + 1600 + 160 samples on R if placement worked (minus overlap nuances).
	idx := bytes.Index(wav, []byte("data"))
	if idx < 0 {
		t.Fatal("missing data chunk")
	}
	dataSize := int(binary.LittleEndian.Uint32(wav[idx+4 : idx+8]))
	nStereoSamples := dataSize / 4
	if nStereoSamples < 160+1400 { // allow slack; concat-without-gap would be ~320 stereo samples total length ~640 bytes PCM -> much shorter timeline
		t.Fatalf("stereo timeline too short (gap likely missing): nStereoSamples=%d dataSize=%d", nStereoSamples, dataSize)
	}
}

// SN3 must preserve real pause between TTS phrases even when RTP timestamps stay back-to-back (common for one RTP sender).
func TestSN3_G711Stereo_WallClockGapBetweenAI_RTPContinuous(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("SN3")
	pcm160 := bytes.Repeat([]byte{0x7f}, 160)
	appendSN3 := func(dir byte, seq uint16, rtpTs uint32, wallNs uint64, payload []byte) {
		buf.WriteByte(dir)
		_ = binary.Write(&buf, binary.LittleEndian, seq)
		_ = binary.Write(&buf, binary.LittleEndian, rtpTs)
		_ = binary.Write(&buf, binary.LittleEndian, wallNs)
		_ = binary.Write(&buf, binary.LittleEndian, uint16(len(payload)))
		buf.Write(payload)
	}
	appendSN3(recTagAILeg, 1, 0, 0, pcm160)
	appendSN3(recTagAILeg, 2, 160, 300_000_000, pcm160) // 300ms wall-clock gap

	wav := G711TaggedRecordingToStereoWav(buf.Bytes(), "pcmu")
	if len(wav) == 0 {
		t.Fatal("expected wav")
	}
	idx := bytes.Index(wav, []byte("data"))
	if idx < 0 {
		t.Fatal("missing data chunk")
	}
	dataSize := int(binary.LittleEndian.Uint32(wav[idx+4 : idx+8]))
	nStereoSamples := dataSize / 4
	if nStereoSamples < 2500 {
		t.Fatalf("SN3 wall gap not reflected in stereo length: nStereoSamples=%d dataSize=%d", nStereoSamples, dataSize)
	}
}
