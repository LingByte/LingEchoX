package sfu

import (
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/models"
	"go.uber.org/zap"
)

// Config holds SFU configuration
type Config struct {
	Port                      int           // central node port
	StatusUpdateInterval      time.Duration // Status update interval
	HealthCheckInterval       time.Duration // Health check interval
	MaxForwardingStreams      int           // Maximum number of forwarding streams
	MinBandwidthForForwarding float64       // Minimum bandwidth for forwarding
	MaxCPUForForwarding       float64       // Maximum CPU usage for forwarding
	MaxMemoryForForwarding    float64       // Maximum memory usage for forwarding
}

// CentralNode represents the central SFU node
type CentralNode struct {
	ID           string
	Rooms        map[string]*models.Room
	mu           sync.RWMutex
	Logger       *zap.Logger
	Config       *Config
	roleAssigner *RoleAssigner
}

// NewCentralNode creates a new central SFU node
func NewCentralNode(id string, config *Config, logger *zap.Logger) (*CentralNode, error) {
	return &CentralNode{
		ID:           id,
		Rooms:        make(map[string]*models.Room),
		Config:       config,
		Logger:       logger,
		roleAssigner: NewRoleAssigner(config, logger),
	}, nil
}

// CreateRoom creates a new room
func (cn *CentralNode) CreateRoom(roomID string) *models.Room {
	cn.mu.Lock()
	defer cn.mu.Unlock()

	room := models.NewRoom(roomID)
	cn.Rooms[roomID] = room
	cn.Logger.Info("Room created", zap.String("room_id", roomID))
	return room
}

// GetRoom retrieves a room by ID
func (cn *CentralNode) GetRoom(roomID string) (*models.Room, bool) {
	cn.mu.RLock()
	defer cn.mu.RUnlock()
	room, exists := cn.Rooms[roomID]
	return room, exists
}
