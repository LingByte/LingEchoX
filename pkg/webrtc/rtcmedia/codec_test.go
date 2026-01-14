package rtcmedia

import (
	"testing"

	"github.com/LingByte/LingEchoX/pkg/constants"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
)

func TestNewCodecManager(t *testing.T) {
	tests := []struct {
		name      string
		codecName string
		expected  string
	}{
		{"PCMA codec", constants.CodecPCMA, "pcma"},
		{"PCMU codec", constants.CodecPCMU, "pcmu"},
		{"G722 codec", constants.CodecG722, "g722"},
		{"OPUS codec", constants.CodecOPUS, "opus"},
		{"Unknown codec", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewCodecManager(tt.codecName)
			assert.NotNil(t, cm)
			assert.Equal(t, tt.codecName, cm.codecName)
			assert.Equal(t, tt.expected, cm.config.Codec)
			assert.Equal(t, 8000, cm.config.SampleRate)
			assert.Equal(t, 1, cm.config.Channels)
			assert.Equal(t, 8, cm.config.BitDepth)
			assert.Equal(t, "20ms", cm.config.FrameDuration)
		})
	}
}

func TestCodecManager_GetConfig(t *testing.T) {
	cm := NewCodecManager(constants.CodecPCMA)
	config := cm.GetConfig()

	assert.Equal(t, "pcma", config.Codec)
	assert.Equal(t, 8000, config.SampleRate)
	assert.Equal(t, 1, config.Channels)
	assert.Equal(t, 8, config.BitDepth)
	assert.Equal(t, "20ms", config.FrameDuration)
}

func TestCodecManager_GetCodecParameters(t *testing.T) {
	tests := []struct {
		name         string
		codecName    string
		expectedMime string
		expectedPT   webrtc.PayloadType
		expectedRate uint32
	}{
		{
			name:         "PCMA codec",
			codecName:    constants.CodecPCMA,
			expectedMime: webrtc.MimeTypePCMA,
			expectedPT:   8,
			expectedRate: 8000,
		},
		{
			name:         "PCMU codec",
			codecName:    constants.CodecPCMU,
			expectedMime: webrtc.MimeTypePCMU,
			expectedPT:   0,
			expectedRate: 8000,
		},
		{
			name:         "G722 codec",
			codecName:    constants.CodecG722,
			expectedMime: webrtc.MimeTypeG722,
			expectedPT:   9,
			expectedRate: 8000,
		},
		{
			name:         "OPUS codec",
			codecName:    constants.CodecOPUS,
			expectedMime: webrtc.MimeTypeOpus,
			expectedPT:   111,
			expectedRate: 48000,
		},
		{
			name:         "Unknown codec defaults to PCMU",
			codecName:    "unknown",
			expectedMime: webrtc.MimeTypePCMU,
			expectedPT:   0,
			expectedRate: 8000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewCodecManager(tt.codecName)
			params := cm.GetCodecParameters()

			assert.Equal(t, tt.expectedMime, params.RTPCodecCapability.MimeType)
			assert.Equal(t, tt.expectedPT, params.PayloadType)
			assert.Equal(t, tt.expectedRate, params.RTPCodecCapability.ClockRate)
		})
	}
}

func TestGetMediaEngine(t *testing.T) {
	engine := GetMediaEngine()
	assert.NotNil(t, engine)

	// 测试媒体引擎是否正确注册了所有编解码器
	// 这里我们无法直接验证注册的编解码器，但可以确保函数不会panic
	engine2 := GetMediaEngine()
	assert.NotNil(t, engine2)
}

func TestCodecManager_SelectPreferredCodec(t *testing.T) {
	cm := NewCodecManager(constants.CodecPCMA)

	// 测试空连接
	codec, err := cm.SelectPreferredCodec(nil)
	assert.Nil(t, codec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")
}

func TestGetSampleSize(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		bitDepth   int
		channels   int
		expected   int
	}{
		{
			name:       "8kHz 8-bit mono",
			sampleRate: 8000,
			bitDepth:   8,
			channels:   1,
			expected:   8, // 8000 * 8 / 1000 / 8 = 8
		},
		{
			name:       "8kHz 16-bit mono",
			sampleRate: 8000,
			bitDepth:   16,
			channels:   1,
			expected:   16, // 8000 * 16 / 1000 / 8 = 16
		},
		{
			name:       "48kHz 16-bit stereo",
			sampleRate: 48000,
			bitDepth:   16,
			channels:   2,
			expected:   96, // 48000 * 16 / 1000 / 8 = 96
		},
		{
			name:       "Zero values",
			sampleRate: 0,
			bitDepth:   0,
			channels:   0,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSampleSize(tt.sampleRate, tt.bitDepth, tt.channels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCodecManager_SelectPreferredCodec_WithMockPC(t *testing.T) {
	// 这个测试需要一个真实的PeerConnection来测试SDP解析
	// 由于创建真实的PeerConnection比较复杂，我们先跳过这个测试
	// 在实际使用中，这个方法会在WebRTC连接建立后被调用
	t.Skip("Requires real PeerConnection with SDP for testing")
}

// 基准测试
func BenchmarkNewCodecManager(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewCodecManager(constants.CodecPCMA)
	}
}

func BenchmarkGetCodecParameters(b *testing.B) {
	cm := NewCodecManager(constants.CodecPCMA)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cm.GetCodecParameters()
	}
}

func BenchmarkGetMediaEngine(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetMediaEngine()
	}
}

func BenchmarkGetSampleSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetSampleSize(8000, 16, 1)
	}
}
