package persist

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/scriptlisten"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	sipMetrics "github.com/LinByte/VoiceServer/pkg/sip/metrics"
	sipServer "github.com/LinByte/VoiceServer/pkg/sip/server"
	"github.com/LinByte/VoiceServer/pkg/stores"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CallStore persists SIPCall (including AI dialog JSON in `turns`) and uploads call recordings via pkg/stores.
type CallStore struct {
	db *gorm.DB
	lg *zap.Logger
}

func NewCallStore(db *gorm.DB, lg *zap.Logger) *CallStore {
	if lg == nil {
		lg = zap.NewNop()
	}
	return &CallStore{db: db, lg: lg}
}

// OnInvite upserts SIPCall in ringing state.
func (s *CallStore) OnInvite(ctx context.Context, p sipServer.InvitePersistParams) {
	if s == nil || s.db == nil || p.CallID == "" {
		return
	}
	now := time.Now()
	dir := p.Direction
	if dir == "" {
		dir = DirectionInbound
	}
	row, err := FindSIPCallByCallID(ctx, s.db, p.CallID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = NewSIPCallRinging(
			p.CallID, p.From, p.To, p.CSeqInvite, p.RemoteSig, dir,
			p.RemoteRTP, p.LocalRTP, p.PayloadType, p.Codec, p.ClockRate, now,
			p.TenantID, p.InboundTrunkNumberID,
		)
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			s.lg.Warn("sippersist invite create", zap.String("call_id", p.CallID), zap.Error(err))
		}
		return
	}
	if err != nil {
		s.lg.Warn("sippersist invite lookup", zap.String("call_id", p.CallID), zap.Error(err))
		return
	}
	_ = s.db.WithContext(ctx).Model(&row).Updates(SIPCallInviteRefreshUpdateMap(
		p.From, p.To, p.RemoteSig, p.RemoteRTP, p.LocalRTP, p.Codec, p.PayloadType, p.ClockRate, p.TenantID, p.InboundTrunkNumberID, now,
	)).Error
}

// OnEstablished marks call established (ACK / media start).
func (s *CallStore) OnEstablished(ctx context.Context, callID string) {
	if s == nil || s.db == nil || callID == "" {
		return
	}
	now := time.Now()
	_ = s.db.WithContext(ctx).Model(&SIPCall{}).Where("call_id = ?", callID).Updates(SIPCallEstablishedUpdateMap(now)).Error
}

// OnBye finalizes SIPCall, optionally uploads SN3/SN2 recording as stereo WAV (L=user R=AI per-leg decode),
// falling back to legacy mono mix, via pkg/stores.Default().
func (s *CallStore) OnBye(ctx context.Context, p sipServer.ByePersistParams) {
	if s == nil || s.db == nil || p.CallID == "" {
		return
	}
	callID := p.CallID
	raw := p.RawPayload
	codecName := p.CodecName
	initiator := p.Initiator
	if initiator == "" {
		initiator = "remote"
	}

	sipAgent, webSeat := conversation.TakeInboundTransferFlags(callID)
	transferTargetID := conversation.TakeInboundTransferACDTargetID(callID)
	endStatus := SIPCallEndStatusForBye(initiator, sipAgent, webSeat)

	now := time.Now()
	var call SIPCall
	if err := s.db.WithContext(ctx).Where("call_id = ?", callID).First(&call).Error; err != nil {
		s.lg.Warn("sippersist bye lookup", zap.String("call_id", callID), zap.Error(err))
	}
	updates := SIPCallByeFinalizeUpdateMap(now, endStatus, sipAgent, webSeat, 0)
	recordingSec := 0
	if transferTargetID > 0 {
		updates["transfer_acd_target_id"] = transferTargetID
	}
	if bi := strings.ToLower(strings.TrimSpace(initiator)); bi != "" {
		updates["bye_initiator"] = bi
	}
	clockRate := p.RecordSampleRate
	if clockRate <= 0 && call.ClockRate > 0 {
		clockRate = call.ClockRate
	}
	if qosUp := QoSDBUpdatesFromRTCP(codecName, clockRate, p.RTCP); len(qosUp) > 0 {
		for k, v := range qosUp {
			updates[k] = v
		}
		if rtt, ok := qosUp["qos_rtt_ms"].(uint32); ok && rtt > 0 {
			jitterMs := float64(0)
			if j, ok := qosUp["qos_jitter_ms"].(float32); ok {
				jitterMs = float64(j)
			}
			lossPct := float64(0)
			if l, ok := qosUp["qos_packet_loss_pct"].(float32); ok {
				lossPct = float64(l) / 100.0
			}
			mos := float64(0)
			if m, ok := qosUp["qos_mos_estimate"].(float32); ok {
				mos = float64(m)
			}
			sipMetrics.ObserveCallQoS(rtt, jitterMs, lossPct, mos)
		}
	}
	if len(raw) > 0 {
		updates["recording_raw_bytes"] = len(raw)
	}

	// New stereo PCM recorder fast-path — when pkg/voice/recorder produced
	// and uploaded a stereo WAV during the call, we just record the
	// resulting URL/Bytes and skip the SN3 → WAV decode below entirely.
	// SN3 RawPayload remains stored as recording_raw_bytes for now in case
	// dashboards still want the per-frame timeline; it is not re-uploaded.
	if wav := p.WAVRecording; wav.URL != "" || wav.Key != "" {
		if wav.URL != "" {
			updates["recording_url"] = wav.URL
		}
		if wav.Bytes > 0 {
			updates["recording_wav_bytes"] = wav.Bytes
		}
		if wav.DurationMs > 0 {
			if sec := int((wav.DurationMs + 500) / 1000); sec > 0 {
				recordingSec = sec
			}
		}
		if wav.Hash != "" {
			updates["recording_hash"] = wav.Hash
		}
		ApplySIPCallDurationFromRecording(updates, call, recordingSec, now)
		s.lg.Info("sippersist recording attached from voice/recorder",
			zap.String("call_id", callID),
			zap.String("url", wav.URL),
			zap.Int("wav_bytes", wav.Bytes),
			zap.Int64("duration_ms", wav.DurationMs),
			zap.String("hash", wav.Hash),
		)
		if err := s.db.WithContext(ctx).Model(&SIPCall{}).Where("call_id = ?", callID).Updates(updates).Error; err != nil {
			s.lg.Warn("sippersist bye update", zap.String("call_id", callID), zap.Error(err))
		}
		return
	}

	c := strings.ToLower(codecName)
	store := stores.Default()
	var wav []byte
	if len(raw) > 0 && store != nil {
		switch {
		case strings.Contains(c, "pcmu") || strings.Contains(c, "pcma"):
			wav = utils.G711TaggedRecordingToStereoWav(raw, codecName)
			if len(wav) == 0 {
				wav = utils.G711TaggedRecordingToWav(raw, codecName)
			}
		case strings.Contains(c, "g722"):
			wav = utils.G722TaggedRecordingToStereoWav(raw)
			if len(wav) == 0 {
				wav = utils.G722TaggedRecordingToWav(raw)
			}
		case strings.Contains(c, "opus"):
			sr := p.RecordSampleRate
			if sr <= 0 {
				sr = 48000
			}
			ch := p.RecordOpusChannels
			if ch < 1 {
				ch = 1
			}
			if ch > 2 {
				ch = 2
			}
			wav = utils.MixedOpusRecordingToStereoWav(raw, sr, ch)
			if len(wav) == 0 {
				wav = utils.MixedOpusRecordingToWav(raw, sr, ch)
			}
			if len(wav) == 0 && ch == 1 {
				wav = utils.MixedOpusRecordingToStereoWav(raw, sr, 2)
			}
			if len(wav) == 0 && ch == 1 {
				wav = utils.MixedOpusRecordingToWav(raw, sr, 2)
			}
			if len(wav) == 0 {
				s.lg.Warn("sippersist opus recording decode failed or empty",
					zap.String("call_id", callID),
					zap.Int("raw_bytes", len(raw)),
					zap.Int("opus_ch", ch),
					zap.Int("sample_rate", sr),
				)
			}
		default:
			s.lg.Warn("sippersist recording skipped (codec not supported for WAV upload)",
				zap.String("call_id", callID), zap.String("codec", codecName), zap.Int("raw_bytes", len(raw)))
		}
		if len(wav) > 0 {
			if wavSec := WAVDurationSec(wav); wavSec > 0 {
				recordingSec = wavSec
			}
			key := fmt.Sprintf("sip/recordings/%s_%d.wav", sanitizeRecordingKey(callID), now.Unix())
			if err := store.Write(key, bytes.NewReader(wav)); err != nil {
				s.lg.Warn("sippersist recording upload", zap.String("call_id", callID), zap.Error(err))
			} else if pub := strings.TrimSpace(stores.PublicObjectURL(store, key)); pub != "" {
				updates["recording_url"] = pub
				updates["recording_wav_bytes"] = len(wav)
				s.lg.Info("sippersist recording uploaded", zap.String("call_id", callID), zap.String("codec", codecName))
			}
		} else if len(raw) >= 3 && raw[0] == 'S' && raw[1] == 'N' && (raw[2] == '3' || raw[2] == '2' || raw[2] == '1') {
			snKey := fmt.Sprintf("sip/recordings/%s_%d.sn2", sanitizeRecordingKey(callID), now.Unix())
			if err := store.Write(snKey, bytes.NewReader(raw)); err != nil {
				s.lg.Warn("sippersist raw recording upload", zap.String("call_id", callID), zap.Error(err))
			} else if pub := strings.TrimSpace(stores.PublicObjectURL(store, snKey)); pub != "" {
				updates["recording_url"] = pub
				s.lg.Info("sippersist raw SN recording uploaded (no WAV)", zap.String("call_id", callID), zap.String("codec", codecName), zap.Int("raw_bytes", len(raw)))
			}
		} else if len(raw) > 0 {
			s.lg.Warn("sippersist recording not converted to WAV and not SN1/SN2/SN3 tagged",
				zap.String("call_id", callID), zap.String("codec", codecName), zap.Int("raw_bytes", len(raw)))
		}
	} else if len(raw) > 0 && store == nil {
		s.lg.Warn("sippersist recording not uploaded (storage backend unavailable)",
			zap.String("call_id", callID), zap.String("codec", codecName))
	}

	ApplySIPCallDurationFromRecording(updates, call, recordingSec, now)
	if err := s.db.WithContext(ctx).Model(&SIPCall{}).Where("call_id = ?", callID).Updates(updates).Error; err != nil {
		s.lg.Warn("sippersist bye update", zap.String("call_id", callID), zap.Error(err))
	}

}

// SaveConversationTurn appends one ASR→LLM turn onto sip_calls.turns for callID (creates a minimal call row if missing).
func (s *CallStore) SaveConversationTurn(ctx context.Context, callID string, t conversation.DialogTurn) {
	if s == nil || s.db == nil || callID == "" {
		return
	}
	userText := strings.TrimSpace(t.ASRText)
	assistantText := strings.TrimSpace(t.LLMText)
	if userText == "" && assistantText == "" {
		return
	}
	now := time.Now()
	if !t.At.IsZero() {
		now = t.At
	}
	turn := SIPCallDialogTurn{
		ASRText:      userText,
		LLMText:      assistantText,
		ASRProvider:  t.ASRProvider,
		TTSProvider:  t.TTSProvider,
		LLMModel:     t.LLMModel,
		At:           now,
		Trigger:      t.Trigger,
		ScriptStepID: t.ScriptStepID,
		RouteIntent:  t.RouteIntent,
		LLMFirstMs:   t.LLMFirstMs,
		LLMWallMs:    t.LLMWallMs,
		TTSMs:        t.TTSMs,
		PipelineMs:   t.PipelineMs,
		TurnGroupID:  t.TurnGroupID,
	}

	row, err := FindActiveSIPCallByCallID(ctx, s.db, callID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		turnsBytes, jErr := MarshalSIPCallTurns([]SIPCallDialogTurn{turn})
		if jErr != nil {
			s.lg.Warn("sippersist call turns marshal failed", zap.String("call_id", callID), zap.Error(jErr))
			return
		}
		row = NewSIPCallMinimalEstablishedWithFirstTurn(callID, turnsBytes, now)
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			row, err = FindActiveSIPCallByCallID(ctx, s.db, callID)
			if err != nil {
				s.lg.Warn("sippersist call create/first for turn", zap.String("call_id", callID), zap.Error(err))
				return
			}
		} else {
			if s.lg != nil {
				s.lg.Info("sippersist call created for first AI turn", zap.String("call_id", callID), zap.Uint("row_id", row.ID))
			}
			scriptlisten.Notify(callID)
			return
		}
	} else if err != nil {
		s.lg.Warn("sippersist call load for turn", zap.String("call_id", callID), zap.Error(err))
		return
	}

	upd, turnCount, uErr := SIPCallAppendTurnUpdateMap(row, turn, now)
	if uErr != nil {
		s.lg.Warn("sippersist call turns merge failed", zap.String("call_id", callID), zap.Error(uErr))
		return
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(upd).Error; err != nil {
		s.lg.Warn("sippersist call turn update failed", zap.String("call_id", callID), zap.Error(err))
		return
	}
	scriptlisten.Notify(callID)
	if s.lg != nil {
		s.lg.Info("sippersist call turn appended",
			zap.String("call_id", callID),
			zap.Int("turn_count", turnCount),
		)
	}
}

func sanitizeRecordingKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "call"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 120 {
		return out[:120]
	}
	return out
}

// WAVDurationSec parses a minimal PCM WAV header and returns rounded seconds.
// Used to align sip_calls.duration_sec with the actual uploaded audio.
func WAVDurationSec(wav []byte) int {
	if len(wav) < 44 {
		return 0
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return 0
	}
	var byteRate uint32
	var dataSize uint32
	i := 12
	for i+8 <= len(wav) {
		chunkID := string(wav[i : i+4])
		chunkSize := int(binary.LittleEndian.Uint32(wav[i+4 : i+8]))
		if chunkSize < 0 {
			return 0
		}
		payloadStart := i + 8
		payloadEnd := payloadStart + chunkSize
		if payloadEnd > len(wav) {
			break
		}
		switch chunkID {
		case "fmt ":
			if chunkSize >= 16 {
				byteRate = binary.LittleEndian.Uint32(wav[payloadStart+8 : payloadStart+12])
			}
		case "data":
			dataSize = uint32(chunkSize)
		}
		advance := 8 + chunkSize
		if chunkSize%2 == 1 {
			advance++
		}
		i += advance
	}
	if byteRate == 0 || dataSize == 0 {
		return 0
	}
	sec := int((float64(dataSize) / float64(byteRate)) + 0.5) // round to nearest second
	if sec < 0 {
		return 0
	}
	return sec
}
