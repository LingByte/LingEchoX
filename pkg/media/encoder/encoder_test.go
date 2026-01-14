package encoder

import (
	"bytes"
	"github.com/LingByte/LingEchoX/pkg/media"
	"testing"
)

// Test G.711 A-law encoding/decoding
func TestG711ALaw(t *testing.T) {
	// Test linear to A-law conversion
	tests := []struct {
		name     string
		pcmValue int
	}{
		{"Zero", 0},
		{"Positive small", 100},
		{"Positive medium", 1000},
		{"Positive large", 10000},
		{"Negative small", -100},
		{"Negative medium", -1000},
		{"Negative large", -10000},
		{"Max positive", 32767},
		{"Max negative", -32768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alaw := linear2alaw(tt.pcmValue)
			decoded := alaw2linear(alaw)

			// Check if decoded value is close to original (within tolerance)
			diff := int(decoded) - tt.pcmValue
			if diff < 0 {
				diff = -diff
			}

			// A-law has quantization error, allow some tolerance
			// Extreme values have larger quantization errors
			tolerance := 100
			if tt.pcmValue > 30000 || tt.pcmValue < -30000 {
				tolerance = 600
			}
			if diff > tolerance && tt.pcmValue != 0 {
				t.Errorf("A-law round-trip error too large: input=%d, decoded=%d, diff=%d",
					tt.pcmValue, decoded, diff)
			}
		})
	}
}

func TestG711ULaw(t *testing.T) {
	// Test linear to μ-law conversion
	tests := []struct {
		name     string
		pcmValue int
	}{
		{"Zero", 0},
		{"Positive small", 100},
		{"Positive medium", 1000},
		{"Positive large", 10000},
		{"Negative small", -100},
		{"Negative medium", -1000},
		{"Negative large", -10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ulaw := linear2ulaw(tt.pcmValue)
			decoded := ulaw2linear(ulaw)

			// Check if decoded value is close to original
			diff := decoded - tt.pcmValue
			if diff < 0 {
				diff = -diff
			}

			// μ-law has quantization error, allow some tolerance
			// Larger values have larger quantization errors
			tolerance := 100
			if tt.pcmValue > 5000 || tt.pcmValue < -5000 {
				tolerance = 200
			}
			if diff > tolerance && tt.pcmValue != 0 {
				t.Errorf("μ-law round-trip error too large: input=%d, decoded=%d, diff=%d",
					tt.pcmValue, decoded, diff)
			}
		})
	}
}

func TestPcma2pcm(t *testing.T) {
	// Create test A-law data
	alawData := []byte{0x00, 0x55, 0xAA, 0xFF}

	pcmData, err := pcma2pcm(alawData)

	if err != nil {
		t.Errorf("pcma2pcm failed: %v", err)
	}

	// PCM data should be 2x the size (16-bit samples)
	expectedLen := len(alawData) * 2
	if len(pcmData) != expectedLen {
		t.Errorf("expected PCM length %d, got %d", expectedLen, len(pcmData))
	}
}

func TestPcm2pcma(t *testing.T) {
	// Create test PCM data (16-bit samples)
	pcmData := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80, 0xFF, 0xFF}

	alawData, err := Pcm2pcma(pcmData)

	if err != nil {
		t.Errorf("Pcm2pcma failed: %v", err)
	}

	// A-law data should be half the size
	expectedLen := len(pcmData) / 2
	if len(alawData) != expectedLen {
		t.Errorf("expected A-law length %d, got %d", expectedLen, len(alawData))
	}
}

func TestEncodePCMA(t *testing.T) {
	pcmData := []byte{0x00, 0x00, 0xFF, 0x7F}

	encoded, err := EncodePCMA(pcmData)

	if err != nil {
		t.Errorf("EncodePCMA failed: %v", err)
	}

	if len(encoded) != len(pcmData)/2 {
		t.Errorf("unexpected encoded length: got %d, want %d", len(encoded), len(pcmData)/2)
	}
}

func TestPcmu2pcm(t *testing.T) {
	// Create test μ-law data
	ulawData := []byte{0x00, 0x55, 0xAA, 0xFF}

	pcmData, err := pcmu2pcm(ulawData)

	if err != nil {
		t.Errorf("pcmu2pcm failed: %v", err)
	}

	// PCM data should be 2x the size
	expectedLen := len(ulawData) * 2
	if len(pcmData) != expectedLen {
		t.Errorf("expected PCM length %d, got %d", expectedLen, len(pcmData))
	}
}

func TestPcm2pcmu(t *testing.T) {
	// Create test PCM data
	pcmData := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80, 0xFF, 0xFF}

	ulawData, err := pcm2pcmu(pcmData)

	if err != nil {
		t.Errorf("pcm2pcmu failed: %v", err)
	}

	// μ-law data should be half the size
	expectedLen := len(pcmData) / 2
	if len(ulawData) != expectedLen {
		t.Errorf("expected μ-law length %d, got %d", expectedLen, len(ulawData))
	}
}

func TestFindSegmentIndex(t *testing.T) {
	boundaries := []int{10, 20, 30, 40, 50}

	tests := []struct {
		value    int
		expected int
	}{
		{5, 0},
		{10, 0},
		{15, 1},
		{25, 2},
		{35, 3},
		{45, 4},
		{55, 5},
	}

	for _, tt := range tests {
		result := findSegmentIndex(tt.value, boundaries, len(boundaries))
		if result != tt.expected {
			t.Errorf("findSegmentIndex(%d) = %d, want %d", tt.value, result, tt.expected)
		}
	}
}

// Test G.722 Encoder
func TestG722Encoder(t *testing.T) {
	encoder := NewG722Encoder(G722_RATE_DEFAULT, G722_DEFAULT)

	if encoder == nil {
		t.Fatal("NewG722Encoder returned nil")
	}

	// Create test PCM data (16-bit samples, must be even length)
	pcmData := make([]byte, 160) // 80 samples
	for i := 0; i < len(pcmData); i += 2 {
		pcmData[i] = byte(i)
		pcmData[i+1] = byte(i >> 8)
	}

	encoded := encoder.Encode(pcmData)

	if encoded == nil {
		t.Error("Encode returned nil")
	}

	// G.722 compresses 2:1, so output should be half the input
	expectedLen := len(pcmData) / 4 // 2 samples per byte
	if len(encoded) != expectedLen {
		t.Errorf("expected encoded length %d, got %d", expectedLen, len(encoded))
	}
}

func TestG722Encoder_EmptyData(t *testing.T) {
	encoder := NewG722Encoder(G722_RATE_DEFAULT, G722_DEFAULT)

	encoded := encoder.Encode([]byte{})

	if encoded != nil {
		t.Error("expected nil for empty data")
	}
}

func TestG722Encoder_OddLength(t *testing.T) {
	encoder := NewG722Encoder(G722_RATE_DEFAULT, G722_DEFAULT)

	// Odd length data - encoder should handle gracefully
	// G.722 requires even-length input (pairs of samples)
	pcmData := []byte{0x00, 0x00, 0xFF, 0x7F, 0xAA} // 5 bytes (odd)
	encoded := encoder.Encode(pcmData)

	// Should encode the even portion (first 4 bytes)
	if encoded == nil {
		t.Error("Encode should handle odd length data gracefully")
	}

	// Should encode 4 bytes (2 samples) into 1 byte
	expectedLen := 1
	if len(encoded) != expectedLen {
		t.Errorf("expected encoded length %d, got %d", expectedLen, len(encoded))
	}
}

func TestG722Decoder(t *testing.T) {
	decoder := NewG722Decoder(G722_RATE_DEFAULT, G722_DEFAULT)

	if decoder == nil {
		t.Fatal("NewG722Decoder returned nil")
	}

	// Create test G.722 data
	g722Data := []byte{0x00, 0x55, 0xAA, 0xFF}

	decoded := decoder.Decode(g722Data)

	if decoded == nil {
		t.Error("Decode returned nil")
	}

	// G.722 expands 1:4 (each byte becomes 2 16-bit samples)
	expectedLen := len(g722Data) * 4
	if len(decoded) != expectedLen {
		t.Errorf("expected decoded length %d, got %d", expectedLen, len(decoded))
	}
}

func TestG722Decoder_EmptyData(t *testing.T) {
	decoder := NewG722Decoder(G722_RATE_DEFAULT, G722_DEFAULT)

	decoded := decoder.Decode([]byte{})

	if decoded != nil {
		t.Error("expected nil for empty data")
	}
}

func TestG722RoundTrip(t *testing.T) {
	encoder := NewG722Encoder(G722_RATE_DEFAULT, G722_DEFAULT)
	decoder := NewG722Decoder(G722_RATE_DEFAULT, G722_DEFAULT)

	// Create test PCM data
	pcmData := make([]byte, 160)
	for i := 0; i < len(pcmData); i += 2 {
		pcmData[i] = byte(i * 100)
		pcmData[i+1] = byte((i * 100) >> 8)
	}

	// Encode
	encoded := encoder.Encode(pcmData)

	// Decode
	decoded := decoder.Decode(encoded)

	// Decoded should be same length as original
	if len(decoded) != len(pcmData) {
		t.Errorf("round-trip length mismatch: original=%d, decoded=%d",
			len(pcmData), len(decoded))
	}
}

func TestG722Quantize(t *testing.T) {
	encoder := NewG722Encoder(G722_RATE_DEFAULT, G722_DEFAULT)

	tests := []struct {
		sample   int16
		expected int
	}{
		{0, 0},
		{10, 0},
		{20, 1},
		{50, 2},
		{100, 3},
		{200, 4},
		{400, 5},
		{800, 6},
		{1500, 7},
		{3000, 8},
		{6000, 9},
		{12000, 10},
		{20000, 11},
	}

	for _, tt := range tests {
		result := encoder.quantize(tt.sample)
		if result != tt.expected {
			t.Errorf("quantize(%d) = %d, want %d", tt.sample, result, tt.expected)
		}
	}
}

// Test PCM to PCM (resampling)
func TestPcmToPcm(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	encoderFunc := PcmToPcm(srcConfig, pcmConfig)

	if encoderFunc == nil {
		t.Fatal("PcmToPcm returned nil")
	}

	// Create test audio packet
	pcmData := make([]byte, 160)
	packet := &media.AudioPacket{
		Payload: pcmData,
	}

	result, err := encoderFunc(packet)

	if err != nil {
		t.Errorf("PcmToPcm failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

// Test codec registry
func TestRegisterCodec(t *testing.T) {
	// Test registering a custom codec
	customEncoder := func(src, pcm media.CodecConfig) media.EncoderFunc {
		return func(packet media.MediaPacket) ([]media.MediaPacket, error) {
			return []media.MediaPacket{packet}, nil
		}
	}

	customDecoder := func(src, pcm media.CodecConfig) media.EncoderFunc {
		return func(packet media.MediaPacket) ([]media.MediaPacket, error) {
			return []media.MediaPacket{packet}, nil
		}
	}

	RegisterCodec("test-codec", customEncoder, customDecoder)

	if !HasCodec("test-codec") {
		t.Error("codec not registered")
	}

	// Test case insensitivity
	if !HasCodec("TEST-CODEC") {
		t.Error("codec lookup should be case-insensitive")
	}
}

func TestCreateEncode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcmu",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	encoder, err := CreateEncode(srcConfig, pcmConfig)

	if err != nil {
		t.Errorf("CreateEncode failed: %v", err)
	}

	if encoder == nil {
		t.Error("encoder is nil")
	}
}

func TestCreateDecode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcmu",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	decoder, err := CreateDecode(srcConfig, pcmConfig)

	if err != nil {
		t.Errorf("CreateDecode failed: %v", err)
	}

	if decoder == nil {
		t.Error("decoder is nil")
	}
}

func TestCreateEncode_UnsupportedCodec(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "unsupported-codec",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	_, err := CreateEncode(srcConfig, pcmConfig)

	if err != media.ErrCodecNotSupported {
		t.Errorf("expected ErrCodecNotSupported, got %v", err)
	}
}

func TestHasCodec(t *testing.T) {
	tests := []struct {
		codec    string
		expected bool
	}{
		{"pcm", true},
		{"pcmu", true},
		{"pcma", true},
		{"g722", true},
		{"unknown", false},
		{"PCM", true},  // Case insensitive
		{"PCMU", true}, // Case insensitive
	}

	for _, tt := range tests {
		result := HasCodec(tt.codec)
		if result != tt.expected {
			t.Errorf("HasCodec(%s) = %v, want %v", tt.codec, result, tt.expected)
		}
	}
}

func TestStripWavHeader(t *testing.T) {
	// Test with WAV header
	wavData := []byte("RIFF")
	wavData = append(wavData, make([]byte, 40)...) // Complete 44-byte header
	wavData = append(wavData, []byte("actual audio data")...)

	stripped := StripWavHeader(wavData)

	if bytes.Equal(stripped, wavData) {
		t.Error("WAV header not stripped")
	}

	if !bytes.Equal(stripped, []byte("actual audio data")) {
		t.Error("incorrect data after stripping header")
	}

	// Test without WAV header
	rawData := []byte("raw audio data")
	stripped = StripWavHeader(rawData)

	if !bytes.Equal(stripped, rawData) {
		t.Error("raw data should not be modified")
	}

	// Test with short data
	shortData := []byte("RIFF")
	stripped = StripWavHeader(shortData)

	if !bytes.Equal(stripped, shortData) {
		t.Error("short data should not be modified")
	}
}

func TestSplitFrames(t *testing.T) {
	data := make([]byte, 1600) // 1600 bytes

	// Test with frame duration
	srcConfig := media.CodecConfig{
		SampleRate:    8000,
		FrameDuration: "20ms",
	}

	packets := splitFrames(data, &srcConfig)

	if len(packets) == 0 {
		t.Error("expected non-empty packets")
	}

	// Test without frame duration
	srcConfig.FrameDuration = ""
	packets = splitFrames(data, &srcConfig)

	if len(packets) != 1 {
		t.Errorf("expected 1 packet without frame duration, got %d", len(packets))
	}

	// Test with invalid frame duration
	srcConfig.FrameDuration = "5ms" // Too short
	packets = splitFrames(data, &srcConfig)

	if len(packets) == 0 {
		t.Error("should handle invalid frame duration")
	}
}

func TestCreatePCMADecode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcma",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	decoder := createPCMADecode(srcConfig, pcmConfig)

	if decoder == nil {
		t.Fatal("createPCMADecode returned nil")
	}

	// Test with audio packet
	alawData := []byte{0x00, 0x55, 0xAA, 0xFF}
	packet := &media.AudioPacket{
		Payload: alawData,
	}

	result, err := decoder(packet)

	if err != nil {
		t.Errorf("decoder failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestCreatePCMAEncode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcma",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	encoder := createPCMAEncode(srcConfig, pcmConfig)

	if encoder == nil {
		t.Fatal("createPCMAEncode returned nil")
	}

	// Test with audio packet
	pcmData := make([]byte, 160)
	packet := &media.AudioPacket{
		Payload: pcmData,
	}

	result, err := encoder(packet)

	if err != nil {
		t.Errorf("encoder failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestCreatePCMUDecode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcmu",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	decoder := createPCMUDecode(srcConfig, pcmConfig)

	if decoder == nil {
		t.Fatal("createPCMUDecode returned nil")
	}

	// Test with audio packet
	ulawData := []byte{0x00, 0x55, 0xAA, 0xFF}
	packet := &media.AudioPacket{
		Payload: ulawData,
	}

	result, err := decoder(packet)

	if err != nil {
		t.Errorf("decoder failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestCreatePCMUEncode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcmu",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	encoder := createPCMUEncode(srcConfig, pcmConfig)

	if encoder == nil {
		t.Fatal("createPCMUEncode returned nil")
	}

	// Test with audio packet
	pcmData := make([]byte, 160)
	packet := &media.AudioPacket{
		Payload: pcmData,
	}

	result, err := encoder(packet)

	if err != nil {
		t.Errorf("encoder failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestCreateG722Decode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "g722",
		SampleRate: 16000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	decoder := createG722Decode(srcConfig, pcmConfig)

	if decoder == nil {
		t.Fatal("createG722Decode returned nil")
	}

	// Test with audio packet
	g722Data := []byte{0x00, 0x55, 0xAA, 0xFF}
	packet := &media.AudioPacket{
		Payload: g722Data,
	}

	result, err := decoder(packet)

	if err != nil {
		t.Errorf("decoder failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestCreateG722Encode(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "g722",
		SampleRate: 16000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	encoder := createG722Encode(srcConfig, pcmConfig)

	if encoder == nil {
		t.Fatal("createG722Encode returned nil")
	}

	// Test with audio packet
	pcmData := make([]byte, 160)
	packet := &media.AudioPacket{
		Payload: pcmData,
	}

	result, err := encoder(packet)

	if err != nil {
		t.Errorf("encoder failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestEncoderWithNonAudioPacket(t *testing.T) {
	srcConfig := media.CodecConfig{
		Codec:      "pcmu",
		SampleRate: 8000,
	}

	pcmConfig := media.CodecConfig{
		Codec:      "pcm",
		SampleRate: 16000,
	}

	encoder := createPCMUEncode(srcConfig, pcmConfig)

	// Test with non-audio packet (should pass through)
	videoPacket := &media.VideoPacket{
		Payload: []byte("video data"),
	}

	result, err := encoder(videoPacket)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 packet, got %d", len(result))
	}

	if result[0] != videoPacket {
		t.Error("non-audio packet should pass through unchanged")
	}
}
