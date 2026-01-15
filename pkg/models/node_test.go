package models

import (
	"testing"
	"time"

	"github.com/LingByte/LingEchoX/pkg/protocol"
)

func TestNewNode(t *testing.T) {
	node := NewNode("node1", "user1", "room1")

	if node.ID != "node1" {
		t.Errorf("Expected ID 'node1', got '%s'", node.ID)
	}

	if node.UserID != "user1" {
		t.Errorf("Expected UserID 'user1', got '%s'", node.UserID)
	}

	if node.RoomID != "room1" {
		t.Errorf("Expected RoomID 'room1', got '%s'", node.RoomID)
	}

	if node.Role != protocol.RoleReceiver {
		t.Errorf("Expected default role RECEIVER, got %v", node.Role)
	}

	if node.Connections == nil {
		t.Error("Connections map should be initialized")
	}
}

func TestUpdateMetrics(t *testing.T) {
	node := NewNode("node1", "user1", "room1")

	metrics := protocol.NodeMetrics{
		BandwidthMbps:      10.0,
		CPUUsagePercent:    50.0,
		MemoryUsagePercent: 60.0,
		RTTMs:              50,
		PacketLossPercent:  1.0,
		Timestamp:          time.Now().Unix(),
	}

	node.UpdateMetrics(metrics)

	if node.Metrics.BandwidthMbps != 10.0 {
		t.Errorf("Expected bandwidth 10.0, got %f", node.Metrics.BandwidthMbps)
	}

	if node.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}
}

func TestCanForward(t *testing.T) {
	tests := []struct {
		name         string
		capabilities protocol.NodeCapabilities
		metrics      protocol.NodeMetrics
		expected     bool
	}{
		{
			name: "can forward with good metrics",
			capabilities: protocol.NodeCapabilities{
				CanForward:       true,
				MinBandwidthMbps: 2.0,
			},
			metrics: protocol.NodeMetrics{
				BandwidthMbps:      10.0,
				CPUUsagePercent:    50.0,
				MemoryUsagePercent: 60.0,
			},
			expected: true,
		},
		{
			name: "cannot forward - capability disabled",
			capabilities: protocol.NodeCapabilities{
				CanForward: false,
			},
			metrics: protocol.NodeMetrics{
				BandwidthMbps: 10.0,
			},
			expected: false,
		},
		{
			name: "cannot forward - low bandwidth",
			capabilities: protocol.NodeCapabilities{
				CanForward:       true,
				MinBandwidthMbps: 5.0,
			},
			metrics: protocol.NodeMetrics{
				BandwidthMbps:      1.0,
				CPUUsagePercent:    50.0,
				MemoryUsagePercent: 60.0,
			},
			expected: false,
		},
		{
			name: "cannot forward - high CPU",
			capabilities: protocol.NodeCapabilities{
				CanForward:       true,
				MinBandwidthMbps: 2.0,
			},
			metrics: protocol.NodeMetrics{
				BandwidthMbps:      10.0,
				CPUUsagePercent:    90.0,
				MemoryUsagePercent: 60.0,
			},
			expected: false,
		},
		{
			name: "cannot forward - high memory",
			capabilities: protocol.NodeCapabilities{
				CanForward:       true,
				MinBandwidthMbps: 2.0,
			},
			metrics: protocol.NodeMetrics{
				BandwidthMbps:      10.0,
				CPUUsagePercent:    50.0,
				MemoryUsagePercent: 90.0,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode("node1", "user1", "room1")
			node.Capabilities = tt.capabilities
			node.Metrics = tt.metrics

			result := node.CanForward()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNewStreamBuffer(t *testing.T) {
	buffer := NewStreamBuffer(100)

	if buffer.MaxSize != 100 {
		t.Errorf("Expected MaxSize 100, got %d", buffer.MaxSize)
	}

	if buffer.CurrentSize != 0 {
		t.Errorf("Expected CurrentSize 0, got %d", buffer.CurrentSize)
	}

	if buffer.Packets == nil {
		t.Error("Packets slice should be initialized")
	}
}

func TestStreamBuffer_AddPacket(t *testing.T) {
	buffer := NewStreamBuffer(3)

	// Add packets
	for i := 0; i < 5; i++ {
		packet := &RTPPacket{
			Data:      []byte{byte(i)},
			Timestamp: time.Now(),
			Sequence:  uint16(i),
		}
		buffer.AddPacket(packet)
	}

	// Should only keep last 3 packets
	if buffer.CurrentSize != 3 {
		t.Errorf("Expected CurrentSize 3, got %d", buffer.CurrentSize)
	}

	if buffer.Packets[0].Sequence != 2 {
		t.Errorf("Expected first packet sequence 2, got %d", buffer.Packets[0].Sequence)
	}
}

func TestStreamBuffer_GetPacketsSince(t *testing.T) {
	buffer := NewStreamBuffer(10)

	// Add packets with sequences 1-5
	for i := 1; i <= 5; i++ {
		packet := &RTPPacket{
			Data:      []byte{byte(i)},
			Timestamp: time.Now(),
			Sequence:  uint16(i),
		}
		buffer.AddPacket(packet)
	}

	// Get packets since sequence 3
	packets := buffer.GetPacketsSince(3)

	if len(packets) != 2 {
		t.Errorf("Expected 2 packets, got %d", len(packets))
	}

	if packets[0].Sequence != 4 {
		t.Errorf("Expected first packet sequence 4, got %d", packets[0].Sequence)
	}
}

func TestNewRoom(t *testing.T) {
	room := NewRoom("room1")

	if room.ID != "room1" {
		t.Errorf("Expected ID 'room1', got '%s'", room.ID)
	}

	if room.Nodes == nil {
		t.Error("Nodes map should be initialized")
	}

	if room.Streams == nil {
		t.Error("Streams map should be initialized")
	}

	if room.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestRoom_AddNode(t *testing.T) {
	room := NewRoom("room1")
	node := NewNode("node1", "user1", "room1")

	room.AddNode(node)

	if len(room.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(room.Nodes))
	}

	retrieved, exists := room.GetNode("node1")
	if !exists {
		t.Error("Node should exist")
	}

	if retrieved.ID != "node1" {
		t.Errorf("Expected node ID 'node1', got '%s'", retrieved.ID)
	}
}

func TestRoom_RemoveNode(t *testing.T) {
	room := NewRoom("room1")
	node := NewNode("node1", "user1", "room1")

	room.AddNode(node)
	room.RemoveNode("node1")

	if len(room.Nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(room.Nodes))
	}

	_, exists := room.GetNode("node1")
	if exists {
		t.Error("Node should not exist")
	}
}

func TestRoom_GetAllNodes(t *testing.T) {
	room := NewRoom("room1")

	node1 := NewNode("node1", "user1", "room1")
	node2 := NewNode("node2", "user2", "room1")

	room.AddNode(node1)
	room.AddNode(node2)

	nodes := room.GetAllNodes()

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}
}

func TestStream_SetForwarding(t *testing.T) {
	stream := &Stream{
		ID:           "stream1",
		SourceNodeID: "node1",
		Type:         protocol.StreamTypeAudio,
		IsForwarding: false,
	}

	stream.SetForwarding(true)

	if !stream.GetForwarding() {
		t.Error("Stream should be forwarding")
	}

	stream.SetForwarding(false)

	if stream.GetForwarding() {
		t.Error("Stream should not be forwarding")
	}
}

func TestStream_GetTargetNodes(t *testing.T) {
	stream := &Stream{
		ID:          "stream1",
		TargetNodes: []string{"node1", "node2", "node3"},
	}

	nodes := stream.GetTargetNodes()

	if len(nodes) != 3 {
		t.Errorf("Expected 3 target nodes, got %d", len(nodes))
	}

	// Verify it's a copy
	nodes[0] = "modified"
	if stream.TargetNodes[0] == "modified" {
		t.Error("TargetNodes should be a copy, not a reference")
	}
}
