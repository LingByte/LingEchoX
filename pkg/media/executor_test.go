package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewAsyncTaskRunner(t *testing.T) {
	runner := NewAsyncTaskRunner[string](10)

	if runner.WorkerPoolSize != 10 {
		t.Errorf("expected WorkerPoolSize 10, got %d", runner.WorkerPoolSize)
	}

	if runner.MaxTaskTimeout != 1*time.Minute {
		t.Errorf("expected MaxTaskTimeout 1 minute, got %v", runner.MaxTaskTimeout)
	}

	if !runner.ConcurrentMode {
		t.Error("expected ConcurrentMode to be true")
	}
}

func TestAsyncTaskRunner_HandleState(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		return nil
	}

	initCalled := false
	terminateCalled := false
	stateCalled := false

	runner.InitCallback = func(h MediaHandler) error {
		initCalled = true
		return nil
	}

	runner.TerminateCallback = func(h MediaHandler) error {
		terminateCalled = true
		return nil
	}

	runner.StateCallback = func(h MediaHandler, event StateChange) error {
		stateCalled = true
		return nil
	}

	// Test Begin state
	runner.HandleState(session, StateChange{State: Begin})

	if !initCalled {
		t.Error("InitCallback not called")
	}

	if !stateCalled {
		t.Error("StateCallback not called")
	}

	if !runner.workersActive {
		t.Error("workers not started")
	}

	// Test End state
	runner.HandleState(session, StateChange{State: End})

	if !terminateCalled {
		t.Error("TerminateCallback not called")
	}

	if runner.workersActive {
		t.Error("workers not stopped")
	}

	runner.ReleaseResources()
}

func TestAsyncTaskRunner_HandlePacket(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.ConcurrentMode = false // Test synchronous mode

	executed := false
	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return &PacketRequest[string]{
			Req: "test-request",
		}, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		executed = true
		if req.Req != "test-request" {
			t.Errorf("expected 'test-request', got '%s'", req.Req)
		}
		return nil
	}

	packet := &AudioPacket{Payload: []byte("test")}
	runner.HandlePacket(session, packet)

	if !executed {
		t.Error("TaskExecutor not called")
	}
}

func TestAsyncTaskRunner_HandlePacketConcurrent(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.ConcurrentMode = true

	var wg sync.WaitGroup
	wg.Add(5)

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return &PacketRequest[string]{
			Req: "test",
		}, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		wg.Done()
		return nil
	}

	// Start workers
	runner.HandleState(session, StateChange{State: Begin})

	// Send packets
	for i := 0; i < 5; i++ {
		runner.HandlePacket(session, &AudioPacket{Payload: []byte("test")})
	}

	wg.Wait()
	runner.ReleaseResources()
}

func TestAsyncTaskRunner_RequestBuilderError(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)

	errorCaught := false
	session.Error(func(sender any, err error) {
		errorCaught = true
	})

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return nil, errors.New("builder error")
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		return nil
	}

	runner.HandlePacket(session, &AudioPacket{})

	if !errorCaught {
		t.Error("error not caught")
	}
}

func TestAsyncTaskRunner_RequestBuilderNil(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return nil, nil // Return nil request
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		t.Error("TaskExecutor should not be called")
		return nil
	}

	runner.HandlePacket(session, &AudioPacket{})
}

func TestAsyncTaskRunner_TaskExecutorError(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.ConcurrentMode = false

	errorCaught := false
	session.Error(func(sender any, err error) {
		errorCaught = true
	})

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return &PacketRequest[string]{Req: "test"}, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		return errors.New("executor error")
	}

	runner.HandlePacket(session, &AudioPacket{})

	if !errorCaught {
		t.Error("error not caught")
	}
}

func TestAsyncTaskRunner_TaskTimeout(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.ConcurrentMode = false
	runner.TaskTimeout = 100 * time.Millisecond

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return &PacketRequest[string]{Req: "test"}, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	}

	runner.HandlePacket(session, &AudioPacket{})
}

func TestAsyncTaskRunner_InterruptSignal(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](2)

	var wg sync.WaitGroup
	wg.Add(1)

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		return &PacketRequest[string]{Req: "test"}, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		wg.Done()
		return nil
	}

	// Start workers
	runner.HandleState(session, StateChange{State: Begin})

	// Send normal packet
	runner.HandlePacket(session, &AudioPacket{})
	wg.Wait()

	// Send interrupt
	if runner.taskQueue != nil {
		runner.taskQueue <- &PacketRequest[string]{Interrupt: true}
	}

	time.Sleep(100 * time.Millisecond)
	runner.ReleaseResources()
}

func TestAsyncTaskRunner_HandleMediaData(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.ConcurrentMode = false

	packetHandled := false
	stateHandled := false

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[string], error) {
		packetHandled = true
		return &PacketRequest[string]{Req: "test"}, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		return nil
	}

	runner.StateCallback = func(h MediaHandler, event StateChange) error {
		stateHandled = true
		return nil
	}

	// Test packet data
	runner.HandleMediaData(session, MediaData{
		Type:   MediaDataTypePacket,
		Packet: &AudioPacket{},
	})

	if !packetHandled {
		t.Error("packet not handled")
	}

	// Test state data
	runner.HandleMediaData(session, MediaData{
		Type:  MediaDataTypeState,
		State: StateChange{State: Begin},
	})

	if !stateHandled {
		t.Error("state not handled")
	}
}

func TestAsyncTaskRunner_MultipleWorkers(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[int](4)
	runner.ConcurrentMode = true

	var mu sync.Mutex
	processed := make(map[int]bool)
	var wg sync.WaitGroup
	wg.Add(10)

	runner.RequestBuilder = func(h MediaHandler, packet MediaPacket) (*PacketRequest[int], error) {
		if ap, ok := packet.(*AudioPacket); ok {
			return &PacketRequest[int]{Req: int(ap.Payload[0])}, nil
		}
		return nil, nil
	}

	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[int]) error {
		mu.Lock()
		processed[req.Req] = true
		mu.Unlock()
		wg.Done()
		time.Sleep(10 * time.Millisecond) // Simulate work
		return nil
	}

	// Start workers
	runner.HandleState(session, StateChange{State: Begin})

	// Send packets
	for i := 0; i < 10; i++ {
		runner.HandlePacket(session, &AudioPacket{Payload: []byte{byte(i)}})
	}

	wg.Wait()

	if len(processed) != 10 {
		t.Errorf("expected 10 processed packets, got %d", len(processed))
	}

	runner.ReleaseResources()
}

func TestAsyncTaskRunner_ReleaseResources(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		return nil
	}

	// Start workers
	runner.HandleState(session, StateChange{State: Begin})

	if !runner.workersActive {
		t.Error("workers not active")
	}

	runner.ReleaseResources()

	if runner.workersActive {
		t.Error("workers still active after release")
	}

	if runner.taskQueue != nil {
		t.Error("taskQueue not nil after release")
	}
}

func TestAsyncTaskRunner_CancelActiveTask(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	runner := NewAsyncTaskRunner[string](4)
	runner.TaskExecutor = func(ctx context.Context, h MediaHandler, req PacketRequest[string]) error {
		return nil
	}

	// Start workers
	runner.HandleState(session, StateChange{State: Begin})

	runner.CancelActiveTask()

	if runner.workersActive {
		t.Error("workers still active after cancel")
	}
}
