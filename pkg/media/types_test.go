package media

import (
	"testing"
	"time"
)

func TestTextPacket(t *testing.T) {
	t.Run("Body", func(t *testing.T) {
		packet := &TextPacket{Text: "hello world"}
		if string(packet.Body()) != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", string(packet.Body()))
		}
	})

	t.Run("String_User", func(t *testing.T) {
		packet := &TextPacket{
			Text:     "test",
			Sequence: 1,
		}
		str := packet.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})

	t.Run("String_Transcribed", func(t *testing.T) {
		packet := &TextPacket{
			Text:          "test",
			IsTranscribed: true,
		}
		str := packet.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})

	t.Run("String_LLMGenerated", func(t *testing.T) {
		packet := &TextPacket{
			Text:           "test",
			IsLLMGenerated: true,
		}
		str := packet.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestAudioPacket(t *testing.T) {
	t.Run("Body", func(t *testing.T) {
		payload := []byte{1, 2, 3, 4}
		packet := &AudioPacket{Payload: payload}
		if len(packet.Body()) != 4 {
			t.Errorf("expected length 4, got %d", len(packet.Body()))
		}
	})

	t.Run("String", func(t *testing.T) {
		packet := &AudioPacket{
			Payload:       []byte{1, 2, 3},
			IsFirstPacket: true,
			IsSynthesized: true,
		}
		str := packet.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestVideoPacket(t *testing.T) {
	t.Run("Body", func(t *testing.T) {
		payload := []byte{1, 2, 3, 4, 5}
		packet := &VideoPacket{Payload: payload}
		if len(packet.Body()) != 5 {
			t.Errorf("expected length 5, got %d", len(packet.Body()))
		}
	})

	t.Run("String", func(t *testing.T) {
		packet := &VideoPacket{
			Payload:       []byte{1, 2, 3},
			IsFirstPacket: true,
			IsKeyFrame:    true,
			Width:         1920,
			Height:        1080,
		}
		str := packet.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestClosePacket(t *testing.T) {
	t.Run("Body", func(t *testing.T) {
		packet := &ClosePacket{Reason: "test"}
		if packet.Body() != nil {
			t.Error("expected nil body")
		}
	})

	t.Run("String", func(t *testing.T) {
		packet := &ClosePacket{Reason: "connection closed"}
		str := packet.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestStateChange(t *testing.T) {
	t.Run("SafeGetStr_Valid", func(t *testing.T) {
		sc := &StateChange{
			State:  "test",
			Params: []any{"param1", "param2"},
		}
		if sc.SafeGetStr(0) != "param1" {
			t.Errorf("expected 'param1', got '%s'", sc.SafeGetStr(0))
		}
	})

	t.Run("SafeGetStr_OutOfBounds", func(t *testing.T) {
		sc := &StateChange{
			State:  "test",
			Params: []any{"param1"},
		}
		if sc.SafeGetStr(5) != "" {
			t.Error("expected empty string for out of bounds")
		}
	})

	t.Run("SafeGetStr_NegativeIndex", func(t *testing.T) {
		sc := &StateChange{
			State:  "test",
			Params: []any{"param1"},
		}
		if sc.SafeGetStr(-1) != "" {
			t.Error("expected empty string for negative index")
		}
	})

	t.Run("SafeGetStr_NonString", func(t *testing.T) {
		sc := &StateChange{
			State:  "test",
			Params: []any{123},
		}
		if sc.SafeGetStr(0) != "" {
			t.Error("expected empty string for non-string param")
		}
	})
}

func TestMediaData(t *testing.T) {
	t.Run("String_State", func(t *testing.T) {
		data := &MediaData{
			Type:   MediaDataTypeState,
			Sender: "test",
			State:  StateChange{State: "begin"},
		}
		str := data.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})

	t.Run("String_Packet", func(t *testing.T) {
		data := &MediaData{
			Type:   MediaDataTypePacket,
			Sender: "test",
			Packet: &TextPacket{Text: "hello"},
		}
		str := data.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})

	t.Run("String_Other", func(t *testing.T) {
		data := &MediaData{
			Type:   "unknown",
			Sender: "test",
		}
		str := data.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestCompletedData(t *testing.T) {
	t.Run("MarshalJSON", func(t *testing.T) {
		data := &CompletedData{
			SenderName: "test",
			Duration:   time.Second,
			Result:     "success",
			DialogID:   "dialog123",
		}
		json, err := data.MarshalJSON()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(json) == 0 {
			t.Error("expected non-empty JSON")
		}
	})

	t.Run("String", func(t *testing.T) {
		data := &CompletedData{
			SenderName: "test",
			Duration:   time.Second,
			Result:     "success",
			DialogID:   "dialog123",
		}
		str := data.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestTranscribingData(t *testing.T) {
	t.Run("MarshalJSON", func(t *testing.T) {
		data := &TranscribingData{
			SenderName: "test",
			Duration:   time.Second,
			Result:     "transcribed text",
			DialogID:   "dialog123",
		}
		json, err := data.MarshalJSON()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(json) == 0 {
			t.Error("expected non-empty JSON")
		}
	})

	t.Run("String", func(t *testing.T) {
		data := &TranscribingData{
			SenderName: "test",
			Duration:   time.Second,
			Result:     "transcribed text",
			DialogID:   "dialog123",
		}
		str := data.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestCodecConfig(t *testing.T) {
	t.Run("DefaultCodecConfig", func(t *testing.T) {
		cfg := DefaultCodecConfig()
		if cfg.Codec != "pcm" {
			t.Errorf("expected codec 'pcm', got '%s'", cfg.Codec)
		}
		if cfg.SampleRate != 16000 {
			t.Errorf("expected sample rate 16000, got %d", cfg.SampleRate)
		}
		if cfg.Channels != 1 {
			t.Errorf("expected channels 1, got %d", cfg.Channels)
		}
		if cfg.BitDepth != 16 {
			t.Errorf("expected bit depth 16, got %d", cfg.BitDepth)
		}
	})

	t.Run("String", func(t *testing.T) {
		cfg := CodecConfig{
			Codec:         "opus",
			SampleRate:    48000,
			Channels:      2,
			BitDepth:      16,
			FrameDuration: "20ms",
		}
		str := cfg.String()
		if str == "" {
			t.Error("expected non-empty string")
		}
	})
}

func TestConstants(t *testing.T) {
	t.Run("States", func(t *testing.T) {
		states := []string{
			AllStates, Begin, End, Hangup, StartSpeaking,
			StartSilence, Transcribing, Synthesizing,
			StartPlay, StopPlay, Completed, Interruption,
		}
		for _, state := range states {
			if state == "" {
				t.Error("state constant should not be empty")
			}
		}
	})

	t.Run("MediaDataTypes", func(t *testing.T) {
		types := []string{
			MediaDataTypeState, MediaDataTypePacket, MediaDataTypeMetric,
		}
		for _, typ := range types {
			if typ == "" {
				t.Error("media data type constant should not be empty")
			}
		}
	})

	t.Run("Directions", func(t *testing.T) {
		if DirectionInput == "" {
			t.Error("DirectionInput should not be empty")
		}
		if DirectionOutput == "" {
			t.Error("DirectionOutput should not be empty")
		}
	})

	t.Run("Errors", func(t *testing.T) {
		if ErrNotInputTransport == nil {
			t.Error("ErrNotInputTransport should not be nil")
		}
		if ErrNotOutputTransport == nil {
			t.Error("ErrNotOutputTransport should not be nil")
		}
		if ErrCodecNotSupported == nil {
			t.Error("ErrCodecNotSupported should not be nil")
		}
	})
}
