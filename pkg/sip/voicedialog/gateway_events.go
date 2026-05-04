package voicedialog

import (
	"strings"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"go.uber.org/zap"
)

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func (sess *dialogSession) emitGateway(ev map[string]any) {
	if sess == nil || ev == nil {
		return
	}
	callID := sess.meta.CallID
	t, _ := ev[KeyType].(string)
	switch t {
	case EvASRPartial, EvASRFinal:
		txt, _ := ev[KeyText].(string)
		logger.Info("voicedialog → ws asr",
			zap.String(KeyCallID, callID),
			zap.String("event", t),
			zap.Bool(KeyFinal, t == EvASRFinal),
			zap.Int("text_len", len([]rune(txt))),
			zap.String("text_preview", truncateRunes(txt, 100)),
		)
	case EvASRError:
		msg, _ := ev[KeyMessage].(string)
		fatal, _ := ev[KeyFatal].(bool)
		logger.Warn("voicedialog → ws asr.error",
			zap.String(KeyCallID, callID),
			zap.String(KeyMessage, msg),
			zap.Bool(KeyFatal, fatal),
		)
	case EvDTMF:
		d, _ := ev[KeyDigit].(string)
		logger.Info("voicedialog → ws dtmf",
			zap.String(KeyCallID, callID),
			zap.String(KeyDigit, d),
		)
	case EvInterrupt:
		origin, _ := ev[KeyOrigin].(string)
		cause, _ := ev[KeyCause].(string)
		reason, _ := ev[KeyReason].(string)
		logger.Info("voicedialog → ws interrupt",
			zap.String(KeyCallID, callID),
			zap.String(KeyOrigin, origin),
			zap.String(KeyCause, cause),
			zap.String(KeyReason, reason),
		)
	case EvTTSStarted:
		prev, _ := ev[KeyTextPreview].(string)
		uid, _ := ev[KeyUtteranceID].(string)
		logger.Info("voicedialog → ws tts.started",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, uid),
			zap.String(KeyTextPreview, prev),
		)
	case EvTTSEnded:
		uid, _ := ev[KeyUtteranceID].(string)
		ok, _ := ev[KeyOK].(bool)
		logger.Info("voicedialog → ws tts.ended",
			zap.String(KeyCallID, callID),
			zap.String(KeyUtteranceID, uid),
			zap.Bool(KeyOK, ok),
		)
	case EvTTSCancelled:
		logger.Info("voicedialog → ws tts.cancelled", zap.String(KeyCallID, callID))
	case EvDialogWelcome, EvDialogTransfer:
		ph, _ := ev[KeyPhase].(string)
		sk, _ := ev[KeySourceKind].(string)
		src, _ := ev[KeySource].(string)
		logger.Info("voicedialog → ws "+t,
			zap.String(KeyCallID, callID),
			zap.String(KeyPhase, ph),
			zap.String(KeySourceKind, sk),
			zap.String(KeySource, src),
		)
	default:
		if strings.HasPrefix(t, "call.") || t == EvHello || t == EvError {
			logger.Debug("voicedialog → ws",
				zap.String(KeyCallID, callID),
				zap.String(KeyType, t),
			)
		} else {
			logger.Info("voicedialog → ws",
				zap.String(KeyCallID, callID),
				zap.String(KeyType, t),
			)
		}
	}

	sess.mu.Lock()
	c := sess.conn
	sess.mu.Unlock()
	if c == nil {
		return
	}
	_ = writeJSONDeadline(c, ev)
}
