package voicedialog

import (
	"context"

	sipSession "github.com/LingByte/SoulNexus/pkg/sip/session"
)

// AttachInboundVoiceDialog registers the voicedialog gateway on this inbound leg: SIP does RTP/PCM,
// ASR/TTS on the gateway; dialogue uses HTTP WebSocket (loopback and/or external clients).
// Call from SIP ACK handling (pkg/sip/server). There is no embedded AttachVoicePipeline on inbound.
func AttachInboundVoiceDialog(ctx context.Context, cs *sipSession.CallSession, from, to, remote string) error {
	if cs == nil {
		return nil
	}
	meta := InboundMeta{
		CallID:        cs.CallID,
		FromHeader:    from,
		ToHeader:      to,
		RemoteSig:     remote,
		CodecName:     cs.NegotiatedCodec().Name,
		PCMSampleRate: cs.PCMSampleRate(),
	}
	return cs.AttachVoiceConversation(func() error {
		return AttachInbound(ctx, cs, meta)
	})
}
