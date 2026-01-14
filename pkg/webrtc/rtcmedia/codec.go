package rtcmedia

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LingByte/LingEchoX/pkg/constants"
	"github.com/LingByte/LingEchoX/pkg/media"
	"github.com/pion/webrtc/v3"
)

// CodecManager 编解码器管理器
type CodecManager struct {
	codecName string
	config    media.CodecConfig
}

// NewCodecManager 创建新的编解码器管理器
func NewCodecManager(codecName string) *CodecManager {
	config := media.CodecConfig{
		Codec:         strings.ToLower(codecName),
		SampleRate:    8000,
		Channels:      1,
		BitDepth:      8,
		FrameDuration: "20ms",
	}

	return &CodecManager{
		codecName: codecName,
		config:    config,
	}
}

// GetConfig 获取编解码器配置
func (cm *CodecManager) GetConfig() media.CodecConfig {
	return cm.config
}

// GetCodecParameters 根据编解码器名称获取参数
func (cm *CodecManager) GetCodecParameters() webrtc.RTPCodecParameters {
	switch cm.codecName {
	case constants.CodecPCMA:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA, ClockRate: 8000},
			PayloadType:        8,
		}
	case constants.CodecG722:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeG722, ClockRate: 8000},
			PayloadType:        9,
		}
	case constants.CodecOPUS:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000},
			PayloadType:        111,
		}
	case constants.CodecH264:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
			PayloadType:        96,
		}
	case constants.CodecVP8:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
			PayloadType:        97,
		}
	case constants.CodecVP9:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000},
			PayloadType:        98,
		}
	default: // pcmu
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
			PayloadType:        0,
		}
	}
}

// GetMediaEngine 获取媒体引擎配置
func GetMediaEngine() *webrtc.MediaEngine {
	m := &webrtc.MediaEngine{}

	// 注册音频编解码器
	// 注册 G.722
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeG722, ClockRate: 8000},
		PayloadType:        9,
	}, webrtc.RTPCodecTypeAudio)

	// 注册 Opus
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio)

	// 注册 PCMU (G.711 μ-law)
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
		PayloadType:        0,
	}, webrtc.RTPCodecTypeAudio)

	// 注册 PCMA (G.711 A-law)
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA, ClockRate: 8000},
		PayloadType:        8,
	}, webrtc.RTPCodecTypeAudio)

	// 注册 telephone-event
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/telephone-event", ClockRate: 8000},
		PayloadType:        101,
	}, webrtc.RTPCodecTypeAudio)

	// 注册视频编解码器
	// 注册 H.264
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo)

	// 注册 VP8
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		PayloadType:        97,
	}, webrtc.RTPCodecTypeVideo)

	// 注册 VP9
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000},
		PayloadType:        98,
	}, webrtc.RTPCodecTypeVideo)

	return m
}

// SelectPreferredCodec 选择首选编解码器
func (cm *CodecManager) SelectPreferredCodec(pc *webrtc.PeerConnection) (*media.CodecConfig, error) {
	if pc == nil {
		return nil, fmt.Errorf("peer connection is nil")
	}

	localDesc := pc.LocalDescription()
	if localDesc == nil {
		return nil, fmt.Errorf("local description is nil")
	}

	sdp, err := localDesc.Unmarshal()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal local description: %w", err)
	}

	codec := media.CodecConfig{}
	for _, m := range sdp.MediaDescriptions {
		if m.MediaName.Media == string(webrtc.MediaKindAudio) {
			for _, attr := range m.Attributes {
				if attr.Key == "rtpmap" {
					if strings.HasPrefix(attr.Value, m.MediaName.Formats[0]) {
						vals := strings.Split(attr.Value, " ")[1]
						codec.Codec = strings.ToLower(strings.Split(vals, "/")[0])
						codec.SampleRate, _ = strconv.Atoi(strings.Split(vals, "/")[1])
						codec.Channels = 1
						codec.BitDepth = 8
						codec.FrameDuration = "20ms"
						return &codec, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("did not find codec in SDP")
}

// GetSampleSize 返回音频样本的字节大小
func GetSampleSize(sampleRate, bitDepth, channels int) int {
	return sampleRate * bitDepth / 1000 / 8
}
