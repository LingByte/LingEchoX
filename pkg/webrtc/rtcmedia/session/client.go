package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/LingByte/LingEchoX/pkg/media"
	"github.com/LingByte/LingEchoX/pkg/media/encoder"
	"github.com/LingByte/LingEchoX/pkg/webrtc/rtcmedia"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	webrtcmedia "github.com/pion/webrtc/v3/pkg/media"
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
	conn            *websocket.Conn           // client websocket connection
	Transport       *rtcmedia.WebRTCTransport // webrtc transport
	SessionID       string                    // sessionID
	AudioReceived   bool                      // Track if we've started receiving audio
	Mu              sync.Mutex
	Interrupt       chan os.Signal
	Done            chan struct{}
	logger          *zap.Logger
	OnConnected     func(client *Client, msg rtcmedia.SignalMessage)
	ctx             context.Context
	cancel          context.CancelFunc
	isReceiving     bool
	isSending       bool
	audioFileWriter io.WriteCloser // File writer for received audio
}

// NewClient creates a new WebRTC client with context
func NewClient(logger *zap.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		logger: logger,
		Done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *Client) SetConnection(conn *websocket.Conn) {
	c.conn = conn
}

// SetAudioFileWriter sets the file writer for received audio
func (c *Client) SetAudioFileWriter(writer io.WriteCloser) {
	c.audioFileWriter = writer
}

// ProcessAudioPacket processes a single RTP audio packet and saves to file
func (c *Client) ProcessAudioPacket(
	packet *rtp.Packet,
	decodeFunc media.EncoderFunc,
	packetCount int,
) error {
	payload := packet.Payload
	if len(payload) == 0 {
		return nil
	}

	// Decode PCMA to PCM
	audioPacket := &media.AudioPacket{Payload: payload}
	decodedPackets, err := decodeFunc(audioPacket)
	if err != nil {
		if packetCount%packetLogInterval == 0 {
			fmt.Printf("[Client] Error decoding frame %d: %v\n", packetCount, err)
		}
		return err
	}

	// Collect all decoded PCM data and write to file
	allPCMData := c.collectPCMData(decodedPackets, packetCount)
	if len(allPCMData) > 0 {
		// Write to audio file
		if _, err := c.audioFileWriter.Write(allPCMData); err != nil {
			fmt.Printf("[Client] Error writing to audio file: %v\n", err)
			return err
		}
	}

	return nil
}

// collectPCMData collects and validates PCM data from decoded packets
func (c *Client) collectPCMData(decodedPackets []media.MediaPacket, packetCount int) []byte {
	var allPCMData []byte

	for _, packet := range decodedPackets {
		af, ok := packet.(*media.AudioPacket)
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

	// Load and process audio file
	pcmaData, err := c.loadAndProcessAudioFile()
	if err != nil {
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
		return fmt.Errorf("failed to load audio: %w", err)
	}

	// Send audio frames
	defer func() {
		c.Mu.Lock()
		c.isSending = false
		c.Mu.Unlock()
	}()

	return c.sendAudioFrames(txTrack, pcmaData)
}

// extractCandidates extracts candidate strings from the interface slice
func (c *Client) extractCandidates(candidates []interface{}) []string {
	var candidateStrs []string
	for _, candidate := range candidates {
		switch v := candidate.(type) {
		case string:
			// Direct string format
			candidateStrs = append(candidateStrs, v)
		case map[string]interface{}:
			// Object format with candidate field
			if candStr, ok := v["candidate"].(string); ok {
				candidateStrs = append(candidateStrs, candStr)
			}
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

	// Close audio file writer
	if c.audioFileWriter != nil {
		c.audioFileWriter.Close()
		c.audioFileWriter = nil
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

// loadAndProcessAudioFile loads and processes the audio file
func (c *Client) loadAndProcessAudioFile() ([]byte, error) {
	// Try to open audio file with different paths
	var file *os.File
	var err error

	// First try the configured audio file path
	file, err = os.Open("ringing.wav")
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

// resampleAudio resamples audio to target sample rate
func (c *Client) resampleAudio(data []byte, sourceRate int) ([]byte, error) {
	fmt.Printf("[Client] Resampling from %dHz to %dHz...\n", sourceRate, targetSampleRate)

	resampled, err := media.ResamplePCM(data, sourceRate, targetSampleRate)
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

		sample := webrtcmedia.Sample{
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

// IsSending returns whether audio is being sent
func (c *Client) IsSending() bool {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return c.isSending
}

// StopAudioSender stops the audio sender
func (c *Client) StopAudioSender() {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.isSending = false
}
