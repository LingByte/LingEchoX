package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/LingByte/LingEchoX/pkg/constants"
	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/LingByte/LingEchoX/pkg/utils"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

// Constants
const (
	wsScheme = "ws"
	wsHost   = "localhost:8080"
	wsPath   = "/websocket"
)

// Client represents a WebRTC client
type Client struct {
	wsConn    *websocket.Conn
	transport *rtcmedia.WebRTCTransport
	sessionID string
	interrupt chan os.Signal
	done      chan struct{}
}

// NewClient creates a new WebRTC client
func NewClient() (*Client, error) {
	// Initialize logger
	logger.Init(&logger.LogConfig{
		Daily:    true,
		Filename: "logs/client.log",
		Level:    "debug",
		MaxAge:   7,
	}, "development")

	// Connect to WebSocket signaling server
	u := url.URL{
		Scheme: wsScheme,
		Host:   wsHost,
		Path:   wsPath,
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket server: %w", err)
	}

	// Create WebRTC transport
	transport := rtcmedia.NewWebRTCTransport(rtcmedia.WebRTCOption{
		Codec:      constants.CodecPCMA,
		ICETimeout: constants.DefaultICETimeout,
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
		StreamID: "lingecho_client",
	})

	// Setup signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	return &Client{
		wsConn:    conn,
		transport: transport,
		interrupt: interrupt,
		done:      make(chan struct{}),
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if c.transport != nil {
		c.transport.Close()
	}
	if c.wsConn != nil {
		return c.wsConn.Close()
	}
	return nil
}

// InitializeSession initializes the WebSocket session and returns the session ID
func (c *Client) InitializeSession() (string, error) {
	_, initMsg, err := c.wsConn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("failed to read init message: %w", err)
	}

	var initSignal rtcmedia.SignalMessage
	if err := json.Unmarshal(initMsg, &initSignal); err != nil {
		return "", fmt.Errorf("failed to unmarshal init message: %w", err)
	}

	if initSignal.Type != constants.MESSAGE_TYPE_INIT {
		return "", fmt.Errorf("unexpected message type: %s", initSignal.Type)
	}

	c.sessionID = initSignal.SessionID
	logger.Info("[Client] Connected with session ID", zap.String("session", c.sessionID))
	return c.sessionID, nil
}

// CreateAndSendOffer creates a WebRTC offer and sends it to the server
func (c *Client) CreateAndSendOffer() error {
	c.transport.NewPeerConnection()

	// Create audio track before creating offer
	trackMgr := c.transport.GetTrackManager()
	if err := trackMgr.CreateAudioTxTrack(); err != nil {
		return fmt.Errorf("failed to create audio track: %w", err)
	}

	offer, candidates, err := c.transport.CreateOffer()
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}

	logger.Info("[Client] Created offer", zap.Int("candidates", len(candidates)))

	offerMsg := rtcmedia.SignalMessage{
		Type:      constants.MESSAGE_TYPE_OFFER,
		SessionID: c.sessionID,
		Data: map[string]interface{}{
			"sdp":        offer,
			"candidates": candidates,
		},
	}

	offerBytes, err := json.Marshal(offerMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal offer: %w", err)
	}

	if err := c.wsConn.WriteMessage(websocket.TextMessage, offerBytes); err != nil {
		return fmt.Errorf("failed to send offer: %w", err)
	}

	logger.Info("[Client] Offer sent to server")
	return nil
}

// WaitForConnection waits for the WebRTC connection to be established
func (c *Client) WaitForConnection() error {
	for i := 0; i < constants.MaxConnectionRetries; i++ {
		state := c.transport.GetConnectionState()
		if state == webrtc.PeerConnectionStateConnected {
			logger.Info("[Client] WebRTC connection established")
			return nil
		}

		if i%constants.ConnectionStateLogInterval == 0 {
			logger.Info("[Client] Waiting for connection...", zap.String("state", state.String()))
		}

		time.Sleep(constants.ConnectionRetryDelay)
	}

	return fmt.Errorf("connection timeout after %d retries", constants.MaxConnectionRetries)
}

// HandleAnswer handles the answer message from the server
func (c *Client) HandleAnswer(msg rtcmedia.SignalMessage) error {
	answerData, ok := msg.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid answer data")
	}

	// Extract answer SDP
	answerStr, ok := answerData["sdp"].(string)
	if !ok {
		return fmt.Errorf("invalid answer SDP")
	}

	// Set remote description
	if err := c.transport.SetRemoteDescription(answerStr); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	// Extract and add ICE candidates
	candidates, ok := answerData["candidates"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid candidates data")
	}

	candidateStrs := utils.ExtractCandidates(candidates)
	for _, candidate := range candidateStrs {
		if err := c.transport.AddICECandidate(candidate); err != nil {
			logger.Warn("[Client] Error adding ICE candidate", zap.Error(err))
		}
	}

	// Send connected message
	connectedMsg := rtcmedia.SignalMessage{
		Type:      constants.MESSAGE_TYPE_CONNECTED,
		SessionID: c.sessionID,
		Data:      map[string]interface{}{},
	}
	if err := c.wsConn.WriteJSON(connectedMsg); err != nil {
		return fmt.Errorf("failed to send connected message: %w", err)
	}

	logger.Info("[Client] WebRTC connection should now be establishing...")

	// Wait for connection
	if err := c.WaitForConnection(); err != nil {
		return err
	}

	return nil
}

// StartMessageListener starts listening for WebSocket messages
func (c *Client) StartMessageListener() {
	go func() {
		defer close(c.done)
		for {
			_, message, err := c.wsConn.ReadMessage()
			if err != nil {
				logger.Error("[Client] Error reading message", zap.Error(err))
				return
			}

			var signal rtcmedia.SignalMessage
			if err := json.Unmarshal(message, &signal); err != nil {
				logger.Error("[Client] Error unmarshaling message", zap.Error(err))
				continue
			}

			switch signal.Type {
			case constants.MESSAGE_TYPE_ANSWER:
				if err := c.HandleAnswer(signal); err != nil {
					logger.Error("[Client] Error handling answer", zap.Error(err))
				}
			default:
				logger.Warn("[Client] Unknown message type", zap.String("type", signal.Type))
			}
		}
	}()
}

// SetupOnTrack sets up the OnTrack callback
func (c *Client) SetupOnTrack(callback func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	c.transport.OnTrack(callback)
}

// GetTransport returns the WebRTC transport
func (c *Client) GetTransport() *rtcmedia.WebRTCTransport {
	return c.transport
}

// Run runs the client main loop
func (c *Client) Run() error {
	defer c.Close()

	// Initialize session
	if _, err := c.InitializeSession(); err != nil {
		return err
	}

	// Create and send offer
	if err := c.CreateAndSendOffer(); err != nil {
		return err
	}

	// Start message listener
	c.StartMessageListener()

	// Wait for interrupt or done signal
	logger.Info("[Client] Client is running, press Ctrl+C to exit")
	select {
	case <-c.interrupt:
		logger.Info("[Client] Interrupted, closing connection...")
		return nil
	case <-c.done:
		logger.Info("[Client] Connection closed")
		return nil
	}
}

func main() {
	client, err := NewClient()
	if err != nil {
		log.Fatalf("[Client] Failed to create client: %v", err)
	}

	// Setup OnTrack callback to handle incoming media
	client.SetupOnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := track.Codec()
		logger.Info("[Client] Received track",
			zap.String("codec", codec.MimeType),
			zap.Uint32("clockRate", codec.ClockRate))

		// Handle incoming RTP packets
		go func() {
			packetCount := 0
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					logger.Error("[Client] Error reading RTP packet", zap.Error(err))
					return
				}
				packetCount++
				if packetCount%100 == 0 {
					logger.Info("[Client] Received RTP packets", zap.Int("count", packetCount))
				}
			}
		}()
	})

	if err := client.Run(); err != nil {
		log.Fatalf("[Client] Error: %v", err)
	}
}
