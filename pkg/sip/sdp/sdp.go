package sdp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Codec is one RTP payload mapping (typically from a=rtpmap).
type Codec struct {
	PayloadType uint8
	Name        string
	ClockRate   int
	Channels    int
}

// Info holds minimal audio media information extracted from an SDP body.
type Info struct {
	IP     string
	Port   int
	Proto  string
	Codecs []Codec
}

var (
	reIP      = regexp.MustCompile(`c=IN IP4 ([0-9.]+)`)
	reMAudio  = regexp.MustCompile(`m=audio\s+(\d+)\s+([A-Za-z0-9/]+)\s+(.+)`)
	reRtpMap  = regexp.MustCompile(`^a=rtpmap:(\d+)\s+([^/]+)/(\d+)`)
	reRtpMapV = regexp.MustCompile(`^a=rtpmap:(\d+)\s+([^/]+)/(\d+)(?:/(\d+))?$`)
)

func normalizeCodecName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "pcmu", "pcma", "g722", "opus", "pcm", "telephone-event":
		return n
	default:
		return n
	}
}

func staticPayloadCodec(pt uint8) (Codec, bool) {
	switch pt {
	case 0:
		return Codec{PayloadType: 0, Name: "pcmu", ClockRate: 8000, Channels: 1}, true
	case 8:
		return Codec{PayloadType: 8, Name: "pcma", ClockRate: 8000, Channels: 1}, true
	case 9:
		return Codec{PayloadType: 9, Name: "g722", ClockRate: 8000, Channels: 1}, true
	default:
		return Codec{}, false
	}
}

// NormalizeBody trims whitespace and collapses line endings to LF so parsing is stable across CRLF/LF peers.
func NormalizeBody(body string) string {
	s := strings.TrimSpace(body)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// Parse extracts connection IP, audio m= port/proto, and codec list from an SDP body.
func Parse(body string) (*Info, error) {
	body = NormalizeBody(body)
	if body == "" {
		return nil, fmt.Errorf("sip1/sdp: empty body")
	}

	info := &Info{}
	if m := reIP.FindStringSubmatch(body); len(m) >= 2 {
		info.IP = m[1]
	}

	var payloadTypes []uint8
	var mediaProto string
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "m=audio") {
			m := reMAudio.FindStringSubmatch(line)
			if len(m) >= 4 {
				port, err := strconv.Atoi(strings.TrimSpace(m[1]))
				if err != nil {
					return nil, fmt.Errorf("sip1/sdp: invalid m=audio port: %w", err)
				}
				info.Port = port
				mediaProto = strings.ToUpper(strings.TrimSpace(m[2]))
				info.Proto = mediaProto

				pts := strings.Fields(strings.TrimSpace(m[3]))
				for _, ptStr := range pts {
					ptInt, err := strconv.Atoi(ptStr)
					if err != nil {
						continue
					}
					if ptInt < 0 || ptInt > 255 {
						continue
					}
					payloadTypes = append(payloadTypes, uint8(ptInt))
				}
			}
		}

		if strings.HasPrefix(line, "a=rtpmap:") {
			m := reRtpMapV.FindStringSubmatch(line)
			if len(m) >= 4 {
				ptInt, err := strconv.Atoi(m[1])
				if err != nil {
					continue
				}
				name := m[2]
				clock, err := strconv.Atoi(m[3])
				if err != nil {
					continue
				}
				if ptInt < 0 || ptInt > 255 {
					continue
				}
				channels := 1
				if len(m) >= 5 && strings.TrimSpace(m[4]) != "" {
					if ch, err := strconv.Atoi(strings.TrimSpace(m[4])); err == nil && ch > 0 {
						channels = ch
					}
				}
				info.Codecs = append(info.Codecs, Codec{
					PayloadType: uint8(ptInt),
					Name:        normalizeCodecName(name),
					ClockRate:   clock,
					Channels:    channels,
				})
			} else if m2 := reRtpMap.FindStringSubmatch(line); len(m2) >= 4 {
				ptInt, err := strconv.Atoi(m2[1])
				if err != nil {
					continue
				}
				name := m2[2]
				clock, err := strconv.Atoi(m2[3])
				if err != nil {
					continue
				}
				if ptInt < 0 || ptInt > 255 {
					continue
				}
				info.Codecs = append(info.Codecs, Codec{
					PayloadType: uint8(ptInt),
					Name:        normalizeCodecName(name),
					ClockRate:   clock,
					Channels:    1,
				})
			}
		}
	}

	if mediaProto != "" && strings.Contains(mediaProto, "SAVP") {
		return nil, fmt.Errorf("sip1/sdp: unsupported media proto: %s", mediaProto)
	}

	if len(payloadTypes) > 0 {
		seen := make(map[uint8]struct{}, len(info.Codecs)+len(payloadTypes))
		for _, c := range info.Codecs {
			seen[c.PayloadType] = struct{}{}
		}
		for _, pt := range payloadTypes {
			if _, ok := seen[pt]; ok {
				continue
			}
			if sc, ok := staticPayloadCodec(pt); ok {
				info.Codecs = append(info.Codecs, sc)
				seen[pt] = struct{}{}
			}
		}
	}

	if len(info.Codecs) == 0 && len(payloadTypes) > 0 {
		for _, pt := range payloadTypes {
			if sc, ok := staticPayloadCodec(pt); ok {
				info.Codecs = append(info.Codecs, sc)
			}
		}
	}

	if len(info.Codecs) == 0 {
		return nil, fmt.Errorf("sip1/sdp: no codec found")
	}

	if len(payloadTypes) > 0 {
		want := make(map[uint8]struct{}, len(payloadTypes))
		for _, pt := range payloadTypes {
			want[pt] = struct{}{}
		}
		filtered := make([]Codec, 0, len(info.Codecs))
		for _, c := range info.Codecs {
			if _, ok := want[c.PayloadType]; ok {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			byPT := make(map[uint8]Codec, len(filtered))
			for _, c := range filtered {
				byPT[c.PayloadType] = c
			}
			ordered := make([]Codec, 0, len(filtered))
			for _, pt := range payloadTypes {
				if c, ok := byPT[pt]; ok {
					ordered = append(ordered, c)
				}
			}
			info.Codecs = ordered
		}
	}

	return info, nil
}

// DefaultSessionName is the fixed s= line used by Generate / GenerateWithProto.
const DefaultSessionName = "SoulNexus SIP"

// Generate builds a minimal audio SDP offer/answer (m= uses proto RTP/AVP).
func Generate(localIP string, localPort int, codecs []Codec) string {
	return GenerateWithProto(localIP, localPort, "RTP/AVP", codecs)
}

// GenerateWithProto builds minimal audio SDP with a given m=audio proto (e.g. RTP/AVP, RTP/AVPF).
func GenerateWithProto(localIP string, localPort int, proto string, codecs []Codec) string {
	if localPort <= 0 {
		localPort = 49172
	}
	if localIP == "" {
		localIP = "127.0.0.1"
	}
	proto = strings.ToUpper(strings.TrimSpace(proto))
	if proto == "" {
		proto = "RTP/AVP"
	}

	pts := make([]string, 0, len(codecs))
	for _, c := range codecs {
		pts = append(pts, strconv.Itoa(int(c.PayloadType)))
	}

	var b strings.Builder
	b.WriteString("v=0\r\n")
	sess := time.Now().Unix()
	b.WriteString(fmt.Sprintf("o=- %d %d IN IP4 %s\r\n", sess, sess, localIP))
	b.WriteString("s=" + DefaultSessionName + "\r\n")
	b.WriteString(fmt.Sprintf("c=IN IP4 %s\r\n", localIP))
	b.WriteString("t=0 0\r\n")
	b.WriteString(fmt.Sprintf("m=audio %d %s %s\r\n", localPort, proto, strings.Join(pts, " ")))
	b.WriteString(fmt.Sprintf("a=rtcp:%d IN IP4 %s\r\n", localPort+1, localIP))
	b.WriteString("a=sendrecv\r\n")
	b.WriteString("a=ptime:20\r\n")
	for _, c := range codecs {
		if c.Channels > 1 {
			b.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d/%d\r\n", c.PayloadType, strings.ToUpper(c.Name), c.ClockRate, c.Channels))
		} else {
			b.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d\r\n", c.PayloadType, strings.ToUpper(c.Name), c.ClockRate))
		}
		if strings.EqualFold(c.Name, "opus") {
			b.WriteString(fmt.Sprintf("a=fmtp:%d minptime=10;useinbandfec=1\r\n", c.PayloadType))
		}
		if strings.EqualFold(c.Name, "telephone-event") {
			b.WriteString(fmt.Sprintf("a=fmtp:%d 0-15\r\n", c.PayloadType))
		}
	}
	return b.String()
}

// PickTelephoneEventFromOffer picks a telephone-event codec; prefers clock rate match when matchClockRate > 0.
func PickTelephoneEventFromOffer(offer []Codec, matchClockRate int) (Codec, bool) {
	var fallback Codec
	var hasFallback bool
	for _, c := range offer {
		if !strings.EqualFold(strings.TrimSpace(c.Name), "telephone-event") {
			continue
		}
		if matchClockRate > 0 && c.ClockRate == matchClockRate {
			return c, true
		}
		if !hasFallback {
			fallback = c
			hasFallback = true
		}
	}
	if hasFallback {
		return fallback, true
	}
	return Codec{}, false
}
