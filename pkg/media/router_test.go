package media

import (
	"context"
	"testing"
)

func TestNewRouter(t *testing.T) {
	router := NewRouter(StrategyBroadcast)

	if router == nil {
		t.Fatal("NewRouter returned nil")
	}

	if router.defaultStrategy != StrategyBroadcast {
		t.Errorf("expected default strategy %d, got %d", StrategyBroadcast, router.defaultStrategy)
	}

	if router.rules == nil {
		t.Error("rules not initialized")
	}
}

func TestRouter_AddRule(t *testing.T) {
	router := NewRouter(StrategyBroadcast)

	rule := RouteRule{
		Condition: func(packet MediaPacket) bool {
			return true
		},
		Targets:  []string{"output1"},
		Strategy: StrategyFirstAvailable,
	}

	router.AddRule(rule)

	if len(router.rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(router.rules))
	}
}

func TestRouter_Route_Broadcast(t *testing.T) {
	router := NewRouter(StrategyBroadcast)

	transports := []*TransportConnector{
		NewTransportConnector("t1", &mockTransport{}, DirectionOutput),
		NewTransportConnector("t2", &mockTransport{}, DirectionOutput),
		NewTransportConnector("t3", &mockTransport{}, DirectionOutput),
	}

	packet := &AudioPacket{}
	result := router.Route(packet, transports)

	if len(result) != 3 {
		t.Errorf("expected 3 transports, got %d", len(result))
	}
}

func TestRouter_Route_RoundRobin(t *testing.T) {
	router := NewRouter(StrategyRoundRobin)

	transports := []*TransportConnector{
		NewTransportConnector("t1", &mockTransport{}, DirectionOutput),
		NewTransportConnector("t2", &mockTransport{}, DirectionOutput),
		NewTransportConnector("t3", &mockTransport{}, DirectionOutput),
	}

	packet := &AudioPacket{}

	// First call
	result1 := router.Route(packet, transports)
	if len(result1) != 1 {
		t.Errorf("expected 1 transport, got %d", len(result1))
	}

	// Second call should return different transport
	result2 := router.Route(packet, transports)
	if len(result2) != 1 {
		t.Errorf("expected 1 transport, got %d", len(result2))
	}

	if result1[0].ID == result2[0].ID {
		t.Error("round robin should rotate transports")
	}
}

func TestRouter_Route_FirstAvailable(t *testing.T) {
	router := NewRouter(StrategyFirstAvailable)

	transports := []*TransportConnector{
		NewTransportConnector("t1", &mockTransport{}, DirectionOutput),
		NewTransportConnector("t2", &mockTransport{}, DirectionOutput),
	}

	packet := &AudioPacket{}
	result := router.Route(packet, transports)

	if len(result) != 1 {
		t.Errorf("expected 1 transport, got %d", len(result))
	}

	if result[0].ID != "t1" {
		t.Errorf("expected first transport 't1', got '%s'", result[0].ID)
	}
}

func TestRouter_Route_WithRule(t *testing.T) {
	router := NewRouter(StrategyBroadcast)

	// Add rule for audio packets
	router.AddRule(RouteRule{
		Condition: func(packet MediaPacket) bool {
			_, ok := packet.(*AudioPacket)
			return ok
		},
		Targets:  []string{"audio-output"},
		Strategy: StrategyFirstAvailable,
	})

	transports := []*TransportConnector{
		NewTransportConnector("audio-output", &mockTransport{}, DirectionOutput),
		NewTransportConnector("video-output", &mockTransport{}, DirectionOutput),
	}

	// Audio packet should match rule
	audioPacket := &AudioPacket{}
	result := router.Route(audioPacket, transports)

	if len(result) != 1 {
		t.Errorf("expected 1 transport for audio, got %d", len(result))
	}

	// Video packet should use default strategy
	videoPacket := &VideoPacket{}
	result = router.Route(videoPacket, transports)

	if len(result) != 2 {
		t.Errorf("expected 2 transports for video (broadcast), got %d", len(result))
	}
}

func TestRouter_Route_EmptyTransports(t *testing.T) {
	router := NewRouter(StrategyBroadcast)

	packet := &AudioPacket{}
	result := router.Route(packet, []*TransportConnector{})

	if result != nil {
		t.Error("expected nil for empty transports")
	}
}

func TestNewTransportConnector(t *testing.T) {
	transport := &mockTransport{}
	connector := NewTransportConnector("test", transport, DirectionOutput)

	if connector == nil {
		t.Fatal("NewTransportConnector returned nil")
	}

	if connector.ID != "test" {
		t.Errorf("expected ID 'test', got '%s'", connector.ID)
	}

	if connector.Direction != DirectionOutput {
		t.Errorf("expected direction '%s', got '%s'", DirectionOutput, connector.Direction)
	}

	if !connector.Active {
		t.Error("connector should be active by default")
	}
}

func TestTransportConnector_SetActive(t *testing.T) {
	connector := NewTransportConnector("test", &mockTransport{}, DirectionOutput)

	connector.SetActive(false)

	if connector.IsActive() {
		t.Error("connector should be inactive")
	}

	connector.SetActive(true)

	if !connector.IsActive() {
		t.Error("connector should be active")
	}
}

func TestTransportConnector_String(t *testing.T) {
	connector := NewTransportConnector("test", &mockTransport{}, DirectionOutput)

	str := connector.String()

	if str == "" {
		t.Error("String() should not be empty")
	}

	// Should contain ID, Direction, and Active status
	if !contains(str, "test") || !contains(str, DirectionOutput) {
		t.Error("String() should contain connector info")
	}
}

func TestRouter_ConcurrentAccess(t *testing.T) {
	router := NewRouter(StrategyRoundRobin)

	transports := []*TransportConnector{
		NewTransportConnector("t1", &mockTransport{}, DirectionOutput),
		NewTransportConnector("t2", &mockTransport{}, DirectionOutput),
	}

	done := make(chan bool)

	// Concurrent routing
	for i := 0; i < 10; i++ {
		go func() {
			packet := &AudioPacket{}
			router.Route(packet, transports)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent rule addition
	for i := 0; i < 10; i++ {
		go func() {
			router.AddRule(RouteRule{
				Strategy: StrategyBroadcast,
			})
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// Mock transport for testing
type mockTransport struct{}

func (m *mockTransport) Next(ctx context.Context) (MediaPacket, error) {
	return nil, nil
}

func (m *mockTransport) Send(ctx context.Context, packet MediaPacket) (int, error) {
	return 0, nil
}

func (m *mockTransport) Close() error {
	return nil
}

func (m *mockTransport) Attach(session *MediaSession) {}

func (m *mockTransport) Codec() CodecConfig {
	return CodecConfig{Codec: "pcm", SampleRate: 16000}
}

func (m *mockTransport) String() string {
	return "mockTransport"
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
