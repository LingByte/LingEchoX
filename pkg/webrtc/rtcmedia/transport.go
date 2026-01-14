package rtcmedia

import (
	"context"
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/constants"
	media2 "github.com/LingByte/LingEchoX/pkg/media"
	"github.com/pion/webrtc/v3"
)

// WebRTCOption WebRTC 配置选项
type WebRTCOption struct {
	ICEServers []webrtc.ICEServer `json:"iceServers"` // ICE 服务器
	StreamID   string             `json:"streamId"`   // 流 ID
	ICETimeout time.Duration      `json:"iceTimeout"` // ICE 超时时间
	Codec      string             `json:"codec"`      // 编解码器名称
}

func (opt *WebRTCOption) GetICETimeout() time.Duration {
	if opt.ICETimeout == 0 {
		return constants.DefaultICETimeout
	}
	return opt.ICETimeout
}

// WebRTCTransport WebRTC传输层
type WebRTCTransport struct {
	opt        WebRTCOption
	connection *Connection
	signaling  *Signaling
	trackMgr   *TrackManager
	codecMgr   *CodecManager
	mu         sync.RWMutex
}

// NewWebRTCTransport 创建新的 WebRTC 传输
func NewWebRTCTransport(opt WebRTCOption) *WebRTCTransport {
	if opt.StreamID == "" {
		opt.StreamID = constants.DefaultStreamID
	}
	if opt.ICETimeout == 0 {
		opt.ICETimeout = constants.DefaultICETimeout
	}
	if opt.Codec == "" {
		opt.Codec = constants.CodecOPUS
	}

	transport := &WebRTCTransport{
		opt: opt,
	}

	// 初始化各个管理器
	transport.codecMgr = NewCodecManager(opt.Codec)
	transport.connection = NewConnection(opt)
	transport.signaling = NewSignaling(transport.connection)
	transport.trackMgr = NewTrackManager(opt.StreamID, transport.codecMgr)

	return transport
}

// NewPeerConnection 创建新的对等连接
func (t *WebRTCTransport) NewPeerConnection() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 创建连接
	if err := t.connection.Create(); err != nil {
		return
	}

	// 设置轨道管理器
	t.trackMgr.SetPeerConnection(t.connection.pc)

	// 不自动创建音频轨道，让调用者决定创建什么类型的轨道
}

// Codec 获取编解码器配置
func (t *WebRTCTransport) Codec() media2.CodecConfig {
	return t.codecMgr.GetConfig()
}

// SetRemoteDescription 设置远程描述
func (t *WebRTCTransport) SetRemoteDescription(sdp string) error {
	return t.signaling.SetRemoteDescription(sdp)
}

// CreateOffer 创建Offer
func (t *WebRTCTransport) CreateOffer() (string, []string, error) {
	return t.signaling.CreateOffer()
}

// CreateAnswer 创建Answer
func (t *WebRTCTransport) CreateAnswer(clientCandidates []string) (string, []string, error) {
	return t.signaling.CreateAnswer(clientCandidates)
}

// AddICECandidate 添加ICE候选者
func (t *WebRTCTransport) AddICECandidate(candidate string) error {
	return t.connection.AddICECandidate(candidate)
}

// GetConnectionState 获取连接状态
func (t *WebRTCTransport) GetConnectionState() webrtc.PeerConnectionState {
	return t.connection.GetState()
}

// GetTrackManager 获取轨道管理器
func (t *WebRTCTransport) GetTrackManager() *TrackManager {
	return t.trackMgr
}

// GetRxTrack 获取接收轨道
func (t *WebRTCTransport) GetRxTrack() *webrtc.TrackRemote {
	return t.trackMgr.GetRxTrack()
}

// GetTxTrack 获取发送轨道
func (t *WebRTCTransport) GetTxTrack() *webrtc.TrackLocalStaticSample {
	return t.trackMgr.GetTxTrack()
}

// OnTrack 设置OnTrack回调
func (t *WebRTCTransport) OnTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	t.trackMgr.OnTrack(f)
}

// Next 获取下一个媒体包
func (t *WebRTCTransport) Next(ctx context.Context) (media2.MediaPacket, error) {
	return t.trackMgr.Next(ctx, t.connection.GetState())
}

// Send 发送媒体包
func (t *WebRTCTransport) Send(frame media2.MediaPacket) (int, error) {
	return t.trackMgr.Send(frame, t.codecMgr.GetConfig())
}

// Close 关闭传输
func (t *WebRTCTransport) Close() error {
	if t.trackMgr != nil {
		t.trackMgr.Close()
	}
	if t.connection != nil {
		t.connection.Close()
	}
	return nil
}

// SelectPreferredCodec 选择首选编解码器
func (t *WebRTCTransport) SelectPreferredCodec() (*media2.CodecConfig, error) {
	return t.codecMgr.SelectPreferredCodec(t.connection.pc)
}
