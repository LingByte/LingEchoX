package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/devices"
	media2 "github.com/LingByte/LingEchoX/pkg/media"
	"github.com/LingByte/LingEchoX/pkg/media/encoder"
	"github.com/LingByte/LingEchoX/pkg/webrtc/constants"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia"
	"github.com/gen2brain/malgo"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/youpy/go-wav"
	"go.uber.org/zap"
)

const (
	packetLogInterval = 100
	warningLogLimit   = 3

	// Frame configuration
	frameDurationMs = 20
	bytesPerFrame   = 160 // 20ms * 8000Hz = 160 samples = 160 bytes (PCMA is 1 byte per sample)
	// Audio configuration
	targetSampleRate = 8000 // PCMA standard sample rate
	audioChannels    = 1
	audioBitDepth    = 16
)

// Client represents a WebRTC client connection
type Client struct {
	conn             *websocket.Conn           // client websocket connection
	Transport        *rtcmedia.WebRTCTransport // webrtc transport
	SessionID        string                    // sessionID
	AudioReceived    bool                      // Track if we've started receiving audio
	Mu               sync.Mutex
	Interrupt        chan os.Signal
	Done             chan struct{}
	logger           *zap.Logger
	OnConnected      func(client *Client, msg rtcmedia.SignalMessage)
	ctx              context.Context
	cancel           context.CancelFunc
	audioSource      AudioSource
	audioFile        string
	isReceiving      bool
	isSending        bool
	mixedAudioPlayer *devices.StreamAudioPlayer
	micRecorder      *devices.AudioDevice
}

// NewClient creates a new WebRTC client with context
func NewClient(logger *zap.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		logger:      logger,
		Done:        make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		audioSource: AudioSourceFile,
		audioFile:   constants.ClientAudioFile,
	}
}

func (c *Client) SetConnection(conn *websocket.Conn) {
	c.conn = conn
}

// StartBidirectionalAudio starts both audio sending and receiving simultaneously
func (c *Client) StartBidirectionalAudio() error {
	// Setup mixed audio player for receiving audio
	if err := c.setupMixedAudioPlayer(); err != nil {
		return fmt.Errorf("failed to setup mixed audio player: %w", err)
	}

	// Start audio receiver in goroutine
	go func() {
		if err := c.StartAudioReceiver(); err != nil {
			c.logger.Error("audio receiver error", zap.Error(err))
		}
	}()

	// Start audio sender in goroutine
	go func() {
		if err := c.StartAudioSender(); err != nil {
			c.logger.Error("audio sender error", zap.Error(err))
		}
	}()

	return nil
}

// setupMixedAudioPlayer sets up the mixed audio player for receiving audio
func (c *Client) setupMixedAudioPlayer() error {
	player, err := devices.NewStreamAudioPlayer(
		audioChannels,
		targetSampleRate,
		malgo.FormatS16,
	)
	if err != nil {
		return fmt.Errorf("failed to create mixed audio player: %w", err)
	}

	if err := player.Play(); err != nil {
		player.Close()
		return fmt.Errorf("failed to start mixed audio player: %w", err)
	}

	c.mixedAudioPlayer = player
	fmt.Printf("[Client] Mixed audio player started: %dHz, %d channel(s)\n",
		targetSampleRate, audioChannels)

	return nil
}

// ProcessAudioPacket processes a single RTP audio packet
func (c *Client) ProcessAudioPacket(
	packet *rtp.Packet,
	decodeFunc media2.EncoderFunc,
	packetCount int,
) error {
	payload := packet.Payload
	if len(payload) == 0 {
		return nil
	}

	// Decode PCMA to PCM
	audioPacket := &media2.AudioPacket{Payload: payload}
	decodedPackets, err := decodeFunc(audioPacket)
	if err != nil {
		if packetCount%packetLogInterval == 0 {
			fmt.Printf("[Client] Error decoding frame %d: %v\n", packetCount, err)
		}
		return err
	}

	// Collect all decoded PCM data and write to mixed audio player
	allPCMData := c.collectPCMData(decodedPackets, packetCount)
	if len(allPCMData) > 0 && c.mixedAudioPlayer != nil {
		if err := c.mixedAudioPlayer.Write(allPCMData); err != nil {
			// Buffer full is not critical, only log other errors
			if packetCount%packetLogInterval == 0 && err.Error() != "音频缓冲区已满" {
				fmt.Printf("[Client] Error writing to mixed player: %v\n", err)
			}
		}
	}

	return nil
}

// collectPCMData collects and validates PCM data from decoded packets
func (c *Client) collectPCMData(decodedPackets []media2.MediaPacket, packetCount int) []byte {
	var allPCMData []byte

	for _, packet := range decodedPackets {
		af, ok := packet.(*media2.AudioPacket)
		if !ok {
			continue
		}

		// Skip empty frames
		if len(af.Payload) == 0 {
			continue
		}

		// Validate PCM data (should be 16-bit, so length must be even)
		if len(af.Payload)%2 != 0 {
			if packetCount <= warningLogLimit {
				fmt.Printf("[Client] Warning: Odd PCM length at packet %d: %d bytes\n",
					packetCount, len(af.Payload))
			}
			continue
		}

		allPCMData = append(allPCMData, af.Payload...)
	}

	return allPCMData
}

// StartAudioSender starts sending audio data to the server
func (c *Client) StartAudioSender() error {
	c.Mu.Lock()
	if c.isSending {
		c.Mu.Unlock()
		return nil // Already sending
	}
	c.isSending = true
	c.Mu.Unlock()

	fmt.Println("[Client] Starting to send audio to server...")

	// Get transmit track
	txTrack := c.Transport.GetTxTrack()
	if txTrack == nil {
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
		return fmt.Errorf("txTrack is nil")
	}

	// Send audio based on source type
	switch c.audioSource {
	case AudioSourceFile:
		return c.sendAudioFromFile(txTrack)
	case AudioSourceMicrophone:
		return c.sendAudioFromMicrophone(txTrack)
	case AudioSourceMixed:
		return c.sendMixedAudio(txTrack)
	default:
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
		return fmt.Errorf("unsupported audio source: %v", c.audioSource)
	}
}

// sendAudioFromFile sends audio from a file
func (c *Client) sendAudioFromFile(txTrack *webrtc.TrackLocalStaticSample) error {
	defer func() {
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
	}()

	// Load and process audio file
	pcmaData, err := c.loadAndProcessAudioFile()
	if err != nil {
		return fmt.Errorf("failed to load audio: %w", err)
	}

	// Send audio frames
	return c.sendAudioFrames(txTrack, pcmaData)
}

// sendAudioFromMicrophone sends audio from microphone
func (c *Client) sendAudioFromMicrophone(txTrack *webrtc.TrackLocalStaticSample) error {
	defer func() {
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
	}()

	// Setup microphone recording
	if err := c.setupMicrophoneRecording(); err != nil {
		return fmt.Errorf("failed to setup microphone: %w", err)
	}
	defer c.micRecorder.Close()

	fmt.Println("[Client] Started microphone recording and sending")

	frameDuration := time.Duration(frameDurationMs) * time.Millisecond
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	frameCount := 0

	for {
		select {
		case <-c.ctx.Done():
			fmt.Println("[Client] Microphone audio sender stopped")
			return nil
		case <-ticker.C:
			// Get audio data from microphone
			audioData := c.micRecorder.GetCapturedData()
			if len(audioData) == 0 {
				// Send silence if no audio data
				silenceData := make([]byte, bytesPerFrame)
				sample := media.Sample{
					Data:     silenceData,
					Duration: frameDuration,
				}
				txTrack.WriteSample(sample)
				continue
			}

			// Ensure we have the right amount of data for one frame
			if len(audioData) > bytesPerFrame*2 { // *2 because it's 16-bit PCM
				audioData = audioData[:bytesPerFrame*2]
			}

			// Encode PCM to PCMA
			pcmaData, err := encoder.EncodePCMA(audioData)
			if err != nil {
				c.logger.Error("failed to encode microphone audio", zap.Error(err))
				continue
			}

			// Ensure PCMA frame size
			if len(pcmaData) > bytesPerFrame {
				pcmaData = pcmaData[:bytesPerFrame]
			}

			// Send frame
			sample := media.Sample{
				Data:     pcmaData,
				Duration: frameDuration,
			}

			if err := txTrack.WriteSample(sample); err != nil {
				c.logger.Error("failed to send microphone sample", zap.Error(err))
			} else {
				frameCount++
				if frameCount%100 == 0 {
					fmt.Printf("[Client] Sent %d microphone frames\n", frameCount)
				}
			}

			// Clear captured data
			c.micRecorder.ClearCapturedData()
		}
	}
}

// sendMixedAudio sends mixed audio (file + microphone)
func (c *Client) sendMixedAudio(txTrack *webrtc.TrackLocalStaticSample) error {
	defer func() {
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
	}()

	// Setup microphone recording
	if err := c.setupMicrophoneRecording(); err != nil {
		return fmt.Errorf("failed to setup microphone: %w", err)
	}
	defer c.micRecorder.Close()

	// Load and process audio file
	fileAudioData, err := c.loadAndProcessAudioFile()
	if err != nil {
		return fmt.Errorf("failed to load background audio: %w", err)
	}

	fmt.Println("[Client] Started mixed audio (microphone + file background)")

	frameDuration := time.Duration(frameDurationMs) * time.Millisecond
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	frameCount := 0
	filePosition := 0

	for {
		select {
		case <-c.ctx.Done():
			fmt.Println("[Client] Mixed audio sender stopped")
			return nil
		case <-ticker.C:
			// Get microphone audio data
			micAudioData := c.micRecorder.GetCapturedData()

			// Get file audio data for this frame
			fileFrameData := c.getFileAudioFrame(fileAudioData, &filePosition, bytesPerFrame)

			// Mix the audio
			mixedPCMAData := c.mixAudioSources(micAudioData, fileFrameData)

			// Send mixed frame
			sample := media.Sample{
				Data:     mixedPCMAData,
				Duration: frameDuration,
			}

			if err := txTrack.WriteSample(sample); err != nil {
				c.logger.Error("failed to send mixed audio sample", zap.Error(err))
			} else {
				frameCount++
				if frameCount%100 == 0 {
					fmt.Printf("[Client] Sent %d mixed audio frames\n", frameCount)
				}
			}

			// Clear captured microphone data
			c.micRecorder.ClearCapturedData()
		}
	}
}

// getFileAudioFrame gets a frame of audio data from the file, looping if necessary
func (c *Client) getFileAudioFrame(fileData []byte, position *int, frameSize int) []byte {
	if len(fileData) == 0 {
		return make([]byte, frameSize) // Return silence if no file data
	}

	// Calculate the end position for this frame
	end := *position + frameSize

	// If we've reached the end of the file, loop back to the beginning
	if *position >= len(fileData) {
		*position = 0
		end = frameSize
	}

	// If the frame would extend beyond the file, we need to handle wrapping
	if end > len(fileData) {
		// Get the remaining data from current position to end of file
		remaining := fileData[*position:]
		// Get the needed data from the beginning of the file
		needed := frameSize - len(remaining)
		beginning := fileData[:needed]

		// Combine them
		frame := make([]byte, frameSize)
		copy(frame, remaining)
		copy(frame[len(remaining):], beginning)

		*position = needed
		return frame
	}

	// Normal case: get frame data and advance position
	frame := fileData[*position:end]
	*position = end

	return frame
}

// mixAudioSources mixes microphone and file audio with volume control
func (c *Client) mixAudioSources(micData []byte, fileData []byte) []byte {
	// Configuration for mixing
	const (
		micVolumeRatio  = 0.7 // Microphone volume (70%)
		fileVolumeRatio = 0.4 // File background volume (40%)
	)

	targetSize := bytesPerFrame

	// Prepare microphone PCM data
	var micPCMData []byte
	if len(micData) > 0 {
		// Ensure mic data is reasonable size for one frame (16-bit PCM)
		expectedPCMSize := targetSize * 2 // PCMA frame -> PCM frame (roughly 2x)
		if len(micData) > expectedPCMSize {
			micData = micData[:expectedPCMSize]
		}
		micPCMData = micData
	} else {
		// Create silence for microphone
		micPCMData = make([]byte, targetSize*2)
	}

	// Prepare file PCM data (file data is already PCMA, need to decode to PCM for mixing)
	var filePCMData []byte
	if len(fileData) > 0 {
		// Decode PCMA file data to PCM for mixing
		audioPacket := &media2.AudioPacket{Payload: fileData}

		// Create decoder
		decodeFunc, err := encoder.CreateDecode(
			media2.CodecConfig{
				Codec:         "pcma",
				SampleRate:    targetSampleRate,
				Channels:      audioChannels,
				BitDepth:      8,
				FrameDuration: "20ms",
			},
			media2.CodecConfig{
				Codec:         "pcm",
				SampleRate:    targetSampleRate,
				Channels:      audioChannels,
				BitDepth:      audioBitDepth,
				FrameDuration: "20ms",
			},
		)

		if err == nil {
			decodedPackets, err := decodeFunc(audioPacket)
			if err == nil && len(decodedPackets) > 0 {
				if pcmPacket, ok := decodedPackets[0].(*media2.AudioPacket); ok {
					filePCMData = pcmPacket.Payload
				}
			}
		}
	}

	if len(filePCMData) == 0 {
		// Create silence for file
		filePCMData = make([]byte, len(micPCMData))
	}

	// Ensure both PCM data have the same length
	maxLen := len(micPCMData)
	if len(filePCMData) > maxLen {
		maxLen = len(filePCMData)
	}

	if len(micPCMData) < maxLen {
		padded := make([]byte, maxLen)
		copy(padded, micPCMData)
		micPCMData = padded
	}

	if len(filePCMData) < maxLen {
		padded := make([]byte, maxLen)
		copy(padded, filePCMData)
		filePCMData = padded
	}

	// Mix PCM audio (16-bit samples)
	mixedPCMData := make([]byte, maxLen)
	for i := 0; i < maxLen-1; i += 2 {
		// Get 16-bit samples
		micSample := int16(micPCMData[i]) | int16(micPCMData[i+1])<<8
		fileSample := int16(filePCMData[i]) | int16(filePCMData[i+1])<<8

		// Mix with volume control
		mixedSample := int16(float64(micSample)*micVolumeRatio + float64(fileSample)*fileVolumeRatio)

		// Write back mixed sample
		mixedPCMData[i] = byte(mixedSample)
		mixedPCMData[i+1] = byte(mixedSample >> 8)
	}

	// Encode mixed PCM back to PCMA
	mixedPCMAData, err := encoder.EncodePCMA(mixedPCMData)
	if err != nil {
		c.logger.Error("failed to encode mixed audio", zap.Error(err))
		// Return silence on error
		return make([]byte, targetSize)
	}

	// Ensure correct frame size
	if len(mixedPCMAData) > targetSize {
		mixedPCMAData = mixedPCMAData[:targetSize]
	} else if len(mixedPCMAData) < targetSize {
		padded := make([]byte, targetSize)
		copy(padded, mixedPCMAData)
		mixedPCMAData = padded
	}

	return mixedPCMAData
}

// pcmaToLinear converts PCMA sample to linear 16-bit value
func (c *Client) pcmaToLinear(pcma byte) int16 {
	// Simplified PCMA to linear conversion
	// This is a basic implementation - for production use, you'd want a proper lookup table
	sign := (pcma & 0x80) != 0
	exponent := (pcma & 0x70) >> 4
	mantissa := pcma & 0x0F

	linear := int16((mantissa << 4) + 8)
	linear <<= exponent

	if sign {
		linear = -linear
	}

	return linear
}

// linearToPCMA converts linear 16-bit value to PCMA sample
func (c *Client) linearToPCMA(linear int16) byte {
	// Simplified linear to PCMA conversion
	// This is a basic implementation - for production use, you'd want a proper lookup table
	if linear < 0 {
		linear = -linear
		if linear > 32767 {
			linear = 32767
		}
		// Find exponent and mantissa
		exponent := 7
		for exponent > 0 && linear < (1<<(exponent+3)) {
			exponent--
		}
		mantissa := int16((linear >> (exponent + 3)) & 0x0F)
		return byte(0x80 | (exponent << 4) | int(mantissa))
	} else {
		if linear > 32767 {
			linear = 32767
		}
		// Find exponent and mantissa
		exponent := 7
		for exponent > 0 && linear < (1<<(exponent+3)) {
			exponent--
		}
		mantissa := int16((linear >> (exponent + 3)) & 0x0F)
		return byte((exponent << 4) | int(mantissa))
	}
}

// setupMicrophoneRecording sets up microphone recording
func (c *Client) setupMicrophoneRecording() error {
	config := devices.DefaultDeviceConfig()
	config.SampleRate = targetSampleRate
	config.Channels = audioChannels
	config.Format = malgo.FormatS16 // 16-bit signed integer

	fmt.Printf("[Client] Setting up microphone: %dHz, %d channels, format=%v\n",
		config.SampleRate, config.Channels, config.Format)

	recorder, err := devices.NewAudioDevice(config)
	if err != nil {
		return fmt.Errorf("failed to create audio device: %w", err)
	}

	if err := recorder.StartRecording(); err != nil {
		recorder.Close()
		return fmt.Errorf("failed to start recording: %w", err)
	}

	c.micRecorder = recorder
	fmt.Println("[Client] Microphone recording started successfully")
	return nil
}

// resampleAudio resamples audio to target sample rate
func (c *Client) resampleAudio(data []byte, sourceRate int) ([]byte, error) {
	fmt.Printf("[Client] Resampling from %dHz to %dHz...\n", sourceRate, targetSampleRate)

	resampled, err := media2.ResamplePCM(data, sourceRate, targetSampleRate)
	if err != nil {
		return nil, fmt.Errorf("resampling failed: %w", err)
	}

	fmt.Printf("[Client] Resampled to %dHz (%d bytes)\n", targetSampleRate, len(resampled))
	return resampled, nil
}

// sendAudioFrames sends audio frames with precise timing
func (c *Client) sendAudioFrames(txTrack *webrtc.TrackLocalStaticSample, pcmaData []byte) error {
	frameDuration := time.Duration(frameDurationMs) * time.Millisecond
	startTime := time.Now()
	frameCount := 0

	for i := 0; i < len(pcmaData); i += bytesPerFrame {
		end := i + bytesPerFrame
		if end > len(pcmaData) {
			end = len(pcmaData)
		}

		// Calculate exact send time to maintain consistent frame rate
		expectedTime := startTime.Add(time.Duration(frameCount) * frameDuration)
		if now := time.Now(); expectedTime.After(now) {
			time.Sleep(expectedTime.Sub(now))
		}

		sample := media.Sample{
			Data:     pcmaData[i:end],
			Duration: frameDuration,
		}

		if err := txTrack.WriteSample(sample); err != nil {
			return fmt.Errorf("failed to write sample: %w", err)
		}

		frameCount++
		if frameCount%50 == 0 {
			fmt.Printf("[Client] Sent %d frames (PCMA: %d bytes)...\n",
				frameCount, len(pcmaData[i:end]))
		}
	}

	fmt.Printf("[Client] Finished sending audio (%d frames, %d bytes PCMA)\n",
		frameCount, len(pcmaData))

	return nil
}

// loadAndProcessAudioFile loads and processes the audio file
func (c *Client) loadAndProcessAudioFile() ([]byte, error) {
	// Try to open audio file with different paths
	var file *os.File
	var err error

	// First try the configured audio file path
	file, err = os.Open(c.audioFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	// Read WAV format
	w := wav.NewReader(file)
	format, err := w.Format()
	if err != nil {
		return nil, fmt.Errorf("failed to get WAV format: %w", err)
	}

	fmt.Printf("[Client] WAV format: %dHz, %d channels, %d bits\n",
		format.SampleRate, format.NumChannels, format.BitsPerSample)

	// Read entire file
	allPCMData, err := c.readWAVFile(w)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAV file: %w", err)
	}

	fmt.Printf("[Client] Read %d bytes from WAV file\n", len(allPCMData))

	// Convert to mono if needed
	channels := int(format.NumChannels)
	if channels > 1 {
		allPCMData = c.convertToMono(allPCMData, channels, int(format.BitsPerSample))
		channels = 1
	}

	// Resample if needed
	if int(format.SampleRate) != targetSampleRate {
		allPCMData, err = c.resampleAudio(allPCMData, int(format.SampleRate))
		if err != nil {
			return nil, err
		}
	}

	// Encode to PCMA
	pcmaData, err := encoder.EncodePCMA(allPCMData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode to PCMA: %w", err)
	}

	fmt.Printf("[Client] Encoded %d bytes PCM to %d bytes PCMA\n",
		len(allPCMData), len(pcmaData))

	return pcmaData, nil
}

// readWAVFile reads the entire WAV file
func (c *Client) readWAVFile(w *wav.Reader) ([]byte, error) {
	var allPCMData []byte
	tempBuffer := make([]byte, 8192)

	for {
		n, err := w.Read(tempBuffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		allPCMData = append(allPCMData, tempBuffer[:n]...)
	}

	return allPCMData, nil
}

// convertToMono converts stereo audio to mono by averaging channels
func (c *Client) convertToMono(data []byte, channels, bytesPerSample int) []byte {
	if bytesPerSample != 2 {
		return data // Only support 16-bit
	}

	sampleCount := len(data) / (bytesPerSample * channels)
	monoData := make([]byte, sampleCount*bytesPerSample)

	for i := 0; i < sampleCount; i++ {
		leftIdx := i * bytesPerSample * channels
		rightIdx := leftIdx + bytesPerSample

		if rightIdx+bytesPerSample <= len(data) {
			leftSample := int16(data[leftIdx]) | int16(data[leftIdx+1])<<8
			rightSample := int16(data[rightIdx]) | int16(data[rightIdx+1])<<8

			// Average the two channels
			avg := int16((int32(leftSample) + int32(rightSample)) / 2)

			monoIdx := i * bytesPerSample
			monoData[monoIdx] = byte(avg)
			monoData[monoIdx+1] = byte(avg >> 8)
		}
	}

	fmt.Printf("[Client] Converted stereo to mono (%d samples)\n", sampleCount)
	return monoData
}

// extractCandidates extracts candidate strings from the interface slice
func (c *Client) extractCandidates(candidates []interface{}) []string {
	var candidateStrs []string
	for _, candidate := range candidates {
		if candidateStr, ok := candidate.(string); ok {
			candidateStrs = append(candidateStrs, candidateStr)
		}
	}
	return candidateStrs
}

// Close closes the client connection and cleans up resources
func (c *Client) Close() error {
	// Cancel context to stop all goroutines
	if c.cancel != nil {
		c.cancel()
	}

	// Close audio devices
	if c.mixedAudioPlayer != nil {
		c.mixedAudioPlayer.Close()
		c.mixedAudioPlayer = nil
	}

	if c.micRecorder != nil {
		c.micRecorder.Close()
		c.micRecorder = nil
	}

	// Close transport
	if c.Transport != nil {
		c.Transport.Close()
	}

	// Close WebSocket connection
	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// StopAudioSender stops the audio sender
func (c *Client) StopAudioSender() {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.isSending = false
}

// StopAudioReceiver stops the audio receiver
func (c *Client) StopAudioReceiver() {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.isReceiving = false
}

// IsReceiving returns whether audio is being received
func (c *Client) IsReceiving() bool {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return c.isReceiving
}

// IsSending returns whether audio is being sent
func (c *Client) IsSending() bool {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return c.isSending
}
