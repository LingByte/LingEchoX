package voicedialog

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
)

// le16 builds a little-endian PCM16 buffer from int16 samples.
func le16(samples ...int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(s))
	}
	return b
}

// readLE16 decodes a PCM16 LE byte slice into int16 samples.
func readLE16(b []byte) []int16 {
	if len(b)%2 != 0 {
		panic("odd byte count")
	}
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out
}

// TestStreamingHalveDecimate16to8_ChunkBoundary verifies that splitting an input PCM stream
// into many small chunks produces IDENTICAL output to feeding it as one big chunk. This is
// the core property that the broken InterpolatingConverter lacked (chunk-boundary phase
// reset → broadband click train heard as "电流音").
func TestStreamingHalveDecimate16to8_ChunkBoundary(t *testing.T) {
	// 64 samples = 128 bytes, easily splittable into many odd-size chunks.
	want := make([]int16, 64)
	for i := range want {
		want[i] = int16(i*500 - 16000) // some signal that varies
	}
	wantPCM := le16(want...)

	// Reference: feed all bytes at once.
	refSvc := &chanReplayService{}
	refOut := refSvc.streamingHalveDecimate16to8(wantPCM)

	// Variant: split at every odd boundary that would split a sample-pair.
	// We feed 1, 3, 7, then rest.
	splits := [][]int{
		{1, 3, 7},
		{3, 5, 11, 17},
		{2, 4, 6, 8, 10, 12, 14, 16}, // pair-aligned
		{5, 9, 13, 21, 33, 49},
	}
	for _, sp := range splits {
		var got []byte
		streamSvc := &chanReplayService{}
		offset := 0
		for _, end := range sp {
			if end > len(wantPCM) {
				end = len(wantPCM)
			}
			if end <= offset {
				continue
			}
			got = append(got, streamSvc.streamingHalveDecimate16to8(wantPCM[offset:end])...)
			offset = end
		}
		if offset < len(wantPCM) {
			got = append(got, streamSvc.streamingHalveDecimate16to8(wantPCM[offset:])...)
		}
		if !bytes.Equal(got, refOut) {
			t.Fatalf("split %v: streaming output differs from one-shot output\n got=%v\nwant=%v",
				sp, readLE16(got), readLE16(refOut))
		}
	}
}

// TestStreamingHalveDecimate16to8_AverageCorrect verifies the averaging math.
func TestStreamingHalveDecimate16to8_AverageCorrect(t *testing.T) {
	c := &chanReplayService{}
	in := le16(100, 200, -100, -300, 1000, 2000)
	got := readLE16(c.streamingHalveDecimate16to8(in))
	// (100+200)/2=150, (-100+-300)/2=-200, (1000+2000)/2=1500
	want := []int16{150, -200, 1500}
	if !equalI16(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestStream2to1Decimate_FullPipeline runs the channel-driven decimation end-to-end.
func TestStream2to1Decimate_FullPipeline(t *testing.T) {
	in := le16(10, 20, 30, 40, 50, 60, 70, 80) // 8 samples → 4 output
	ch := make(chan []byte, 4)
	// Feed the input as 3 unevenly-sized chunks (pair-misaligned).
	ch <- in[:3]   // 1.5 samples
	ch <- in[3:9]  // 3 samples
	ch <- in[9:16] // 3.5 samples
	close(ch)
	stop := make(chan struct{})
	c := &chanReplayService{ch: ch, ctxBlockChan: stop, cloudSR: 16000, bridgeSR: 8000}

	var got []byte
	err := c.SynthesizeStream(context.Background(), "", func(out []byte) error {
		got = append(got, out...)
		return nil
	})
	if err != nil {
		t.Fatalf("SynthesizeStream err: %v", err)
	}
	want := []int16{15, 35, 55, 75}
	gotS := readLE16(got)
	if !equalI16(gotS, want) {
		t.Fatalf("got %v want %v", gotS, want)
	}
}

func equalI16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
