package sdp

import (
	"strings"
	"testing"
)

func TestParse_Basic(t *testing.T) {
	sdpBody := strings.Join([]string{
		"v=0",
		"o=- 123456 123456 IN IP4 192.168.1.100",
		"s=Session",
		"c=IN IP4 192.168.1.100",
		"t=0 0",
		"m=audio 49170 RTP/AVP 0",
		"a=rtpmap:0 PCMU/8000",
	}, "\r\n")

	info, err := Parse(sdpBody)
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "192.168.1.100" || info.Port != 49170 {
		t.Fatalf("ip/port: %+v", info)
	}
	if len(info.Codecs) != 1 || info.Codecs[0].PayloadType != 0 || info.Codecs[0].Name != "pcmu" {
		t.Fatalf("codec: %+v", info.Codecs)
	}
}

func TestParse_StaticPCMU_PlusTelephoneEvent(t *testing.T) {
	sdpBody := strings.Join([]string{
		"v=0",
		"o=- 1 1 IN IP4 10.0.0.2",
		"s=-",
		"c=IN IP4 10.0.0.2",
		"t=0 0",
		"m=audio 8000 RTP/AVP 0 101",
		"a=rtpmap:101 telephone-event/8000",
		"a=fmtp:101 0-15",
	}, "\r\n")

	info, err := Parse(sdpBody)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Codecs) != 2 {
		t.Fatalf("want 2 codecs, got %d", len(info.Codecs))
	}
	if info.Codecs[0].PayloadType != 0 || info.Codecs[0].Name != "pcmu" {
		t.Fatalf("first: %#v", info.Codecs[0])
	}
	if info.Codecs[1].Name != "telephone-event" {
		t.Fatalf("second: %#v", info.Codecs[1])
	}
}

func TestParse_SAVPRejected(t *testing.T) {
	sdpBody := strings.Join([]string{
		"v=0",
		"o=- 1 1 IN IP4 10.0.0.2",
		"s=-",
		"c=IN IP4 10.0.0.2",
		"t=0 0",
		"m=audio 8000 RTP/SAVP 0",
	}, "\r\n")
	_, err := Parse(sdpBody)
	if err == nil {
		t.Fatal("expected error for SAVP")
	}
}

func TestGenerate_RoundTrip(t *testing.T) {
	codecs := []Codec{
		{PayloadType: 0, Name: "pcmu", ClockRate: 8000},
		{PayloadType: 8, Name: "pcma", ClockRate: 8000},
	}
	body := Generate("127.0.0.1", 5004, codecs)
	info, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "127.0.0.1" || info.Port != 5004 || len(info.Codecs) == 0 {
		t.Fatalf("%+v", info)
	}
}

func TestGenerate_PayloadOrderPreserved(t *testing.T) {
	codecs := []Codec{
		{PayloadType: 8, Name: "pcma", ClockRate: 8000},
		{PayloadType: 111, Name: "opus", ClockRate: 48000, Channels: 1},
		{PayloadType: 0, Name: "pcmu", ClockRate: 8000},
		{PayloadType: 101, Name: "telephone-event", ClockRate: 8000},
	}
	body := Generate("127.0.0.1", 5004, codecs)
	info, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Codecs) != len(codecs) {
		t.Fatalf("count got=%d want=%d", len(info.Codecs), len(codecs))
	}
	for i := range codecs {
		if info.Codecs[i].PayloadType != codecs[i].PayloadType || info.Codecs[i].Name != codecs[i].Name {
			t.Fatalf("[%d] got=%#v want=%#v", i, info.Codecs[i], codecs[i])
		}
	}
}

func TestPickTelephoneEventFromOffer(t *testing.T) {
	offer := []Codec{
		{Name: "opus", ClockRate: 48000, PayloadType: 111},
		{Name: "telephone-event", ClockRate: 8000, PayloadType: 101},
		{Name: "telephone-event", ClockRate: 48000, PayloadType: 112},
	}
	if c, ok := PickTelephoneEventFromOffer(offer, 48000); !ok || c.PayloadType != 112 {
		t.Fatalf("got %#v ok=%v", c, ok)
	}
}
