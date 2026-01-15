package rtcmedia

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

// Signaling 信令处理器
type Signaling struct {
	connection *Connection
	offerSDP   string
	answerSDP  string
}

// NewSignaling 创建新的信令处理器
func NewSignaling(conn *Connection) *Signaling {
	return &Signaling{
		connection: conn,
	}
}

// SetRemoteDescription 设置远程描述
// 支持两种格式：
// 1. JSON 格式的 SessionDescription: {"type":"offer","sdp":"v=0\r\n..."}
// 2. 纯 SDP 字符串: "v=0\r\n..."
func (s *Signaling) SetRemoteDescription(sdp string) error {
	logger.Info("[WebRTC] SetRemoteDescription called, OnTrack should fire if SDP contains media tracks")

	var sessionDescription webrtc.SessionDescription

	// 尝试解析为 JSON 格式
	err := json.Unmarshal([]byte(sdp), &sessionDescription)
	if err != nil {
		// 如果 JSON 解析失败，假设是纯 SDP 字符串
		logger.Info("[WebRTC] SDP is not JSON format, treating as plain SDP string")

		// 根据当前信令状态确定 SDP 类型
		sdpType := s.determineSdpType(sdp)

		sessionDescription = webrtc.SessionDescription{
			Type: sdpType,
			SDP:  sdp,
		}
		logger.Info("[WebRTC] Determined SDP type: %s", zap.String("sdpType", sdpType.String()))
	}

	// 验证和记录SDP信息
	s.validateAndLogSDP(sessionDescription)

	// 设置远程描述
	err = s.connection.SetRemoteDescription(sessionDescription)
	if err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	logger.Info("[WebRTC] SetRemoteDescription completed")
	return nil
}

// determineSdpType 根据当前状态确定SDP类型
func (s *Signaling) determineSdpType(sdp string) webrtc.SDPType {
	signalingState := s.connection.GetSignalingState()

	switch signalingState {
	case webrtc.SignalingStateStable:
		// 如果是 stable 状态，接收到的应该是 offer
		return webrtc.SDPTypeOffer
	case webrtc.SignalingStateHaveLocalOffer:
		// 如果是 have-local-offer 状态，接收到的应该是 answer
		return webrtc.SDPTypeAnswer
	default:
		// 默认情况，尝试从 SDP 内容推断类型
		if strings.Contains(sdp, "a=sendrecv") || strings.Contains(sdp, "a=recvonly") {
			return webrtc.SDPTypeAnswer
		}
		return webrtc.SDPTypeOffer
	}
}

// validateAndLogSDP 验证和记录SDP信息
func (s *Signaling) validateAndLogSDP(desc webrtc.SessionDescription) {
	if desc.SDP == "" {
		return
	}

	// 检查 SDP 中是否包含 "m=audio"（音频媒体行）
	if strings.Contains(desc.SDP, "m=audio") {
		logger.Info("[WebRTC] ✓ SDP contains audio media line (m=audio), OnTrack should fire")
	} else {
		logger.Info("[WebRTC] ✗ WARNING: SDP does NOT contain audio media line, OnTrack will NOT fire")
	}

	// 打印 SDP 的前 300 个字符用于调试
	sdpPreview := desc.SDP
	if len(sdpPreview) > 300 {
		sdpPreview = sdpPreview[:300] + "..."
	}
	logger.Info("[WebRTC] SDP preview: ", zap.String("sdpPreview", sdpPreview))
}

// CreateOffer 创建Offer
func (s *Signaling) CreateOffer() (string, []string, error) {
	// 创建 offer
	offerSDP, err := s.connection.CreateOffer(nil)
	if err != nil {
		logger.Error("[WebRTC] Failed to create offer", zap.Error(err))
		return "", nil, err
	}

	// 设置本地描述
	err = s.connection.SetLocalDescription(offerSDP)
	if err != nil {
		logger.Error("[WebRTC] Failed to set local description", zap.Error(err))
		return "", nil, err
	}

	// 等待 ICE gathering 完成
	if err := s.waitForICEGathering(); err != nil {
		return "", nil, err
	}

	// 获取候选者
	candidates := s.connection.GetCandidates()
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("no ICE candidates generated")
	}

	// 获取 offer SDP 字符串
	localDesc := s.connection.GetLocalDescription()
	if localDesc == nil {
		return "", nil, fmt.Errorf("local description is nil")
	}

	offer := localDesc.SDP
	s.offerSDP = offer

	// 记录日志
	s.logOfferGenerated(offer, len(candidates))

	return offer, candidates, nil
}

// CreateAnswer 创建Answer
func (s *Signaling) CreateAnswer(clientCandidates []string) (string, []string, error) {
	// 创建 answer
	answerSDP, err := s.connection.CreateAnswer(nil)
	if err != nil {
		logrus.WithError(err).Error("Failed to create answer")
		return "", nil, err
	}

	// 设置本地描述
	err = s.connection.SetLocalDescription(answerSDP)
	if err != nil {
		logrus.WithError(err).Error("Failed to set local description")
		return "", nil, err
	}

	// 等待 ICE gathering
	if err := s.waitForICEGathering(); err != nil {
		return "", nil, err
	}

	// 添加客户端的 ICE candidates
	for _, candidate := range clientCandidates {
		if err := s.connection.AddICECandidate(candidate); err != nil {
			logrus.WithError(err).WithField("candidate", candidate).Warn("Failed to add ICE candidate")
		}
	}

	// 获取服务器的 candidates
	serverCandidates := s.connection.GetCandidates()
	if len(serverCandidates) == 0 {
		return "", nil, fmt.Errorf("no ICE candidates generated")
	}

	// 获取 answer SDP 字符串
	localDesc := s.connection.GetLocalDescription()
	if localDesc == nil {
		return "", nil, fmt.Errorf("local description is nil")
	}

	answer := localDesc.SDP
	s.answerSDP = answer

	// 记录日志
	s.logAnswerGenerated(answer, len(serverCandidates))

	return answer, serverCandidates, nil
}

// waitForICEGathering 等待ICE收集完成
func (s *Signaling) waitForICEGathering() error {
	// 这里需要实现ICE收集等待逻辑
	// 由于Connection结构中没有直接的ICE收集完成通知，
	// 我们使用简单的超时等待
	timeout := time.After(s.connection.opt.GetICETimeout())
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("ICE gathering timeout")
		case <-ticker.C:
			candidates := s.connection.GetCandidates()
			if len(candidates) > 0 {
				time.Sleep(200 * time.Millisecond)
				return nil
			}
		}
	}
}

// logOfferGenerated 记录Offer生成日志
func (s *Signaling) logOfferGenerated(offer string, candidateCount int) {
	offerPreview := offer
	if len(offer) > 50 {
		offerPreview = offer[:50] + "..."
	}
	logger.Info("[WebRTC] Offer generated",
		zap.String("offer", offerPreview),
		zap.Int("candidateCount", candidateCount),
	)
}

// logAnswerGenerated 记录Answer生成日志
func (s *Signaling) logAnswerGenerated(answer string, candidateCount int) {
	answerPreview := answer
	if len(answer) > 50 {
		answerPreview = answer[:50] + "..."
	}
	logger.Info("[WebRTC] Answer generated",
		zap.String("answer", answerPreview),
		zap.Int("candidateCount", candidateCount),
	)
}

// GetOfferSDP 获取Offer SDP
func (s *Signaling) GetOfferSDP() string {
	return s.offerSDP
}

// GetAnswerSDP 获取Answer SDP
func (s *Signaling) GetAnswerSDP() string {
	return s.answerSDP
}
