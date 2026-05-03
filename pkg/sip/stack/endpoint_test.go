package stack

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEndpoint_OPTIONS_handler(t *testing.T) {
	ep := NewEndpoint(EndpointConfig{
		Host:         "127.0.0.1",
		Port:         0,
		ReadDeadline: 200 * time.Millisecond,
	})
	if err := ep.Open(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ep.Close() }()

	local := ep.ListenAddr().(*net.UDPAddr)

	ep.RegisterHandler(MethodOptions, func(msg *Message, addr *net.UDPAddr) *Message {
		resp := &Message{
			IsRequest:    false,
			Version:      "SIP/2.0",
			StatusCode:   200,
			StatusText:   "OK",
			Headers:      map[string]string{},
			HeadersMulti: map[string][]string{},
		}
		resp.SetHeader("Call-ID", msg.GetHeader("Call-ID"))
		resp.SetHeader("CSeq", msg.GetHeader("CSeq"))
		resp.SetHeader("Content-Length", "0")
		return resp
	})

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = ep.Serve(ctx)
	}()

	c, err := net.DialUDP("udp4", nil, local)
	if err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	req := strings.Join([]string{
		"OPTIONS sip:u@" + local.String() + " SIP/2.0",
		"Via: SIP/2.0/UDP 127.0.0.1;branch=z9hG4bKtest",
		"From: <sip:a@b>;tag=1",
		"To: <sip:a@b>",
		"Call-ID: test-call",
		"CSeq: 1 OPTIONS",
		"Content-Length: 0",
		"",
		"",
	}, "\r\n")
	if _, err := c.Write([]byte(req)); err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}
	resp, err := Parse(string(buf[:n]))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}

	cancel()
	wg.Wait()
}

func TestEndpoint_OnResponseSent(t *testing.T) {
	var sent atomic.Int32
	ep := NewEndpoint(EndpointConfig{
		Host:           "127.0.0.1",
		Port:           0,
		ReadDeadline:   200 * time.Millisecond,
		OnResponseSent: func(req, resp *Message, addr *net.UDPAddr) { sent.Add(1) },
	})
	if err := ep.Open(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ep.Close() }()
	local := ep.ListenAddr().(*net.UDPAddr)
	ep.RegisterHandler(MethodOptions, func(msg *Message, addr *net.UDPAddr) *Message {
		resp := &Message{IsRequest: false, Version: "SIP/2.0", StatusCode: 200, StatusText: "OK",
			Headers: map[string]string{}, HeadersMulti: map[string][]string{}}
		resp.SetHeader("Call-ID", msg.GetHeader("Call-ID"))
		resp.SetHeader("CSeq", msg.GetHeader("CSeq"))
		resp.SetHeader("Content-Length", "0")
		return resp
	})
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = ep.Serve(ctx) }()
	c, err := net.DialUDP("udp4", nil, local)
	if err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	req := "OPTIONS sip:x SIP/2.0\r\nVia: SIP/2.0/UDP 1.1.1.1;branch=z9hG4bKx\r\nFrom: f\r\nTo: t\r\nCall-ID: c\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	if _, err := c.Read(buf); err != nil {
		cancel()
		wg.Wait()
		t.Fatal(err)
	}
	cancel()
	wg.Wait()
	if sent.Load() != 1 {
		t.Fatalf("OnResponseSent calls=%d", sent.Load())
	}
}
