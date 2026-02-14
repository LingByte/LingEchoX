package session

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/LingByte/LingEchoX/pkg/webrtc/constants"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

// StartMessageListener starts listening for WebSocket messages
func (c *Client) StartMessageListener() {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("[Client] Error reading message: %v", err)
			return
		}
		var signal rtcmedia.SignalMessage
		if err := json.Unmarshal(message, &signal); err != nil {
			log.Printf("[Client] Error unmarshaling message: %v", err)
			continue
		}
		switch signal.Type {
		case constants.MESSAGE_OFFER:
			if err := c.CreateAndSendAnswer(signal); err != nil {
				log.Printf("[Client] Error creating answer: %v", err)
			}
		case constants.MESSAGE_ANSWER:
			if err := c.HandleAnswerAndSendConnected(signal); err != nil {
				log.Printf("[Client] Error handling answer: %v", err)
			}
		case constants.MESSAGE_CONNECTED:
			c.OnConnected(c, signal)
		default:
			log.Printf("[Client] Unknown message type: %s", signal.Type)
		}
	}
}

func (c *Client) SendInitSession(sessionID string) error {
	initMsg := rtcmedia.SignalMessage{
		Type:      constants.MESSAGE_INIT,
		SessionID: sessionID,
	}
	if err := c.conn.WriteJSON(initMsg); err != nil {
		c.logger.Error(fmt.Sprintf("[Client -> Server] Failed to send init message: %v", err))
		return err
	} else {
		c.logger.Info(fmt.Sprintf("[Client <- Server] Sent init message with session ID: %s", sessionID))
	}
	return nil
}

// InitializeSession initializes the WebSocket session and returns the session ID
func (c *Client) InitializeSession() (string, error) {
	_, initMsg, err := c.conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("failed to read init message: %w", err)
	}
	var initSignal rtcmedia.SignalMessage
	if err := json.Unmarshal(initMsg, &initSignal); err != nil {
		return "", fmt.Errorf("failed to unmarshal init message: %w", err)
	}
	if initSignal.Type != constants.MESSAGE_INIT {
		return "", fmt.Errorf("unexpected message type: %s", initSignal.Type)
	}
	c.SessionID = initSignal.SessionID
	c.logger.Info(fmt.Sprintf("[Client <- Server] Connected with session ID: %s", c.SessionID))
	return c.SessionID, nil
}

// CreateAndSendOffer creates a WebRTC offer and sends it to the server
func (c *Client) CreateAndSendOffer() error {
	c.Transport.NewPeerConnection()
	offer, candidates, err := c.Transport.CreateOffer()
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}
	if err := c.conn.WriteJSON(rtcmedia.SignalMessage{
		Type:      constants.MESSAGE_OFFER,
		SessionID: c.SessionID,
		Data: map[string]interface{}{
			"sdp":        offer,
			"candidates": candidates,
		},
	}); err != nil {
		return fmt.Errorf("failed to send offer: %w", err)
	}
	c.logger.Info(fmt.Sprintf("[Client -> Server] Sent offer with SDP Created offer with %d candidates", len(candidates)))
	return nil
}

func (c *Client) CreateAndSendAnswer(msg rtcmedia.SignalMessage) error {
	offerData, ok := msg.Data.(map[string]interface{})
	if !ok {
		c.logger.Error("[Server] Invalid signal message")
		return fmt.Errorf("[Server] Invalid offer data")
	}
	offerStr, ok := offerData["sdp"].(string)
	if !ok {
		c.logger.Error("[Server] Invalid offer data")
		return fmt.Errorf("[Server] Invalid offer data")
	}
	if err := c.Transport.SetRemoteDescription(offerStr); err != nil {
		c.logger.Error("[Server] Error setting remote description")
		return err
	}
	candidates, ok := offerData["candidates"].([]interface{})
	if !ok {
		c.logger.Error("[Server] Invalid candidates data")
		return fmt.Errorf("[Server] Invalid candidates data")
	}
	candidateStrs := c.extractCandidates(candidates)
	answer, serverCandidates, err := c.Transport.CreateAnswer(candidateStrs)
	if err != nil {
		c.logger.Error("[Server] Error creating answer")
		return err
	}
	if err := c.conn.WriteJSON(rtcmedia.SignalMessage{
		Type:      constants.MESSAGE_ANSWER,
		SessionID: c.SessionID,
		Data: map[string]interface{}{
			"sdp":        answer,
			"candidates": serverCandidates,
		},
	}); err != nil {
		c.logger.Error("[Server] Error sending answer")
		return err
	}
	c.logger.Info("[Client <- Server] Sent answer with SDP")
	return nil
}

// HandleAnswerAndSendConnected handles the answer message from the server
func (c *Client) HandleAnswerAndSendConnected(msg rtcmedia.SignalMessage) error {
	answerData, ok := msg.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid answer data")
	}
	answerStr, ok := answerData["sdp"].(string)
	if !ok {
		return fmt.Errorf("invalid answer SDP")
	}
	if err := c.Transport.SetRemoteDescription(answerStr); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}
	candidates, ok := answerData["candidates"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid candidates data")
	}
	candidateStrs := c.extractCandidates(candidates)
	for _, candidate := range candidateStrs {
		if err := c.Transport.AddICECandidate(candidate); err != nil {
			log.Printf("[Client] Error adding ICE candidate: %v", err)
		}
	}
	if err := c.conn.WriteJSON(rtcmedia.SignalMessage{
		Type:      constants.MESSAGE_CONNECTED,
		SessionID: c.SessionID,
		Data:      map[string]interface{}{},
	}); err != nil {
		return fmt.Errorf("failed to send connected message: %w", err)
	}
	c.logger.Info("[Client -> Server] Sent connected message")
	if err := c.WaitForConnection(); err != nil {
		return err
	}
	return c.StartBidirectionalAudio()
}

// WaitForConnection waits for the WebRTC connection to be established
func (c *Client) WaitForConnection() error {
	for i := 0; i < constants.MaxConnectionRetries; i++ {
		state := c.Transport.GetConnectionState()
		if state == webrtc.PeerConnectionStateConnected {
			fmt.Println("[Client] WebRTC connection established")
			return nil
		}
		if i%constants.ConnectionStateLogInterval == 0 {
			fmt.Printf("[Client] Waiting for connection... (state: %s)\n", state.String())
		}
		time.Sleep(constants.ConnectionRetryDelay)
	}
	return fmt.Errorf("connection timeout after %d retries", constants.MaxConnectionRetries)
}

// StartBidirectionalAudio starts both audio sending and receiving simultaneously
func (c *Client) StartBidirectionalAudio() error {
	// Start audio receiver in goroutine
	go func() {
		if err := c.StartAudioReceiver(); err != nil {
			c.logger.Error("audio receiver error", zap.Error(err))
		}
	}()
	// Start audio sender in goroutine
	go func() {
		if err := c.StartAudioSender(); err != nil {
			c.logger.Error("audio sender error", zap.Error(err))
		}
	}()
	return nil
}
