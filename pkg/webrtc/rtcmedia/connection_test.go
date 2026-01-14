package rtcmedia

import (
	"testing"
	"time"

	"github.com/LingByte/LingEchoX/pkg/constants"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnection(t *testing.T) {
	opt := WebRTCOption{
		Codec:      constants.CodecPCMA,
		ICETimeout: constants.DefaultICETimeout,
		StreamID:   constants.DefaultStreamID,
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	conn := NewConnection(opt)
	assert.NotNil(t, conn)
	assert.Equal(t, opt, conn.opt)
	assert.Equal(t, opt.ICEServers, conn.config.ICEServers)
	assert.NotNil(t, conn.stopChannel)
	assert.Empty(t, conn.candidates)
	assert.Nil(t, conn.pc)
}

func TestConnection_Create(t *testing.T) {
	opt := WebRTCOption{
		Codec:      constants.CodecPCMA,
		ICETimeout: constants.DefaultICETimeout,
		StreamID:   constants.DefaultStreamID,
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	conn := NewConnection(opt)
	err := conn.Create()
	require.NoError(t, err)
	assert.NotNil(t, conn.pc)

	// 清理
	defer conn.Close()
}

func TestConnection_GetState(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时的状态
	state := conn.GetState()
	assert.Equal(t, webrtc.PeerConnectionStateNew, state)

	// 创建连接后测试状态
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	state = conn.GetState()
	assert.Equal(t, webrtc.PeerConnectionStateNew, state)
}

func TestConnection_GetSignalingState(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时的状态
	state := conn.GetSignalingState()
	assert.Equal(t, webrtc.SignalingStateStable, state)

	// 创建连接后测试状态
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	state = conn.GetSignalingState()
	assert.Equal(t, webrtc.SignalingStateStable, state)
}

func TestConnection_GetLocalDescription(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时
	desc := conn.GetLocalDescription()
	assert.Nil(t, desc)

	// 创建连接后测试
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	desc = conn.GetLocalDescription()
	assert.Nil(t, desc) // 没有设置本地描述时应该为nil
}

func TestConnection_GetCandidates(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试初始状态
	candidates := conn.GetCandidates()
	assert.Empty(t, candidates)

	// 手动添加一些候选者进行测试
	conn.candidates = []webrtc.ICECandidateInit{
		{Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 54400 typ host"},
		{Candidate: "candidate:2 1 UDP 2130706431 192.168.1.2 54401 typ host"},
	}

	candidates = conn.GetCandidates()
	assert.Len(t, candidates, 2)
	assert.Equal(t, "candidate:1 1 UDP 2130706431 192.168.1.1 54400 typ host", candidates[0])
	assert.Equal(t, "candidate:2 1 UDP 2130706431 192.168.1.2 54401 typ host", candidates[1])
}

func TestConnection_AddICECandidate(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时
	err := conn.AddICECandidate("candidate:1 1 UDP 2130706431 192.168.1.1 54400 typ host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建连接后测试
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 添加有效的ICE候选者
	err = conn.AddICECandidate("candidate:1 1 UDP 2130706431 192.168.1.1 54400 typ host")
	// 注意：这可能会失败，因为没有设置远程描述，但不应该panic
	// 我们主要测试函数不会崩溃
	assert.NotPanics(t, func() {
		conn.AddICECandidate("candidate:1 1 UDP 2130706431 192.168.1.1 54400 typ host")
	})
}

func TestConnection_CreateOffer(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时
	_, err := conn.CreateOffer(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建连接后测试
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	offer, err := conn.CreateOffer(nil)
	assert.NoError(t, err)
	assert.Equal(t, webrtc.SDPTypeOffer, offer.Type)
	assert.NotEmpty(t, offer.SDP)
}

func TestConnection_CreateAnswer(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时
	_, err := conn.CreateAnswer(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建连接后测试基本功能
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 不设置远程描述直接创建answer会失败，但不应该panic
	_, err = conn.CreateAnswer(nil)
	assert.Error(t, err) // 应该失败，因为没有远程描述
}

func TestConnection_SetLocalDescription(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	desc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	}

	// 测试未创建连接时
	err := conn.SetLocalDescription(desc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建连接后测试
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 先创建一个有效的offer
	offer, err := conn.CreateOffer(nil)
	require.NoError(t, err)

	err = conn.SetLocalDescription(offer)
	assert.NoError(t, err)
}

func TestConnection_SetRemoteDescription(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	desc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	}

	// 测试未创建连接时
	err := conn.SetRemoteDescription(desc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建连接后测试
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 设置简单的SDP会失败，但不应该panic
	err = conn.SetRemoteDescription(desc)
	assert.Error(t, err) // 预期失败，因为SDP格式不完整
}

func TestConnection_AddTrack(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时
	_, err := conn.AddTrack(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建连接后测试
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 创建一个音频轨道
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA, ClockRate: 8000},
		"audio",
		"test-stream",
	)
	require.NoError(t, err)

	sender, err := conn.AddTrack(track)
	assert.NoError(t, err)
	assert.NotNil(t, sender)
}

func TestConnection_OnTrack(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时
	callbackCalled := false
	conn.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		callbackCalled = true
	})
	assert.False(t, callbackCalled) // 回调不应该被调用

	// 创建连接后测试
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	conn.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		callbackCalled = true
	})
	// 这里我们无法直接触发OnTrack回调，但可以确保设置不会出错
	assert.False(t, callbackCalled)
}

func TestConnection_Close(t *testing.T) {
	conn := NewConnection(WebRTCOption{})

	// 测试未创建连接时关闭
	err := conn.Close()
	assert.NoError(t, err)

	// 创建连接后关闭
	err = conn.Create()
	require.NoError(t, err)

	err = conn.Close()
	assert.NoError(t, err)
	assert.Nil(t, conn.pc)

	// 再次关闭应该也不会出错
	err = conn.Close()
	assert.NoError(t, err)
}

func TestConnection_setupCallbacks(t *testing.T) {
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 测试ICE候选者回调
	// 我们无法直接触发ICE候选者生成，但可以确保回调设置不会出错
	assert.NotNil(t, conn.pc)

	// 测试连接状态变化回调
	// 同样，我们无法直接触发状态变化，但可以确保设置正确
	assert.NotNil(t, conn.stopChannel)
}

// 基准测试
func BenchmarkNewConnection(b *testing.B) {
	opt := WebRTCOption{
		Codec:      constants.CodecPCMA,
		ICETimeout: constants.DefaultICETimeout,
		StreamID:   constants.DefaultStreamID,
	}

	for i := 0; i < b.N; i++ {
		NewConnection(opt)
	}
}

func BenchmarkConnection_Create(b *testing.B) {
	opt := WebRTCOption{
		Codec:      constants.CodecPCMA,
		ICETimeout: constants.DefaultICETimeout,
		StreamID:   constants.DefaultStreamID,
	}

	for i := 0; i < b.N; i++ {
		conn := NewConnection(opt)
		conn.Create()
		conn.Close()
	}
}

func BenchmarkConnection_GetState(b *testing.B) {
	conn := NewConnection(WebRTCOption{})
	conn.Create()
	defer conn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.GetState()
	}
}

// 测试并发安全性
func TestConnection_ConcurrentAccess(t *testing.T) {
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	// 并发读取状态
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				conn.GetState()
				conn.GetSignalingState()
				conn.GetLocalDescription()
				conn.GetCandidates()
			}
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent access test")
		}
	}
}
