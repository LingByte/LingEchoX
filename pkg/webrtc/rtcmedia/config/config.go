package config

import (
	"fmt"
	"time"

	"github.com/LingByte/LingEchoX/pkg/webrtc/constants"
	"github.com/pion/webrtc/v3"
)

// WebRTCOption WebRTC Config options
type WebRTCOption struct {
	ICEServers []webrtc.ICEServer `json:"iceServers"` // ICE servers
	StreamID   string             `json:"streamId"`   // stream ID
	ICETimeout time.Duration      `json:"iceTimeout"` // ICE timeout
	Codec      string             `json:"codec"`      // codes names
}

func DefaultWebRTCOption(codec string) *WebRTCOption {
	return &WebRTCOption{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
					"stun:stun.stunprotocol.org:3478",
				},
			},
		},
		StreamID:   constants.DefaultStreamID,
		ICETimeout: constants.DefaultICETimeout,
		Codec:      codec,
	}
}

// GetCodec get codec name
func (wts *WebRTCOption) GetCodec() string {
	return wts.Codec
}

// GetStreamID get stream ID
func (wts *WebRTCOption) GetStreamID() string {
	return wts.StreamID
}

// GetICETimeout get ICE timeout
func (wts *WebRTCOption) GetICETimeout() time.Duration {
	if wts.ICETimeout == 0 {
		return constants.DefaultICETimeout
	}
	return wts.ICETimeout
}

// String config to string
func (wts WebRTCOption) String() string {
	return fmt.Sprintf("WebRTCOption{ICEServers: %d, StreamID: %s,ICETimeout: %v}",
		len(wts.ICEServers), wts.StreamID, wts.ICETimeout)
}
