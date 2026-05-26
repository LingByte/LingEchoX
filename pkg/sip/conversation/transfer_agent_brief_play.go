package conversation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/synthesizer"
	siptts "github.com/LinByte/VoiceServer/pkg/voice/tts"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

const transferAgentBriefMaxPlay = 45 * time.Second

var transferAgentBriefRunning sync.Map // inbound Call-ID -> struct{}

// SpeakTextOnLeg synthesizes text to playbackCS using tenant TTS creds from tenantCS.
func SpeakTextOnLeg(ctx context.Context, playbackCS, tenantCS *sipSession.CallSession, text string, lg *zap.Logger) error {
	if playbackCS == nil || tenantCS == nil {
		return fmt.Errorf("sip conversation: nil session for leg TTS")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	ms := playbackCS.MediaSession()
	if ms == nil {
		return fmt.Errorf("sip conversation: media session not ready")
	}
	if lg == nil {
		if logger.Lg != nil {
			lg = logger.Lg
		} else {
			lg = zap.NewNop()
		}
	}
	env, loaded, err := ResolveTenantVoiceEnv(ctx, tenantCS)
	if err != nil {
		return err
	}
	if !loaded {
		return fmt.Errorf("sip conversation: missing tenant voice config")
	}
	if env.TTSConfigRaw == nil {
		return fmt.Errorf("sip conversation: missing tenant TTS config")
	}
	ttsHandle, err := synthesizer.NewStreamingFromCredential(env.TTSConfigRaw)
	if err != nil {
		return fmt.Errorf("sip conversation: tts provider: %w", err)
	}
	pcmBridgeSR := sipVoicePCMBridgeRate(playbackCS)
	ttsCloudSR := sipVoiceTTSCloudSampleRate(env, ttsHandle.SampleRate, pcmBridgeSR)

	ttsPipe, err := siptts.New(siptts.Config{
		Service:       ttsHandle.Stream,
		SampleRate:    ttsCloudSR,
		Channels:      1,
		FrameDuration: 20 * time.Millisecond,
		PaceRealtime:  true,
		SendPCMFrame: func(frame []byte) error {
			if len(frame) == 0 {
				return nil
			}
			pcmOut := frame
			if ttsCloudSR != pcmBridgeSR && len(frame) >= 2 {
				if out, err := media.ResamplePCM(frame, ttsCloudSR, pcmBridgeSR); err == nil && len(out) > 0 {
					pcmOut = out
				}
			}
			ms.SendToOutput("sip-agent-brief", &media.AudioPacket{
				Payload:       pcmOut,
				IsSynthesized: true,
			})
			return nil
		},
		Logger: lg,
	})
	if err != nil {
		return fmt.Errorf("sip conversation: tts pipeline: %w", err)
	}
	runCtx := ctx
	if runCtx == nil {
		runCtx = ms.GetContext()
	}
	ttsPipe.Start(runCtx)
	defer ttsPipe.Stop()
	if err := ttsPipe.Speak(text); err != nil {
		return err
	}
	return ttsPipe.Finalize()
}

func playTransferAgentBriefThenBridge(
	inboundCallID string,
	outboundCS *sipSession.CallSession,
	outboundCallID string,
	tmpl string,
	lg *zap.Logger,
) {
	inboundCallID = normCallID(inboundCallID)
	outboundCallID = normCallID(outboundCallID)
	if lookupInbound == nil {
		startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
		return
	}
	inbound := lookupInbound(inboundCallID)
	if inbound == nil || outboundCS == nil {
		startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
		return
	}
	ms := inbound.MediaSession()
	if ms == nil {
		startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
		return
	}
	rendered := RenderTransferAgentBriefTemplate(tmpl, buildTransferAgentBriefVars(inboundCallID))
	if rendered == "" {
		startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
		return
	}
	lg.Info("sip transfer: playing agent brief before bridge (caller keeps hold music)",
		zap.String("inbound_call_id", inboundCallID),
		zap.String("outbound_call_id", outboundCallID),
		zap.String("text", rendered),
	)
	playCtx, cancel := context.WithTimeout(ms.GetContext(), transferAgentBriefMaxPlay)
	defer cancel()
	if err := SpeakTextOnLeg(playCtx, outboundCS, inbound, rendered, lg); err != nil {
		lg.Warn("sip transfer: agent brief TTS failed, bridging anyway",
			zap.String("inbound_call_id", inboundCallID),
			zap.Error(err),
		)
	}
	if lookupInbound(inboundCallID) == nil {
		if bridgeSendOutboundBYE != nil && outboundCallID != "" {
			_ = bridgeSendOutboundBYE(outboundCallID)
		}
		if outboundCS != nil {
			outboundCS.Stop()
		}
		if callStore != nil && outboundCallID != "" {
			callStore.RemoveCallSession(outboundCallID)
		}
		return
	}
	startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
}
