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
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"github.com/LinByte/VoiceServer/pkg/synthesizer"
	siptts "github.com/LinByte/VoiceServer/pkg/voice/tts"
	"go.uber.org/zap"
)

const webSeatAgentBriefSampleRate = 8000
const webSeatAgentBriefPCMFrameBytes = webSeatAgentBriefSampleRate * 20 / 1000 * 2 // 20ms mono s16le @ 8kHz

const transferAgentBriefMaxPlay = 45 * time.Second

var transferAgentBriefRunning sync.Map // inbound Call-ID -> struct{}

// briefPCMSink receives resampled PCM frames from a single TTS pipeline.
type briefPCMSink interface {
	sendPCM(ctx context.Context, pcm []byte, srcSampleRate int) error
}

type callSessionBriefSink struct {
	cs *sipSession.CallSession
}

func (s *callSessionBriefSink) sendPCM(_ context.Context, pcm []byte, srcSampleRate int) error {
	if s == nil || s.cs == nil {
		return fmt.Errorf("sip conversation: nil call session brief sink")
	}
	ms := s.cs.MediaSession()
	if ms == nil {
		return fmt.Errorf("sip conversation: media session not ready")
	}
	targetSR := sipVoicePCMBridgeRate(s.cs)
	pcmOut := pcm
	if srcSampleRate != targetSR && len(pcm) >= 2 {
		if out, err := media.ResamplePCM(pcm, srcSampleRate, targetSR); err == nil && len(out) > 0 {
			pcmOut = out
		}
	}
	ms.SendToOutput("sip-transfer-brief", &media.AudioPacket{
		Payload:       pcmOut,
		IsSynthesized: true,
	})
	return nil
}

type mediaTransportBriefSink struct {
	transport media.MediaTransport
	txCodec   media.CodecConfig
}

func (s *mediaTransportBriefSink) sendPCM(ctx context.Context, pcm []byte, srcSampleRate int) error {
	if s == nil || s.transport == nil {
		return fmt.Errorf("sip conversation: nil media transport brief sink")
	}
	targetSR := webSeatAgentBriefSampleRate
	if s.txCodec.SampleRate > 0 {
		targetSR = s.txCodec.SampleRate
	}
	pcmOut := pcm
	if srcSampleRate != targetSR && len(pcm) >= 2 {
		if out, err := media.ResamplePCM(pcm, srcSampleRate, targetSR); err == nil && len(out) > 0 {
			pcmOut = out
		}
	}
	return sendPCMToMediaTransport(ctx, s.transport, s.txCodec, pcmOut)
}

// playTransferAgentBrief synthesizes text once and fans PCM out to every sink (caller + agent legs).
func playTransferAgentBrief(ctx context.Context, tenantCS *sipSession.CallSession, text string, lg *zap.Logger, sinks ...briefPCMSink) error {
	if tenantCS == nil {
		return fmt.Errorf("sip conversation: nil tenant session for transfer brief")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	sinks = filterBriefPCMSinks(sinks)
	if len(sinks) == 0 {
		return nil
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
	bridgeSR := sipVoicePCMBridgeRate(tenantCS)
	ttsCloudSR := sipVoiceTTSCloudSampleRate(env, ttsHandle.SampleRate, bridgeSR)

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
			for _, sink := range sinks {
				if err := sink.sendPCM(ctx, frame, ttsCloudSR); err != nil {
					return err
				}
			}
			return nil
		},
		Logger: lg,
	})
	if err != nil {
		return fmt.Errorf("sip conversation: tts pipeline: %w", err)
	}
	runCtx := ctx
	if runCtx == nil {
		if ms := tenantCS.MediaSession(); ms != nil {
			runCtx = ms.GetContext()
		}
	}
	ttsPipe.Start(runCtx)
	defer ttsPipe.Stop()
	if err := ttsPipe.Speak(text); err != nil {
		return err
	}
	return ttsPipe.Finalize()
}

func filterBriefPCMSinks(sinks []briefPCMSink) []briefPCMSink {
	out := make([]briefPCMSink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

// playTransferBriefPair plays caller/agent brief text simultaneously.
// Same text → one TTS pipeline fanned to all sinks; different text → parallel pipelines.
func playTransferBriefPair(
	ctx context.Context,
	tenantCS *sipSession.CallSession,
	callerText, agentText string,
	lg *zap.Logger,
	callerSinks, agentSinks []briefPCMSink,
) error {
	callerText = strings.TrimSpace(callerText)
	agentText = strings.TrimSpace(agentText)
	callerSinks = filterBriefPCMSinks(callerSinks)
	agentSinks = filterBriefPCMSinks(agentSinks)
	if callerText == "" && agentText == "" {
		return nil
	}
	if callerText != "" && agentText != "" && callerText == agentText {
		all := append(append([]briefPCMSink{}, callerSinks...), agentSinks...)
		return playTransferAgentBrief(ctx, tenantCS, callerText, lg, all...)
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	if callerText != "" && len(callerSinks) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordErr(playTransferAgentBrief(ctx, tenantCS, callerText, lg, callerSinks...))
		}()
	}
	if agentText != "" && len(agentSinks) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordErr(playTransferAgentBrief(ctx, tenantCS, agentText, lg, agentSinks...))
		}()
	}
	wg.Wait()
	return firstErr
}

// SpeakTextOnLeg synthesizes text to playbackCS using tenant TTS creds from tenantCS.
func SpeakTextOnLeg(ctx context.Context, playbackCS, tenantCS *sipSession.CallSession, text string, lg *zap.Logger) error {
	if playbackCS == nil || tenantCS == nil {
		return fmt.Errorf("sip conversation: nil session for leg TTS")
	}
	return playTransferAgentBrief(ctx, tenantCS, text, lg, &callSessionBriefSink{cs: playbackCS})
}

// PlayTransferAgentBriefForWebSeat plays trunk brief templates to caller and browser downlink before bridge.
func PlayTransferAgentBriefForWebSeat(ctx context.Context, inboundCallID string, agentDownlink media.MediaTransport, lg *zap.Logger) (bool, error) {
	inboundCallID = normCallID(inboundCallID)
	if !hasTransferBriefConfigured(inboundCallID) || agentDownlink == nil {
		return false, nil
	}
	if _, loaded := transferAgentBriefRunning.LoadOrStore(inboundCallID, struct{}{}); loaded {
		return false, nil
	}
	defer transferAgentBriefRunning.Delete(inboundCallID)

	callerText, agentText, ok := resolveTransferBriefForCall(inboundCallID)
	if !ok {
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
	stopTransferRinging(inboundCallID)
	lg.Info("sip transfer: playing brief to caller and webseat before bridge",
		zap.String("inbound_call_id", inboundCallID),
		zap.String("caller_text", callerText),
		zap.String("agent_text", agentText),
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
	callerSinks := []briefPCMSink{&callSessionBriefSink{cs: inbound}}
	agentSinks := []briefPCMSink{&mediaTransportBriefSink{transport: agentDownlink, txCodec: txCodec}}
	err := playTransferBriefPair(playCtx, inbound, callerText, agentText, lg, callerSinks, agentSinks)
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
	callerText, agentText, ok := resolveTransferBriefForCall(inboundCallID)
	if !ok {
		startTransferBridgeNow(inboundCallID, outboundCS, outboundCallID, lg)
		return
	}
	stopTransferRinging(inboundCallID)
	lg.Info("sip transfer: playing brief to caller and agent before bridge",
		zap.String("inbound_call_id", inboundCallID),
		zap.String("outbound_call_id", outboundCallID),
		zap.String("caller_text", callerText),
		zap.String("agent_text", agentText),
	)
	outboundCS.StartOnACK()
	playCtx, cancel := context.WithTimeout(ms.GetContext(), transferAgentBriefMaxPlay)
	defer cancel()
	callerSinks := []briefPCMSink{&callSessionBriefSink{cs: inbound}}
	agentSinks := []briefPCMSink{&callSessionBriefSink{cs: outboundCS}}
	if err := playTransferBriefPair(playCtx, inbound, callerText, agentText, lg, callerSinks, agentSinks); err != nil {
		lg.Warn("sip transfer: brief TTS failed, bridging anyway",
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
