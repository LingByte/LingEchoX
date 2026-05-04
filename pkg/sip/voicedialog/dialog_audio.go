package voicedialog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/media"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	"github.com/LingByte/SoulNexus/pkg/utils"
)

const (
	// WSReadBufferSize / WSWriteBufferSize are gorilla/websocket hijack buffers.
	WSReadBufferSize  = 1024 * 1024
	WSWriteBufferSize = 1024 * 1024
)

func wsTokenExpected() string {
	return strings.TrimSpace(utils.GetEnv("VOICE_DIALOG_WS_TOKEN"))
}

// transferLoadingAudioRef returns SIP_TRANSFER_RINGING_WAV_PATH when set (same clip family as SIP transfer ringback;
// optional “loading” loop before ringing). Empty = no loading clip.
func transferLoadingAudioRef() string {
	return strings.TrimSpace(utils.GetEnv("SIP_TRANSFER_RINGING_WAV_PATH"))
}

// ParseVoicedialogAudioRef returns source_kind, client-visible source, and filesystem path or URL for loading WAV PCM.
func ParseVoicedialogAudioRef(ref string) (kind string, sourceDisplay string, pathOrURL string) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "script:")
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", ""
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return SourceKindURL, ref, ref
	}
	if filepath.IsAbs(ref) {
		clean := filepath.Clean(ref)
		return SourceKindScript, filepath.Base(clean), clean
	}
	ref = strings.TrimPrefix(ref, "scripts/")
	ref = filepath.Base(ref)
	if ref == "" || ref == "." {
		return "", "", ""
	}
	path := filepath.Join("scripts", ref)
	return SourceKindScript, ref, path
}

func loadVoicedialogWAVPCM(ctx context.Context, kind, pathOrURL string, sampleRate int) ([]byte, error) {
	if kind == SourceKindURL {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pathOrURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("http %d fetching wav", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			return nil, err
		}
		return conversation.LoadWAVAsPCM16FromBytes(body, sampleRate)
	}
	return conversation.LoadWAVAsPCM16Mono(pathOrURL, sampleRate)
}

func playPCMFrames(ctx context.Context, ms *media.MediaSession, pcm []byte, pcmSR int, outputTag string) error {
	if ms == nil || len(pcm) == 0 || pcmSR <= 0 {
		return nil
	}
	bytesPerFrame := pcmSR * 2 * 20 / 1000
	if bytesPerFrame <= 0 {
		bytesPerFrame = 640
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for off := 0; off < len(pcm); off += bytesPerFrame {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ms.GetContext().Done():
			return ms.GetContext().Err()
		case <-ticker.C:
		}
		end := off + bytesPerFrame
		if end > len(pcm) {
			end = len(pcm)
		}
		frame := pcm[off:end]
		if len(frame) == 0 {
			continue
		}
		ms.SendToOutput(outputTag, &media.AudioPacket{
			Payload:       frame,
			IsSynthesized: true,
		})
	}
	return nil
}
