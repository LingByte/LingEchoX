package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewProcessorRegistry(t *testing.T) {
	registry := NewProcessorRegistry()

	if registry == nil {
		t.Fatal("NewProcessorRegistry returned nil")
	}

	if registry.processors == nil {
		t.Error("processors not initialized")
	}
}

func TestProcessorRegistry_Register(t *testing.T) {
	registry := NewProcessorRegistry()

	p1 := NewFuncProcessor("test1", PriorityNormal, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})
	p2 := NewFuncProcessor("test2", PriorityHigh, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})
	p3 := NewFuncProcessor("test3", PriorityLow, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})

	registry.Register(p1)
	registry.Register(p2)
	registry.Register(p3)

	processors := registry.GetAllProcessors()
	if len(processors) != 3 {
		t.Errorf("expected 3 processors, got %d", len(processors))
	}

	// Check priority order (high to low)
	if processors[0].Priority() != PriorityHigh {
		t.Error("processors not sorted by priority")
	}
	if processors[1].Priority() != PriorityNormal {
		t.Error("processors not sorted by priority")
	}
	if processors[2].Priority() != PriorityLow {
		t.Error("processors not sorted by priority")
	}
}

func TestProcessorRegistry_Unregister(t *testing.T) {
	registry := NewProcessorRegistry()

	p1 := NewFuncProcessor("test1", PriorityNormal, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})
	p2 := NewFuncProcessor("test2", PriorityHigh, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})

	registry.Register(p1)
	registry.Register(p2)

	registry.Unregister("test1")

	processors := registry.GetAllProcessors()
	if len(processors) != 1 {
		t.Errorf("expected 1 processor, got %d", len(processors))
	}

	if processors[0].Name() != "test2" {
		t.Errorf("wrong processor remaining: %s", processors[0].Name())
	}
}

func TestProcessorRegistry_GetProcessors(t *testing.T) {
	registry := NewProcessorRegistry()
	ctx := context.Background()

	p1 := NewFuncProcessor("test1", PriorityNormal, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})
	p1.WithCondition(func(ctx context.Context, event *MediaEvent) bool {
		return event.Type == EventTypePacket
	})

	p2 := NewFuncProcessor("test2", PriorityHigh, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return nil
	})
	p2.WithCondition(func(ctx context.Context, event *MediaEvent) bool {
		return event.Type == EventTypeState
	})

	registry.Register(p1)
	registry.Register(p2)

	// Test packet event
	packetEvent := &MediaEvent{Type: EventTypePacket}
	processors := registry.GetProcessors(ctx, packetEvent)

	if len(processors) != 1 {
		t.Errorf("expected 1 processor for packet event, got %d", len(processors))
	}

	if processors[0].Name() != "test1" {
		t.Error("wrong processor selected for packet event")
	}

	// Test state event
	stateEvent := &MediaEvent{Type: EventTypeState}
	processors = registry.GetProcessors(ctx, stateEvent)

	if len(processors) != 1 {
		t.Errorf("expected 1 processor for state event, got %d", len(processors))
	}

	if processors[0].Name() != "test2" {
		t.Error("wrong processor selected for state event")
	}
}

func TestBaseProcessor(t *testing.T) {
	processor := NewBaseProcessor("test", PriorityNormal)

	if processor.Name() != "test" {
		t.Errorf("expected name 'test', got '%s'", processor.Name())
	}

	if processor.Priority() != PriorityNormal {
		t.Errorf("expected priority %d, got %d", PriorityNormal, processor.Priority())
	}

	// Test default CanHandle (should return true)
	ctx := context.Background()
	event := &MediaEvent{Type: EventTypePacket}

	if !processor.CanHandle(ctx, event) {
		t.Error("default CanHandle should return true")
	}
}

func TestBaseProcessor_WithCondition(t *testing.T) {
	processor := NewBaseProcessor("test", PriorityNormal).WithCondition(func(ctx context.Context, event *MediaEvent) bool {
		return event.SessionID == "test-session"
	})

	ctx := context.Background()

	// Test matching condition
	event1 := &MediaEvent{SessionID: "test-session"}
	if !processor.CanHandle(ctx, event1) {
		t.Error("condition should match")
	}

	// Test non-matching condition
	event2 := &MediaEvent{SessionID: "other-session"}
	if processor.CanHandle(ctx, event2) {
		t.Error("condition should not match")
	}
}

func TestFuncProcessor(t *testing.T) {
	called := false
	var receivedEvent *MediaEvent

	processor := NewFuncProcessor("test", PriorityNormal, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		called = true
		receivedEvent = event
		return nil
	})

	ctx := context.Background()
	session := NewDefaultSession()
	defer session.Close()

	event := &MediaEvent{
		Type:      EventTypePacket,
		SessionID: "test",
		Timestamp: time.Now(),
	}

	err := processor.Process(ctx, session, event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called {
		t.Error("processor function not called")
	}

	if receivedEvent != event {
		t.Error("wrong event received")
	}
}

func TestFuncProcessor_Error(t *testing.T) {
	expectedErr := errors.New("test error")

	processor := NewFuncProcessor("test", PriorityNormal, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
		return expectedErr
	})

	ctx := context.Background()
	session := NewDefaultSession()
	defer session.Close()

	event := &MediaEvent{Type: EventTypePacket}

	err := processor.Process(ctx, session, event)

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestPacketProcessor(t *testing.T) {
	called := false
	var receivedPacket MediaPacket

	processor := NewPacketProcessor("test", PriorityNormal, func(ctx context.Context, session *MediaSession, packet MediaPacket) error {
		called = true
		receivedPacket = packet
		return nil
	})

	ctx := context.Background()
	session := NewDefaultSession()
	defer session.Close()

	packet := &AudioPacket{Payload: []byte("test")}
	event := &MediaEvent{
		Type:    EventTypePacket,
		Payload: packet,
	}

	// Test CanHandle for packet event
	if !processor.CanHandle(ctx, event) {
		t.Error("should handle packet event")
	}

	// Test CanHandle for non-packet event
	stateEvent := &MediaEvent{Type: EventTypeState}
	if processor.CanHandle(ctx, stateEvent) {
		t.Error("should not handle non-packet event")
	}

	// Test Process
	err := processor.Process(ctx, session, event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called {
		t.Error("processor function not called")
	}

	if receivedPacket != packet {
		t.Error("wrong packet received")
	}
}

func TestPacketProcessor_InvalidPayload(t *testing.T) {
	processor := NewPacketProcessor("test", PriorityNormal, func(ctx context.Context, session *MediaSession, packet MediaPacket) error {
		return nil
	})

	ctx := context.Background()
	session := NewDefaultSession()
	defer session.Close()

	// Event with non-packet payload
	event := &MediaEvent{
		Type:    EventTypePacket,
		Payload: "not a packet",
	}

	err := processor.Process(ctx, session, event)

	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestProcessorPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority ProcessorPriority
		expected int
	}{
		{"Low", PriorityLow, 0},
		{"Normal", PriorityNormal, 50},
		{"High", PriorityHigh, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.priority) != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, int(tt.priority))
			}
		})
	}
}

func TestProcessorRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewProcessorRegistry()
	ctx := context.Background()

	// Concurrent registration
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			p := NewFuncProcessor("test", PriorityNormal, func(ctx context.Context, session *MediaSession, event *MediaEvent) error {
				return nil
			})
			registry.Register(p)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent access
	event := &MediaEvent{Type: EventTypePacket}
	for i := 0; i < 10; i++ {
		go func() {
			registry.GetProcessors(ctx, event)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
