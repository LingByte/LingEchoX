package rtcmedia

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/constants"
	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/LingByte/LingEchoX/pkg/utils"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

// RTCClient represents a WebRTC client connection
type RTCClient struct {
	Conn          *websocket.Conn
	Transport     *WebRTCTransport
	SessionID     string
	mu            sync.Mutex
	Interrupt     chan os.Signal
	Done          chan struct{}
	ConnectedFunc func(*WebRTCTransport, string)
	OnTrackFunc   func(*webrtc.TrackRemote, *webrtc.RTPReceiver, *RTCClient)
}

// SignalMessage represents a WebSocket signaling message
type SignalMessage struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

func NewRTCClient(conn *websocket.Conn, opt *WebRTCOption) (*RTCClient, error) {
	if opt == nil {
		opt = &WebRTCOption{
			Codec: constants.CodecPCMA,
			ICEServers: []webrtc.ICEServer{
				{URLs: constants.DefaultStunServers},
			},
			StreamID:   constants.DEFAULT_LINGECHOX_STREAM_ID,
			ICETimeout: constants.DefaultICETimeout,
		}
	}
	// Create session ID
	sessionID := fmt.Sprintf("session_%d", time.Now().UnixNano())
	// Create WebRTC transport
	transport := NewWebRTCTransport(*opt)
	transport.NewPeerConnection()
	// Create audio track before creating answer
	trackMgr := transport.GetTrackManager()
	if err := trackMgr.CreateAudioTxTrack(); err != nil {
		logger.Error("[Server] Failed to create audio track: ", zap.Error(err))
		return nil, err
	}
	// Setup signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	return &RTCClient{
		Conn:      conn,
		Transport: transport,
		SessionID: sessionID,
		Interrupt: interrupt,
		Done:      make(chan struct{}),
	}, nil
}

func (rtc *RTCClient) OnConnected(fn func(*WebRTCTransport, string)) {
	rtc.ConnectedFunc = fn
}

func (rtc *RTCClient) OnTrack(fn func(*webrtc.TrackRemote, *webrtc.RTPReceiver, *RTCClient)) {
	rtc.OnTrackFunc = fn
}

func (rtc *RTCClient) RegisterFunc() {
	// Set up OnTrack callback to execute custom logic
	rtc.Transport.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		rtc.mu.Lock()
		go func() {
			// 如果设置了自定义回调函数，则执行它
			if rtc.OnTrackFunc != nil {
				rtc.OnTrackFunc(track, receiver, rtc)
			} else {
				logger.Error("[Server] OnTrack callback completed: no custom callback function set")
			}
		}()
		rtc.mu.Unlock()
	})
}

func (rtc *RTCClient) SendInitMessage() {
	// Send session ID to client
	initMsg := SignalMessage{
		Type:      constants.MESSAGE_TYPE_INIT,
		SessionID: rtc.SessionID,
	}
	if err := rtc.Conn.WriteJSON(initMsg); err != nil {
		logger.Error("[Server] Failed to send init message:", zap.Error(err))
		return
	}
}

func (rtc *RTCClient) HandleSignalMessage(msg SignalMessage) {
	switch msg.Type {
	case constants.MESSAGE_TYPE_OFFER:
		rtc.handleOffer(msg)
	case constants.MESSAGE_TYPE_CONNECTED:
		rtc.handleConnection()
	default:
		log.Printf("[Server] Unknown message type: %s", msg.Type)
	}
}

func (rtc *RTCClient) handleConnection() {
	rxTrack := rtc.Transport.GetRxTrack()
	if rxTrack == nil {
		time.Sleep(100 * time.Millisecond)
		rxTrack = rtc.Transport.GetRxTrack()
	}

	if rxTrack != nil && rtc.ConnectedFunc != nil {
		rtc.ConnectedFunc(rtc.Transport, rtc.SessionID)
	}
}

func (rtc *RTCClient) handleOffer(msg SignalMessage) {
	offerData, ok := msg.Data.(map[string]interface{})
	if !ok {
		logger.Error("[Server] Invalid offer data")
		return
	}

	offerStr, ok := offerData["sdp"].(string)
	if !ok {
		log.Println("[Server] Invalid offer SDP")
		return
	}

	// Set remote description
	if err := rtc.Transport.SetRemoteDescription(offerStr); err != nil {
		log.Printf("[Server] Error setting remote description: %v", err)
		return
	}

	// Extract candidates
	candidates, ok := offerData["candidates"].([]interface{})
	if !ok {
		log.Println("[Server] Invalid candidates data")
		return
	}

	candidateStrs := utils.ExtractCandidates(candidates)
	answer, serverCandidates, err := rtc.Transport.CreateAnswer(candidateStrs)
	if err != nil {
		log.Printf("[Server] Error creating answer: %v", err)
		return
	}

	if err := rtc.Conn.WriteJSON(SignalMessage{
		Type:      constants.MESSAGE_TYPE_ANSWER,
		SessionID: rtc.SessionID,
		Data: map[string]interface{}{
			"sdp":        answer,
			"candidates": serverCandidates,
		},
	}); err != nil {
		log.Printf("[Server] Error sending answer: %v", err)
		return
	}

	logger.Info("[Server] Sent answer to client", zap.String("session", rtc.SessionID))
}

// WaitForConnection waits for the WebRTC connection to be established
func WaitForConnection(client *RTCClient) error {
	for i := 0; i < constants.MaxConnectionRetries; i++ {
		state := client.Transport.GetConnectionState()
		if state == webrtc.PeerConnectionStateConnected {
			return nil
		}

		if i%constants.ConnectionStateLogInterval == 0 {
			fmt.Printf("[Server] Waiting for connection... (state: %s)\n", state.String())
		}

		time.Sleep(constants.ConnectionRetryDelay)
	}

	return fmt.Errorf("connection timeout after %d retries", constants.MaxConnectionRetries)
}

// Close releases all resources held by the RTCClient
func (rtc *RTCClient) Close() error {
	var err error
	if rtc.Done != nil {
		close(rtc.Done)
	}
	if rtc.Transport != nil {
		if closeErr := rtc.Transport.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if rtc.Conn != nil {
		if closeErr := rtc.Conn.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if rtc.Interrupt != nil {
		signal.Stop(rtc.Interrupt)
	}

	return err
}
