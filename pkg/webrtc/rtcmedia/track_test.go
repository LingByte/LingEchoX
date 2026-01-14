package rtcmedia

import (
	"context"
	"testing"
	"time"

	"github.com/LingByte/LingEchoX/pkg/constants"
	media2 "github.com/LingByte/LingEchoX/pkg/media"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTrackManager(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	assert.NotNil(t, trackMgr)
	assert.Equal(t, "test-stream", trackMgr.streamID)
	assert.Equal(t, codecMgr, trackMgr.codecMgr)
	assert.Nil(t, trackMgr.pc)
	assert.Nil(t, trackMgr.audioTxTrack)
	assert.Nil(t, trackMgr.audioRxTrack)
	assert.Nil(t, trackMgr.videoTxTrack)
	assert.Nil(t, trackMgr.videoRxTrack)
	assert.Nil(t, trackMgr.onTrackFn)
}

func TestTrackManager_SetPeerConnection(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 创建一个真实的PeerConnection
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	assert.Equal(t, conn.pc, trackMgr.pc)
}

func TestTrackManager_CreateTxTrack(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 测试没有PeerConnection的情况
	err := trackMgr.CreateTxTrack()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "peer connection is nil")

	// 创建PeerConnection后测试
	conn := NewConnection(WebRTCOption{})
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	err = trackMgr.CreateTxTrack()
	assert.NoError(t, err)
	assert.NotNil(t, trackMgr.GetTxTrack())
}

func TestTrackManager_GetRxTrack(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 初始状态应该为nil
	assert.Nil(t, trackMgr.GetRxTrack())

	// 手动设置一个rxTrack进行测试
	// 注意：这里我们无法创建真实的TrackRemote，所以只测试getter/setter逻辑
	trackMgr.mu.Lock()
	// trackMgr.rxTrack = someTrack // 无法创建真实的TrackRemote
	trackMgr.mu.Unlock()

	// 仍然应该是nil，因为我们没有设置真实的track
	assert.Nil(t, trackMgr.GetRxTrack())
}

func TestTrackManager_GetTxTrack(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 初始状态应该为nil
	assert.Nil(t, trackMgr.GetTxTrack())

	// 创建发送轨道后测试
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	err = trackMgr.CreateTxTrack()
	require.NoError(t, err)

	txTrack := trackMgr.GetTxTrack()
	assert.NotNil(t, txTrack)
}

func TestTrackManager_OnTrack(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	callbackCalled := false

	// 设置回调函数
	trackMgr.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		callbackCalled = true
		// 在实际使用中，这里会处理接收到的轨道
		_ = track
		_ = receiver
	})

	assert.NotNil(t, trackMgr.onTrackFn)

	// 测试设置PeerConnection后回调是否正确设置
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	assert.NotNil(t, trackMgr.pc)

	// 我们无法直接触发OnTrack回调，但可以确保设置不会出错
	assert.False(t, callbackCalled) // 回调还没有被触发
}

func TestTrackManager_Next(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	ctx := context.Background()

	// 测试没有rxTrack的情况
	packet, err := trackMgr.Next(ctx, webrtc.PeerConnectionStateConnected)
	assert.Nil(t, packet)
	assert.Nil(t, err)

	// 测试连接状态不是connected的情况
	packet, err = trackMgr.Next(ctx, webrtc.PeerConnectionStateConnecting)
	assert.Nil(t, packet)
	assert.Nil(t, err)

	// 测试context取消的情况
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// 在非连接状态下，应该快速返回而不是等待context取消
	packet, err = trackMgr.Next(cancelCtx, webrtc.PeerConnectionStateConnecting)
	assert.Nil(t, packet)
	assert.Nil(t, err) // 应该是nil，因为会快速返回
}

func TestTrackManager_Send(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	codecConfig := codecMgr.GetConfig()

	// 测试没有PeerConnection的情况
	audioPacket := &media2.AudioPacket{
		Payload: []byte{1, 2, 3, 4},
	}

	n, err := trackMgr.Send(audioPacket, codecConfig)
	assert.Equal(t, 0, n)
	assert.Nil(t, err)

	// 创建PeerConnection和TxTrack后测试
	conn := NewConnection(WebRTCOption{})
	err = conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	err = trackMgr.CreateTxTrack()
	require.NoError(t, err)

	// 现在发送音频包应该成功
	n, err = trackMgr.Send(audioPacket, codecConfig)
	assert.Equal(t, len(audioPacket.Body()), n)
	assert.Nil(t, err)
}

// 定义测试用的非音频包类型
type TestPacket struct {
	data []byte
}

// 实现MediaPacket接口
func (tp *TestPacket) Body() []byte   { return tp.data }
func (tp *TestPacket) String() string { return "test packet" }

func TestTrackManager_Send_NonAudioPacket(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)
	codecConfig := codecMgr.GetConfig()

	// 创建PeerConnection和TxTrack
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	err = trackMgr.CreateTxTrack()
	require.NoError(t, err)

	// 测试非音频包的情况
	testPacket := &TestPacket{data: []byte{1, 2, 3, 4}}

	// TestPacket没有实现AudioPacket接口，所以Send应该返回0
	n, err := trackMgr.Send(testPacket, codecConfig)
	assert.Equal(t, 0, n)
	assert.Nil(t, err)
}

func TestTrackManager_Close(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 创建一些资源
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	err = trackMgr.CreateTxTrack()
	require.NoError(t, err)

	trackMgr.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {})

	// 关闭应该清理所有资源
	err = trackMgr.Close()
	assert.NoError(t, err)
	assert.Nil(t, trackMgr.audioTxTrack)
	assert.Nil(t, trackMgr.audioRxTrack)
	assert.Nil(t, trackMgr.videoTxTrack)
	assert.Nil(t, trackMgr.videoRxTrack)
	assert.Nil(t, trackMgr.pc)
	assert.Nil(t, trackMgr.onTrackFn)
}

func TestTrackManager_setupOnTrackCallback(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 测试没有PeerConnection的情况
	trackMgr.setupOnTrackCallback()
	// 应该不会panic

	// 设置PeerConnection后测试
	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	trackMgr.setupOnTrackCallback()
	// 应该不会panic
}

// 测试并发安全性
func TestTrackManager_ConcurrentAccess(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	conn := NewConnection(WebRTCOption{})
	err := conn.Create()
	require.NoError(t, err)
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	err = trackMgr.CreateTxTrack()
	require.NoError(t, err)

	// 并发读取轨道
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				trackMgr.GetRxTrack()
				trackMgr.GetTxTrack()
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

// 基准测试
func BenchmarkNewTrackManager(b *testing.B) {
	codecMgr := NewCodecManager(constants.CodecPCMA)

	for i := 0; i < b.N; i++ {
		NewTrackManager("test-stream", codecMgr)
	}
}

func BenchmarkTrackManager_GetTxTrack(b *testing.B) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	conn := NewConnection(WebRTCOption{})
	conn.Create()
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	trackMgr.CreateTxTrack()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trackMgr.GetTxTrack()
	}
}

func BenchmarkTrackManager_GetRxTrack(b *testing.B) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trackMgr.GetRxTrack()
	}
}

func BenchmarkTrackManager_Send(b *testing.B) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)
	codecConfig := codecMgr.GetConfig()

	conn := NewConnection(WebRTCOption{})
	conn.Create()
	defer conn.Close()

	trackMgr.SetPeerConnection(conn.pc)
	trackMgr.CreateTxTrack()

	audioPacket := &media2.AudioPacket{
		Payload: []byte{1, 2, 3, 4},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trackMgr.Send(audioPacket, codecConfig)
	}
}

// 测试边界情况
func TestTrackManager_EdgeCases(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("", codecMgr) // 空streamID

	assert.Equal(t, "", trackMgr.streamID)

	// 测试nil codecMgr
	trackMgr2 := NewTrackManager("test", nil)
	assert.Nil(t, trackMgr2.codecMgr)

	// 测试多次Close
	err := trackMgr.Close()
	assert.NoError(t, err)

	err = trackMgr.Close()
	assert.NoError(t, err) // 应该不会出错
}

// 测试Next方法的超时情况
func TestTrackManager_Next_Timeout(t *testing.T) {
	codecMgr := NewCodecManager(constants.CodecPCMA)
	trackMgr := NewTrackManager("test-stream", codecMgr)

	// 创建一个带超时的context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 测试非连接状态下的超时
	start := time.Now()
	packet, err := trackMgr.Next(ctx, webrtc.PeerConnectionStateConnecting)
	duration := time.Since(start)

	assert.Nil(t, packet)
	assert.Nil(t, err)                              // 应该在超时前返回
	assert.True(t, duration < 200*time.Millisecond) // 应该很快返回，不会等到超时
}
