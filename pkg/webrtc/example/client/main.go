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

func main() {
	logger.Init(&logger.LogConfig{
		Level:      "debug",
		Filename:   "log",
		MaxSize:    5,
		MaxAge:     1,
		MaxBackups: 1,
	}, "dev")
	u := url.URL{
		Scheme: "ws",
		Host:   "localhost:8080",
		Path:   "/websocket",
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {

		fmt.Errorf("failed to connect to WebSocket server: %w", err)
	}
	transport, err := rtcmedia.NewWebRTCTransport(config.DefaultWebRTCOption(constants.CodecPCMA), logger.Lg)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	client := session.NewClient(logger.Lg)
	client.SetConnection(conn)
	client.Transport = transport
	client.Interrupt = interrupt
	client.SetAudioSource(session.AudioSourceMixed, "ringing.wav")
	if err != nil {
		log.Fatalf("[Client] Failed to create client: %v", err)
	}
	defer client.Close()
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
