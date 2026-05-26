package webseat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/media"
)

func TestTransport_StringAndNilTracks(t *testing.T) {
	rx := media.CodecConfig{Codec: "opus", SampleRate: 48000}
	tr := NewTransport(nil, nil, rx, rx)
	if !strings.Contains(tr.String(), "opus") {
		t.Fatal(tr.String())
	}
	ctx := context.Background()
	mp, err := tr.Next(ctx)
	if err != nil || mp != nil {
		t.Fatalf("next=%v err=%v", mp, err)
	}
	n, err := tr.Send(ctx, &media.AudioPacket{Payload: []byte{1, 2}})
	if err != nil || n != 0 {
		t.Fatalf("send nil tx: n=%d err=%v", n, err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	tr.WakeupRead()
	tr.Attach(nil)
	if tr.Codec().Codec != "opus" {
		t.Fatal()
	}
}

func TestTransport_NextCancelledContext(t *testing.T) {
	tr := NewTransport(nil, nil, media.CodecConfig{}, media.CodecConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mp, err := tr.Next(ctx)
	if mp != nil || err != nil {
		t.Fatalf("%v %v", mp, err)
	}
}

func TestTransport_SendCancelledContext_NoTxShortCircuit(t *testing.T) {
	tr := NewTransport(nil, nil, media.CodecConfig{}, media.CodecConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := tr.Send(ctx, &media.AudioPacket{Payload: []byte{1}})
	if n != 0 || err != nil {
		t.Fatalf("nil tx ignores ctx: n=%d err=%v", n, err)
	}
}

func TestTransport_SendNonAudioIgnored(t *testing.T) {
	tr := NewTransport(nil, nil, media.CodecConfig{}, media.CodecConfig{})
	n, err := tr.Send(context.Background(), &media.TextPacket{Text: "x"})
	if err != nil || n != 0 {
		t.Fatal()
	}
}

func TestTransport_DurationFromCodecFrameDuration(t *testing.T) {
	tx := media.CodecConfig{
		Codec:         "opus",
		SampleRate:    48000,
		FrameDuration: "40ms",
	}
	tr := NewTransport(nil, nil, media.CodecConfig{}, tx)
	if fd := strings.TrimSpace(tr.TxCodec().FrameDuration); fd != "40ms" {
		t.Fatal(fd)
	}
	_ = time.Millisecond // silence unused in older Go linters
}
