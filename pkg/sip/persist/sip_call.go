package persist

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

const (
	SIPCallStateInit        = "init"
	SIPCallStateRinging     = "ringing"
	SIPCallStateEstablished = "established"
	SIPCallStateEnded       = "ended"
	SIPCallStateFailed      = "failed"
)

const (
	SIPCallEndUnknown             = "unknown"
	SIPCallEndDeclined            = "declined"
	SIPCallEndBusy                = "busy"
	SIPCallEndCancelled           = "cancelled"
	SIPCallEndRequestTimeout      = "request_timeout"
	SIPCallEndNormalClearing      = "normal_clearing"
	SIPCallEndTransportError      = "transport_error"
	SIPCallEndServerError         = "server_error"
	SIPCallEndCompletedRemote     = "completed_remote"
	SIPCallEndCompletedLocal      = "completed_local"
	SIPCallEndAfterTransferRemote = "after_transfer_remote"
	SIPCallEndAfterTransferLocal  = "after_transfer_local"
)

const (
	ByeInitiatorLocal  = "local"
	ByeInitiatorRemote = "remote"
)

// SIPCallDialogTurn is one ASR→LLM exchange stored in SIPCall.Turns (JSON array).
type SIPCallDialogTurn struct {
	ASRText      string    `json:"asrText"`
	LLMText      string    `json:"llmText"`
	ASRProvider  string    `json:"asrProvider,omitempty"`
	TTSProvider  string    `json:"ttsProvider,omitempty"`
	LLMModel     string    `json:"llmModel,omitempty"`
	At           time.Time `json:"at"`
	Trigger      string    `json:"trigger,omitempty"`
	ScriptStepID string    `json:"scriptStepId,omitempty"`
	RouteIntent  string    `json:"routeIntent,omitempty"`
	LLMFirstMs   int       `json:"llmFirstMs,omitempty"`
	LLMWallMs    int       `json:"llmWallMs,omitempty"`
	TTSMs        int       `json:"ttsMs,omitempty"`
	PipelineMs   int       `json:"pipelineMs,omitempty"`
}

// SIPCall records one SIP call lifecycle (sip_calls).
type SIPCall struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime;comment:Creation time"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime;comment:Update time"`
	IsDeleted int8      `json:"isDeleted,omitempty" gorm:"default:0;index;comment:Soft delete flag (0:not deleted, 1:deleted)"`
	CreateBy  string    `json:"createBy,omitempty" gorm:"size:128;index;comment:Creator"`
	UpdateBy  string    `json:"updateBy,omitempty" gorm:"size:128;index;comment:Updater"`

	CallID            string         `json:"callId" gorm:"size:128;uniqueIndex;not null"`
	FromHeader        string         `json:"fromHeader" gorm:"type:text"`
	ToHeader          string         `json:"toHeader" gorm:"type:text"`
	CSeqInvite        string         `json:"cseqInvite" gorm:"size:64"`
	RemoteAddr        string         `json:"remoteAddr" gorm:"size:128;index"`
	Direction         string         `json:"direction" gorm:"size:16;index"`
	RemoteRTPAddr     string         `json:"remoteRtpAddr" gorm:"size:128;index"`
	LocalRTPAddr      string         `json:"localRtpAddr" gorm:"size:128;index"`
	PayloadType       uint8          `json:"payloadType" gorm:"index"`
	Codec             string         `json:"codec" gorm:"size:32;index"`
	ClockRate         int            `json:"clockRate"`
	State             string         `json:"state" gorm:"size:32;index"`
	InviteAt          *time.Time     `json:"inviteAt" gorm:"index"`
	AckAt             *time.Time     `json:"ackAt" gorm:"index"`
	ByeAt             *time.Time     `json:"byeAt" gorm:"index"`
	EndedAt           *time.Time     `json:"endedAt" gorm:"index"`
	FailureReason     string         `json:"failureReason" gorm:"type:text"`
	RecordingURL      string         `json:"recordingUrl" gorm:"size:1024"`
	RecordingRawBytes int            `json:"recordingRawBytes" gorm:"column:recording_raw_bytes;default:0"`
	RecordingWavBytes int            `json:"recordingWavBytes" gorm:"column:recording_wav_bytes;default:0"`
	ByeInitiator      string         `json:"byeInitiator" gorm:"column:bye_initiator;size:16"`
	DurationSec       int            `json:"durationSec" gorm:"default:0"`
	EndStatus         string         `json:"endStatus" gorm:"size:64;index"`
	Turns             datatypes.JSON `json:"turns" gorm:"type:json"`
	TurnCount         int            `json:"turnCount" gorm:"default:0"`
	FirstTurnAt       *time.Time     `json:"firstTurnAt"`
	LastTurnAt        *time.Time     `json:"lastTurnAt"`
	HadSIPTransfer    bool           `json:"hadSipTransfer" gorm:"column:had_sip_transfer;default:0"`
	HadWebSeat        bool           `json:"hadWebSeat" gorm:"column:had_web_seat;default:0"`
}

func (SIPCall) TableName() string { return constants.SIP_CALL_TABLE_NAME }

func (s *SIPCall) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	if s.IsDeleted == 0 {
		s.IsDeleted = SoftDeleteStatusActive
	}
	return nil
}

func (s *SIPCall) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = time.Now()
	return nil
}

// MergeSIPCall applies non-empty fields from patch onto dst (JSON/GORM store updates).
func MergeSIPCall(dst, patch *SIPCall) {
	if dst == nil || patch == nil {
		return
	}
	if patch.FromHeader != "" {
		dst.FromHeader = patch.FromHeader
	}
	if patch.ToHeader != "" {
		dst.ToHeader = patch.ToHeader
	}
	if patch.CSeqInvite != "" {
		dst.CSeqInvite = patch.CSeqInvite
	}
	if patch.RemoteAddr != "" {
		dst.RemoteAddr = patch.RemoteAddr
	}
	if patch.Direction != "" {
		dst.Direction = patch.Direction
	}
	if patch.RemoteRTPAddr != "" {
		dst.RemoteRTPAddr = patch.RemoteRTPAddr
	}
	if patch.LocalRTPAddr != "" {
		dst.LocalRTPAddr = patch.LocalRTPAddr
	}
	if patch.Codec != "" {
		dst.Codec = patch.Codec
	}
	if patch.PayloadType != 0 {
		dst.PayloadType = patch.PayloadType
	}
	if patch.ClockRate != 0 {
		dst.ClockRate = patch.ClockRate
	}
	if patch.State != "" {
		dst.State = patch.State
	}
	if patch.FailureReason != "" {
		dst.FailureReason = patch.FailureReason
	}
	if patch.RecordingURL != "" {
		dst.RecordingURL = patch.RecordingURL
	}
	if patch.RecordingRawBytes != 0 {
		dst.RecordingRawBytes = patch.RecordingRawBytes
	}
	if patch.RecordingWavBytes != 0 {
		dst.RecordingWavBytes = patch.RecordingWavBytes
	}
	if patch.ByeInitiator != "" {
		dst.ByeInitiator = patch.ByeInitiator
	}
	if patch.EndStatus != "" {
		dst.EndStatus = patch.EndStatus
	}
	if patch.InviteAt != nil {
		dst.InviteAt = patch.InviteAt
	}
	if patch.AckAt != nil {
		dst.AckAt = patch.AckAt
	}
	if patch.ByeAt != nil {
		dst.ByeAt = patch.ByeAt
	}
	if patch.EndedAt != nil {
		dst.EndedAt = patch.EndedAt
	}
	if patch.FirstTurnAt != nil {
		dst.FirstTurnAt = patch.FirstTurnAt
	}
	if patch.LastTurnAt != nil {
		dst.LastTurnAt = patch.LastTurnAt
	}
	if patch.TurnCount != 0 {
		dst.TurnCount = patch.TurnCount
	}
	if len(patch.Turns) > 0 {
		dst.Turns = datatypes.JSON(append(json.RawMessage(nil), patch.Turns...))
	}
	if patch.DurationSec != 0 {
		dst.DurationSec = patch.DurationSec
	}
	dst.HadSIPTransfer = dst.HadSIPTransfer || patch.HadSIPTransfer
	dst.HadWebSeat = dst.HadWebSeat || patch.HadWebSeat
	dst.UpdatedAt = time.Now()
}

func UnmarshalSIPCallTurns(j datatypes.JSON) ([]SIPCallDialogTurn, error) {
	if len(j) == 0 {
		return nil, nil
	}
	var turns []SIPCallDialogTurn
	if err := json.Unmarshal(j, &turns); err != nil {
		return nil, err
	}
	return turns, nil
}

func MarshalSIPCallTurns(turns []SIPCallDialogTurn) (datatypes.JSON, error) {
	b, err := json.Marshal(turns)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func NewSIPCallMinimalEstablishedWithFirstTurn(callID string, turns datatypes.JSON, now time.Time) SIPCall {
	return SIPCall{
		CallID:      callID,
		State:       SIPCallStateEstablished,
		Turns:       turns,
		TurnCount:   1,
		FirstTurnAt: &now,
		LastTurnAt:  &now,
	}
}

func SIPCallAppendTurnUpdateMap(row SIPCall, newTurn SIPCallDialogTurn, now time.Time) (map[string]interface{}, int, error) {
	turnList, err := UnmarshalSIPCallTurns(row.Turns)
	if err != nil {
		return nil, 0, err
	}
	turnList = append(turnList, newTurn)
	turnsBytes, err := MarshalSIPCallTurns(turnList)
	if err != nil {
		return nil, 0, err
	}
	n := len(turnList)
	upd := map[string]interface{}{
		"turns":        turnsBytes,
		"turn_count":   n,
		"last_turn_at": now,
		"updated_at":   now,
	}
	if row.FirstTurnAt == nil || row.FirstTurnAt.IsZero() {
		upd["first_turn_at"] = now
	}
	return upd, n, nil
}

// ComputeCallDurationSec returns duration from timestamps when DurationSec was not stored.
func ComputeCallDurationSec(c *SIPCall) int {
	if c == nil {
		return 0
	}
	if c.DurationSec > 0 {
		return c.DurationSec
	}
	var start, end *time.Time
	if c.AckAt != nil {
		start = c.AckAt
	} else if c.InviteAt != nil {
		start = c.InviteAt
	}
	if c.EndedAt != nil {
		end = c.EndedAt
	} else if c.ByeAt != nil {
		end = c.ByeAt
	}
	if start == nil || end == nil || end.Before(*start) {
		return 0
	}
	return int(end.Sub(*start).Round(time.Second) / time.Second)
}

// EffectiveEndStatus fills missing disposition for ended calls.
func EffectiveEndStatus(c *SIPCall) string {
	if c == nil {
		return ""
	}
	if strings.TrimSpace(c.EndStatus) != "" {
		return c.EndStatus
	}
	st := strings.ToLower(strings.TrimSpace(c.State))
	if st == SIPCallStateEnded || st == SIPCallStateFailed {
		return SIPCallEndUnknown
	}
	return ""
}

// DeriveTurnCount uses stored TurnCount or counts JSON turns array.
func DeriveTurnCount(c *SIPCall) int {
	if c == nil {
		return 0
	}
	if c.TurnCount > 0 {
		return c.TurnCount
	}
	if len(c.Turns) == 0 {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(c.Turns, &arr); err != nil {
		return 0
	}
	return len(arr)
}

// EnrichSIPCallResponse sets derived duration / disposition / turn count for API responses.
func EnrichSIPCallResponse(c *SIPCall) {
	if c == nil {
		return
	}
	if d := ComputeCallDurationSec(c); d > 0 {
		c.DurationSec = d
	}
	if es := EffectiveEndStatus(c); es != "" {
		c.EndStatus = es
	}
	if tc := DeriveTurnCount(c); tc > 0 {
		c.TurnCount = tc
	}
}
