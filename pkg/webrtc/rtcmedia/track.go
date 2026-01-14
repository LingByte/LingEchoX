package rtcmedia

import (
	"context"
	"fmt"
	"sync"
	"time"

	media2 "github.com/LingByte/LingEchoX/pkg/media"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/sirupsen/logrus"
)

// TrackManager 音视频轨道管理器
type TrackManager struct {
	streamID     string
	codecMgr     *CodecManager
	pc           *webrtc.PeerConnection
	audioTxTrack *webrtc.TrackLocalStaticSample
	videoTxTrack *webrtc.TrackLocalStaticSample
	audioRxTrack *webrtc.TrackRemote
	videoRxTrack *webrtc.TrackRemote
	mu           sync.RWMutex
	onTrackFn    func(*webrtc.TrackRemote, *webrtc.RTPReceiver)
}

// NewTrackManager 创建新的音视频轨道管理器
func NewTrackManager(streamID string, codecMgr *CodecManager) *TrackManager {
	return &TrackManager{
		streamID: streamID,
		codecMgr: codecMgr,
	}
}

// SetPeerConnection 设置对等连接
func (tm *TrackManager) SetPeerConnection(pc *webrtc.PeerConnection) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.pc = pc
	tm.setupOnTrackCallback()
}

// setupOnTrackCallback 设置OnTrack回调
func (tm *TrackManager) setupOnTrackCallback() {
	if tm.pc == nil {
		return
	}

	tm.pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		// 打印调试信息
		fmt.Printf("[WebRTC] ===== OnTrack callback FIRED! =====\n")
		fmt.Printf("[WebRTC] OnTrack: codec=%s, ssrc=%d, streamID=%s, kind=%s\n",
			remoteTrack.Codec().MimeType, remoteTrack.SSRC(), remoteTrack.StreamID(), remoteTrack.Kind().String())

		// 根据轨道类型保存到对应的变量
		tm.mu.Lock()
		if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
			tm.audioRxTrack = remoteTrack
		} else if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			tm.videoRxTrack = remoteTrack
		}
		tm.mu.Unlock()

		// 记录日志
		logrus.WithFields(logrus.Fields{
			"codec":    remoteTrack.Codec().MimeType,
			"ssrc":     remoteTrack.SSRC(),
			"streamID": remoteTrack.StreamID(),
			"kind":     remoteTrack.Kind().String(),
		}).Info("Received remote track")

		fmt.Printf("[WebRTC] OnTrack callback completed: %s track saved\n", remoteTrack.Kind().String())

		// 调用用户自定义回调
		if tm.onTrackFn != nil {
			tm.onTrackFn(remoteTrack, receiver)
		}
	})
}

// CreateTxTrack 创建发送轨道
func (tm *TrackManager) CreateTxTrack() error {
	return tm.CreateAudioTxTrack()
}

// CreateAudioTxTrack 创建音频发送轨道
func (tm *TrackManager) CreateAudioTxTrack() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.pc == nil {
		return fmt.Errorf("peer connection is nil")
	}

	// 创建音频发送轨道
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		tm.codecMgr.GetCodecParameters().RTPCodecCapability,
		"audio",
		tm.streamID,
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to create audio track")
		return err
	}

	tm.audioTxTrack = audioTrack

	// 添加音频发送轨道到连接
	_, err = tm.pc.AddTrack(tm.audioTxTrack)
	if err != nil {
		logrus.WithError(err).Error("Failed to add audio track")
		return err
	}

	return nil
}

// CreateVideoTxTrack 创建视频发送轨道
func (tm *TrackManager) CreateVideoTxTrack(codecName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.pc == nil {
		return fmt.Errorf("peer connection is nil")
	}

	// 创建视频编解码器管理器
	videoCodecMgr := NewCodecManager(codecName)

	// 创建视频发送轨道
	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		videoCodecMgr.GetCodecParameters().RTPCodecCapability,
		"video",
		tm.streamID,
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to create video track")
		return err
	}

	tm.videoTxTrack = videoTrack

	// 添加视频发送轨道到连接
	_, err = tm.pc.AddTrack(tm.videoTxTrack)
	if err != nil {
		logrus.WithError(err).Error("Failed to add video track")
		return err
	}

	return nil
}

// GetRxTrack 获取接收轨道（音频）
func (tm *TrackManager) GetRxTrack() *webrtc.TrackRemote {
	return tm.GetAudioRxTrack()
}

// GetAudioRxTrack 获取音频接收轨道
func (tm *TrackManager) GetAudioRxTrack() *webrtc.TrackRemote {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.audioRxTrack
}

// GetVideoRxTrack 获取视频接收轨道
func (tm *TrackManager) GetVideoRxTrack() *webrtc.TrackRemote {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.videoRxTrack
}

// GetTxTrack 获取发送轨道（音频）
func (tm *TrackManager) GetTxTrack() *webrtc.TrackLocalStaticSample {
	return tm.GetAudioTxTrack()
}

// GetAudioTxTrack 获取音频发送轨道
func (tm *TrackManager) GetAudioTxTrack() *webrtc.TrackLocalStaticSample {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.audioTxTrack
}

// GetVideoTxTrack 获取视频发送轨道
func (tm *TrackManager) GetVideoTxTrack() *webrtc.TrackLocalStaticSample {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.videoTxTrack
}

// OnTrack 设置OnTrack回调
func (tm *TrackManager) OnTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.onTrackFn = f

	// 如果已经有连接，重新设置回调
	if tm.pc != nil {
		tm.setupOnTrackCallback()
	}
}

// Next 获取下一个媒体包
func (tm *TrackManager) Next(ctx context.Context, connectionState webrtc.PeerConnectionState) (media2.MediaPacket, error) {
	tm.mu.RLock()
	audioRxTrack := tm.audioRxTrack
	videoRxTrack := tm.videoRxTrack
	tm.mu.RUnlock()

	// 优先处理音频轨道
	if audioRxTrack != nil {
		return tm.nextFromTrack(ctx, connectionState, audioRxTrack, "audio")
	}

	// 如果没有音频轨道，处理视频轨道
	if videoRxTrack != nil {
		return tm.nextFromTrack(ctx, connectionState, videoRxTrack, "video")
	}

	time.Sleep(10 * time.Millisecond)
	return nil, nil
}

// nextFromTrack 从指定轨道获取下一个媒体包
func (tm *TrackManager) nextFromTrack(ctx context.Context, connectionState webrtc.PeerConnectionState, track *webrtc.TrackRemote, trackType string) (media2.MediaPacket, error) {
	// 检查连接状态
	switch connectionState {
	case webrtc.PeerConnectionStateConnected:
		// 连接正常，继续处理
	default:
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// 等待连接建立
		}
		logrus.WithField("connectionState", connectionState).Info("webrtc: connection state is not connected")
		return nil, nil
	}

	// 读取RTP包
	rtpPacket, _, err := track.ReadRTP()
	if err != nil {
		logrus.WithError(err).Errorf("webrtc: Error reading RTP packet from %s track", trackType)
		return nil, err
	}

	// 根据轨道类型创建对应的媒体包
	if trackType == "audio" {
		return &media2.AudioPacket{
			Payload: rtpPacket.Payload,
		}, nil
	} else {
		return &media2.VideoPacket{
			Payload: rtpPacket.Payload,
		}, nil
	}
}

// Send 发送媒体包
func (tm *TrackManager) Send(frame media2.MediaPacket, codecConfig media2.CodecConfig) (int, error) {
	tm.mu.RLock()
	audioTxTrack := tm.audioTxTrack
	videoTxTrack := tm.videoTxTrack
	pc := tm.pc
	tm.mu.RUnlock()

	if pc == nil {
		return 0, nil
	}

	// 根据媒体包类型选择对应的轨道
	switch packet := frame.(type) {
	case *media2.AudioPacket:
		if audioTxTrack == nil {
			return 0, nil
		}
		return tm.sendToTrack(audioTxTrack, packet.Body(), codecConfig)
	case *media2.VideoPacket:
		if videoTxTrack == nil {
			return 0, nil
		}
		return tm.sendToTrack(videoTxTrack, packet.Body(), codecConfig)
	default:
		return 0, nil
	}
}

// sendToTrack 发送数据到指定轨道
func (tm *TrackManager) sendToTrack(track *webrtc.TrackLocalStaticSample, data []byte, codecConfig media2.CodecConfig) (int, error) {
	// 对于视频轨道，使用固定的帧持续时间
	var duration time.Duration
	if track.Kind() == webrtc.RTPCodecTypeVideo {
		// 假设30fps，每帧33.33ms
		duration = time.Millisecond * 33
	} else {
		// 音频轨道使用原来的计算方法
		durationMs := len(data) / GetSampleSize(codecConfig.SampleRate, codecConfig.BitDepth, codecConfig.Channels)
		duration = time.Duration(durationMs) * time.Millisecond
	}

	sample := media.Sample{
		Data:     data,
		Duration: duration,
	}

	// 发送样本
	if err := track.WriteSample(sample); err != nil {
		return 0, err
	}

	return len(data), nil
}

// Close 关闭音视频轨道管理器
func (tm *TrackManager) Close() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.audioTxTrack = nil
	tm.videoTxTrack = nil
	tm.audioRxTrack = nil
	tm.videoRxTrack = nil
	tm.pc = nil
	tm.onTrackFn = nil

	return nil
}
