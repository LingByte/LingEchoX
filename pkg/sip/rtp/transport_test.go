package rtp

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/media"
)

// This test verifies an end-to-end path:
// client RTP session -> server MediaSession(Input SIPRTPTransport) -> output-router -> server SIPRTPTransport -> client RTP session.
//
// We intentionally use two UDP sockets (client/server) to avoid creating an infinite echo loop on a single socket.
func TestMediaSession_WithSIPRTPTransport_Echo(t *testing.T) {
	// media package relies on pkg/logger global zap logger.
	// Initialize it to avoid nil pointer panics in tests.
	tmp := t.TempDir()
	if err := logger.Init(&logger.LogConfig{
		Level:      "debug",
		Filename:   tmp + "/test.log",
		MaxSize:    1,
		MaxAge:     1,
		MaxBackups: 1,
		Daily:      false,
	}, "dev"); err != nil {
		t.Fatalf("logger.Init failed: %v", err)
	}

	// Server RTP socket
	serverSess, err := NewSession(0)
	if err != nil {
		t.Fatalf("server NewSession failed: %v", err)
	}
	defer serverSess.Close()

	// Client RTP socket
	clientSess, err := NewSession(0)
	if err != nil {
		t.Fatalf("client NewSession failed: %v", err)
	}
	defer clientSess.Close()

	// Wire remote addresses.
	serverSess.SetRemoteAddr(clientSess.LocalAddr)
	clientSess.SetRemoteAddr(serverSess.LocalAddr)

	codec := media.CodecConfig{
		Codec:       "pcmu",
		SampleRate:  8000,
		Channels:    1,
		BitDepth:    8,
		PayloadType: 0,
	}

	rx := NewSIPRTPTransport(serverSess, codec, media.DirectionInput, 0)
	tx := NewSIPRTPTransport(serverSess, codec, media.DirectionOutput, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ms := media.NewDefaultSession().
		Context(ctx).
		SetSessionID("test-sip-rtp-echo").
		Input(rx).
		Output(tx)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- ms.Serve()
	}()

	// Give the server a moment to start its goroutines.
	time.Sleep(50 * time.Millisecond)

	original := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if err := clientSess.SendRTP(original, 0, 160); err != nil {
		t.Fatalf("client SendRTP failed: %v", err)
	}

	// Receive echoed RTP on the client socket.
	buf := make([]byte, 1500)
	recvCh := make(chan *RTPPacket, 1)
	go func() {
		_, _, pkt, err := clientSess.ReceiveRTP(buf)
		if err != nil {
			t.Errorf("client ReceiveRTP failed: %v", err)
			recvCh <- nil
			return
		}
		recvCh <- pkt
	}()

	select {
	case pkt := <-recvCh:
		if pkt == nil {
			t.Fatalf("expected packet, got nil")
		}
		if !bytes.Equal(pkt.Payload, original) {
			t.Fatalf("echo payload mismatch: got=%v want=%v", pkt.Payload, original)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for echoed RTP")
	}

	// Stop the media session and ensure Serve() returns.
	_ = ms.Close()
	cancel()

	select {
	case err := <-serveErr:
		// Serve may return nil or context cancellation related errors depending on internals.
		_ = err
	case <-time.After(8 * time.Second):
		t.Fatalf("timeout waiting for MediaSession.Serve() to return")
	}
}

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
