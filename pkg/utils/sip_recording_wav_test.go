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

// TestSN3_G711Stereo_JitterSnap_ContiguousTTS asserts that consecutive AI/TTS
// frames whose wallNs has ±1-3ms scheduler jitter (the realistic Pipeline
// PaceRealtime case) are concatenated back-to-back in the output WAV instead
// of getting zero-padded between every 20ms frame — the latter was the root
// cause of the user-reported "入库录音电流音" (2026-05-16).
//
// We simulate 10 contiguous AI frames at "20ms ± 2ms" intervals; with the
// jitter snap, the total PCM timeline should be ~ exactly 10 * 160 = 1600
// samples (zero padding bytes inside the stereo data == no clicks).
func TestSN3_G711Stereo_JitterSnap_ContiguousTTS(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("SN3")
	pcm160 := bytes.Repeat([]byte{0x7f}, 160) // non-zero PCMU silence-ish; decodes to a non-zero int16
	appendSN3 := func(dir byte, seq uint16, rtpTs uint32, wallNs uint64, payload []byte) {
		buf.WriteByte(dir)
		_ = binary.Write(&buf, binary.LittleEndian, seq)
		_ = binary.Write(&buf, binary.LittleEndian, rtpTs)
		_ = binary.Write(&buf, binary.LittleEndian, wallNs)
		_ = binary.Write(&buf, binary.LittleEndian, uint16(len(payload)))
		buf.Write(payload)
	}
	// Frames 0..9, each "20ms ± 2ms" apart in wall clock — typical Pipeline jitter.
	jitter := []int64{0, 18, 22, 19, 21, 20, 23, 17, 21, 20}
	var t0 uint64
	for i, gap := range jitter {
		t0 += uint64(gap) * 1_000_000
		appendSN3(recTagAILeg, uint16(i+1), uint32(i*160), t0, pcm160)
	}
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
	// 10 contiguous frames × 160 samples = 1600. Allow ±a couple samples of
	// timeline rounding slack. Pre-fix this number was ~1600 + ~200 zero pad.
	if nStereoSamples < 1590 || nStereoSamples > 1650 {
		t.Fatalf("jitter snap not applied or too aggressive: got %d samples, want ~1600", nStereoSamples)
	}
	// Critically: there must be NO long run of zero bytes inside the AI (right)
	// channel data. A zero-pad between every 20ms frame would produce 9 islands
	// of zeros each <= a few samples — we scan for any 4+ consecutive zero
	// stereo-sample-frames on the R channel.
	dataStart := idx + 8
	dataEnd := dataStart + dataSize
	const sampleStride = 4 // L lo, L hi, R lo, R hi
	zeroRun := 0
	for off := dataStart; off+sampleStride <= dataEnd; off += sampleStride {
		rLo := wav[off+2]
		rHi := wav[off+3]
		if rLo == 0 && rHi == 0 {
			zeroRun++
			if zeroRun >= 4 {
				t.Fatalf("found %d+ consecutive zero stereo samples on R channel at offset %d — jitter snap regression", zeroRun, off)
			}
		} else {
			zeroRun = 0
		}
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
