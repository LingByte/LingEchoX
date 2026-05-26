package webseat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/pion/webrtc/v3"
	pionmedia "github.com/pion/webrtc/v3/pkg/media"
)

// Transport is a minimal WebRTC audio transport that implements media.MediaTransport.
//
// Notes:
// - This does not expose ICE/SDP signaling; it's a media transport only.
// - Duration calculation is approximate and depends on codec + payload framing.
type Transport struct {
	rxTrack *webrtc.TrackRemote
	txTrack *webrtc.TrackLocalStaticSample
	rxCodec media.CodecConfig
	txCodec media.CodecConfig
}

// NewTransport bridges browser uplink (rxCodec) and downlink (txCodec). They may differ
// when SDP picks PCMA for browser→server and PCMU for server→browser.
func NewTransport(rx *webrtc.TrackRemote, tx *webrtc.TrackLocalStaticSample, rxCodec, txCodec media.CodecConfig) *Transport {
	return &Transport{
		rxTrack: rx,
		txTrack: tx,
		rxCodec: rxCodec,
		txCodec: txCodec,
	}
}

func (t *Transport) String() string {
	return fmt.Sprintf("SipWebRTCTransport{rx=%s tx=%s, rxTr=%v, txTr=%v}", t.rxCodec.String(), t.txCodec.String(), t.rxTrack != nil, t.txTrack != nil)
}

func (t *Transport) Attach(s *media.MediaSession) {
	_ = s
}

func (t *Transport) Codec() media.CodecConfig {
	return t.rxCodec
}

func (t *Transport) TxCodec() media.CodecConfig {
	return t.txCodec
}

func (t *Transport) Next(ctx context.Context) (media.MediaPacket, error) {
	if t.rxTrack == nil {
		time.Sleep(10 * time.Millisecond)
		return nil, nil
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, nil
	}
	pkt, _, err := t.rxTrack.ReadRTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("webrtc: read rtp: %w", err)
	}
	if len(pkt.Payload) == 0 {
		return nil, nil
	}
	return &media.AudioPacket{Payload: pkt.Payload}, nil
}

func (t *Transport) Send(ctx context.Context, packet media.MediaPacket) (int, error) {
	if t.txTrack == nil {
		return 0, nil
	}
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}

	audio, ok := packet.(*media.AudioPacket)
	if !ok {
		return 0, nil
	}
	if len(audio.Payload) == 0 {
		return 0, nil
	}

	dur := 20 * time.Millisecond
	if fd := strings.TrimSpace(t.txCodec.FrameDuration); fd != "" {
		if d, err := time.ParseDuration(fd); err == nil && d > 0 {
			dur = d
		}
	}
	encName := strings.ToLower(strings.TrimSpace(t.txCodec.Codec))
	if encName != "opus" && t.txCodec.SampleRate > 0 {
		bytesPerSample := (t.txCodec.BitDepth / 8) * t.txCodec.Channels
		if bytesPerSample > 0 {
			samples := len(audio.Payload) / bytesPerSample
			if samples > 0 {
				dur = time.Duration(float64(samples) / float64(t.txCodec.SampleRate) * float64(time.Second))
			}
		}
	}

	sample := pionmedia.Sample{Data: audio.Payload, Duration: dur}
	if err := t.txTrack.WriteSample(sample); err != nil {
		return 0, fmt.Errorf("webrtc: write sample: %w", err)
	}
	return len(audio.Payload), nil
}

func (t *Transport) Close() error {
	return nil
}

// WakeupRead is a no-op; closing the PeerConnection unblocks ReadRTP in Next.
func (t *Transport) WakeupRead() {}
