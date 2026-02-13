package session

import (
	"fmt"

	media2 "github.com/LingByte/LingEchoX/pkg/media"
	"github.com/LingByte/LingEchoX/pkg/media/encoder"
	"go.uber.org/zap"
)

// StartAudioReceiver starts receiving and playing audio packets (non-blocking)
func (c *Client) StartAudioReceiver() error {
	c.Mu.Lock()
	if c.isReceiving {
		c.Mu.Unlock()
		return nil // Already receiving
	}
	c.isReceiving = true
	c.Mu.Unlock()

	// Wait for track to be available
	rxTrack, err := c.WaitForTrack()
	if err != nil {
		c.Mu.Lock()
		c.isReceiving = false
		c.Mu.Unlock()
		return err
	}

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
	if err != nil {
		c.Mu.Lock()
		c.isReceiving = false
		c.Mu.Unlock()
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	codec := rxTrack.Codec()
	c.Logger.Info(fmt.Sprintf("[Client] Started receiving audio: %s, %dHz", codec.MimeType, codec.ClockRate))

	packetCount := 0

	// Continuous audio receiving loop
	for {
		select {
		case <-c.ctx.Done():
			c.Mu.Lock()
			c.isReceiving = false
			c.Mu.Unlock()
			fmt.Println("[Client] Audio receiver stopped")
			return nil
		default:
			packet, _, err := rxTrack.ReadRTP()
			if err != nil {
				c.Logger.Error("error reading RTP packet", zap.Error(err))
				continue
			}

			if err := c.ProcessAudioPacket(packet, decodeFunc, packetCount); err != nil {
				continue
			}

			packetCount++
			if packetCount%packetLogInterval == 0 {
				fmt.Printf("[Client] Received and played %d RTP packets\n", packetCount)
			}
		}
	}
}
