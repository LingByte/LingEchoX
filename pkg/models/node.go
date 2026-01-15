package models

import (
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/protocol"
)

// Node represents a client node in the system
type Node struct {
	ID           string
	UserID       string
	RoomID       string
	Role         protocol.Role
	Capabilities protocol.NodeCapabilities // Node capabilities
	Metrics      protocol.NodeMetrics
	LastUpdate   time.Time
	Connections  map[string]*Connection
	mu           sync.RWMutex
}

// Connection represents a WebRTC connection
type Connection struct {
	ID             string
	PeerID         string
	PeerConnection interface{} // *webrtc.PeerConnection
	DataChannel    interface{} // *webrtc.DataChannel
	Streams        map[string]*Stream
	State          ConnectionState
	mu             sync.RWMutex
}

type ConnectionState int

const (
	ConnectionStateNew ConnectionState = iota
	ConnectionStateConnecting
	ConnectionStateConnected
	ConnectionStateDisconnected
	ConnectionStateFailed
)

// Stream represents a media stream
type Stream struct {
	ID           string
	SourceNodeID string
	Type         protocol.StreamType
	Track        interface{} // *webrtc.TrackLocal or *webrtc.TrackRemote
	Buffer       *StreamBuffer
	IsForwarding bool
	TargetNodes  []string
	Mu           sync.RWMutex
}

// SetForwarding sets the forwarding state of the stream
func (s *Stream) SetForwarding(forwarding bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.IsForwarding = forwarding
}

// GetForwarding gets the forwarding state of the stream
func (s *Stream) GetForwarding() bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.IsForwarding
}

// GetTargetNodes gets a copy of target nodes
func (s *Stream) GetTargetNodes() []string {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	nodes := make([]string, len(s.TargetNodes))
	copy(nodes, s.TargetNodes)
	return nodes
}

// StreamBuffer handles buffering for stream recovery
type StreamBuffer struct {
	Packets     []*RTPPacket
	MaxSize     int
	CurrentSize int
	mu          sync.RWMutex
}

// RTPPacket represents a buffered RTP packet
type RTPPacket struct {
	Data      []byte
	Timestamp time.Time
	Sequence  uint16
}

// Room represents a communication room
type Room struct {
	ID        string
	Nodes     map[string]*Node
	Streams   map[string]*Stream
	CreatedAt time.Time
	Mu        sync.RWMutex
}

// NewNode creates a new node instance
func NewNode(id, userID, roomID string) *Node {
	return &Node{
		ID:          id,
		UserID:      userID,
		RoomID:      roomID,
		Role:        protocol.RoleReceiver,
		Connections: make(map[string]*Connection),
		LastUpdate:  time.Now(),
	}
}

// UpdateMetrics updates node metrics
func (n *Node) UpdateMetrics(metrics protocol.NodeMetrics) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Metrics = metrics
	n.LastUpdate = time.Now()
}

// CanForward checks if node can forward streams
func (n *Node) CanForward() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.Capabilities.CanForward {
		return false
	}

	// Check if metrics allow forwarding
	if n.Metrics.BandwidthMbps < n.Capabilities.MinBandwidthMbps {
		return false
	}

	if n.Metrics.CPUUsagePercent > 80.0 {
		return false
	}

	if n.Metrics.MemoryUsagePercent > 85.0 {
		return false
	}

	return true
}

// NewStreamBuffer creates a new stream buffer
func NewStreamBuffer(maxSize int) *StreamBuffer {
	return &StreamBuffer{
		Packets:     make([]*RTPPacket, 0, maxSize),
		MaxSize:     maxSize,
		CurrentSize: 0,
	}
}

// AddPacket adds a packet to the buffer
func (sb *StreamBuffer) AddPacket(packet *RTPPacket) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.CurrentSize >= sb.MaxSize {
		// Remove oldest packet
		sb.Packets = sb.Packets[1:]
		sb.CurrentSize--
	}

	sb.Packets = append(sb.Packets, packet)
	sb.CurrentSize++
}

// GetPacketsSince returns packets since a given sequence number
func (sb *StreamBuffer) GetPacketsSince(seq uint16) []*RTPPacket {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	var packets []*RTPPacket
	for _, pkt := range sb.Packets {
		if pkt.Sequence > seq {
			packets = append(packets, pkt)
		}
	}
	return packets
}

// NewRoom creates a new room
func NewRoom(id string) *Room {
	return &Room{
		ID:        id,
		Nodes:     make(map[string]*Node),
		Streams:   make(map[string]*Stream),
		CreatedAt: time.Now(),
	}
}

// AddNode adds a node to the room
func (r *Room) AddNode(node *Node) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Nodes[node.ID] = node
}

// RemoveNode removes a node from the room
func (r *Room) RemoveNode(nodeID string) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	delete(r.Nodes, nodeID)
}

// GetNode retrieves a node by ID
func (r *Room) GetNode(nodeID string) (*Node, bool) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	node, exists := r.Nodes[nodeID]
	return node, exists
}

// GetAllNodes returns all nodes in the room (thread-safe copy)
func (r *Room) GetAllNodes() []*Node {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	nodes := make([]*Node, 0, len(r.Nodes))
	for _, node := range r.Nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetAllStreams returns all streams in the room (thread-safe copy)
func (r *Room) GetAllStreams() []*Stream {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	streams := make([]*Stream, 0, len(r.Streams))
	for _, stream := range r.Streams {
		streams = append(streams, stream)
	}
	return streams
}
