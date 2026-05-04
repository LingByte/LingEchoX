package voicedialog

import (
	"context"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/media"
	"github.com/LingByte/SoulNexus/pkg/sip/conversation"
	"go.uber.org/zap"
)

// RegisterTransferPhaseNotifier wires SIP transfer phases to dialog.transfer WebSocket events.
func RegisterTransferPhaseNotifier() {
	conversation.SetTransferPhaseNotifier(deliverConversationTransferPhase)
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
