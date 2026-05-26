package conversation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/media"
	"github.com/LinByte/VoiceServer/pkg/media/encoder"
	"github.com/LinByte/VoiceServer/pkg/synthesizer"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	siptts "github.com/LinByte/VoiceServer/pkg/voice/tts"
	"go.uber.org/zap"
)

const webSeatAgentBriefSampleRate = 8000
const webSeatAgentBriefPCMFrameBytes = webSeatAgentBriefSampleRate * 20 / 1000 * 2 // 20ms mono s16le @ 8kHz

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

// PlayTransferAgentBriefForWebSeat plays the trunk template to the browser downlink before PSTN bridge.
// Caller keeps transfer ringing until StopTransferRingingForCall (hub invokes that after this returns).
func PlayTransferAgentBriefForWebSeat(ctx context.Context, inboundCallID string, agentDownlink media.MediaTransport, lg *zap.Logger) (bool, error) {
	inboundCallID = normCallID(inboundCallID)
	tmpl := resolveTransferAgentBriefTemplate(inboundCallID)
	if tmpl == "" || agentDownlink == nil {
		return false, nil
	}
	if _, loaded := transferAgentBriefRunning.LoadOrStore(inboundCallID, struct{}{}); loaded {
		return false, nil
	}
	defer transferAgentBriefRunning.Delete(inboundCallID)

	rendered := RenderTransferAgentBriefTemplate(tmpl, buildTransferAgentBriefVars(inboundCallID))
	if rendered == "" {
		return false, nil
	}
	if lookupInbound == nil {
		return false, nil
	}
	inbound := lookupInbound(inboundCallID)
	if inbound == nil {
		return false, nil
	}
	if lg == nil && logger.Lg != nil {
		lg = logger.Lg
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	txCodec := webSeatDownlinkCodec(agentDownlink)
	lg.Info("sip transfer: playing agent brief before webseat bridge (caller keeps hold music)",
		zap.String("inbound_call_id", inboundCallID),
		zap.String("text", rendered),
	)
	if ctx == nil {
		ms := inbound.MediaSession()
		if ms != nil {
			ctx = ms.GetContext()
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	playCtx, cancel := context.WithTimeout(ctx, transferAgentBriefMaxPlay)
	defer cancel()
	err := speakTextOnMediaTransport(playCtx, agentDownlink, inbound, txCodec, rendered, lg)
	return true, err
}

func webSeatDownlinkCodec(agentDownlink media.MediaTransport) media.CodecConfig {
	type txCodecProvider interface {
		TxCodec() media.CodecConfig
	}
	if p, ok := agentDownlink.(txCodecProvider); ok {
		if c := p.TxCodec(); strings.TrimSpace(c.Codec) != "" {
			return c
		}
	}
	return media.CodecConfig{Codec: "pcma", SampleRate: webSeatAgentBriefSampleRate, Channels: 1, BitDepth: 16, FrameDuration: "20ms"}
}

func speakTextOnMediaTransport(ctx context.Context, agentDownlink media.MediaTransport, tenantCS *sipSession.CallSession, txCodec media.CodecConfig, text string, lg *zap.Logger) error {
	if agentDownlink == nil || tenantCS == nil {
		return fmt.Errorf("sip conversation: nil transport or tenant session")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	env, loaded, err := ResolveTenantVoiceEnv(ctx, tenantCS)
	if err != nil {
		return err
	}
	if !loaded || env.TTSConfigRaw == nil {
		return fmt.Errorf("sip conversation: missing tenant TTS config")
	}
	ttsHandle, err := synthesizer.NewStreamingFromCredential(env.TTSConfigRaw)
	if err != nil {
		return fmt.Errorf("sip conversation: tts provider: %w", err)
	}
	targetSR := webSeatAgentBriefSampleRate
	if txCodec.SampleRate > 0 {
		targetSR = txCodec.SampleRate
	}
	ttsCloudSR := sipVoiceTTSCloudSampleRate(env, ttsHandle.SampleRate, targetSR)

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
			if ttsCloudSR != targetSR && len(frame) >= 2 {
				if out, err := media.ResamplePCM(frame, ttsCloudSR, targetSR); err == nil && len(out) > 0 {
					pcmOut = out
				}
			}
			return sendPCMToMediaTransport(ctx, agentDownlink, txCodec, pcmOut)
		},
		Logger: lg,
	})
	if err != nil {
		return fmt.Errorf("sip conversation: tts pipeline: %w", err)
	}
	ttsPipe.Start(ctx)
	defer ttsPipe.Stop()
	if err := ttsPipe.Speak(text); err != nil {
		return err
	}
	return ttsPipe.Finalize()
}

func sendPCMToMediaTransport(ctx context.Context, transport media.MediaTransport, txCodec media.CodecConfig, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	frameBytes := webSeatAgentBriefPCMFrameBytes
	if txCodec.SampleRate > 0 && txCodec.SampleRate != webSeatAgentBriefSampleRate {
		frameBytes = txCodec.SampleRate * 20 / 1000 * 2
	}
	for off := 0; off < len(pcm); off += frameBytes {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		end := off + frameBytes
		chunk := pcm[off:]
		if end < len(pcm) {
			chunk = pcm[off:end]
		}
		if len(chunk) < frameBytes {
			padded := make([]byte, frameBytes)
			copy(padded, chunk)
			chunk = padded
		}
		enc, err := encodePCMForWebSeatDownlink(chunk, txCodec)
		if err != nil {
			return err
		}
		if _, err := transport.Send(ctx, &media.AudioPacket{Payload: enc, IsSynthesized: true}); err != nil {
			return err
		}
	}
	return nil
}

func encodePCMForWebSeatDownlink(pcm []byte, txCodec media.CodecConfig) ([]byte, error) {
	_ = txCodec
	return encoder.EncodePCMA(pcm)
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
