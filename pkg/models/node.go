package models

import (
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/protocol"
)

// Node 表示系统中的客户端节点
type Node struct {
	ID           string
	UserID       string
	RoomID       string
	Role         protocol.Role
	Capabilities protocol.NodeCapabilities
	Metrics      protocol.NodeMetrics
	LastUpdate   time.Time
	Connections  map[string]*Connection

	// 该节点当前正在转发的流
	ForwardingStreams []string // 流 ID 列表

	mu sync.RWMutex
}

// Connection 表示 WebRTC 连接
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

// Stream 表示媒体流
type Stream struct {
	ID           string
	SourceNodeID string
	Type         protocol.StreamType
	Track        interface{} // *webrtc.TrackLocal 或 *webrtc.TrackRemote
	Buffer       *StreamBuffer
	IsForwarding bool
	TargetNodes  []string
	Mu           sync.RWMutex
}

// SetForwarding 设置流的转发状态
func (s *Stream) SetForwarding(forwarding bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.IsForwarding = forwarding
}

// GetForwarding 获取流的转发状态
func (s *Stream) GetForwarding() bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.IsForwarding
}

// GetTargetNodes 获取目标节点的副本
func (s *Stream) GetTargetNodes() []string {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	nodes := make([]string, len(s.TargetNodes))
	copy(nodes, s.TargetNodes)
	return nodes
}

// StreamBuffer 处理流恢复的缓冲
type StreamBuffer struct {
	Packets     []*RTPPacket
	MaxSize     int
	CurrentSize int
	mu          sync.RWMutex
}

// RTPPacket 表示缓冲的 RTP 包
type RTPPacket struct {
	Data      []byte
	Timestamp time.Time
	Sequence  uint16
}

// Room 表示通信房间
type Room struct {
	ID        string
	Nodes     map[string]*Node
	Streams   map[string]*Stream
	CreatedAt time.Time
	Mu        sync.RWMutex
}

// NewNode 创建新的节点实例
func NewNode(id, userID, roomID string) *Node {
	return &Node{
		ID:                id,
		UserID:            userID,
		RoomID:            roomID,
		Role:              protocol.RoleReceiver,
		Connections:       make(map[string]*Connection),
		ForwardingStreams: make([]string, 0),
		LastUpdate:        time.Now(),
	}
}

// UpdateMetrics 更新节点指标
func (n *Node) UpdateMetrics(metrics protocol.NodeMetrics) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Metrics = metrics
	n.LastUpdate = time.Now()
}

// CanForward 检查节点是否能够转发流
func (n *Node) CanForward() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.Capabilities.CanForward {
		return false
	}

	// 检查指标是否允许转发
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

// SetForwardingStreams 设置该节点正在转发的流列表
func (n *Node) SetForwardingStreams(streamIDs []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ForwardingStreams = make([]string, len(streamIDs))
	copy(n.ForwardingStreams, streamIDs)
}

// GetForwardingStreams 返回转发流列表的副本
func (n *Node) GetForwardingStreams() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	streams := make([]string, len(n.ForwardingStreams))
	copy(streams, n.ForwardingStreams)
	return streams
}

// AddForwardingStream 添加流到转发列表
func (n *Node) AddForwardingStream(streamID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	// 检查是否已存在
	for _, id := range n.ForwardingStreams {
		if id == streamID {
			return
		}
	}
	n.ForwardingStreams = append(n.ForwardingStreams, streamID)
}

// RemoveForwardingStream 从转发列表中移除流
func (n *Node) RemoveForwardingStream(streamID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, id := range n.ForwardingStreams {
		if id == streamID {
			n.ForwardingStreams = append(n.ForwardingStreams[:i], n.ForwardingStreams[i+1:]...)
			return
		}
	}
}

// NewStreamBuffer 创建新的流缓冲
func NewStreamBuffer(maxSize int) *StreamBuffer {
	return &StreamBuffer{
		Packets:     make([]*RTPPacket, 0, maxSize),
		MaxSize:     maxSize,
		CurrentSize: 0,
	}
}

// AddPacket 添加包到缓冲区
func (sb *StreamBuffer) AddPacket(packet *RTPPacket) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.CurrentSize >= sb.MaxSize {
		// 移除最旧的包
		sb.Packets = sb.Packets[1:]
		sb.CurrentSize--
	}

	sb.Packets = append(sb.Packets, packet)
	sb.CurrentSize++
}

// GetPacketsSince 返回给定序列号之后的包
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

// NewRoom 创建新房间
func NewRoom(id string) *Room {
	return &Room{
		ID:        id,
		Nodes:     make(map[string]*Node),
		Streams:   make(map[string]*Stream),
		CreatedAt: time.Now(),
	}
}

// AddNode 添加节点到房间
func (r *Room) AddNode(node *Node) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Nodes[node.ID] = node
}

// RemoveNode 从房间移除节点
func (r *Room) RemoveNode(nodeID string) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	delete(r.Nodes, nodeID)
}

// GetNode 通过 ID 获取节点
func (r *Room) GetNode(nodeID string) (*Node, bool) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	node, exists := r.Nodes[nodeID]
	return node, exists
}

// GetAllNodes 返回房间中所有节点（线程安全的副本）
func (r *Room) GetAllNodes() []*Node {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	nodes := make([]*Node, 0, len(r.Nodes))
	for _, node := range r.Nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetAllStreams 返回房间中所有流（线程安全的副本）
func (r *Room) GetAllStreams() []*Stream {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	streams := make([]*Stream, 0, len(r.Streams))
	for _, stream := range r.Streams {
		streams = append(streams, stream)
	}
	return streams
}
