package conversation

import (
	"context"
	"fmt"
	"os"
	"strings"

	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

const defaultTransferConfirmNormalWAV = "scripts/transfer_confirm_normal.wav"

func transferConfirmReplyText(execute bool) string {
	if execute {
		return transferConfirmExecuteReplyZH
	}
	return transferConfirmNormalReplyZH
}

func transferConfirmNormalWAVPath() string {
	if p := strings.TrimSpace(os.Getenv("SIP_TRANSFER_CONFIRM_NORMAL_WAV_PATH")); p != "" {
		return p
	}
	return defaultTransferConfirmNormalWAV
}

// PlayTransferConfirmReply plays the fixed transfer-confirm phrase on the inbound leg.
// execute=false → scripts/transfer_confirm_normal.wav (or SIP_TRANSFER_CONFIRM_NORMAL_WAV_PATH).
// execute=true → tenant TTS for the final phrase before dial.
func PlayTransferConfirmReply(ctx context.Context, cs *sipSession.CallSession, execute bool, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	if !execute {
		return playTransferConfirmWAV(ctx, cs, transferConfirmNormalWAVPath(), lg)
	}
	return SpeakTextOnce(ctx, cs, transferConfirmReplyText(true), lg)
}

func playTransferConfirmWAV(ctx context.Context, cs *sipSession.CallSession, wavPath string, lg *zap.Logger) error {
	ms := cs.MediaSession()
	if ms == nil {
		return fmt.Errorf("sip transfer confirm: nil media session")
	}
	pcmSR := cs.PCMSampleRate()
	if pcmSR <= 0 {
		pcmSR = 8000
	}
	pcm, err := LoadWAVAsPCM16Mono(wavPath, pcmSR)
	if err != nil {
		return fmt.Errorf("load transfer confirm wav %q: %w", wavPath, err)
	}
	if lg != nil {
		lg.Info("sip transfer confirm: playing wav",
			zap.String("call_id", cs.CallID),
			zap.String("path", wavPath),
			zap.Int("pcm_bytes", len(pcm)),
			zap.Int("sample_rate", pcmSR),
		)
	}
	runCtx := ctx
	if runCtx == nil {
		runCtx = ms.GetContext()
	}
	return playWelcomePCM(runCtx, pcm, ms, lg, pcmSR, cs.WriteAIPCM)
}
