package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	serverPort = ":8083"
	wsPath     = "/websocket"
)

// SignalMessage WebSocket信令消息
type SignalMessage struct {
	Type   string      `json:"type"`
	From   string      `json:"from,omitempty"`
	To     string      `json:"to,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	RoomID string      `json:"room_id,omitempty"`
}

// User 用户信息
type User struct {
	ID   string          `json:"id"`
	Conn *websocket.Conn `json:"-"`
}

// Room 房间信息
type Room struct {
	ID    string           `json:"id"`
	Users map[string]*User `json:"users"`
	mutex sync.RWMutex
}

// Server 视频通话服务器
type Server struct {
	rooms    map[string]*Room
	users    map[string]*User
	mutex    sync.RWMutex
	upgrader websocket.Upgrader
}

// NewServer 创建新的服务器
func NewServer() *Server {
	return &Server{
		rooms: make(map[string]*Room),
		users: make(map[string]*User),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 开发环境允许所有来源
			},
		},
	}
}

// CreateRoom 创建房间
func (s *Server) CreateRoom(roomID string) *Room {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if room, exists := s.rooms[roomID]; exists {
		return room
	}

	room := &Room{
		ID:    roomID,
		Users: make(map[string]*User),
	}
	s.rooms[roomID] = room
	return room
}

// JoinRoom 用户加入房间
func (s *Server) JoinRoom(userID, roomID string, conn *websocket.Conn) error {
	log.Printf("[Server] JoinRoom called: user=%s, room=%s", userID, roomID)

	// 创建用户
	user := &User{
		ID:   userID,
		Conn: conn,
	}

	s.mutex.Lock()
	s.users[userID] = user
	log.Printf("[Server] User %s added to global users list", userID)

	// 获取或创建房间
	room := s.rooms[roomID]
	if room == nil {
		room = &Room{
			ID:    roomID,
			Users: make(map[string]*User),
		}
		s.rooms[roomID] = room
		log.Printf("[Server] Created new room: %s", roomID)
	}
	s.mutex.Unlock()

	// 用户加入房间
	room.mutex.Lock()
	room.Users[userID] = user
	userCount := len(room.Users)
	room.mutex.Unlock()

	log.Printf("[Server] User %s joined room %s (total users: %d)", userID, roomID, userCount)

	// 通知房间内其他用户
	go s.broadcastToRoom(roomID, SignalMessage{
		Type:   "user_joined",
		From:   userID,
		RoomID: roomID,
		Data: map[string]interface{}{
			"user_id": userID,
		},
	}, userID)

	log.Printf("[Server] Initiated broadcast for user_joined message for %s", userID)
	return nil
}

// LeaveRoom 用户离开房间
func (s *Server) LeaveRoom(userID, roomID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 从用户列表删除
	delete(s.users, userID)

	// 从房间删除用户
	if room, exists := s.rooms[roomID]; exists {
		room.mutex.Lock()
		delete(room.Users, userID)
		userCount := len(room.Users)
		room.mutex.Unlock()

		// 如果房间为空，删除房间
		if userCount == 0 {
			delete(s.rooms, roomID)
			log.Printf("[Server] Room %s deleted (empty)", roomID)
		} else {
			// 通知房间内其他用户
			s.broadcastToRoom(roomID, SignalMessage{
				Type:   "user_left",
				From:   userID,
				RoomID: roomID,
				Data: map[string]interface{}{
					"user_id": userID,
				},
			}, "")
		}
	}

	log.Printf("[Server] User %s left room %s", userID, roomID)
}

// ForwardMessage 转发消息给指定用户
func (s *Server) ForwardMessage(msg SignalMessage) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if msg.To == "" {
		return fmt.Errorf("no target user specified")
	}

	targetUser, exists := s.users[msg.To]
	if !exists {
		return fmt.Errorf("target user %s not found", msg.To)
	}

	return targetUser.Conn.WriteJSON(msg)
}

// broadcastToRoom 向房间内所有用户广播消息（除了排除的用户）
func (s *Server) broadcastToRoom(roomID string, msg SignalMessage, excludeUserID string) {
	s.mutex.RLock()
	room, exists := s.rooms[roomID]
	s.mutex.RUnlock()

	if !exists {
		log.Printf("[Server] Room %s not found for broadcast", roomID)
		return
	}

	room.mutex.RLock()
	defer room.mutex.RUnlock()

	log.Printf("[Server] Broadcasting %s to room %s (excluding %s)", msg.Type, roomID, excludeUserID)
	broadcastCount := 0

	for userID, user := range room.Users {
		if userID != excludeUserID {
			if err := user.Conn.WriteJSON(msg); err != nil {
				log.Printf("[Server] Error broadcasting to user %s: %v", userID, err)
			} else {
				log.Printf("[Server] Successfully broadcasted to user %s", userID)
				broadcastCount++
			}
		}
	}

	log.Printf("[Server] Broadcast completed: %d users notified", broadcastCount)
}

// GetRoomUsers 获取房间用户列表
func (s *Server) GetRoomUsers(roomID string) []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return []string{}
	}

	room.mutex.RLock()
	defer room.mutex.RUnlock()

	users := make([]string, 0, len(room.Users))
	for userID := range room.Users {
		users = append(users, userID)
	}
	return users
}

// websocketHandler 处理WebSocket连接
func (s *Server) websocketHandler(c *gin.Context) {
	log.Printf("[Server] New WebSocket connection from %s", c.ClientIP())

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Server] Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	var userID, roomID string

	log.Printf("[Server] WebSocket connection established, waiting for messages...")

	// 处理消息循环
	for {
		var msg SignalMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("[Server] Error reading message: %v", err)
			break
		}

		log.Printf("[Server] Received message: type=%s, from=%s, to=%s", msg.Type, msg.From, msg.To)

		switch msg.Type {
		case "join":
			// 用户加入房间
			data, ok := msg.Data.(map[string]interface{})
			if !ok {
				log.Println("[Server] Invalid join data format")
				continue
			}

			userIDInterface, userOk := data["user_id"]
			roomIDInterface, roomOk := data["room_id"]

			if !userOk || !roomOk {
				log.Println("[Server] Missing user_id or room_id in join data")
				continue
			}

			userID, ok = userIDInterface.(string)
			if !ok {
				log.Println("[Server] user_id is not a string")
				continue
			}

			roomID, ok = roomIDInterface.(string)
			if !ok {
				log.Println("[Server] room_id is not a string")
				continue
			}

			log.Printf("[Server] Processing join request: user=%s, room=%s", userID, roomID)

			if err := s.JoinRoom(userID, roomID, conn); err != nil {
				log.Printf("[Server] Error joining room: %v", err)
				continue
			}

			// 发送房间用户列表
			users := s.GetRoomUsers(roomID)
			response := SignalMessage{
				Type:   "room_users",
				RoomID: roomID,
				Data: map[string]interface{}{
					"users": users,
				},
			}

			if err := conn.WriteJSON(response); err != nil {
				log.Printf("[Server] Error sending room users: %v", err)
			} else {
				log.Printf("[Server] Sent room users to %s: %v", userID, users)
			}

		case "offer", "answer", "ice_candidate":
			// 转发WebRTC信令消息
			log.Printf("[Server] Forwarding %s from %s to %s", msg.Type, msg.From, msg.To)
			if err := s.ForwardMessage(msg); err != nil {
				log.Printf("[Server] Error forwarding message: %v", err)
			}

		default:
			log.Printf("[Server] Unknown message type: %s", msg.Type)
		}
	}

	// 连接断开时清理
	if userID != "" && roomID != "" {
		log.Printf("[Server] Cleaning up disconnected user: %s from room: %s", userID, roomID)
		s.LeaveRoom(userID, roomID)
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	server := NewServer()
	router := gin.Default()

	// WebSocket路由
	router.GET(wsPath, server.websocketHandler)

	// 静态文件服务
	router.Static("/static", "./static")
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/index.html")
	})

	// API路由
	router.GET("/api/rooms", func(c *gin.Context) {
		server.mutex.RLock()
		rooms := make([]map[string]interface{}, 0, len(server.rooms))
		for roomID, room := range server.rooms {
			room.mutex.RLock()
			userCount := len(room.Users)
			room.mutex.RUnlock()

			rooms = append(rooms, map[string]interface{}{
				"id":         roomID,
				"user_count": userCount,
			})
		}
		server.mutex.RUnlock()

		c.JSON(http.StatusOK, map[string]interface{}{
			"rooms": rooms,
		})
	})

	fmt.Printf("[Server] Starting video call server on %s\n", serverPort)
	fmt.Printf("[Server] Open http://localhost%s in your browser\n", serverPort)
	log.Fatal(router.Run(serverPort))
}
