// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LinByte/VoiceServer/pkg/sip/stack"
)

// startEchoSIPServer returns a goroutine that accepts one TCP conn,
// reads one SIP request via stack.ReadMessage, writes a fixed
// "SIP/2.0 200 OK" response framed correctly, then closes. listener
// is closed before this returns. The caller gets the bound port so
// it can dial.
func startEchoSIPServer(t *testing.T, tlsCfg *tls.Config) (addr *net.TCPAddr, gotReq <-chan *stack.Message, stop func()) {
	t.Helper()
	var ln net.Listener
	var err error
	if tlsCfg != nil {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr = ln.Addr().(*net.TCPAddr)
	reqCh := make(chan *stack.Message, 4)
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		br := bufio.NewReader(conn)
		msg, err := stack.ReadMessage(br)
		if err != nil {
			return
		}
		select {
		case reqCh <- msg:
		default:
		}
		// Craft a minimal 200 OK echoing Via/From/To/Call-ID/CSeq so the
		// outbound side can match it. The connPeer doesn't validate
		// these; we just need *some* well-framed response.
		resp := &stack.Message{
			IsRequest:  false,
			Version:    "SIP/2.0",
			StatusCode: 200,
			StatusText: "OK",
		}
		resp.SetHeader("Via", msg.GetHeader("Via"))
		resp.SetHeader("From", msg.GetHeader("From"))
		resp.SetHeader("To", msg.GetHeader("To")+";tag=srv")
		resp.SetHeader("Call-ID", msg.GetHeader("Call-ID"))
		resp.SetHeader("CSeq", msg.GetHeader("CSeq"))
		resp.SetHeader("Content-Length", "0")
		_, _ = conn.Write([]byte(resp.String()))
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	}()

	stop = func() {
		_ = ln.Close()
		<-done
	}
	return addr, reqCh, stop
}

func minimalSIPInvite() *stack.Message {
	m := &stack.Message{
		IsRequest:  true,
		Method:     "INVITE",
		RequestURI: "sip:bob@127.0.0.1",
		Version:    "SIP/2.0",
	}
	m.SetHeader("Via", "SIP/2.0/TCP 127.0.0.1:6050;branch=z9hG4bKtest;rport")
	m.SetHeader("From", "<sip:alice@127.0.0.1>;tag=fr")
	m.SetHeader("To", "<sip:bob@127.0.0.1>")
	m.SetHeader("Call-ID", "test-call-id@127.0.0.1")
	m.SetHeader("CSeq", "1 INVITE")
	m.SetHeader("Max-Forwards", "70")
	m.SetHeader("Content-Length", "0")
	return m
}

func TestConnPeer_TCP_SendAndReceiveResponse(t *testing.T) {
	addr, gotReq, stop := startEchoSIPServer(t, nil)
	defer stop()

	var respCount int32
	var receivedCallID atomic.Value
	sink := func(resp *stack.Message, _ *net.UDPAddr) {
		atomic.AddInt32(&respCount, 1)
		receivedCallID.Store(strings.TrimSpace(resp.GetHeader("Call-ID")))
	}

	peer, err := dialConnPeer(TransportTCP, addr.IP.String(), addr.Port, nil, sink, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer peer.Close()

	if err := peer.Send(minimalSIPInvite()); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case <-gotReq:
	case <-time.After(2 * time.Second):
		t.Fatal("server didn't receive request")
	}

	// Wait for response sink to fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&respCount) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&respCount); got != 1 {
		t.Fatalf("expected 1 response, got %d", got)
	}
	if cid, _ := receivedCallID.Load().(string); cid != "test-call-id@127.0.0.1" {
		t.Errorf("response Call-ID = %q, want test-call-id@127.0.0.1", cid)
	}
	if peer.Transport() != TransportTCP {
		t.Errorf("transport = %q, want tcp", peer.Transport())
	}
	if peer.Remote() == nil || peer.Remote().Port != addr.Port {
		t.Errorf("Remote() port mismatch: %v", peer.Remote())
	}
}

func TestConnPeer_TLS_HandshakeAndSend(t *testing.T) {
	cert, serverCfg := selfSignedTLSConfig(t)
	addr, gotReq, stop := startEchoSIPServer(t, serverCfg)
	defer stop()

	// Client trusts our self-signed cert via a pinned RootCAs pool.
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	clientCfg := &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}

	var respCount int32
	sink := func(*stack.Message, *net.UDPAddr) { atomic.AddInt32(&respCount, 1) }

	peer, err := dialConnPeer(TransportTLS, addr.IP.String(), addr.Port, clientCfg, sink, nil, 3*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer peer.Close()

	if err := peer.Send(minimalSIPInvite()); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case <-gotReq:
	case <-time.After(3 * time.Second):
		t.Fatal("server didn't receive request")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&respCount) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt32(&respCount) != 1 {
		t.Fatalf("expected 1 TLS response, got %d", respCount)
	}
}

func TestConnPeer_SendAfterClose(t *testing.T) {
	addr, _, stop := startEchoSIPServer(t, nil)
	defer stop()
	peer, err := dialConnPeer(TransportTCP, addr.IP.String(), addr.Port, nil, func(*stack.Message, *net.UDPAddr) {}, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = peer.Close()
	if err := peer.Send(minimalSIPInvite()); err == nil {
		t.Fatal("expected send after close to error")
	}
}

func TestSignalingPool_UDP_DoesNotPool(t *testing.T) {
	udpAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5060}
	var sent int32
	send := func(*stack.Message, *net.UDPAddr) error { atomic.AddInt32(&sent, 1); return nil }
	pool := newSignalingPool(poolConfig{UDPSend: send})
	defer pool.Close()

	p1, err := pool.Get(context.Background(), TransportUDP, udpAddr)
	if err != nil {
		t.Fatalf("Get udp: %v", err)
	}
	p2, err := pool.Get(context.Background(), TransportUDP, udpAddr)
	if err != nil {
		t.Fatalf("Get udp 2: %v", err)
	}
	if p1 == p2 {
		t.Error("UDP peers should be per-call, not pooled")
	}
	if err := p1.Send(minimalSIPInvite()); err != nil {
		t.Errorf("UDP send: %v", err)
	}
	if atomic.LoadInt32(&sent) != 1 {
		t.Errorf("expected 1 udp send, got %d", sent)
	}
}

func TestSignalingPool_TCP_ReusesConn(t *testing.T) {
	// Custom echo server that accepts MULTIPLE connections so we can
	// detect whether the pool dialed once or twice.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var accepts int32
	var wg sync.WaitGroup
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&accepts, 1)
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
					if _, err := stack.ReadMessage(br); err != nil {
						return
					}
				}
			}(c)
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	dst := &net.UDPAddr{IP: addr.IP, Port: addr.Port}
	pool := newSignalingPool(poolConfig{
		ResponseSink: func(*stack.Message, *net.UDPAddr) {},
	})
	defer pool.Close()

	p1, err := pool.Get(context.Background(), TransportTCP, dst)
	if err != nil {
		t.Fatalf("Get tcp 1: %v", err)
	}
	p2, err := pool.Get(context.Background(), TransportTCP, dst)
	if err != nil {
		t.Fatalf("Get tcp 2: %v", err)
	}
	if p1 != p2 {
		t.Error("second Get to same target must reuse peer")
	}
	if err := p1.Send(minimalSIPInvite()); err != nil {
		t.Fatalf("send: %v", err)
	}
	// Tiny pause so server accept goroutine runs.
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&accepts); got != 1 {
		t.Errorf("expected 1 accept (conn reuse), got %d", got)
	}
}

func TestSignalingPool_EvictsOnPeerClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Accept once then close → client sees EOF.
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}()
	defer ln.Close()

	addr := ln.Addr().(*net.TCPAddr)
	dst := &net.UDPAddr{IP: addr.IP, Port: addr.Port}
	pool := newSignalingPool(poolConfig{
		ResponseSink: func(*stack.Message, *net.UDPAddr) {},
	})
	defer pool.Close()

	p1, err := pool.Get(context.Background(), TransportTCP, dst)
	if err != nil {
		t.Fatalf("Get tcp 1: %v", err)
	}
	_ = p1
	// Wait for the read goroutine to notice EOF and evict.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pool.mu.Lock()
		n := len(pool.conns)
		pool.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pool did not evict dead conn")
}

func TestRemoteAsUDPAddr(t *testing.T) {
	tcp := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 5060}
	got := remoteAsUDPAddr(tcp)
	if got == nil || got.Port != 5060 || !got.IP.Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("TCPAddr conversion failed: %v", got)
	}
	if remoteAsUDPAddr(nil) != nil {
		t.Error("nil addr must yield nil")
	}
}

// --- helpers ----------------------------------------------------------------

// selfSignedTLSConfig returns a *tls.Config bound to a fresh self-
// signed cert for "127.0.0.1" valid for 1h, plus the leaf *x509
// cert so tests can pin RootCAs on the client side.
func selfSignedTLSConfig(t *testing.T) (*x509.Certificate, *tls.Config) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "lingbyte-test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	_ = fmt.Sprintf // keep fmt import used in case of future debug
	return leaf, cfg
}
