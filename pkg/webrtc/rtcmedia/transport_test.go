package rtcmedia

import (
	"testing"

	"github.com/LingByte/LingEchoX/pkg/constants"
	"github.com/stretchr/testify/assert"
)

func TestNewWebRTCTransport(t *testing.T) {
	transport := NewWebRTCTransport(WebRTCOption{
		Codec:      constants.DefaultCodec,
		ICETimeout: constants.DefaultICETimeout,
		StreamID:   constants.DefaultStreamID,
	})

	assert.NotNil(t, transport)
	assert.Equal(t, transport.opt.Codec, constants.DefaultCodec)
	assert.Equal(t, transport.opt.ICETimeout, constants.DefaultICETimeout)
	assert.Equal(t, transport.opt.StreamID, constants.DefaultStreamID)

	// 测试各个组件是否正确初始化
	assert.NotNil(t, transport.codecMgr, "codec manager should not be nil")
	assert.NotNil(t, transport.connection, "connection should not be nil")
	assert.NotNil(t, transport.signaling, "signaling should not be nil")
	assert.NotNil(t, transport.trackMgr, "track manager should not be nil")
}

func TestCodecManager(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)

	assert.NotNil(t, codecMgr)
	assert.Equal(t, constants.CodecPCMA, codecMgr.codecName)

	config := codecMgr.GetConfig()
	assert.Equal(t, "pcma", config.Codec)
	assert.Equal(t, 8000, config.SampleRate)
	assert.Equal(t, 1, config.Channels)
}

func TestConnection(t *testing.T) {
	opt := WebRTCOption{
		Codec:      constants.CodecPCMA,
		ICETimeout: constants.DefaultICETimeout,
		StreamID:   constants.DefaultStreamID,
	}

	conn := NewConnection(opt)
	assert.NotNil(t, conn)
	assert.Equal(t, opt, conn.opt)
}
