package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/media"
	"github.com/LingByte/SoulNexus/pkg/sip/sdp"
)

// NegotiateOffer picks the first supported audio codec from a remote SDP offer (ordered by preference).
func NegotiateOffer(offer []sdp.Codec) (src media.CodecConfig, neg sdp.Codec, err error) {
	if len(offer) == 0 {
		return media.CodecConfig{}, sdp.Codec{}, fmt.Errorf("sip1/session: empty offer")
	}
	preferred := map[string]int{
		"pcma": 0, "pcmu": 1, "g722": 2, "opus": 3,
	}
	codecs := make([]sdp.Codec, len(offer))
	copy(codecs, offer)
	sort.SliceStable(codecs, func(i, j int) bool {
		ci := strings.ToLower(strings.TrimSpace(codecs[i].Name))
		cj := strings.ToLower(strings.TrimSpace(codecs[j].Name))
		ri, okI := preferred[ci]
		rj, okJ := preferred[cj]
		if !okI {
			ri = 100
		}
		if !okJ {
			rj = 100
		}
		return ri < rj
	})

	for _, c := range codecs {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		switch name {
		case "pcmu", "pcma":
			ch := c.Channels
			if ch < 1 {
				ch = 1
			}
			neg = sdp.Codec{PayloadType: c.PayloadType, Name: name, ClockRate: c.ClockRate, Channels: ch}
			src = media.CodecConfig{
				Codec:         name,
				SampleRate:    c.ClockRate,
				Channels:      1,
				BitDepth:      8,
				PayloadType:   c.PayloadType,
				FrameDuration: "20ms",
			}
			return src, neg, nil
		case "g722":
			neg = sdp.Codec{PayloadType: c.PayloadType, Name: "g722", ClockRate: 8000, Channels: 1}
			src = media.CodecConfig{
				Codec:         "g722",
				SampleRate:    16000,
				Channels:      1,
				BitDepth:      16,
				PayloadType:   c.PayloadType,
				FrameDuration: "20ms",
			}
			return src, neg, nil
		case "opus":
			decodeCh := c.Channels
			if decodeCh < 1 {
				decodeCh = 1
			}
			if decodeCh > 2 {
				decodeCh = 2
			}
			neg = sdp.Codec{PayloadType: c.PayloadType, Name: "opus", ClockRate: c.ClockRate, Channels: decodeCh}
			src = media.CodecConfig{
				Codec:              "opus",
				SampleRate:         c.ClockRate,
				Channels:           1,
				OpusDecodeChannels: decodeCh,
				BitDepth:           16,
				PayloadType:        c.PayloadType,
				FrameDuration:      "20ms",
			}
			return src, neg, nil
		}
	}
	return media.CodecConfig{}, sdp.Codec{}, fmt.Errorf("sip1/session: no supported codec in offer (need pcma/pcmu/g722/opus)")
}

// InternalPCMSampleRate chooses the MediaSession PCM bridge rate for a negotiated RTP codec so decode/encode
// avoids unnecessary resampling (e.g. keep G.711 at 8 kHz, Opus at negotiated clock, G.722 at 16 kHz PCM).
func InternalPCMSampleRate(src media.CodecConfig) int {
	name := strings.ToLower(strings.TrimSpace(src.Codec))
	switch name {
	case "pcmu", "pcma":
		if src.SampleRate > 0 {
			return src.SampleRate
		}
		return 8000
	case "g722":
		return 16000
	case "opus":
		switch src.SampleRate {
		case 8000, 12000, 16000, 24000, 48000:
			return src.SampleRate
		}
		if src.SampleRate > 36000 {
			return 48000
		}
		if src.SampleRate > 20000 {
			return 24000
		}
		if src.SampleRate > 14000 {
			return 16000
		}
		if src.SampleRate > 10000 {
			return 12000
		}
		if src.SampleRate > 0 {
			return 8000
		}
		return 48000
	default:
		if src.SampleRate > 0 {
			return src.SampleRate
		}
		return 16000
	}
}

func telephoneEventPT(offer []sdp.Codec, matchClock int) uint8 {
	c, ok := sdp.PickTelephoneEventFromOffer(offer, matchClock)
	if !ok {
		return 0
	}
	return c.PayloadType
}
