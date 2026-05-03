package protocol

import "github.com/LingByte/SoulNexus/pkg/sip/sdp"

// SDPCodec is one RTP payload from SDP (alias of pkg/sip/sdp.Codec).
type SDPCodec = sdp.Codec

// SDPInfo is parsed audio SDP summary (alias of pkg/sip/sdp.Info).
type SDPInfo = sdp.Info

// ParseSDP extracts IP/port/codecs from an SDP body (delegates to pkg/sip/sdp.Parse).
func ParseSDP(body string) (*SDPInfo, error) {
	return sdp.Parse(body)
}

// GenerateSDP builds a minimal audio SDP body.
func GenerateSDP(localIP string, localPort int, codecs []SDPCodec) string {
	return sdp.Generate(localIP, localPort, codecs)
}

// GenerateSDPWithProto builds SDP with a specific m=audio proto (e.g. RTP/AVP).
func GenerateSDPWithProto(localIP string, localPort int, proto string, codecs []SDPCodec) string {
	return sdp.GenerateWithProto(localIP, localPort, proto, codecs)
}

// PickTelephoneEventFromOffer selects telephone-event PT, preferring clock-rate match.
func PickTelephoneEventFromOffer(offer []SDPCodec, matchClockRate int) (SDPCodec, bool) {
	return sdp.PickTelephoneEventFromOffer(offer, matchClockRate)
}
