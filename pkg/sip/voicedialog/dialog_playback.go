package voicedialog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/media"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"go.uber.org/zap"
)

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

func waitFirstRTP(ctx context.Context, cs *sipSession.CallSession, waitMs int) {
	if waitMs <= 0 || cs == nil {
		return
	}
	rs := cs.RTPSession()
	if rs == nil {
		return
	}
	t := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
	defer t.Stop()
	select {
	case <-rs.FirstPacket():
	case <-t.C:
	case <-ctx.Done():
		return
	}
}

func (sess *dialogSession) runDialogWelcome() {
	if sess == nil || sess.cs == nil {
		return
	}
	ms := sess.cs.MediaSession()
	if ms == nil {
		return
	}
	callID := sess.meta.CallID
	ref := utils.GetEnv("SIP_WELCOME_WAV_PATH")
	if strings.TrimSpace(ref) == "" {
		ref = "scripts/welcome.wav"
	}
	kind, srcDisp, loc := ParseVoicedialogAudioRef(ref)
	if kind == "" || loc == "" {
		sess.emitGateway(event(EvDialogWelcome, callID, map[string]any{
			KeyPhase:  PhaseWelcomeSkipped,
			KeyDetail: "invalid SIP_WELCOME_WAV_PATH or scripts/welcome.wav",
		}))
		return
	}

	ctx := ms.GetContext()
	sess.emitGateway(event(EvDialogWelcome, callID, map[string]any{
		KeyPhase:      PhaseWelcomeStarted,
		KeySourceKind: kind,
		KeySource:     srcDisp,
	}))

	waitFirstRTP(ctx, sess.cs, 2000)

	sess.emitGateway(event(EvDialogWelcome, callID, map[string]any{
		KeyPhase:      PhaseWelcomePlaying,
		KeySourceKind: kind,
		KeySource:     srcDisp,
	}))

	pcmSR := pcmBridgeHz(sess)
	pcm, err := loadVoicedialogWAVPCM(ctx, kind, loc, pcmSR)
	if err != nil {
		logger.Warn("voicedialog welcome wav failed",
			zap.String(KeyCallID, callID),
			zap.Error(err),
		)
		sess.emitGateway(event(EvDialogWelcome, callID, map[string]any{
			KeyPhase:      PhaseWelcomeError,
			KeySourceKind: kind,
			KeySource:     srcDisp,
			KeyDetail:     err.Error(),
		}))
		return
	}

	if err := playPCMFrames(ctx, ms, pcm, pcmSR, "voicedialog-welcome"); err != nil && ctx.Err() == nil {
		logger.Debug("voicedialog welcome playback stopped", zap.String(KeyCallID, callID), zap.Error(err))
	}

	sess.emitGateway(event(EvDialogWelcome, callID, map[string]any{
		KeyPhase:      PhaseWelcomeEnded,
		KeySourceKind: kind,
		KeySource:     srcDisp,
	}))
}

func deliverConversationTransferPhase(callID string, phase string, fields map[string]any) {
	if defaultHub == nil {
		return
	}
	defaultHub.mu.Lock()
	sess := defaultHub.sessions[strings.TrimSpace(callID)]
	defaultHub.mu.Unlock()
	if sess == nil {
		return
	}

	ph := strings.TrimSpace(phase)
	extra := map[string]any{KeyPhase: ph}
	for k, v := range fields {
		extra[k] = v
	}

	switch ph {
	case PhaseTransferRequested, PhaseTransferLoading:
		if ref := transferLoadingAudioRef(); ref != "" {
			sk, sd, _ := ParseVoicedialogAudioRef(ref)
			if sk != "" {
				extra[KeySourceKind] = sk
				extra[KeySource] = sd
			}
		}
	}

	sess.emitGateway(event(EvDialogTransfer, callID, extra))

	switch ph {
	case PhaseTransferRequested, PhaseTransferLoading, PhaseTransferRinging, PhaseTransferConnected:
		// Once transfer enters any of these phases the AI is no longer the active speaker:
		// the caller will hear hold music / ringback / agent audio. We must:
		//   1) invalidate every tts.speak still queued in the loopback → gateway pipeline
		//      so trailing AI segments don't bleed into the transition,
		//   2) preempt any in-flight pipe.Speak,
		//   3) ASR / LLM gating is enforced via conversation.IsTransferInProgress() at
		//      the ProcessPCM / EvASRFinal / handleTTSSpeak entrypoints.
		sess.invalidateQueuedTTS()
		sess.stopGatewayTTSPlayback()
	}

	switch ph {
	case PhaseTransferLoading:
		sess.beginTransferLoadingPlayback()
	case PhaseTransferRinging, PhaseTransferConnected, PhaseTransferFailed, PhaseTransferNoAgent:
		sess.stopTransferLoadingPlayback()
	}
}

func (sess *dialogSession) beginTransferLoadingPlayback() {
	if sess == nil || sess.cs == nil {
		return
	}
	sess.transferLoadingMu.Lock()
	if sess.transferLoadingCancel != nil {
		sess.transferLoadingCancel()
		sess.transferLoadingCancel = nil
	}
	ms := sess.cs.MediaSession()
	if ms == nil {
		sess.transferLoadingMu.Unlock()
		return
	}
	ref := transferLoadingAudioRef()
	if strings.TrimSpace(ref) == "" {
		sess.transferLoadingMu.Unlock()
		return
	}
	kind, _, loc := ParseVoicedialogAudioRef(ref)
	if kind == "" || loc == "" {
		sess.transferLoadingMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ms.GetContext())
	sess.transferLoadingCancel = cancel
	sess.transferLoadingMu.Unlock()

	callID := sess.meta.CallID
	go func() {
		defer sess.stopTransferLoadingPlayback()
		pcmSR := pcmBridgeHz(sess)
		pcm, err := loadVoicedialogWAVPCM(ctx, kind, loc, pcmSR)
		if err != nil {
			logger.Warn("voicedialog transfer loading wav failed",
				zap.String(KeyCallID, callID),
				zap.Error(err),
			)
			return
		}
		bytesPerFrame := pcmSR * 2 * 20 / 1000
		if bytesPerFrame <= 0 {
			bytesPerFrame = 640
		}
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		offset := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ms.GetContext().Done():
				return
			case <-ticker.C:
			}
			end := offset + bytesPerFrame
			if end > len(pcm) {
				end = len(pcm)
			}
			frame := pcm[offset:end]
			if len(frame) > 0 {
				ms.SendToOutput("voicedialog-transfer-loading", &media.AudioPacket{
					Payload:       frame,
					IsSynthesized: true,
				})
			}
			offset = end
			if offset >= len(pcm) {
				offset = 0
			}
		}
	}()
}

func (sess *dialogSession) stopTransferLoadingPlayback() {
	if sess == nil {
		return
	}
	sess.transferLoadingMu.Lock()
	defer sess.transferLoadingMu.Unlock()
	if sess.transferLoadingCancel != nil {
		sess.transferLoadingCancel()
		sess.transferLoadingCancel = nil
	}
}
