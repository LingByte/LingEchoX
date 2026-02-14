package rtcmedia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	media2 "github.com/LingByte/LingEchoX/pkg/media"
	"github.com/LingByte/LingEchoX/pkg/webrtc/constants"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia/config"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"go.uber.org/zap"
)

// WebRTCTransport WebRTC Transport
type WebRTCTransport struct {
	opt             *config.WebRTCOption           // WebRTC 配置
	config          webrtc.Configuration           // WebRTC配置
	peerConnection  *webrtc.PeerConnection         // WebRTC连接
	txTrack         *webrtc.TrackLocalStaticSample // 发送音频数据
	rxTrack         *webrtc.TrackRemote            // 接收音频数据
	connectionState webrtc.PeerConnectionState     // 连接状态
	codec           media2.CodecConfig
	Candidates      []webrtc.ICECandidateInit `json:"candidates"`       // ICE 候选者
	OfferSDP        string                    `json:"offer,omitempty"`  // Offer SDP
	AnswerSDP       string                    `json:"answer,omitempty"` // Answer SDP
	mu              sync.RWMutex              // 读写锁
	playAudioStop   chan struct{}             // 用于停止播放音频
	logger          *zap.Logger
}

// NewWebRTCTransport create new WebRTC transport
func NewWebRTCTransport(opt *config.WebRTCOption, logger *zap.Logger) (*WebRTCTransport, error) {
	if opt == nil {
		return nil, errors.New("WebRTCOption is nil")
	}
	if opt.StreamID == "" {
		opt.StreamID = constants.DefaultStreamID
	}
	if opt.ICETimeout == 0 {
		opt.ICETimeout = constants.DefaultICETimeout
	}
	if opt.Codec == "" {
		opt.Codec = constants.CodecOPUS
	}
	return &WebRTCTransport{
		opt: opt,
		config: webrtc.Configuration{
			ICEServers: opt.ICEServers,
		},
		connectionState: webrtc.PeerConnectionStateNew,
		codec: media2.CodecConfig{
			Codec:         strings.ToLower(opt.Codec),
			SampleRate:    8000,
			Channels:      1,
			BitDepth:      8,
			FrameDuration: "20ms",
		},
		logger: logger,
	}, nil
}

// getCodecParameters 根据编解码器名称获取参数
func (wts *WebRTCTransport) getCodecParameters() webrtc.RTPCodecParameters {
	switch wts.opt.Codec {
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

	//telephone-event
	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/telephone-event", ClockRate: 8000},
		PayloadType:        101,
	}, webrtc.RTPCodecTypeAudio)
	return m
}

func (wts *WebRTCTransport) NewPeerConnection() {
	wts.mu.Lock()
	defer wts.mu.Unlock()
	api := webrtc.NewAPI(webrtc.WithMediaEngine(GetMediaEngine()))
	connection, err := api.NewPeerConnection(wts.config)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("webrtc: NewPeerConnection error: %v", err))
		return
	}
	wts.peerConnection = connection
	wts.peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			wts.Candidates = append(wts.Candidates, i.ToJSON())
			wts.logger.Info(fmt.Sprintf("New ICE candidate: %+v", i.ToJSON()))
		}
	})
	wts.peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		wts.logger.Info(fmt.Sprintf("Connection state changed: %s", state.String()))
		if state == webrtc.PeerConnectionStateConnected {
			wts.logger.Info(fmt.Sprintf("[WebRTC] Connected to %s", wts.opt.StreamID))
		} else if state == webrtc.PeerConnectionStateDisconnected ||
			state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			if wts.playAudioStop != nil {
				close(wts.playAudioStop)
				wts.playAudioStop = nil
			}
		}
	})
	// 接收远程音频轨道 处理接收到的远程音轨，保存到 wts.rxTrack
	wts.peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		wts.logger.Info(fmt.Sprintf("[WebRTC] OnTrack: codec=%s, ssrc=%d, streamID=%s, kind=%s\n",
			remoteTrack.Codec().MimeType, remoteTrack.SSRC(), remoteTrack.StreamID(), remoteTrack.Kind().String()))
		wts.mu.Lock()
		wts.rxTrack = remoteTrack
		wts.mu.Unlock()
	})
	// 创建发送轨道 创建发送轨道
	wts.txTrack, err = webrtc.NewTrackLocalStaticSample(
		wts.getCodecParameters().RTPCodecCapability,
		"audio",
		wts.opt.StreamID,
	)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("webrtc: NewTrackLocalStaticSample error: %v", err))
		return
	}
	_, err = wts.peerConnection.AddTrack(wts.txTrack)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("webrtc: AddTrack error: %v", err))
		return
	}
}

func (wts *WebRTCTransport) Codec() media2.CodecConfig {
	return wts.codec
}

// SetRemoteDescription 设置远程描述
// 支持两种格式：
// 1. JSON 格式的 SessionDescription: {"type":"offer","sdp":"v=0\r\n..."}
// 2. 纯 SDP 字符串: "v=0\r\n..."
func (wts *WebRTCTransport) SetRemoteDescription(sdp string) error {
	wts.logger.Info(fmt.Sprintf("[WebRTC] SetRemoteDescription called, OnTrack should fire if SDP contains media tracks"))
	var sessionDescription webrtc.SessionDescription
	// 尝试解析为 JSON 格式
	err := json.Unmarshal([]byte(sdp), &sessionDescription)
	if err != nil {
		var sdpType webrtc.SDPType
		if wts.peerConnection.SignalingState() == webrtc.SignalingStateHaveLocalOffer {
			sdpType = webrtc.SDPTypeAnswer
		} else if wts.peerConnection.SignalingState() == webrtc.SignalingStateStable {
			sdpType = webrtc.SDPTypeOffer
		} else {
			sdpType = webrtc.SDPTypeOffer
		}

		sessionDescription = webrtc.SessionDescription{
			Type: sdpType,
			SDP:  sdp,
		}
	}
	if sessionDescription.SDP != "" {
		if strings.Contains(sessionDescription.SDP, "m=audio") {
			wts.logger.Info(fmt.Sprintf("[WebRTC] ✓ SDP contains audio media line (m=audio), OnTrack should fire"))
		} else {
			wts.logger.Info(fmt.Sprintf("[WebRTC] ✗ WARNING: SDP does NOT contain audio media line, OnTrack will NOT fire"))
		}
	}
	err = wts.peerConnection.SetRemoteDescription(sessionDescription)
	if err != nil {
		return err
	}
	return nil
}

func (wts *WebRTCTransport) CreateOffer() (offer string, candidates []string, err error) {
	if wts.peerConnection == nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] CreateOffer error: peer connection is nil"))
		return "", nil, errors.New("peer connection is nil")
	}
	wts.mu.Lock()
	defer wts.mu.Unlock()
	offerSDP, err := wts.peerConnection.CreateOffer(nil)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] CreateOffer error: %v", err))
		return
	}
	err = wts.peerConnection.SetLocalDescription(offerSDP)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] SetLocalDescription error: %v", err))
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(wts.peerConnection)
	select {
	case <-time.After(wts.opt.ICETimeout):
		err = fmt.Errorf("ICE gathering timeout")
		return
	case <-gatherComplete:
	}
	if len(wts.Candidates) == 0 {
		err = fmt.Errorf("no ICE candidates generated")
		return
	}
	for _, c := range wts.Candidates {
		candidates = append(candidates, c.Candidate)
	}
	localOfferSDP := wts.peerConnection.LocalDescription()
	offer = localOfferSDP.SDP
	wts.OfferSDP = offer
	return offer, candidates, nil
}

func (wts *WebRTCTransport) CreateAnswer(clientCandidates []string) (serverAnswer string, serverCandidates []string, err error) {
	if wts.peerConnection == nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] CreateAnswer error: peer connection is nil"))
		return "", nil, errors.New("peer connection is nil")
	}
	wts.mu.Lock()
	defer wts.mu.Unlock()
	answerSDP, err := wts.peerConnection.CreateAnswer(nil)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] CreateAnswer error: %v", err))
		return
	}
	err = wts.peerConnection.SetLocalDescription(answerSDP)
	if err != nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] SetLocalDescription error: %v", err))
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(wts.peerConnection)
	select {
	case <-time.After(wts.opt.ICETimeout):
		err = fmt.Errorf("ICE gathering timeout")
		return
	case <-gatherComplete:
	}
	if len(wts.Candidates) == 0 {
		err = fmt.Errorf("no ICE candidates generated")
		return
	}
	for _, c := range clientCandidates {
		err = wts.peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: c})
		if err != nil {
			wts.logger.Error(fmt.Sprintf("[WebRTC] AddICECandidate error: %v", err))
		}
	}
	for _, c := range wts.Candidates {
		serverCandidates = append(serverCandidates, c.Candidate)
	}
	localSDP := wts.peerConnection.LocalDescription()
	serverAnswer = localSDP.SDP
	wts.AnswerSDP = serverAnswer
	return serverAnswer, serverCandidates, nil
}

func (wts *WebRTCTransport) SelectPreferredCodec() (*media2.CodecConfig, error) {
	sdp, err := wts.peerConnection.LocalDescription().Unmarshal()
	if err != nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] SelectPreferredCodec error: %v", err))
		return nil, err
	}

	codec := media2.CodecConfig{}
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
	return nil, fmt.Errorf("webrtc: did not find codec in SDP")
}

// AddICECandidate 添加 ICE 候选者
func (wts *WebRTCTransport) AddICECandidate(candidate string) error {
	wts.mu.Lock()
	defer wts.mu.Unlock()
	return wts.peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

// GetConnectionState 获取连接状态
func (wts *WebRTCTransport) GetConnectionState() webrtc.PeerConnectionState {
	wts.mu.RLock()
	defer wts.mu.RUnlock()

	if wts.peerConnection == nil {
		return webrtc.PeerConnectionStateNew
	}

	return wts.peerConnection.ConnectionState()
}

// GetRxTrack 获取接收轨道 (线程安全)
func (wts *WebRTCTransport) GetRxTrack() *webrtc.TrackRemote {
	wts.mu.RLock()
	defer wts.mu.RUnlock()
	return wts.rxTrack
}

// GetTxTrack 获取发送轨道 (线程安全)
func (wts *WebRTCTransport) GetTxTrack() *webrtc.TrackLocalStaticSample {
	wts.mu.RLock()
	defer wts.mu.RUnlock()
	return wts.txTrack
}

func (wts *WebRTCTransport) Next(ctx context.Context) (media2.MediaPacket, error) {
	if wts.rxTrack == nil {
		time.Sleep(10 * time.Millisecond)
		return nil, nil
	}

	switch wts.connectionState {
	case webrtc.PeerConnectionStateConnected:
	default:
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Millisecond):
			//wait for connection established
		}
		wts.logger.Error(fmt.Sprintf("[WebRTC] Next error: connection state is not connected"))
		return nil, nil
	}

	rtpPacket, _, err := wts.rxTrack.ReadRTP()
	if err != nil {
		wts.logger.Error(fmt.Sprintf("[WebRTC] Next error: %v", err))
		return nil, err
	}
	return &media2.AudioPacket{
		Payload: rtpPacket.Payload,
	}, nil
}

func (wts *WebRTCTransport) Send(frame media2.MediaPacket) (int, error) {
	if wts.peerConnection == nil || wts.txTrack == nil {
		return 0, nil
	}
	switch frame.(type) {
	case *media2.AudioPacket:
	default:
		return 0, nil
	}

	audioFrame := frame.(*media2.AudioPacket)
	duration := len(audioFrame.Body()) / GetSampleSize(wts.codec.SampleRate, wts.codec.BitDepth, wts.codec.Channels)
	sample := media.Sample{
		Data:     audioFrame.Body(),
		Duration: time.Duration(duration) * time.Millisecond,
	}
	wts.txTrack.WriteSample(sample)
	return len(frame.Body()), nil
}

func (wts *WebRTCTransport) Close() error {
	if wts.txTrack != nil {
		wts.txTrack = nil
	}
	if wts.rxTrack != nil {
		wts.rxTrack = nil
	}
	if wts.peerConnection != nil {
		wts.peerConnection.Close()
		wts.peerConnection = nil
	}
	return nil
}

// GetSampleSize returns the size of an audio sample in bytes.
func GetSampleSize(sampleRate, bitDepth, channels int) int {
	return sampleRate * bitDepth / 1000 / 8
}

// OnTrack sets the OnTrack callback for the WebRTC connection
func (wts *WebRTCTransport) OnTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	wts.mu.Lock()
	defer wts.mu.Unlock()

	if wts.peerConnection != nil {
		wts.peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			// Save the remote track (this is the default behavior we want to preserve)
			wts.mu.Lock()
			wts.rxTrack = remoteTrack
			wts.mu.Unlock()

			// Log the received track
			wts.logger.Info(fmt.Sprintf("[WebRTC] OnTrack callback fired codec:%s, ssrc:%s", remoteTrack.Codec().MimeType, remoteTrack.SSRC()))
			if f != nil {
				f(remoteTrack, receiver)
			}
		})
	}
}
