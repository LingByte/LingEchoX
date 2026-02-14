package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"

	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/LingByte/LingEchoX/pkg/webrtc/constants"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia/config"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia/session"
	"github.com/gorilla/websocket"
)

// NewClient creates a new WebRTC client
func NewClient() (*session.Client, error) {
	// Initialize logger
	logger.Init(&logger.LogConfig{
		Level:      "debug",
		Filename:   "log",
		MaxSize:    5,
		MaxAge:     1,
		MaxBackups: 1,
	}, "dev")

	// Connect to WebSocket signaling server
	u := url.URL{
		Scheme: "ws",
		Host:   "localhost:8080",
		Path:   "/websocket",
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket server: %w", err)
	}

	// Create WebRTC transport
	transport, err := rtcmedia.NewWebRTCTransport(config.DefaultWebRTCOption(constants.CodecOPUS), logger.Lg)

	// Setup signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Create client using new constructor
	client := session.NewClient(logger.Lg)
	client.SetConnection(conn)
	client.Transport = transport
	client.Interrupt = interrupt

	// Create output audio file for received audio
	audioFile, err := os.Create("client_received_audio.pcm")
	if err != nil {
		return nil, fmt.Errorf("failed to create audio file: %w", err)
	}
	client.SetAudioFileWriter(audioFile)

	return client, nil
}

func main() {
	client, err := NewClient()
	if err != nil {
		log.Fatalf("[Client] Failed to create client: %v", err)
	}
	defer client.Close()

	// Initialize session
	if _, err := client.InitializeSession(); err != nil {
		log.Fatalf("[Client] Failed to initialize session: %v", err)
	}

	// Create and send offer
	if err := client.CreateAndSendOffer(); err != nil {
		log.Fatalf("[Client] Failed to create and send offer: %v", err)
	}

	// Start message listener in goroutine
	go func() {
		defer close(client.Done)
		client.StartMessageListener()
	}()

	// Wait for interrupt or done signal
	fmt.Println("[Client] Waiting for connection to establish...")
	select {
	case <-client.Interrupt:
		fmt.Println("\n[Client] Interrupted, closing connection...")
	case <-client.Done:
		fmt.Println("[Client] Connection closed")
	}
}
