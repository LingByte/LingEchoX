package sfu

import "testing"

func TestNewCentralNode(t *testing.T) {
	centralNode, err := NewCentralNode("test", &Config{
		Port:                      8080,
		StatusUpdateInterval:      0,
		HealthCheckInterval:       0,
		MaxForwardingStreams:      0,
		MaxMemoryForForwarding:    0,
		MinBandwidthForForwarding: 0,
		MaxCPUForForwarding:       0,
	}, nil)
	if err != nil {
		t.Errorf("NewCentralNode() error = %v", err)
		return
	}
	if centralNode == nil {
		t.Errorf("NewCentralNode() returned nil")
		return
	}
	if centralNode.ID != "test" {
		t.Errorf("NewCentralNode() ID = %s, want %s", centralNode.ID, "test")
	}
	if centralNode.Rooms == nil {
		t.Errorf("NewCentralNode() Rooms = nil, want non-nil")
	}
}

func TestCreateRoom(t *testing.T) {
	centralNode, _ := NewCentralNode("test", nil, nil)
	room := centralNode.CreateRoom("test")
	if room == nil {
		t.Errorf("CreateRoom() returned nil")
		return
	}
	if room.ID != "test" {
		t.Errorf("CreateRoom() ID = %s, want %s", room.ID, "test")
	}
	if room.Nodes == nil {
		t.Errorf("CreateRoom() Nodes = nil, want non-nil")
	}
	if room.Streams == nil {
		t.Errorf("CreateRoom() Streams = nil, want non-nil")
	}
	if room.GetAllNodes() == nil {
		t.Errorf("CreateRoom() GetAllNodes() = nil, want non-nil")
	}
	if room.GetAllStreams() == nil {
		t.Errorf("CreateRoom() GetAllStreams() = nil, want non-nil")
	}
}

func TestGetRoom(t *testing.T) {

	centralNode, _ := NewCentralNode("test", nil, nil)
	room := centralNode.CreateRoom("test")
	if room == nil {
		t.Errorf("CreateRoom() returned nil")
	}
	room, ok := centralNode.GetRoom("test")
	if !ok {
		t.Errorf("GetRoom() returned false")
	}
	if room == nil {
		t.Errorf("GetRoom() returned nil")
	}
	if room.ID != "test" {
		t.Errorf("GetRoom() ID = %s, want %s", room.ID, "test")
	}
	if room.Nodes == nil {
		t.Errorf("GetRoom() Nodes = nil, want non-nil")
	}
}
