package rtp

import (
	"context"
	"testing"
	"time"

	"github.com/LingByte/SoulNexus/pkg/media"
)

// Regression: large payloads on the telephone-event PT must not skip recording — audio frames were
// mistaken for partial DTMF (EventFromRFC2833 ok on arbitrary bytes), yielding SN2 with AI-only.
func TestSIPRTPTransport_Input_OnInputRTPBeforeDTMFHeuristic(t *testing.T) {
	a, err := NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	a.SetRemoteAddr(b.LocalAddr)
	b.SetRemoteAddr(a.LocalAddr)

	codec := media.CodecConfig{
		Codec:       "pcmu",
		SampleRate:  8000,
		Channels:    1,
		BitDepth:    8,
		PayloadType: 0,
	}
	tePT := uint8(101)
	rx := NewSIPRTPTransport(a, codec, media.DirectionInput, tePT)

	var gotPayload []byte
	rx.OnInputRTP = func(_ uint16, _ uint32, p []byte) {
		gotPayload = append([]byte(nil), p...)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = rx.Next(ctx)
	}()

	// Looks like digit "3" without end bit + padding like a 20 ms PCMU frame — old logic dropped recording + decode.
	payload := make([]byte, 160)
	payload[0] = 3 // maps to DTMF digit "3"
	payload[1] = 0 // end bit clear

	hdr := RTPHeader{
		Version: 2, PayloadType: tePT, SequenceNumber: 7, Timestamp: 100, SSRC: 1,
	}
	pkt := RTPPacket{Header: hdr, Payload: payload}
	raw, err := pkt.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Conn.WriteToUDP(raw, a.LocalAddr); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)
	cancel()

	if len(gotPayload) != len(payload) {
		t.Fatalf("OnInputRTP payload len: got %d want %d", len(gotPayload), len(payload))
	}
	for i := range payload {
		if gotPayload[i] != payload[i] {
			t.Fatalf("byte %d: got %02x want %02x", i, gotPayload[i], payload[i])
		}
	}
}
