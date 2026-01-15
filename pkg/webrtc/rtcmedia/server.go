package rtcmedia

import (
	"sync"
)

// RTCServer represents a WebRTC server connection
type RTCServer struct {
	clients map[string]*RTCClient
	mutex   sync.RWMutex
}

// NewRTCServer creates a new client manager
func NewRTCServer() *RTCServer {
	return &RTCServer{
		clients: make(map[string]*RTCClient),
	}
}

// AddClient adds a client to the manager
func (m *RTCServer) AddClient(client *RTCClient) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.clients[client.SessionID] = client
}

// RemoveClient removes a client from the manager
func (m *RTCServer) RemoveClient(sessionID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.clients, sessionID)
}

// GetClient retrieves a client by session ID
func (m *RTCServer) GetClient(sessionID string) (*RTCClient, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	client, exists := m.clients[sessionID]
	return client, exists
}
