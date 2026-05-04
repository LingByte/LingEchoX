package voicedialog

import (
	"context"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"go.uber.org/zap"
)

func (sess *dialogSession) runDialogWelcome() {
	if sess == nil || sess.cs == nil {
		return
	}
	ms := sess.cs.MediaSession()
	if ms == nil {
		return
	}
	callID := sess.meta.CallID
	ref := strings.TrimSpace(utils.GetEnv("SIP_WELCOME_WAV_PATH"))
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
