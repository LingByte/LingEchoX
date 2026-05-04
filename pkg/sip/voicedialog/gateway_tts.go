package voicedialog

import (
	"context"
	"fmt"

	"github.com/LingByte/SoulNexus/pkg/media/encoder"
	"github.com/LingByte/SoulNexus/pkg/synthesizer"
)

// gatewayQcloudTTSStream adapts synthesizer.QCloudService to siptts.Service (streaming PCM chunks).
type gatewayQcloudTTSStream struct {
	svc *synthesizer.QCloudService
}

func (q *gatewayQcloudTTSStream) SynthesizeStream(ctx context.Context, text string, callback func(pcm []byte) error) error {
	if q == nil || q.svc == nil {
		return fmt.Errorf("voicedialog gateway: nil tts")
	}
	done := make(chan error, 1)
	go func() {
		h := &gatewayTTSStreamHandler{callback: callback, ctx: ctx}
		done <- q.svc.Synthesize(context.Background(), h, text)
	}()
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-done:
		return err
	}
}

type gatewayTTSStreamHandler struct {
	ctx        context.Context
	callback   func([]byte) error
	firstChunk bool
}

func (h *gatewayTTSStreamHandler) OnMessage(data []byte) {
	if h == nil || len(data) == 0 {
		return
	}
	if h.ctx != nil && h.ctx.Err() != nil {
		return
	}
	if !h.firstChunk {
		h.firstChunk = true
		data = encoder.StripWavHeader(data)
	}
	_ = h.callback(data)
}

func (h *gatewayTTSStreamHandler) OnTimestamp(_ synthesizer.SentenceTimestamp) {}
