package media

import (
	"bytes"
	"testing"
)

func TestDefaultResampler(t *testing.T) {
	converter := DefaultResampler(8000, 16000)

	if converter == nil {
		t.Fatal("DefaultResampler returned nil")
	}
}

func TestSetDefaultResampler(t *testing.T) {
	customCalled := false
	customFactory := func(inputRate, outputRate int) SampleRateConverter {
		customCalled = true
		return NewInterpolatingConverter(inputRate, outputRate)
	}

	SetDefaultResampler(customFactory)
	DefaultResampler(8000, 16000)

	if !customCalled {
		t.Error("custom factory not called")
	}

	// Reset to default
	SetDefaultResampler(NewInterpolatingConverter)
}

func TestResamplePCM_SameRate(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03}

	result, err := ResamplePCM(data, 16000, 16000)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !bytes.Equal(result, data) {
		t.Error("data should be unchanged for same rate")
	}
}

func TestResamplePCM_Upsample(t *testing.T) {
	// Create simple test data (2 samples)
	data := []byte{0x00, 0x00, 0xFF, 0x7F} // 0, 32767

	result, err := ResamplePCM(data, 8000, 16000)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should have approximately 2x samples
	expectedLen := len(data) * 2
	if len(result) < expectedLen-4 || len(result) > expectedLen+4 {
		t.Errorf("expected length around %d, got %d", expectedLen, len(result))
	}
}

func TestResamplePCM_Downsample(t *testing.T) {
	// Create test data (4 samples)
	data := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x00, 0xFF, 0x7F}

	result, err := ResamplePCM(data, 16000, 8000)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should have approximately 0.5x samples
	expectedLen := len(data) / 2
	if len(result) < expectedLen-4 || len(result) > expectedLen+4 {
		t.Errorf("expected length around %d, got %d", expectedLen, len(result))
	}
}

func TestNewInterpolatingConverter(t *testing.T) {
	converter := NewInterpolatingConverter(8000, 16000)

	ic, ok := converter.(*InterpolatingConverter)
	if !ok {
		t.Fatal("converter is not InterpolatingConverter")
	}

	if ic.sourceRate != 8000 {
		t.Errorf("expected sourceRate 8000, got %d", ic.sourceRate)
	}

	if ic.targetRate != 16000 {
		t.Errorf("expected targetRate 16000, got %d", ic.targetRate)
	}

	if ic.useCubic {
		t.Error("expected linear interpolation by default")
	}
}

func TestNewCubicInterpolatingConverter(t *testing.T) {
	converter := NewCubicInterpolatingConverter(8000, 16000)

	ic, ok := converter.(*InterpolatingConverter)
	if !ok {
		t.Fatal("converter is not InterpolatingConverter")
	}

	if !ic.useCubic {
		t.Error("expected cubic interpolation")
	}
}

func TestInterpolatingConverter_Write(t *testing.T) {
	converter := NewInterpolatingConverter(8000, 16000)

	data := []byte{0x00, 0x00, 0xFF, 0x7F}

	n, err := converter.Write(data)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}
}

func TestInterpolatingConverter_Close(t *testing.T) {
	converter := NewInterpolatingConverter(8000, 16000)

	err := converter.Close()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInterpolatingConverter_Samples(t *testing.T) {
	converter := NewInterpolatingConverter(8000, 16000)

	data := []byte{0x00, 0x00, 0xFF, 0x7F}
	converter.Write(data)

	samples := converter.Samples()

	if samples == nil {
		t.Error("samples should not be nil")
	}

	if len(samples) == 0 {
		t.Error("samples should not be empty")
	}

	// Second call should return nil (buffer cleared)
	samples2 := converter.Samples()
	if samples2 != nil {
		t.Error("second call should return nil")
	}
}

func TestInterpolatingConverter_ConvertSamples_SameRate(t *testing.T) {
	converter := &InterpolatingConverter{
		sourceRate: 16000,
		targetRate: 16000,
	}

	data := []byte{0x00, 0x00, 0xFF, 0x7F}
	result := converter.ConvertSamples(data)

	if !bytes.Equal(result, data) {
		t.Error("data should be unchanged for same rate")
	}
}

func TestInterpolatingConverter_ConvertSamples_OddLength(t *testing.T) {
	converter := &InterpolatingConverter{
		sourceRate: 8000,
		targetRate: 16000,
	}

	// Odd length data (invalid for 16-bit samples)
	data := []byte{0x00, 0x00, 0xFF}
	result := converter.ConvertSamples(data)

	if result != nil {
		t.Error("should return nil for odd length data")
	}
}

func TestInterpolatingConverter_LinearInterpolation(t *testing.T) {
	converter := &InterpolatingConverter{
		sourceRate: 8000,
		targetRate: 16000,
		useCubic:   false,
	}

	// Simple test: two samples
	data := []byte{0x00, 0x00, 0x00, 0x10} // 0, 4096
	result := converter.ConvertSamples(data)

	if result == nil {
		t.Fatal("result should not be nil")
	}

	if len(result) < 2 {
		t.Error("result too short")
	}
}

func TestInterpolatingConverter_CubicInterpolation(t *testing.T) {
	converter := &InterpolatingConverter{
		sourceRate: 8000,
		targetRate: 16000,
		useCubic:   true,
	}

	// Need at least 4 samples for cubic interpolation
	data := []byte{
		0x00, 0x00, // 0
		0x00, 0x10, // 4096
		0x00, 0x20, // 8192
		0x00, 0x30, // 12288
	}
	result := converter.ConvertSamples(data)

	if result == nil {
		t.Fatal("result should not be nil")
	}

	if len(result) < 4 {
		t.Error("result too short")
	}
}

func TestInterpolatingConverter_EdgeCases(t *testing.T) {
	converter := &InterpolatingConverter{
		sourceRate: 8000,
		targetRate: 16000,
	}

	// Empty data
	result := converter.ConvertSamples([]byte{})
	if result == nil || len(result) != 0 {
		t.Error("empty data should return empty result")
	}

	// Single sample
	result = converter.ConvertSamples([]byte{0x00, 0x00})
	if result == nil {
		t.Error("single sample should be handled")
	}
}

func TestResamplePCM_MultipleWrites(t *testing.T) {
	converter := NewInterpolatingConverter(8000, 16000)

	data1 := []byte{0x00, 0x00, 0xFF, 0x7F}
	data2 := []byte{0x00, 0x80, 0xFF, 0xFF}

	converter.Write(data1)
	converter.Write(data2)
	converter.Close()

	samples := converter.Samples()

	if samples == nil {
		t.Error("samples should not be nil")
	}

	// Should contain data from both writes
	if len(samples) == 0 {
		t.Error("samples should not be empty")
	}
}

func TestResamplePCM_CommonRates(t *testing.T) {
	tests := []struct {
		name       string
		inputRate  int
		outputRate int
	}{
		{"8k to 16k", 8000, 16000},
		{"16k to 8k", 16000, 8000},
		{"8k to 48k", 8000, 48000},
		{"48k to 16k", 48000, 16000},
		{"44.1k to 48k", 44100, 48000},
	}

	data := make([]byte, 160) // 80 samples
	for i := 0; i < len(data); i += 2 {
		data[i] = byte(i)
		data[i+1] = byte(i >> 8)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResamplePCM(data, tt.inputRate, tt.outputRate)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result == nil {
				t.Error("result should not be nil")
			}

			// Check result length is reasonable
			ratio := float64(tt.outputRate) / float64(tt.inputRate)
			expectedLen := int(float64(len(data)) * ratio)

			if len(result) < expectedLen/2 || len(result) > expectedLen*2 {
				t.Errorf("unexpected result length: got %d, expected around %d", len(result), expectedLen)
			}
		})
	}
}
