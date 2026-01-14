package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewEventBus(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 2)

	if bus == nil {
		t.Fatal("NewEventBus returned nil")
	}

	if bus.eventQueue == nil {
		t.Error("eventQueue not initialized")
	}

	if bus.subscribers == nil {
		t.Error("subscribers not initialized")
	}

	if bus.workers != 2 {
		t.Errorf("expected 2 workers, got %d", bus.workers)
	}

	bus.Close()
}

func TestEventBus_Subscribe(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 1)
	defer bus.Close()

	called := false
	handler := func(ctx context.Context, event *MediaEvent) error {
		called = true
		return nil
	}

	bus.Subscribe(EventTypePacket, handler)

	if len(bus.subscribers[EventTypePacket]) != 1 {
		t.Errorf("expected 1 subscriber, got %d", len(bus.subscribers[EventTypePacket]))
	}

	// Test event delivery
	bus.Publish(&MediaEvent{
		Type:      EventTypePacket,
		Timestamp: time.Now(),
		SessionID: "test",
	})

	time.Sleep(100 * time.Millisecond)

	if !called {
		t.Error("handler was not called")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 1)
	defer bus.Close()

	handler := func(ctx context.Context, event *MediaEvent) error {
		return nil
	}

	bus.Subscribe(EventTypePacket, handler)

	if len(bus.subscribers[EventTypePacket]) != 1 {
		t.Error("handler not subscribed")
	}

	bus.Unsubscribe(EventTypePacket, handler)

	if len(bus.subscribers[EventTypePacket]) != 0 {
		t.Error("handler not unsubscribed")
	}
}

func TestEventBus_Publish(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 2)
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
		if event.SessionID != "test-session" {
			t.Errorf("expected session ID 'test-session', got '%s'", event.SessionID)
		}
		wg.Done()
		return nil
	})

	bus.Publish(&MediaEvent{
		Type:      EventTypePacket,
		Timestamp: time.Now(),
		SessionID: "test-session",
		Payload:   &AudioPacket{},
	})

	wg.Wait()
}

func TestEventBus_PublishQueueFull(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 1, 1)
	defer bus.Close()

	// Block the worker
	blockChan := make(chan struct{})
	bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
		<-blockChan
		return nil
	})

	// Fill the queue
	bus.Publish(&MediaEvent{Type: EventTypePacket, SessionID: "1"})
	bus.Publish(&MediaEvent{Type: EventTypePacket, SessionID: "2"})

	// This should be dropped (queue full)
	bus.Publish(&MediaEvent{Type: EventTypePacket, SessionID: "3"})

	close(blockChan)
}

func TestEventBus_HandlerError(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 1)
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(EventTypeError, func(ctx context.Context, event *MediaEvent) error {
		wg.Done()
		return errors.New("handler error")
	})

	bus.Publish(&MediaEvent{
		Type:      EventTypeError,
		Timestamp: time.Now(),
		SessionID: "test",
	})

	wg.Wait()
}

func TestEventBus_HandlerPanic(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 1)
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
		defer wg.Done()
		panic("test panic")
	})

	bus.Publish(&MediaEvent{
		Type:      EventTypePacket,
		Timestamp: time.Now(),
		SessionID: "test",
	})

	wg.Wait()
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 2)
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
			wg.Done()
			return nil
		})
	}

	bus.Publish(&MediaEvent{
		Type:      EventTypePacket,
		Timestamp: time.Now(),
		SessionID: "test",
	})

	wg.Wait()
}

func TestEventBus_LifecycleEvents(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 1)
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(2) // Lifecycle handler + specific handler

	bus.Subscribe(EventTypeLifecycle, func(ctx context.Context, event *MediaEvent) error {
		wg.Done()
		return nil
	})

	bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
		wg.Done()
		return nil
	})

	bus.Publish(&MediaEvent{
		Type:      EventTypePacket,
		Timestamp: time.Now(),
		SessionID: "test",
	})

	wg.Wait()
}

func TestEventBus_Close(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 2)

	bus.Close()

	// Verify workers stopped
	select {
	case <-bus.ctx.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("context not cancelled after close")
	}
}

func TestEventBus_PublishHelpers(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 10, 1)
	defer bus.Close()

	var wg sync.WaitGroup

	// Test PublishPacket
	wg.Add(1)
	bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
		if _, ok := event.Payload.(MediaPacket); !ok {
			t.Error("payload is not MediaPacket")
		}
		wg.Done()
		return nil
	})

	bus.PublishPacket("session1", &AudioPacket{}, "sender1")
	wg.Wait()

	// Test PublishState
	wg.Add(1)
	bus.Subscribe(EventTypeState, func(ctx context.Context, event *MediaEvent) error {
		if _, ok := event.Payload.(StateChange); !ok {
			t.Error("payload is not StateChange")
		}
		wg.Done()
		return nil
	})

	bus.PublishState("session2", StateChange{State: Begin}, "sender2")
	wg.Wait()

	// Test PublishError
	wg.Add(1)
	bus.Subscribe(EventTypeError, func(ctx context.Context, event *MediaEvent) error {
		if _, ok := event.Payload.(error); !ok {
			t.Error("payload is not error")
		}
		wg.Done()
		return nil
	})

	bus.PublishError("session3", errors.New("test error"), "sender3")
	wg.Wait()
}

func TestEventBus_ConcurrentPublish(t *testing.T) {
	ctx := context.Background()
	bus := NewEventBus(ctx, 100, 4)
	defer bus.Close()

	var wg sync.WaitGroup
	eventCount := 100
	wg.Add(eventCount)

	bus.Subscribe(EventTypePacket, func(ctx context.Context, event *MediaEvent) error {
		wg.Done()
		return nil
	})

	// Publish events concurrently
	for i := 0; i < eventCount; i++ {
		go func(id int) {
			bus.Publish(&MediaEvent{
				Type:      EventTypePacket,
				Timestamp: time.Now(),
				SessionID: "test",
				Payload:   &AudioPacket{},
			})
		}(i)
	}

	wg.Wait()
}
