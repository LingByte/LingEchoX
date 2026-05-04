package rtp

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LingByte/SoulNexus/pkg/media"
)

// TestSession_UDPLoopback verifies that a single Session can send and receive
// an RTP packet to itself over UDP (loopback).
func TestSession_UDPLoopback(t *testing.T) {
	s, err := NewSession(0)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer s.Close()

	// Loopback: send to self.
	s.SetRemoteAddr(s.LocalAddr)

	payload := []byte{0x01, 0x02, 0x03, 0x04}

	done := make(chan *RTPPacket, 1)
	go func() {
		buf := make([]byte, 1500)
		_, addr, pkt, err := s.ReceiveRTP(buf)
		if err != nil {
			t.Errorf("ReceiveRTP error: %v", err)
			done <- nil
			return
		}
		if addr == nil || addr.Port == 0 {
			t.Errorf("unexpected addr: %v", addr)
		}
		done <- pkt
	}()

	if err := s.SendRTP(payload, 0, 160); err != nil {
		t.Fatalf("SendRTP failed: %v", err)
	}

	select {
	case pkt := <-done:
		if pkt == nil {
			t.Fatalf("nil packet from receiver")
		}
		if !bytes.Equal(pkt.Payload, payload) {
			t.Fatalf("payload mismatch: got=%v want=%v", pkt.Payload, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for received packet")
	}
}

func TestSession_ReceiveRTP_DiscardNonV2(t *testing.T) {
	rx, err := NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer rx.Close()
	tx, err := NewSession(0)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Close()

	hdr := RTPHeader{
		Version:        1,
		PayloadType:    0,
		SequenceNumber: 1,
		Timestamp:      1,
		SSRC:           0x11111111,
	}
	raw, err := (&RTPPacket{Header: hdr, Payload: []byte{0x7F}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Conn.WriteToUDP(raw, rx.LocalAddr); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	buf := make([]byte, 1500)
	_, _, pkt, err := rx.ReceiveRTP(buf)
	if !errors.Is(err, ErrRTPDiscard) {
		t.Fatalf("want ErrRTPDiscard, got err=%v pkt=%v", err, pkt)
	}
}

// TestSIPRTPTransport_SendAndNext verifies that SIPRTPTransport can
// send and then receive an AudioPacket over a loopback Session.
func TestSIPRTPTransport_SendAndNext(t *testing.T) {
	s, err := NewSession(0)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer s.Close()

	s.SetRemoteAddr(s.LocalAddr)

	codec := media.CodecConfig{
		Codec:       "pcmu",
		SampleRate:  8000,
		Channels:    1,
		BitDepth:    8,
		PayloadType: 0,
	}

	tx := NewSIPRTPTransport(s, codec, media.DirectionOutput, 0)
	rx := NewSIPRTPTransport(s, codec, media.DirectionInput, 0)

	payload := []byte{0x7F, 0x00, 0x7F, 0x00}

	done := make(chan media.MediaPacket, 1)
	go func() {
		pkt, err := rx.Next(context.Background())
		if err != nil {
			t.Errorf("rx.Next error: %v", err)
			done <- nil
			return
		}
		done <- pkt
	}()

	n, err := tx.Send(context.Background(), &media.AudioPacket{Payload: payload})
	if err != nil {
		t.Fatalf("tx.Send failed: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("unexpected bytes written: got=%d want=%d", n, len(payload))
	}

	select {
	case mpkt := <-done:
		audio, ok := mpkt.(*media.AudioPacket)
		if !ok {
			t.Fatalf("expected *AudioPacket, got %T", mpkt)
		}
		if !bytes.Equal(audio.Payload, payload) {
			t.Fatalf("payload mismatch: got=%v want=%v", audio.Payload, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for rx.Next")
	}
}
