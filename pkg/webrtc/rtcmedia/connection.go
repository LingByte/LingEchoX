package rtcmedia

import (
	"fmt"
	"sync"

	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

// Connection webrtc connection
type Connection struct {
	opt         WebRTCOption
	pc          *webrtc.PeerConnection
	config      webrtc.Configuration
	candidates  []webrtc.ICECandidateInit
	mu          sync.RWMutex
	stopChannel chan struct{}
}

// NewConnection 创建新的连接管理器
func NewConnection(opt WebRTCOption) *Connection {
	return &Connection{
		opt: opt,
		config: webrtc.Configuration{
			ICEServers: opt.ICEServers,
		},
		stopChannel: make(chan struct{}),
	}
}

// Create 创建WebRTC连接
func (c *Connection) Create() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	api := webrtc.NewAPI(webrtc.WithMediaEngine(GetMediaEngine()))
	pc, err := api.NewPeerConnection(c.config)
	if err != nil {
		logrus.WithError(err).Error("Failed to create peer connection")
		return err
	}

	c.pc = pc
	c.registerEventHandlers()
	return nil
}

func (c *Connection) registerEventHandlers() {
	// ice candidate func
	c.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			c.mu.Lock()
			c.candidates = append(c.candidates, candidate.ToJSON())
			c.mu.Unlock()
			logger.Debug("ICE candidate generated", zap.String("candidate", candidate.ToJSON().Candidate))
		}
	})
	// connection state change func
	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Debug("Connection state changed", zap.String("state", state.String()))

		switch state {
		case webrtc.PeerConnectionStateConnected:
			logger.Info("Connection established", zap.String("state", state.String()))
		case webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed:
			logger.Info("Connection disconnected", zap.String("state", state.String()))
			c.mu.RLock()
			stopCh := c.stopChannel
			c.mu.RUnlock()

			if stopCh != nil {
				select {
				case stopCh <- struct{}{}:
				default:
					// channel is full, discard message
				}
			}
		}
	})
}

// GetCandidates 获取ICE候选者
func (c *Connection) GetCandidates() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var candidates []string
	for _, candidate := range c.candidates {
		candidates = append(candidates, candidate.Candidate)
	}
	return candidates
}

// AddICECandidate 添加ICE候选者
func (c *Connection) AddICECandidate(candidate string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return fmt.Errorf("peer connection is nil")
	}

	return c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

// GetState 获取连接状态
func (c *Connection) GetState() webrtc.PeerConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return webrtc.PeerConnectionStateNew
	}

	return c.pc.ConnectionState()
}

// GetSignalingState 获取信令状态
func (c *Connection) GetSignalingState() webrtc.SignalingState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return webrtc.SignalingStateStable
	}

	return c.pc.SignalingState()
}

// GetLocalDescription 获取本地描述
func (c *Connection) GetLocalDescription() *webrtc.SessionDescription {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return nil
	}

	return c.pc.LocalDescription()
}

// SetLocalDescription 设置本地描述
func (c *Connection) SetLocalDescription(desc webrtc.SessionDescription) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return fmt.Errorf("peer connection is nil")
	}

	return c.pc.SetLocalDescription(desc)
}

// SetRemoteDescription 设置远程描述
func (c *Connection) SetRemoteDescription(desc webrtc.SessionDescription) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return fmt.Errorf("peer connection is nil")
	}

	return c.pc.SetRemoteDescription(desc)
}

// CreateOffer 创建Offer
func (c *Connection) CreateOffer(options *webrtc.OfferOptions) (webrtc.SessionDescription, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return webrtc.SessionDescription{}, fmt.Errorf("peer connection is nil")
	}

	return c.pc.CreateOffer(options)
}

// CreateAnswer 创建Answer
func (c *Connection) CreateAnswer(options *webrtc.AnswerOptions) (webrtc.SessionDescription, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return webrtc.SessionDescription{}, fmt.Errorf("peer connection is nil")
	}

	return c.pc.CreateAnswer(options)
}

// AddTrack 添加轨道
func (c *Connection) AddTrack(track webrtc.TrackLocal) (*webrtc.RTPSender, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc == nil {
		return nil, fmt.Errorf("peer connection is nil")
	}

	return c.pc.AddTrack(track)
}

// OnTrack 设置轨道回调
func (c *Connection) OnTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.pc != nil {
		c.pc.OnTrack(f)
	}
}

// Close 关闭连接
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pc != nil {
		err := c.pc.Close()
		c.pc = nil

		// 安全地关闭channel
		if c.stopChannel != nil {
			select {
			case <-c.stopChannel:
				// channel已经关闭
			default:
				close(c.stopChannel)
			}
			c.stopChannel = nil
		}

		return err
	}

	// 如果pc为nil，也要安全地关闭channel
	if c.stopChannel != nil {
		select {
		case <-c.stopChannel:
			// channel已经关闭
		default:
			close(c.stopChannel)
		}
		c.stopChannel = nil
	}

	return nil
}
