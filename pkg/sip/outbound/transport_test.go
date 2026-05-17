// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package outbound

import "testing"

func TestTransport_IsValid(t *testing.T) {
	for _, tr := range []Transport{TransportUDP, TransportTCP, TransportTLS} {
		if !tr.IsValid() {
			t.Errorf("%s should be valid", tr)
		}
	}
	if TransportUnset.IsValid() {
		t.Errorf("Unset must not be valid")
	}
	if Transport("garbage").IsValid() {
		t.Errorf("garbage must not be valid")
	}
}

func TestTransport_IsTLS_IsConnectionOriented(t *testing.T) {
	if !TransportTLS.IsTLS() {
		t.Errorf("TLS.IsTLS() must be true")
	}
	if TransportUDP.IsTLS() || TransportTCP.IsTLS() {
		t.Errorf("only TLS is TLS")
	}
	if !TransportTCP.IsConnectionOriented() || !TransportTLS.IsConnectionOriented() {
		t.Errorf("TCP/TLS must be connection-oriented")
	}
	if TransportUDP.IsConnectionOriented() {
		t.Errorf("UDP must not be connection-oriented")
	}
}

func TestTransport_ViaToken(t *testing.T) {
	cases := map[Transport]string{
		TransportUDP:   "SIP/2.0/UDP",
		TransportTCP:   "SIP/2.0/TCP",
		TransportTLS:   "SIP/2.0/TLS",
		TransportUnset: "SIP/2.0/UDP", // safe default
	}
	for tr, want := range cases {
		if got := tr.ViaToken(); got != want {
			t.Errorf("%q.ViaToken() = %q, want %q", tr, got, want)
		}
	}
}

func TestParseTransportToken(t *testing.T) {
	cases := map[string]Transport{
		"udp":     TransportUDP,
		"UDP":     TransportUDP,
		"  tcp  ": TransportTCP,
		"TLS":     TransportTLS,
		"":        TransportUnset,
		"sctp":    TransportUnset, // we don't support SCTP
		"garbage": TransportUnset,
	}
	for in, want := range cases {
		if got := parseTransportToken(in); got != want {
			t.Errorf("parseTransportToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTransportFromRequestURI(t *testing.T) {
	cases := []struct {
		uri  string
		want Transport
	}{
		{"", TransportUnset},
		{"sip:user@host:5060", TransportUnset},
		{"sip:user@host:5060;transport=udp", TransportUDP},
		{"sip:user@host:5060;transport=tcp", TransportTCP},
		{"sip:user@host:5060;transport=TLS", TransportTLS},
		{"sip:user@host:5060;transport=tls;lr", TransportTLS}, // param after
		{"sip:user@host:5060;lr;transport=tcp", TransportTCP}, // param before
		{"SIP:USER@HOST;TRANSPORT=TCP", TransportTCP},          // case insensitive
		{"sips:user@host", TransportTLS},                       // SIPS scheme implies TLS
		{"sips:user@host;transport=tcp", TransportTCP},         // explicit overrides
		{"sip:user@host;transport=garbage", TransportUnset},    // unknown token
	}
	for _, tc := range cases {
		if got := transportFromRequestURI(tc.uri); got != tc.want {
			t.Errorf("transportFromRequestURI(%q) = %q, want %q", tc.uri, got, tc.want)
		}
	}
}

func TestResolveTransport_Precedence(t *testing.T) {
	// Case 1: URI ;transport=tcp wins over trunk's UDP setting.
	got := ResolveTransport(DialTarget{
		RequestURI: "sip:user@host;transport=tcp",
		Transport:  TransportUDP,
	})
	if got != TransportTCP {
		t.Errorf("URI param must beat trunk config, got %q", got)
	}

	// Case 2: no URI param → trunk wins.
	got = ResolveTransport(DialTarget{
		RequestURI: "sip:user@host",
		Transport:  TransportTLS,
	})
	if got != TransportTLS {
		t.Errorf("trunk config must apply when URI silent, got %q", got)
	}

	// Case 3: nothing → default UDP.
	got = ResolveTransport(DialTarget{RequestURI: "sip:user@host"})
	if got != TransportUDP {
		t.Errorf("default must be UDP, got %q", got)
	}

	// Case 4: SIPS scheme → TLS even without trunk hint.
	got = ResolveTransport(DialTarget{RequestURI: "sips:user@host"})
	if got != TransportTLS {
		t.Errorf("sips scheme must yield TLS, got %q", got)
	}

	// Case 5: SIPS + explicit ;transport=tcp → URI wins (lenient).
	got = ResolveTransport(DialTarget{RequestURI: "sips:user@host;transport=tcp"})
	if got != TransportTCP {
		t.Errorf("explicit URI param must override sips scheme, got %q", got)
	}

	// Case 6: empty URI + invalid trunk Transport → UDP fallback.
	got = ResolveTransport(DialTarget{Transport: Transport("bogus")})
	if got != TransportUDP {
		t.Errorf("bogus trunk Transport should fall to UDP, got %q", got)
	}
}
