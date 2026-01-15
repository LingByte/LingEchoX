package main

import (
	"log"
	"net/http"

	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger.Init(&logger.LogConfig{
		Daily:    true,
		Filename: "logs/server.log",
		Level:    "debug",
		MaxAge:   7,
	}, "development")

	// Set Gin to release mode
	gin.SetMode(gin.ReleaseMode)

	// Create router
	router := gin.Default()
	router.GET("/websocket", websocketHandler)

	// Start server
	logger.Info("[Server] Starting server on 8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("[Server] Failed to start server: %v", err)
	}
}

var rtcServer = rtcmedia.NewRTCServer()

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// websocketHandler handles WebSocket connections
func websocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Server] Failed to upgrade connection: %v", err)
		return
	}

	defer conn.Close()
	client, err := rtcmedia.NewRTCClient(conn, nil)
	client.OnConnected(func(*rtcmedia.WebRTCTransport, string) {})
	if err != nil {
		log.Printf("[Server] Failed to create RTCClient: %v", err)
		return
	}
	client.OnTrack(func(remote *webrtc.TrackRemote, receives *webrtc.RTPReceiver, client *rtcmedia.RTCClient) {
		logger.Info("[Server] New track received")
	})
	client.RegisterFunc()
	rtcServer.AddClient(client)
	defer rtcServer.RemoveClient(client.SessionID)
	client.SendInitMessage()
	for {
		var msg rtcmedia.SignalMessage
		if err := conn.ReadJSON(&msg); err != nil {
			logger.Error("[Server] Error reading message:", zap.Error(err))
			break
		}
		client.HandleSignalMessage(msg)
	}
}
