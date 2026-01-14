package media

import (
	"sync"
	"testing"
	"time"
)

func TestPipelineStage_String(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		index:   0,
		session: session,
	}

	str := stage.String()
	if str == "" {
		t.Error("String() should not be empty")
	}
}

func TestPipelineStage_GetContext(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
	}

	ctx := stage.GetContext()
	if ctx == nil {
		t.Error("GetContext() should not return nil")
	}
}

func TestPipelineStage_GetSession(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
	}

	s := stage.GetSession()
	if s != session {
		t.Error("GetSession() should return the session")
	}
}

func TestPipelineStage_InjectPacket(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
	}

	called := false
	filter := func(packet MediaPacket) (bool, error) {
		called = true
		return false, nil
	}

	stage.InjectPacket(filter)

	if stage.preFilter == nil {
		t.Error("preFilter not set")
	}

	// Test filter is called
	stage.preFilter(&AudioPacket{})
	if !called {
		t.Error("filter not called")
	}
}

func TestPipelineStage_CauseError(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	errorCaught := false
	session.Error(func(sender any, err error) {
		errorCaught = true
	})

	stage := &PipelineStage{
		session: session,
	}

	stage.CauseError(stage, nil)

	time.Sleep(50 * time.Millisecond)

	if !errorCaught {
		t.Error("error not caught")
	}
}

func TestPipelineStage_EmitState(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stateCaught := false
	session.On(Begin, func(event StateChange) {
		stateCaught = true
	})

	stage := &PipelineStage{
		session: session,
	}

	stage.EmitState(stage, Begin)

	time.Sleep(50 * time.Millisecond)

	if !stateCaught {
		t.Error("state not caught")
	}
}

func TestPipelineStage_SendToOutput(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	outputReceived := false
	mockOutput := &mockTransport{}
	session.Output(mockOutput)

	stage := &PipelineStage{
		session: session,
	}

	packet := &AudioPacket{Payload: []byte("test")}
	stage.SendToOutput(stage, packet)

	time.Sleep(50 * time.Millisecond)

	// Note: actual verification would require checking the output transport
	_ = outputReceived
}

func TestPipelineStage_AddMetric(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
	}

	// Should not panic
	stage.AddMetric("test", time.Second)
}

func TestPipelineStage_StartWorker(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
	}

	stage.startWorker(10)

	if !stage.workerRunning {
		t.Error("worker not started")
	}

	if stage.eventQueue == nil {
		t.Error("eventQueue not initialized")
	}

	if stage.ctx == nil {
		t.Error("context not initialized")
	}

	stage.stopWorker()
}

func TestPipelineStage_StopWorker(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
	}

	stage.startWorker(10)
	stage.stopWorker()

	if stage.workerRunning {
		t.Error("worker still running")
	}
}

func TestPipelineStage_EmitPacket_Async(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	stage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			wg.Done()
		},
	}

	stage.startWorker(10)
	defer stage.stopWorker()

	packet := &AudioPacket{Payload: []byte("test")}
	stage.EmitPacket(stage, packet)

	wg.Wait()
}

func TestPipelineStage_EmitPacket_Sync(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	called := false
	stage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			called = true
		},
	}

	// Don't start worker (synchronous mode)
	packet := &AudioPacket{Payload: []byte("test")}
	stage.EmitPacket(stage, packet)

	if !called {
		t.Error("middleware not called in sync mode")
	}
}

func TestPipelineStage_ProcessPacket_WithPreFilter(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	filterCalled := false
	middlewareCalled := false

	stage := &PipelineStage{
		session: session,
		preFilter: func(packet MediaPacket) (bool, error) {
			filterCalled = true
			return false, nil // Don't skip
		},
		middleware: func(h MediaHandler, data MediaData) {
			middlewareCalled = true
		},
	}

	packet := &AudioPacket{Payload: []byte("test")}
	stage.processPacketAsync(stage, packet)

	if !filterCalled {
		t.Error("preFilter not called")
	}

	if !middlewareCalled {
		t.Error("middleware not called")
	}
}

func TestPipelineStage_ProcessPacket_FilterSkip(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	middlewareCalled := false

	stage := &PipelineStage{
		session: session,
		preFilter: func(packet MediaPacket) (bool, error) {
			return true, nil // Skip packet
		},
		middleware: func(h MediaHandler, data MediaData) {
			middlewareCalled = true
		},
	}

	packet := &AudioPacket{Payload: []byte("test")}
	stage.processPacketAsync(stage, packet)

	if middlewareCalled {
		t.Error("middleware should not be called when filter skips")
	}
}

func TestPipelineStage_ProcessPacket_Terminal(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	mockOutput := &mockTransport{}
	session.Output(mockOutput)

	// Terminal stage (no middleware)
	stage := &PipelineStage{
		session:    session,
		middleware: nil,
	}

	packet := &AudioPacket{Payload: []byte("test")}
	stage.processPacketAsync(stage, packet)

	// Should send to output
	time.Sleep(50 * time.Millisecond)
}

func TestPipelineStage_ProcessPacket_WithNextStage(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	nextStage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			wg.Done()
		},
	}
	nextStage.startWorker(10)
	defer nextStage.stopWorker()

	stage := &PipelineStage{
		session:   session,
		nextStage: nextStage,
		middleware: func(h MediaHandler, data MediaData) {
			wg.Done()
		},
	}

	packet := &AudioPacket{Payload: []byte("test")}
	stage.processPacketAsync(stage, packet)

	wg.Wait()
}

func TestPipelineStage_ProcessPacket_WithPostFilter(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	postFilterCalled := false

	stage := &PipelineStage{
		session:    session,
		middleware: func(h MediaHandler, data MediaData) {},
		postFilter: func(packet MediaPacket) (bool, error) {
			postFilterCalled = true
			return false, nil
		},
	}

	packet := &AudioPacket{Payload: []byte("test")}
	stage.processPacketAsync(stage, packet)

	if !postFilterCalled {
		t.Error("postFilter not called")
	}
}

func TestPipelineStage_EventLoop(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	var wg sync.WaitGroup
	wg.Add(3)

	stage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			wg.Done()
		},
	}

	stage.startWorker(10)
	defer stage.stopWorker()

	// Send multiple events
	for i := 0; i < 3; i++ {
		stage.enqueueEvent(&MediaData{
			Type:   MediaDataTypePacket,
			Packet: &AudioPacket{},
		})
	}

	wg.Wait()
}

func TestPipelineStage_EnqueueEvent_QueueFull(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	stage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			time.Sleep(100 * time.Millisecond) // Block processing
		},
	}

	stage.startWorker(1) // Small queue
	defer stage.stopWorker()

	// Fill queue
	stage.enqueueEvent(&MediaData{Type: MediaDataTypePacket, Packet: &AudioPacket{}})
	stage.enqueueEvent(&MediaData{Type: MediaDataTypePacket, Packet: &AudioPacket{}})

	// This should be dropped (queue full)
	stage.enqueueEvent(&MediaData{Type: MediaDataTypePacket, Packet: &AudioPacket{}})
}

func TestPipelineStage_ProcessEvent_StateData(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	stage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			if data.Type == MediaDataTypeState {
				wg.Done()
			}
		},
	}

	stage.processEvent(&MediaData{
		Type:  MediaDataTypeState,
		State: StateChange{State: Begin},
	})

	wg.Wait()
}

func TestPipelineStage_ConcurrentAccess(t *testing.T) {
	session := NewDefaultSession()
	defer session.Close()

	var wg sync.WaitGroup
	wg.Add(10)

	stage := &PipelineStage{
		session: session,
		middleware: func(h MediaHandler, data MediaData) {
			wg.Done()
		},
	}

	stage.startWorker(20)
	defer stage.stopWorker()

	// Concurrent packet emission
	for i := 0; i < 10; i++ {
		go func() {
			packet := &AudioPacket{Payload: []byte("test")}
			stage.EmitPacket(stage, packet)
		}()
	}

	wg.Wait()
}
