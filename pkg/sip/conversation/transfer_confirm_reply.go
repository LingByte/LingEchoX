package conversation

import (
	"context"

	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

func transferConfirmReplyText(execute bool) string {
	if execute {
		return transferConfirmExecuteReplyZH
	}
	return transferConfirmNormalReplyZH
}

// PlayTransferConfirmReply speaks the fixed transfer-confirm phrase via tenant TTS.
// execute=false → first (required-1) intents; execute=true → final phrase before dial.
func PlayTransferConfirmReply(ctx context.Context, cs *sipSession.CallSession, execute bool, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	return SpeakTextOnce(ctx, cs, transferConfirmReplyText(execute), lg)
}
