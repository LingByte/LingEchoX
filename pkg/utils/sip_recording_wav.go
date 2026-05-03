// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// SIP call recording: tagged G.711/G.722/Opus payloads → mono or stereo WAV (sip call persistence).
package utils

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/media/encoder"
	"github.com/hraban/opus"
)

// recMixPeakThreshold mirrors sip/server recording mix: gentle peak scaling before hard clip.
const recMixPeakThreshold = 0.92

func floatToPCM16Clamped(v float64) int16 {
	if v > 1 {
		v = 1
	} else if v < -1 {
		v = -1
	}
	x := int(math.Round(v * 32767))
	if x > 32767 {
		x = 32767
	}
	if x < -32768 {
		x = -32768
	}
	return int16(x)
}

func peakNormalizeInterleavedStereo(lr []int16) {
	if len(lr) < 2 {
		return
	}
	peak := 1e-12
	for i := 0; i < len(lr); i++ {
		if x := math.Abs(float64(lr[i]) / 32768.0); x > peak {
			peak = x
		}
	}
	scale := 1.0
	if peak > recMixPeakThreshold {
		scale = recMixPeakThreshold / peak
	}
	for i := 0; i < len(lr); i++ {
		lr[i] = floatToPCM16Clamped(float64(lr[i]) / 32768.0 * scale)
	}
}

// Recording dir tags must match pkg/sip/session.CallSession (recDirUser=0, recDirAI=1).
const (
	recTagUserLeg byte = 0
	recTagAILeg   byte = 1
)

// sn2PCMTrackSeg is one decoded chunk placed by RTP timestamp on a PCM timeline.
type sn2PCMTrackSeg struct {
	ts  uint32
	seq uint16
	pcm []int16
}

func tsDeltaSigned(base, ts uint32) int64 {
	d := int64(ts) - int64(base)
	if d < -(1 << 31) {
		d += 1 << 32
	}
	if d > (1 << 31) {
		d -= 1 << 32
	}
	return d
}

// placeSN2PCMTrack lays PCM segments on a timeline: sample index = rtpTicks * pcmHz / rtpClockHz.
// Each leg uses its own minimum RTP ts as origin so silence gaps between RTP bursts are preserved.
func placeSN2PCMTrack(segs []sn2PCMTrackSeg, rtpClockHz, pcmHz int) []int16 {
	if len(segs) == 0 || rtpClockHz <= 0 || pcmHz <= 0 {
		return nil
	}
	base := segs[0].ts
	for _, s := range segs[1:] {
		if s.ts < base {
			base = s.ts
		}
	}
	sort.Slice(segs, func(i, j int) bool {
		di := tsDeltaSigned(base, segs[i].ts)
		dj := tsDeltaSigned(base, segs[j].ts)
		if di != dj {
			return di < dj
		}
		return segs[i].seq < segs[j].seq
	})
	var out []int16
	for _, s := range segs {
		delta := tsDeltaSigned(base, s.ts)
		pos := int(delta * int64(pcmHz) / int64(rtpClockHz))
		if pos < 0 {
			pos = 0
		}
		end := pos + len(s.pcm)
		if len(out) < end {
			out = append(out, make([]int16, end-len(out))...)
		}
		copy(out[pos:end], s.pcm)
	}
	return out
}

func sn2StereoInterleaveWAV(left, right []int16, pcmHz int) []byte {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	if n == 0 {
		return nil
	}
	lr := make([]int16, 2*n)
	for i := 0; i < len(left) && i < n; i++ {
		lr[2*i] = left[i]
	}
	for i := 0; i < len(right) && i < n; i++ {
		lr[2*i+1] = right[i]
	}
	peakNormalizeInterleavedStereo(lr)
	return pcm16StereoInterleavedToWav(lr, pcmHz)
}

// recTaggedFrame is one SN2/SN3 payload frame after the magic.
type recTaggedFrame struct {
	Dir     byte
	Seq     uint16
	RTPTs   uint32
	WallNs  uint64 // SN2: 0 (RTP timeline only); SN3: nanoseconds since first frame on this CallSession
	Payload []byte
}

func parseRecordingTaggedFrames(blob []byte) []recTaggedFrame {
	if len(blob) < 3 {
		return nil
	}
	switch string(blob[:3]) {
	case "SN3":
		return parseSN3TaggedFrames(blob[3:])
	case "SN2":
		return parseSN2TaggedFrames(blob[3:])
	default:
		return nil
	}
}

func parseSN2TaggedFrames(body []byte) []recTaggedFrame {
	var out []recTaggedFrame
	for len(body) >= 9 {
		dir := body[0]
		seq := binary.LittleEndian.Uint16(body[1:3])
		ts := binary.LittleEndian.Uint32(body[3:7])
		n := int(binary.LittleEndian.Uint16(body[7:9]))
		body = body[9:]
		if n <= 0 || n > 4000 || len(body) < n {
			break
		}
		p := append([]byte(nil), body[:n]...)
		body = body[n:]
		out = append(out, recTaggedFrame{Dir: dir, Seq: seq, RTPTs: ts, WallNs: 0, Payload: p})
	}
	return out
}

func parseSN3TaggedFrames(body []byte) []recTaggedFrame {
	var out []recTaggedFrame
	for len(body) >= 17 {
		dir := body[0]
		seq := binary.LittleEndian.Uint16(body[1:3])
		ts := binary.LittleEndian.Uint32(body[3:7])
		wall := binary.LittleEndian.Uint64(body[7:15])
		n := int(binary.LittleEndian.Uint16(body[15:17]))
		body = body[17:]
		if n <= 0 || n > 4000 || len(body) < n {
			break
		}
		p := append([]byte(nil), body[:n]...)
		body = body[n:]
		out = append(out, recTaggedFrame{Dir: dir, Seq: seq, RTPTs: ts, WallNs: wall, Payload: p})
	}
	return out
}

type wallPCMSeg struct {
	wallNs uint64
	seq    uint16
	pcm    []int16
}

// placeWallPCMTrack maps capture-time offsets to samples (same idea as NEWLINGECHO server/recording.go timeline mix).
func placeWallPCMTrack(segs []wallPCMSeg, pcmHz int) []int16 {
	if len(segs) == 0 || pcmHz <= 0 {
		return nil
	}
	sort.Slice(segs, func(i, j int) bool {
		if segs[i].wallNs != segs[j].wallNs {
			return segs[i].wallNs < segs[j].wallNs
		}
		return segs[i].seq < segs[j].seq
	})
	base := segs[0].wallNs
	var out []int16
	pen := 0
	for _, s := range segs {
		posFromWall := int(int64(s.wallNs-base) * int64(pcmHz) / 1e9)
		if posFromWall < pen {
			posFromWall = pen
		}
		end := posFromWall + len(s.pcm)
		if len(out) < end {
			out = append(out, make([]int16, end-len(out))...)
		}
		copy(out[posFromWall:], s.pcm)
		if end > pen {
			pen = end
		}
	}
	return out
}

// recordingMonoDuckUserFrames: mono-mix decoders used to drop uplink for N frames after each AI
// packet to reduce interleave stutter; for call archival that removes the caller whenever TTS plays.
// Keep at 0 so mono/stereo WAV reconstructions retain both legs.
const recordingMonoDuckUserFrames = 0

// G711TaggedRecordingToWav decodes tagged SIP recordings:
// - v3: "SN3" + wall-clock ns per frame (see pkg/sip/session CallSession)
// - v2: "SN2" + [dir u8][seq u16LE][ts u32LE][len u16LE][payload]
// - v1: "SN1" + [dir u8][len u16LE][payload]
// otherwise raw G.711 pass-through.
func G711TaggedRecordingToWav(b []byte, codec string) []byte {
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '3' {
		return g711TaggedFramesV3ToPcmWav(b[3:], codec)
	}
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '2' {
		return g711TaggedFramesV2ToPcmWav(b[3:], codec)
	}
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '1' {
		return g711TaggedFramesToPcmWav(b[3:], codec)
	}
	return G711PayloadsToWav(b, codec)
}

func g711TaggedFramesV2ToPcmWav(b []byte, codec string) []byte {
	c := strings.ToLower(strings.TrimSpace(codec))
	var pcm []int16
	aiPriorityLeft := 0
	for len(b) >= 9 {
		dir := b[0]
		n := int(binary.LittleEndian.Uint16(b[7:9]))
		b = b[9:]
		if n <= 0 || n > 2000 || len(b) < n {
			break
		}
		chunk := b[:n]
		b = b[n:]
		if dir == 1 {
			aiPriorityLeft = recordingMonoDuckUserFrames
		} else if aiPriorityLeft > 0 {
			aiPriorityLeft--
			continue
		}
		var decoded []int16
		if strings.Contains(c, "pcma") {
			decoded = decodeALaw(chunk)
		} else {
			decoded = decodeMuLaw(chunk)
		}
		pcm = append(pcm, decoded...)
	}
	return pcm16MonoToWav(pcm, 8000)
}

func g711TaggedFramesV3ToPcmWav(body []byte, codec string) []byte {
	frames := parseSN3TaggedFrames(body)
	sort.Slice(frames, func(i, j int) bool {
		if frames[i].WallNs != frames[j].WallNs {
			return frames[i].WallNs < frames[j].WallNs
		}
		return frames[i].Seq < frames[j].Seq
	})
	c := strings.ToLower(strings.TrimSpace(codec))
	var pcm []int16
	aiPriorityLeft := 0
	for _, fr := range frames {
		dir := fr.Dir
		chunk := fr.Payload
		if len(chunk) == 0 {
			continue
		}
		if dir == recTagAILeg {
			aiPriorityLeft = recordingMonoDuckUserFrames
		} else if aiPriorityLeft > 0 {
			aiPriorityLeft--
			continue
		}
		var decoded []int16
		if strings.Contains(c, "pcma") {
			decoded = decodeALaw(chunk)
		} else {
			decoded = decodeMuLaw(chunk)
		}
		pcm = append(pcm, decoded...)
	}
	return pcm16MonoToWav(pcm, 8000)
}

func g711TaggedFramesToPcmWav(b []byte, codec string) []byte {
	c := strings.ToLower(strings.TrimSpace(codec))
	var pcm []int16
	aiPriorityLeft := 0
	for len(b) >= 3 {
		dir := b[0]
		n := int(binary.LittleEndian.Uint16(b[1:3]))
		b = b[3:]
		if n <= 0 || n > 2000 || len(b) < n {
			break
		}
		chunk := b[:n]
		b = b[n:]
		if dir == 1 {
			aiPriorityLeft = recordingMonoDuckUserFrames
		} else if aiPriorityLeft > 0 {
			aiPriorityLeft--
			continue
		}
		var decoded []int16
		if strings.Contains(c, "pcma") {
			decoded = decodeALaw(chunk)
		} else {
			decoded = decodeMuLaw(chunk)
		}
		pcm = append(pcm, decoded...)
	}
	return pcm16MonoToWav(pcm, 8000)
}

// G711PayloadsToWav builds 8kHz mono WAV from concatenated PCMU or PCMA RTP payloads.
func G711PayloadsToWav(payloads []byte, codec string) []byte {
	if len(payloads) == 0 {
		return nil
	}
	c := strings.ToLower(strings.TrimSpace(codec))
	var pcm []int16
	if strings.Contains(c, "pcma") {
		pcm = decodeALaw(payloads)
	} else {
		pcm = decodeMuLaw(payloads)
	}
	return pcm16MonoToWav(pcm, 8000)
}

// G722TaggedRecordingToWav decodes SN3/SN2/SN1-tagged G.722 RTP payloads to 16 kHz mono WAV (wideband).
func G722TaggedRecordingToWav(b []byte) []byte {
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '3' {
		return g722TaggedFramesV3ToPcmWav(b[3:])
	}
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '2' {
		return g722TaggedFramesV2ToPcmWav(b[3:])
	}
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '1' {
		return g722TaggedFramesV1ToPcmWav(b[3:])
	}
	return g722RawPayloadsToWav(b)
}

func g722PCMBytesToMono16(pcmBytes []byte) []int16 {
	out := make([]int16, 0, len(pcmBytes)/4)
	for i := 0; i+3 < len(pcmBytes); i += 4 {
		s1 := int16(binary.LittleEndian.Uint16(pcmBytes[i:]))
		s2 := int16(binary.LittleEndian.Uint16(pcmBytes[i+2:]))
		out = append(out, int16((int32(s1)+int32(s2))/2))
	}
	return out
}

func g722TaggedFramesV2ToPcmWav(b []byte) []byte {
	decUser := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	decAI := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	var pcm []int16
	aiPriorityLeft := 0
	for len(b) >= 9 {
		dir := b[0]
		n := int(binary.LittleEndian.Uint16(b[7:9]))
		b = b[9:]
		if n <= 0 || n > 2000 || len(b) < n {
			break
		}
		chunk := b[:n]
		b = b[n:]
		if dir == recTagAILeg {
			aiPriorityLeft = recordingMonoDuckUserFrames
		} else if aiPriorityLeft > 0 {
			aiPriorityLeft--
			continue
		}
		dec := decUser
		switch dir {
		case recTagUserLeg:
			dec = decUser
		case recTagAILeg:
			dec = decAI
		default:
			continue
		}
		pcmBytes := dec.Decode(chunk)
		pcm = append(pcm, g722PCMBytesToMono16(pcmBytes)...)
	}
	return pcm16MonoToWav(pcm, 16000)
}

type g722WallPkt struct {
	seq    uint16
	wallNs uint64
	pkt    []byte
}

func decodeG722LegWallTimeline(pkts []g722WallPkt) []wallPCMSeg {
	if len(pkts) == 0 {
		return nil
	}
	sort.Slice(pkts, func(i, j int) bool { return pkts[i].seq < pkts[j].seq })
	dec := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	var out []wallPCMSeg
	for _, p := range pkts {
		pcmBytes := dec.Decode(p.pkt)
		m := g722PCMBytesToMono16(pcmBytes)
		if len(m) == 0 {
			continue
		}
		out = append(out, wallPCMSeg{wallNs: p.wallNs, seq: p.seq, pcm: m})
	}
	return out
}

func g722TaggedFramesV3ToPcmWav(body []byte) []byte {
	frames := parseSN3TaggedFrames(body)
	var ufr, afr []recTaggedFrame
	for _, f := range frames {
		switch f.Dir {
		case recTagUserLeg:
			ufr = append(ufr, f)
		case recTagAILeg:
			afr = append(afr, f)
		default:
		}
	}
	uw := decodeG722LegWallTimeline(recFramesToG722WallPkts(ufr))
	aw := decodeG722LegWallTimeline(recFramesToG722WallPkts(afr))
	L := placeWallPCMTrack(uw, 16000)
	R := placeWallPCMTrack(aw, 16000)
	if len(L) == 0 && len(R) == 0 {
		return nil
	}
	return pcm16MonoToWav(mixLRToMonoSameLength(L, R), 16000)
}

func recFramesToG722WallPkts(leg []recTaggedFrame) []g722WallPkt {
	out := make([]g722WallPkt, 0, len(leg))
	for _, f := range leg {
		out = append(out, g722WallPkt{seq: f.Seq, wallNs: f.WallNs, pkt: append([]byte(nil), f.Payload...)})
	}
	return out
}

func mixLRToMonoSameLength(L, R []int16) []int16 {
	n := len(L)
	if len(R) > n {
		n = len(R)
	}
	if n == 0 {
		return nil
	}
	mix := make([]float64, n)
	for i := 0; i < n; i++ {
		var a, b float64
		if i < len(L) {
			a = float64(L[i]) / 32768.0
		}
		if i < len(R) {
			b = float64(R[i]) / 32768.0
		}
		mix[i] = (a + b) * 0.5
	}
	peak := 1e-12
	for _, v := range mix {
		if x := math.Abs(v); x > peak {
			peak = x
		}
	}
	scale := 1.0
	if peak > recMixPeakThreshold {
		scale = recMixPeakThreshold / peak
	}
	out := make([]int16, n)
	for i, v := range mix {
		out[i] = floatToPCM16Clamped(v * scale)
	}
	return out
}

func g722TaggedFramesV1ToPcmWav(b []byte) []byte {
	decUser := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	decAI := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	var pcm []int16
	aiPriorityLeft := 0
	for len(b) >= 3 {
		dir := b[0]
		n := int(binary.LittleEndian.Uint16(b[1:3]))
		b = b[3:]
		if n <= 0 || n > 2000 || len(b) < n {
			break
		}
		chunk := b[:n]
		b = b[n:]
		if dir == recTagAILeg {
			aiPriorityLeft = recordingMonoDuckUserFrames
		} else if aiPriorityLeft > 0 {
			aiPriorityLeft--
			continue
		}
		dec := decUser
		switch dir {
		case recTagUserLeg:
			dec = decUser
		case recTagAILeg:
			dec = decAI
		default:
			continue
		}
		pcmBytes := dec.Decode(chunk)
		pcm = append(pcm, g722PCMBytesToMono16(pcmBytes)...)
	}
	return pcm16MonoToWav(pcm, 16000)
}

func g722RawPayloadsToWav(payloads []byte) []byte {
	if len(payloads) == 0 {
		return nil
	}
	dec := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	return pcm16MonoToWav(g722PCMBytesToMono16(dec.Decode(payloads)), 16000)
}

func decodeMuLaw(in []byte) []int16 {
	out := make([]int16, len(in))
	for i, b := range in {
		out[i] = ulawToLinear(b)
	}
	return out
}

func decodeALaw(in []byte) []int16 {
	out := make([]int16, len(in))
	for i, b := range in {
		out[i] = alawToLinear(b)
	}
	return out
}

func ulawToLinear(u uint8) int16 {
	u = ^u
	sign := (u & 0x80)
	exponent := (u >> 4) & 0x07
	mantissa := u & 0x0F
	sample := int32(mantissa<<4) + 0x08
	sample <<= uint(exponent + 3)
	sample -= 0x84
	if sign != 0 {
		sample = -sample
	}
	if sample > 32767 {
		sample = 32767
	}
	if sample < -32768 {
		sample = -32768
	}
	return int16(sample)
}

func alawToLinear(a uint8) int16 {
	a ^= 0x55
	t := int32((a & 0x0F) << 4)
	seg := (a >> 4) & 0x07
	switch seg {
	case 0:
		t += 8
	case 1:
		t += 0x108
	default:
		t += 0x108
		t <<= uint(seg - 1)
	}
	if a&0x80 == 0 {
		t = -t
	}
	if t > 32767 {
		t = 32767
	}
	if t < -32768 {
		t = -32768
	}
	return int16(t)
}

func pcm16MonoToWav(samples []int16, sampleRate int) []byte {
	if len(samples) == 0 {
		return nil
	}
	dataSize := len(samples) * 2
	buf := &bytes.Buffer{}
	_, _ = buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	_, _ = buf.WriteString("WAVE")
	_, _ = buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	_, _ = buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	for _, s := range samples {
		_ = binary.Write(buf, binary.LittleEndian, s)
	}
	return buf.Bytes()
}

// pcm16StereoInterleavedToWav builds 16-bit stereo WAV from L,R,L,R,... interleaved samples.
func pcm16StereoInterleavedToWav(lr []int16, sampleRate int) []byte {
	if len(lr) == 0 || len(lr)%2 != 0 {
		return nil
	}
	dataSize := len(lr) * 2
	buf := &bytes.Buffer{}
	_, _ = buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	_, _ = buf.WriteString("WAVE")
	_, _ = buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(2)) // stereo
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(uint32(sampleRate)*4)) // byte rate
	_ = binary.Write(buf, binary.LittleEndian, uint16(4))                     // block align
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	_, _ = buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	for _, s := range lr {
		_ = binary.Write(buf, binary.LittleEndian, s)
	}
	return buf.Bytes()
}

func g711DecodeChunk(codec string, chunk []byte) []int16 {
	c := strings.ToLower(strings.TrimSpace(codec))
	if strings.Contains(c, "pcma") {
		return decodeALaw(chunk)
	}
	return decodeMuLaw(chunk)
}

// G711TaggedRecordingToStereoWav builds stereo WAV: L=user leg, R=AI leg (SN3 preferred, SN2 legacy).
// SN3 uses capture-time wallNs (real gaps between TTS phrases); SN2 falls back to RTP timestamp placement.
func G711TaggedRecordingToStereoWav(b []byte, codec string) []byte {
	frames := parseRecordingTaggedFrames(b)
	if len(frames) == 0 {
		return nil
	}
	useWall := len(b) >= 3 && b[2] == '3'
	var userWall, aiWall []wallPCMSeg
	var userSN2, aiSN2 []sn2PCMTrackSeg
	for _, f := range frames {
		if f.Dir != recTagUserLeg && f.Dir != recTagAILeg {
			continue
		}
		dec := g711DecodeChunk(codec, f.Payload)
		if len(dec) == 0 {
			continue
		}
		if useWall {
			s := wallPCMSeg{wallNs: f.WallNs, seq: f.Seq, pcm: dec}
			if f.Dir == recTagUserLeg {
				userWall = append(userWall, s)
			} else {
				aiWall = append(aiWall, s)
			}
			continue
		}
		s := sn2PCMTrackSeg{ts: f.RTPTs, seq: f.Seq, pcm: dec}
		if f.Dir == recTagUserLeg {
			userSN2 = append(userSN2, s)
		} else {
			aiSN2 = append(aiSN2, s)
		}
	}
	var L, R []int16
	if useWall {
		L = placeWallPCMTrack(userWall, 8000)
		R = placeWallPCMTrack(aiWall, 8000)
	} else {
		L = placeSN2PCMTrack(userSN2, 8000, 8000)
		R = placeSN2PCMTrack(aiSN2, 8000, 8000)
	}
	if len(L) == 0 && len(R) == 0 {
		return nil
	}
	return sn2StereoInterleaveWAV(L, R, 8000)
}

type g722SN2Pkt struct {
	seq uint16
	ts  uint32
	pkt []byte
}

func decodeG722LegTimeline(pkts []g722SN2Pkt) []sn2PCMTrackSeg {
	if len(pkts) == 0 {
		return nil
	}
	sort.Slice(pkts, func(i, j int) bool { return pkts[i].seq < pkts[j].seq })
	dec := encoder.NewG722Decoder(encoder.G722_RATE_DEFAULT, encoder.G722_DEFAULT)
	var out []sn2PCMTrackSeg
	for _, p := range pkts {
		pcmBytes := dec.Decode(p.pkt)
		m := g722PCMBytesToMono16(pcmBytes)
		if len(m) == 0 {
			continue
		}
		out = append(out, sn2PCMTrackSeg{ts: p.ts, seq: p.seq, pcm: m})
	}
	return out
}

// G722TaggedRecordingToStereoWav is like G711 stereo: L=user, R=AI (SN3 preferred, SN2 legacy).
func G722TaggedRecordingToStereoWav(b []byte) []byte {
	if len(b) < 3 || b[0] != 'S' || b[1] != 'N' {
		return nil
	}
	if b[2] == '3' {
		var userPkts, aiPkts []g722WallPkt
		for _, f := range parseSN3TaggedFrames(b[3:]) {
			switch f.Dir {
			case recTagUserLeg:
				userPkts = append(userPkts, g722WallPkt{seq: f.Seq, wallNs: f.WallNs, pkt: append([]byte(nil), f.Payload...)})
			case recTagAILeg:
				aiPkts = append(aiPkts, g722WallPkt{seq: f.Seq, wallNs: f.WallNs, pkt: append([]byte(nil), f.Payload...)})
			default:
			}
		}
		uw := decodeG722LegWallTimeline(userPkts)
		aw := decodeG722LegWallTimeline(aiPkts)
		L := placeWallPCMTrack(uw, 16000)
		R := placeWallPCMTrack(aw, 16000)
		if len(L) == 0 && len(R) == 0 {
			return nil
		}
		return sn2StereoInterleaveWAV(L, R, 16000)
	}
	if b[2] != '2' {
		return nil
	}
	var userPkts, aiPkts []g722SN2Pkt
	body := b[3:]
	for len(body) >= 9 {
		dir := body[0]
		seq := binary.LittleEndian.Uint16(body[1:3])
		ts := binary.LittleEndian.Uint32(body[3:7])
		n := int(binary.LittleEndian.Uint16(body[7:9]))
		body = body[9:]
		if n <= 0 || n > 2000 || len(body) < n {
			break
		}
		chunk := append([]byte(nil), body[:n]...)
		body = body[n:]
		switch dir {
		case recTagUserLeg:
			userPkts = append(userPkts, g722SN2Pkt{seq: seq, ts: ts, pkt: chunk})
		case recTagAILeg:
			aiPkts = append(aiPkts, g722SN2Pkt{seq: seq, ts: ts, pkt: chunk})
		default:
		}
	}
	userSegs := decodeG722LegTimeline(userPkts)
	aiSegs := decodeG722LegTimeline(aiPkts)
	L := placeSN2PCMTrack(userSegs, 8000, 16000)
	R := placeSN2PCMTrack(aiSegs, 8000, 16000)
	if len(L) == 0 && len(R) == 0 {
		return nil
	}
	return sn2StereoInterleaveWAV(L, R, 16000)
}

// MixedOpusRecordingToStereoWav decodes SN3/SN2 Opus per leg into L=user R=AI stereo WAV.
func MixedOpusRecordingToStereoWav(b []byte, sampleRate, decodeChannels int) []byte {
	if len(b) < 3 || b[0] != 'S' || b[1] != 'N' || (b[2] != '2' && b[2] != '3') {
		return nil
	}
	useWall := b[2] == '3'
	frames := parseRecordingTaggedFrames(b)
	var of []taggedOpusFrame
	for _, f := range frames {
		if f.Dir != recTagUserLeg && f.Dir != recTagAILeg {
			continue
		}
		of = append(of, taggedOpusFrame{dir: f.Dir, seq: f.Seq, ts: f.RTPTs, wallNs: f.WallNs, payload: f.Payload})
	}
	if len(of) == 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if decodeChannels < 1 {
		decodeChannels = 1
	}
	if decodeChannels > 2 {
		decodeChannels = 2
	}
	if useWall {
		var uf, af []taggedOpusFrame
		for _, f := range of {
			switch f.dir {
			case recTagUserLeg:
				uf = append(uf, f)
			case recTagAILeg:
				af = append(af, f)
			}
		}
		uw := opusDecodeLegWallSegments(uf, sampleRate, decodeChannels)
		aw := opusDecodeLegWallSegments(af, sampleRate, decodeChannels)
		L := placeWallPCMTrack(uw, sampleRate)
		R := placeWallPCMTrack(aw, sampleRate)
		if len(L) == 0 && len(R) == 0 {
			return nil
		}
		return sn2StereoInterleaveWAV(L, R, sampleRate)
	}
	return opusFramesPerLegStereoWav(of, sampleRate, decodeChannels)
}

func opusDecodeLegWallSegments(frames []taggedOpusFrame, sampleRate, decodeChannels int) []wallPCMSeg {
	if len(frames) == 0 {
		return nil
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].seq < frames[j].seq })
	dec, err := opus.NewDecoder(sampleRate, decodeChannels)
	if err != nil {
		return nil
	}
	maxSamples := sampleRate * 120 / 1000
	if maxSamples < 960 {
		maxSamples = 960
	}
	pcmScratch := make([]int16, maxSamples*decodeChannels)
	defaultFrameSamples := sampleRate / 50
	if defaultFrameSamples <= 0 {
		defaultFrameSamples = 960
	}
	var out []wallPCMSeg
	var lastSeq uint16
	var hasSeq bool
	for _, f := range frames {
		if hasSeq {
			gap := int(uint16(f.seq - lastSeq))
			if gap > 1 && gap < 20 {
				for i := 0; i < gap-1; i++ {
					_, _ = dec.Decode(nil, pcmScratch)
				}
			}
		}
		lastSeq = f.seq
		hasSeq = true
		samples, derr := dec.Decode(f.payload, pcmScratch)
		if derr != nil || samples <= 0 {
			plcSamples, plcErr := dec.Decode(nil, pcmScratch)
			if plcErr == nil && plcSamples > 0 {
				samples = plcSamples
			} else {
				samples = defaultFrameSamples
				if samples > maxSamples {
					samples = maxSamples
				}
				for i := 0; i < samples*decodeChannels; i++ {
					pcmScratch[i] = 0
				}
			}
		}
		var mono []int16
		if decodeChannels == 2 {
			for i := 0; i < samples; i++ {
				L := int32(pcmScratch[i*2])
				R := int32(pcmScratch[i*2+1])
				mono = append(mono, int16((L+R)/2))
			}
		} else {
			mono = append(mono, pcmScratch[:samples]...)
		}
		out = append(out, wallPCMSeg{wallNs: f.wallNs, seq: f.seq, pcm: mono})
	}
	return out
}

func opusDecodeLegTimelineSegments(frames []taggedOpusFrame, sampleRate, decodeChannels int) []sn2PCMTrackSeg {
	if len(frames) == 0 {
		return nil
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].seq < frames[j].seq })
	dec, err := opus.NewDecoder(sampleRate, decodeChannels)
	if err != nil {
		return nil
	}
	maxSamples := sampleRate * 120 / 1000
	if maxSamples < 960 {
		maxSamples = 960
	}
	pcmScratch := make([]int16, maxSamples*decodeChannels)
	defaultFrameSamples := sampleRate / 50
	if defaultFrameSamples <= 0 {
		defaultFrameSamples = 960
	}
	var out []sn2PCMTrackSeg
	var lastSeq uint16
	var hasSeq bool
	for _, f := range frames {
		if hasSeq {
			gap := int(uint16(f.seq - lastSeq))
			if gap > 1 && gap < 20 {
				for i := 0; i < gap-1; i++ {
					_, _ = dec.Decode(nil, pcmScratch)
				}
			}
		}
		lastSeq = f.seq
		hasSeq = true
		samples, derr := dec.Decode(f.payload, pcmScratch)
		if derr != nil || samples <= 0 {
			plcSamples, plcErr := dec.Decode(nil, pcmScratch)
			if plcErr == nil && plcSamples > 0 {
				samples = plcSamples
			} else {
				samples = defaultFrameSamples
				if samples > maxSamples {
					samples = maxSamples
				}
				for i := 0; i < samples*decodeChannels; i++ {
					pcmScratch[i] = 0
				}
			}
		}
		var mono []int16
		if decodeChannels == 2 {
			for i := 0; i < samples; i++ {
				L := int32(pcmScratch[i*2])
				R := int32(pcmScratch[i*2+1])
				mono = append(mono, int16((L+R)/2))
			}
		} else {
			mono = append(mono, pcmScratch[:samples]...)
		}
		out = append(out, sn2PCMTrackSeg{ts: f.ts, seq: f.seq, pcm: mono})
	}
	return out
}

func opusFramesPerLegStereoWav(frames []taggedOpusFrame, sampleRate, decodeChannels int) []byte {
	if len(frames) == 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if decodeChannels < 1 {
		decodeChannels = 1
	}
	if decodeChannels > 2 {
		decodeChannels = 2
	}
	var userFrames, aiFrames []taggedOpusFrame
	for _, f := range frames {
		switch f.dir {
		case recTagUserLeg:
			userFrames = append(userFrames, f)
		case recTagAILeg:
			aiFrames = append(aiFrames, f)
		}
	}
	userSegs := opusDecodeLegTimelineSegments(userFrames, sampleRate, decodeChannels)
	aiSegs := opusDecodeLegTimelineSegments(aiFrames, sampleRate, decodeChannels)
	L := placeSN2PCMTrack(userSegs, sampleRate, sampleRate)
	R := placeSN2PCMTrack(aiSegs, sampleRate, sampleRate)
	if len(L) == 0 && len(R) == 0 {
		return nil
	}
	return sn2StereoInterleaveWAV(L, R, sampleRate)
}

func appendOpusPCMSlice(dst []int16, pcmScratch []int16, samples int, decodeChannels int) []int16 {
	if decodeChannels == 2 {
		for i := 0; i < samples; i++ {
			L := int32(pcmScratch[i*2])
			R := int32(pcmScratch[i*2+1])
			dst = append(dst, int16((L+R)/2))
		}
		return dst
	}
	return append(dst, pcmScratch[:samples]...)
}

func MixedOpusRecordingToWav(b []byte, sampleRate, decodeChannels int) []byte {
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '3' {
		return opusTaggedFramesV3ToWav(b[3:], sampleRate, decodeChannels)
	}
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '2' {
		return opusTaggedFramesV2ToWav(b[3:], sampleRate, decodeChannels)
	}
	if len(b) >= 3 && b[0] == 'S' && b[1] == 'N' && b[2] == '1' {
		return opusTaggedFramesToWav(b[3:], sampleRate, decodeChannels)
	}
	return OpusLengthPrefixedPayloadsToWav(b, sampleRate, decodeChannels)
}

type taggedOpusFrame struct {
	dir     byte
	seq     uint16
	ts      uint32
	wallNs  uint64
	payload []byte
}

func opusTaggedFramesV2ToWav(b []byte, sampleRate, decodeChannels int) []byte {
	if len(b) < 9 {
		return nil
	}
	var frames []taggedOpusFrame
	for len(b) >= 9 {
		dir := b[0]
		seq := binary.LittleEndian.Uint16(b[1:3])
		ts := binary.LittleEndian.Uint32(b[3:7])
		n := int(binary.LittleEndian.Uint16(b[7:9]))
		b = b[9:]
		if n <= 0 || n > 4000 || len(b) < n {
			break
		}
		pkt := make([]byte, n)
		copy(pkt, b[:n])
		b = b[n:]
		if dir != recTagUserLeg && dir != recTagAILeg {
			continue
		}
		frames = append(frames, taggedOpusFrame{dir: dir, seq: seq, ts: ts, wallNs: 0, payload: pkt})
	}
	return decodeTaggedOpusFrames(frames, sampleRate, decodeChannels, true)
}

func opusTaggedFramesV3ToWav(b []byte, sampleRate, decodeChannels int) []byte {
	if len(b) < 17 {
		return nil
	}
	var frames []taggedOpusFrame
	for len(b) >= 17 {
		dir := b[0]
		seq := binary.LittleEndian.Uint16(b[1:3])
		ts := binary.LittleEndian.Uint32(b[3:7])
		wall := binary.LittleEndian.Uint64(b[7:15])
		n := int(binary.LittleEndian.Uint16(b[15:17]))
		b = b[17:]
		if n <= 0 || n > 4000 || len(b) < n {
			break
		}
		pkt := make([]byte, n)
		copy(pkt, b[:n])
		b = b[n:]
		if dir != recTagUserLeg && dir != recTagAILeg {
			continue
		}
		frames = append(frames, taggedOpusFrame{dir: dir, seq: seq, ts: ts, wallNs: wall, payload: pkt})
	}
	return decodeTaggedOpusFrames(frames, sampleRate, decodeChannels, true)
}

func opusTaggedFramesToWav(b []byte, sampleRate, decodeChannels int) []byte {
	if len(b) < 3 {
		return nil
	}
	var frames []taggedOpusFrame
	for len(b) >= 3 {
		dir := b[0]
		n := int(binary.LittleEndian.Uint16(b[1:3]))
		b = b[3:]
		if n <= 0 || n > 4000 || len(b) < n {
			break
		}
		pkt := make([]byte, n)
		copy(pkt, b[:n])
		b = b[n:]
		if dir != recTagUserLeg && dir != recTagAILeg {
			continue
		}
		frames = append(frames, taggedOpusFrame{dir: dir, ts: 0, wallNs: 0, payload: pkt})
	}
	return decodeTaggedOpusFrames(frames, sampleRate, decodeChannels, false)
}

func decodeTaggedOpusFrames(frames []taggedOpusFrame, sampleRate, decodeChannels int, withSeq bool) []byte {
	if len(frames) == 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if decodeChannels < 1 {
		decodeChannels = 1
	}
	if decodeChannels > 2 {
		decodeChannels = 2
	}
	// User uplink and AI/TTS downlink are independent RTP streams; a single OpusDecoder
	// keeps cross-packet state, so alternating legs corrupts decode (often the AI leg).
	decUser, errU := opus.NewDecoder(sampleRate, decodeChannels)
	decAI, errA := opus.NewDecoder(sampleRate, decodeChannels)
	if errU != nil || errA != nil {
		return nil
	}
	maxSamples := sampleRate * 120 / 1000
	if maxSamples < 960 {
		maxSamples = 960
	}
	pcmScratch := make([]int16, maxSamples*decodeChannels)
	var mono []int16
	defaultFrameSamples := sampleRate / 50 // 20ms
	if defaultFrameSamples <= 0 {
		defaultFrameSamples = 960
	}
	var lastSeqUser uint16
	var lastSeqAI uint16
	var hasSeqUser bool
	var hasSeqAI bool
	// archival: keep all uplink (see recordingMonoDuckUserFrames).
	aiPriorityLeft := 0
	for _, f := range frames {
		if f.dir == recTagAILeg {
			aiPriorityLeft = recordingMonoDuckUserFrames
		} else if aiPriorityLeft > 0 {
			aiPriorityLeft--
			// Skip uplink frames during short AI-active window to avoid fragmenting AI audio.
			continue
		}

		dec := decUser
		switch f.dir {
		case recTagUserLeg:
			dec = decUser
		case recTagAILeg:
			dec = decAI
		default:
			continue
		}
		if withSeq {
			var gap int
			if f.dir == recTagUserLeg {
				if hasSeqUser {
					gap = int(uint16(f.seq - lastSeqUser))
					if gap > 1 && gap < 20 {
						for i := 0; i < gap-1; i++ {
							if plc, err := dec.Decode(nil, pcmScratch); err == nil && plc > 0 {
								if decodeChannels == 2 {
									for j := 0; j < plc; j++ {
										L := int32(pcmScratch[j*2])
										R := int32(pcmScratch[j*2+1])
										mono = append(mono, int16((L+R)/2))
									}
								} else {
									mono = append(mono, pcmScratch[:plc]...)
								}
							}
						}
					}
				}
				lastSeqUser = f.seq
				hasSeqUser = true
			} else {
				if hasSeqAI {
					gap = int(uint16(f.seq - lastSeqAI))
					if gap > 1 && gap < 20 {
						for i := 0; i < gap-1; i++ {
							if plc, err := dec.Decode(nil, pcmScratch); err == nil && plc > 0 {
								if decodeChannels == 2 {
									for j := 0; j < plc; j++ {
										L := int32(pcmScratch[j*2])
										R := int32(pcmScratch[j*2+1])
										mono = append(mono, int16((L+R)/2))
									}
								} else {
									mono = append(mono, pcmScratch[:plc]...)
								}
							}
						}
					}
				}
				lastSeqAI = f.seq
				hasSeqAI = true
			}
		}
		samples, derr := dec.Decode(f.payload, pcmScratch)
		if derr != nil || samples <= 0 {
			// Packet loss concealment: use decoder PLC to keep timeline continuous.
			plcSamples, plcErr := dec.Decode(nil, pcmScratch)
			if plcErr == nil && plcSamples > 0 {
				samples = plcSamples
			} else {
				samples = defaultFrameSamples
				if samples > maxSamples {
					samples = maxSamples
				}
				// PLC unavailable; fill with silence to avoid timeline collapse.
				for i := 0; i < samples*decodeChannels; i++ {
					pcmScratch[i] = 0
				}
			}
		}
		if decodeChannels == 2 {
			for i := 0; i < samples; i++ {
				L := int32(pcmScratch[i*2])
				R := int32(pcmScratch[i*2+1])
				mono = append(mono, int16((L+R)/2))
			}
		} else {
			mono = append(mono, pcmScratch[:samples]...)
		}
	}
	return pcm16MonoToWav(mono, sampleRate)
}

// OpusLengthPrefixedPayloadsToWav decodes legacy inbound-only [uint16LE len][opus frame]... to mono WAV.
func OpusLengthPrefixedPayloadsToWav(b []byte, sampleRate, decodeChannels int) []byte {
	if len(b) < 2 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if decodeChannels < 1 {
		decodeChannels = 1
	}
	if decodeChannels > 2 {
		decodeChannels = 2
	}
	dec, err := opus.NewDecoder(sampleRate, decodeChannels)
	if err != nil {
		return nil
	}
	maxSamples := sampleRate * 120 / 1000
	if maxSamples < 960 {
		maxSamples = 960
	}
	pcmScratch := make([]int16, maxSamples*decodeChannels)
	var mono []int16
	defaultFrameSamples := sampleRate / 50 // 20ms
	if defaultFrameSamples <= 0 {
		defaultFrameSamples = 960
	}
	for len(b) >= 2 {
		n := int(binary.LittleEndian.Uint16(b[:2]))
		b = b[2:]
		if n <= 0 || n > 4000 || len(b) < n {
			break
		}
		pkt := b[:n]
		b = b[n:]
		samples, derr := dec.Decode(pkt, pcmScratch)
		if derr != nil || samples <= 0 {
			// Packet loss concealment for legacy stream.
			plcSamples, plcErr := dec.Decode(nil, pcmScratch)
			if plcErr == nil && plcSamples > 0 {
				samples = plcSamples
			} else {
				samples = defaultFrameSamples
				if samples > maxSamples {
					samples = maxSamples
				}
				for i := 0; i < samples*decodeChannels; i++ {
					pcmScratch[i] = 0
				}
			}
		}
		if decodeChannels == 2 {
			for i := 0; i < samples; i++ {
				L := int32(pcmScratch[i*2])
				R := int32(pcmScratch[i*2+1])
				mono = append(mono, int16((L+R)/2))
			}
		} else {
			mono = append(mono, pcmScratch[:samples]...)
		}
	}
	return pcm16MonoToWav(mono, sampleRate)
}
