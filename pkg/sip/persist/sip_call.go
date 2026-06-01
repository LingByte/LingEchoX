package persist

import (
	"context"
	"encoding/json"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/sip/stack"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const DirectionInbound = "inbound"

// DirectionOutbound is stored on SIPCall rows for carrier / campaign originated legs.
const DirectionOutbound = "outbound"

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
	// TurnGroupID 让流式分段的 LLM 回复在持久化时折叠成一条记录。
	// 同一逻辑轮次的所有段共用同一非空 ID（见 conversation.DialogTurn 注释）。
	TurnGroupID string `json:"turnGroupId,omitempty"`
}

// SIPCall records one SIP call lifecycle (sip_calls).
type SIPCall struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime;comment:Creation time"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime;comment:Update time"`
	IsDeleted int8      `json:"isDeleted,omitempty" gorm:"default:0;index;comment:Soft delete flag (0:not deleted, 1:deleted)"`
	CreateBy  string    `json:"createBy,omitempty" gorm:"size:128;index;comment:Creator"`
	UpdateBy  string    `json:"updateBy,omitempty" gorm:"size:128;index;comment:Updater"`

	TenantID uint `json:"tenantId" gorm:"index;not null;default:0"` // 0=legacy / unknown scope
	// InboundTrunkNumberID is sip_trunk_numbers.id when the inbound DID matched that row (same resolver as tenant_id).
	InboundTrunkNumberID uint `json:"inboundTrunkNumberId,omitempty" gorm:"column:inbound_trunk_number_id;index;default:0"`

	CallID     string `json:"callId" gorm:"size:128;uniqueIndex;not null"`
	FromHeader string `json:"fromHeader" gorm:"type:text"`
	ToHeader   string `json:"toHeader" gorm:"type:text"`
	// FromNumber / ToNumber 是从 FromHeader / ToHeader 中提取出来的纯数字号码，用于日志和列表展示。
	// 例如 `"bob" <sip:13800138000@10.0.4.12>;tag=xyz` → "13800138000"。
	FromNumber        string     `json:"fromNumber" gorm:"size:64;index"`
	ToNumber          string     `json:"toNumber" gorm:"size:64;index"`
	CSeqInvite        string     `json:"cseqInvite" gorm:"size:64"`
	RemoteAddr        string     `json:"remoteAddr" gorm:"size:128;index"`
	Direction         string     `json:"direction" gorm:"size:16;index"`
	RemoteRTPAddr     string     `json:"remoteRtpAddr" gorm:"size:128;index"`
	LocalRTPAddr      string     `json:"localRtpAddr" gorm:"size:128;index"`
	PayloadType       uint8      `json:"-" gorm:"index"`
	Codec             string     `json:"codec,omitempty" gorm:"size:32;index"`
	ClockRate         int        `json:"clockRate,omitempty" gorm:"column:clock_rate"`
	// QoS* are filled once at BYE from RTCP snapshot (no in-call sampling overhead).
	QoSRTTMs          uint32  `json:"qosRttMs,omitempty" gorm:"column:qos_rtt_ms;default:0"`
	QoSJitterMs       float32 `json:"qosJitterMs,omitempty" gorm:"column:qos_jitter_ms"`
	QoSPacketLossPct  float32 `json:"qosPacketLossPct,omitempty" gorm:"column:qos_packet_loss_pct"`
	QoSMosEstimate    float32 `json:"qosMosEstimate,omitempty" gorm:"column:qos_mos_estimate"`
	State             string     `json:"state" gorm:"size:32;index"`
	InviteAt          *time.Time `json:"inviteAt" gorm:"index"`
	AckAt             *time.Time `json:"ackAt" gorm:"index"`
	ByeAt             *time.Time `json:"byeAt" gorm:"index"`
	EndedAt           *time.Time `json:"endedAt" gorm:"index"`
	FailureReason     string     `json:"failureReason" gorm:"type:text"`
	RecordingURL      string     `json:"recordingUrl" gorm:"size:1024"`
	RecordingRawBytes int        `json:"recordingRawBytes" gorm:"column:recording_raw_bytes;default:0"`
	RecordingWavBytes int        `json:"recordingWavBytes" gorm:"column:recording_wav_bytes;default:0"`
	// RecordingHash is filled by pkg/voice/recorder (LingEchoX improvement
	// over upstream VoiceServer, which never populated the hash field).
	// Format is "sha256:<64 hex>"; empty for legacy SN3 → WAV decoded
	// recordings whose canonical bytes do not flow through the recorder.
	RecordingHash  string         `json:"recordingHash,omitempty" gorm:"column:recording_hash;size:96"`
	ByeInitiator   string         `json:"byeInitiator" gorm:"column:bye_initiator;size:16"`
	DurationSec    int            `json:"durationSec" gorm:"default:0"`
	EndStatus      string         `json:"endStatus" gorm:"size:64;index"`
	Turns          datatypes.JSON `json:"turns" gorm:"type:json"`
	TurnCount      int            `json:"turnCount" gorm:"default:0"`
	FirstTurnAt    *time.Time     `json:"firstTurnAt"`
	LastTurnAt     *time.Time     `json:"lastTurnAt"`
	HadSIPTransfer bool           `json:"hadSipTransfer" gorm:"column:had_sip_transfer;default:0"`
	HadWebSeat     bool           `json:"hadWebSeat" gorm:"column:had_web_seat;default:0"`
	// TransferACDTargetID is the acd_pool_targets.id selected when the call was transferred to an agent.
	// 0 means no transfer or unknown target.
	TransferACDTargetID uint `json:"transferAcdTargetId,omitempty" gorm:"column:transfer_acd_target_id;index;default:0"`
	// TransferTo is derived for UI (e.g. seat name / targetValue) and is not stored.
	TransferTo string `json:"transferTo,omitempty" gorm:"-"`
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
		dst.FromNumber = ExtractSIPUserPart(patch.FromHeader)
	}
	if patch.ToHeader != "" {
		dst.ToHeader = patch.ToHeader
		dst.ToNumber = ExtractSIPUserPart(patch.ToHeader)
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
	if patch.InboundTrunkNumberID != 0 {
		dst.InboundTrunkNumberID = patch.InboundTrunkNumberID
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
	// 流式分段折叠：当 newTurn.TurnGroupID 非空且与最后一条相同，
	// 不新建条目，而是在最后一条上追加 LLMText、累加 TTSMs/PipelineMs。
	// LLMFirstMs/LLMWallMs 取首段（首段是 LLM 实际首字延迟），后续段一般为 0；
	// At 沿用首段（用户感知的轮次开始时间）。
	merged := false
	if newTurn.TurnGroupID != "" && len(turnList) > 0 {
		last := &turnList[len(turnList)-1]
		if last.TurnGroupID == newTurn.TurnGroupID {
			if newTurn.LLMText != "" {
				last.LLMText = strings.TrimRight(last.LLMText, " ") + newTurn.LLMText
			}
			last.TTSMs += newTurn.TTSMs
			last.PipelineMs += newTurn.PipelineMs
			if last.LLMFirstMs == 0 {
				last.LLMFirstMs = newTurn.LLMFirstMs
			}
			if last.LLMWallMs == 0 {
				last.LLMWallMs = newTurn.LLMWallMs
			}
			if last.TTSProvider == "" {
				last.TTSProvider = newTurn.TTSProvider
			}
			if last.LLMModel == "" {
				last.LLMModel = newTurn.LLMModel
			}
			merged = true
		}
	}
	if !merged {
		turnList = append(turnList, newTurn)
	}
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

var ipv4HostPortRE = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)(?::\d+)?\b`)

// redactNetworkLeakage masks IPv4 (+optional :port) substrings in free-text fields returned to browsers.
func redactNetworkLeakage(s string) string {
	if s == "" {
		return ""
	}
	return ipv4HostPortRE.ReplaceAllString(s, "[redacted]")
}

// RedactSIPCallForAPI clears topology / raw-signaling fields before HTTP JSON serialization.
// Full rows remain in the database for operations; console APIs must not expose them (tenant-facing rule).
func RedactSIPCallForAPI(c *SIPCall) {
	if c == nil {
		return
	}
	c.FromHeader = ""
	c.ToHeader = ""
	c.CSeqInvite = ""
	c.RemoteAddr = ""
	c.RemoteRTPAddr = ""
	c.LocalRTPAddr = ""
	c.FailureReason = redactNetworkLeakage(c.FailureReason)
}

var sipCallActiveStates = []string{SIPCallStateInit, SIPCallStateRinging, SIPCallStateEstablished}

// CountActiveInboundCallsMatchingToNumbers counts non-terminal inbound rows whose to_number is one of keys.
func CountActiveInboundCallsMatchingToNumbers(db *gorm.DB, tenantID uint, toNumbers []string) (int64, error) {
	if db == nil || tenantID == 0 || len(toNumbers) == 0 {
		return 0, nil
	}
	var n int64
	err := ActiveSIPCalls(db).
		Where("tenant_id = ? AND direction = ?", tenantID, DirectionInbound).
		Where("state IN ?", sipCallActiveStates).
		Where("to_number IN ?", toNumbers).
		Count(&n).Error
	return n, err
}

// CountActiveOutboundCallsMatchingFromNumbers counts non-terminal outbound rows whose from_number is one of keys.
func CountActiveOutboundCallsMatchingFromNumbers(db *gorm.DB, tenantID uint, fromNumbers []string) (int64, error) {
	if db == nil || tenantID == 0 || len(fromNumbers) == 0 {
		return 0, nil
	}
	var n int64
	err := ActiveSIPCalls(db).
		Where("tenant_id = ? AND direction = ?", tenantID, DirectionOutbound).
		Where("state IN ?", sipCallActiveStates).
		Where("from_number IN ?", fromNumbers).
		Count(&n).Error
	return n, err
}

func NewSIPCallRinging(callID, from, to, cseqInvite, remoteAddr, direction, remoteRTP, localRTP string, payloadType uint8, codec string, clockRate int, inviteAt time.Time, tenantID uint, inboundTrunkNumberID uint) SIPCall {
	dir := strings.TrimSpace(direction)
	if dir == "" {
		dir = DirectionInbound
	}
	return SIPCall{
		TenantID:             tenantID,
		InboundTrunkNumberID: inboundTrunkNumberID,
		CallID:               callID,
		FromHeader:           from,
		ToHeader:             to,
		FromNumber:           ExtractSIPUserPart(from),
		ToNumber:             ExtractSIPUserPart(to),
		CSeqInvite:           cseqInvite,
		RemoteAddr:           remoteAddr,
		Direction:            dir,
		RemoteRTPAddr:        remoteRTP,
		LocalRTPAddr:         localRTP,
		PayloadType:          payloadType,
		Codec:                codec,
		ClockRate:            clockRate,
		State:                SIPCallStateRinging,
		InviteAt:             &inviteAt,
	}
}

func SIPCallInviteRefreshUpdateMap(from, to, remoteAddr, remoteRTP, localRTP, codec string, payloadType uint8, clockRate int, tenantID, inboundTrunkNumberID uint, now time.Time) map[string]interface{} {
	m := map[string]interface{}{
		"from_header":     from,
		"to_header":       to,
		"from_number":     ExtractSIPUserPart(from),
		"to_number":       ExtractSIPUserPart(to),
		"remote_addr":     remoteAddr,
		"remote_rtp_addr": remoteRTP,
		"local_rtp_addr":  localRTP,
		"codec":           codec,
		"payload_type":    payloadType,
		"clock_rate":      clockRate,
		"state":           SIPCallStateRinging,
		"updated_at":      now,
	}
	if tenantID != 0 {
		m["tenant_id"] = tenantID
	}
	if inboundTrunkNumberID != 0 {
		m["inbound_trunk_number_id"] = inboundTrunkNumberID
	}
	return m
}

func SIPCallEstablishedUpdateMap(now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"state":      SIPCallStateEstablished,
		"ack_at":     now,
		"updated_at": now,
	}
}

func SIPCallEndStatusForBye(initiator string, hadSIPAgentTransfer, hadWebSeat bool) string {
	hadXfer := hadSIPAgentTransfer || hadWebSeat
	local := strings.EqualFold(strings.TrimSpace(initiator), "local")
	if hadXfer {
		if local {
			return SIPCallEndAfterTransferLocal
		}
		return SIPCallEndAfterTransferRemote
	}
	if local {
		return SIPCallEndCompletedLocal
	}
	return SIPCallEndCompletedRemote
}

func SIPCallDurationSince(ackAt, inviteAt *time.Time, end time.Time) int {
	var start time.Time
	if ackAt != nil && !ackAt.IsZero() {
		start = *ackAt
	} else if inviteAt != nil && !inviteAt.IsZero() {
		start = *inviteAt
	}
	if start.IsZero() {
		return 0
	}
	sec := int(end.Sub(start).Seconds())
	if sec < 0 {
		return 0
	}
	return sec
}

// SIPCallStartTime returns ack_at, else invite_at, for duration anchoring.
func SIPCallStartTime(c *SIPCall) time.Time {
	if c == nil {
		return time.Time{}
	}
	if c.AckAt != nil && !c.AckAt.IsZero() {
		return *c.AckAt
	}
	if c.InviteAt != nil && !c.InviteAt.IsZero() {
		return *c.InviteAt
	}
	return time.Time{}
}

// ApplySIPCallDurationFromRecording sets duration_sec and bye_at/ended_at.
// When recordingSec > 0, end time is start + recording length (authoritative).
// Otherwise falls back to wallEnd (SIP BYE arrival time).
func ApplySIPCallDurationFromRecording(updates map[string]interface{}, call SIPCall, recordingSec int, wallEnd time.Time) {
	if updates == nil {
		return
	}
	start := SIPCallStartTime(&call)
	if recordingSec > 0 && !start.IsZero() {
		end := start.Add(time.Duration(recordingSec) * time.Second)
		updates["duration_sec"] = recordingSec
		updates["bye_at"] = end
		updates["ended_at"] = end
		return
	}
	sec := SIPCallDurationSince(call.AckAt, call.InviteAt, wallEnd)
	if sec > 0 {
		updates["duration_sec"] = sec
	}
	updates["bye_at"] = wallEnd
	updates["ended_at"] = wallEnd
}

func SIPCallByeFinalizeUpdateMap(now time.Time, endStatus string, hadSIPTransfer, hadWebSeat bool, durationSec int) map[string]interface{} {
	return map[string]interface{}{
		"state":            SIPCallStateEnded,
		"bye_at":           now,
		"ended_at":         now,
		"updated_at":       now,
		"end_status":       endStatus,
		"had_sip_transfer": hadSIPTransfer,
		"had_web_seat":     hadWebSeat,
		"duration_sec":     durationSec,
	}
}

func ApplyRTPMediaToSIPCall(c *SIPCall, remoteIP string, remotePort int, localIP string, localPort int, codec string, pt uint8, clock int) {
	if c == nil || remoteIP == "" || remotePort <= 0 {
		return
	}
	c.RemoteRTPAddr = net.JoinHostPort(remoteIP, strconv.Itoa(remotePort))
	if localIP != "" && localPort > 0 {
		c.LocalRTPAddr = net.JoinHostPort(localIP, strconv.Itoa(localPort))
	}
	c.Codec = strings.ToLower(strings.TrimSpace(codec))
	c.PayloadType = pt
	c.ClockRate = clock
}

func SIPCallFromInboundInvite(req *stack.Message, peer *net.UDPAddr) *SIPCall {
	if req == nil {
		return nil
	}
	now := time.Now()
	nowPtr := &now
	callID := strings.TrimSpace(req.GetHeader("call-id"))
	from := req.GetHeader("from")
	to := req.GetHeader("to")
	c := &SIPCall{
		CallID:       callID,
		FromHeader:   from,
		ToHeader:     to,
		FromNumber:   ExtractSIPUserPart(from),
		ToNumber:     ExtractSIPUserPart(to),
		CSeqInvite:   req.GetHeader("cseq"),
		Direction:    DirectionInbound,
		State:        SIPCallStateInit,
		InviteAt:     nowPtr,
		RecordingURL: RecordingRelPathForCall(callID),
		EndStatus:    SIPCallEndUnknown,
	}
	if peer != nil {
		c.RemoteAddr = peer.String()
	}
	return c
}

func ApplyInboundInviteFailure(c *SIPCall, sipStatus int, reason string) {
	if c == nil {
		return
	}
	now := time.Now()
	c.State = SIPCallStateFailed
	c.EndedAt = &now
	c.FailureReason = reason
	switch sipStatus {
	case 486:
		c.EndStatus = SIPCallEndBusy
	case 487:
		c.EndStatus = SIPCallEndCancelled
	case 603:
		c.EndStatus = SIPCallEndDeclined
	default:
		if sipStatus >= 500 {
			c.EndStatus = SIPCallEndServerError
		} else {
			c.EndStatus = SIPCallEndDeclined
		}
	}
}

func ActiveSIPCalls(db *gorm.DB) *gorm.DB {
	return db.Model(&SIPCall{}).Where("is_deleted = ?", SoftDeleteStatusActive)
}

func ListSIPCallsPage(db *gorm.DB, tenantID uint, page, size int, callID, state string) ([]SIPCall, int64, error) {
	q := ActiveSIPCalls(db).Where("tenant_id = ?", tenantID)
	if cid := strings.TrimSpace(callID); cid != "" {
		q = q.Where("call_id = ?", cid)
	}
	if st := strings.TrimSpace(state); st != "" {
		q = q.Where("state = ?", st)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []SIPCall
	if err := q.Order("id DESC").Offset(offset).Limit(size).Omit("turns").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListAllSIPCallsPage 平台管理员视角：跨租户查看通话记录，包含 tenant_id=0 的测试通话。
// 当 tenantIDFilter > 0 时按指定租户过滤；为 0 时返回全部。
func ListAllSIPCallsPage(db *gorm.DB, tenantIDFilter uint, page, size int, callID, state string) ([]SIPCall, int64, error) {
	q := ActiveSIPCalls(db)
	if tenantIDFilter > 0 {
		q = q.Where("tenant_id = ?", tenantIDFilter)
	}
	if cid := strings.TrimSpace(callID); cid != "" {
		q = q.Where("call_id = ?", cid)
	}
	if st := strings.TrimSpace(state); st != "" {
		q = q.Where("state = ?", st)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []SIPCall
	if err := q.Order("id DESC").Offset(offset).Limit(size).Omit("turns").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func GetActiveSIPCallByID(db *gorm.DB, id uint) (SIPCall, error) {
	var row SIPCall
	err := ActiveSIPCalls(db).Where("id = ?", id).First(&row).Error
	return row, err
}

// GetActiveSIPCallForTenant returns a call row scoped to tenantID.
func GetActiveSIPCallForTenant(db *gorm.DB, id uint, tenantID uint) (SIPCall, error) {
	var row SIPCall
	err := ActiveSIPCalls(db).Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error
	return row, err
}

func FindSIPCallByCallID(ctx context.Context, db *gorm.DB, callID string) (SIPCall, error) {
	var row SIPCall
	err := db.WithContext(ctx).Where("call_id = ?", callID).First(&row).Error
	return row, err
}

func FindActiveSIPCallByCallID(ctx context.Context, db *gorm.DB, callID string) (SIPCall, error) {
	var row SIPCall
	err := db.WithContext(ctx).
		Where("call_id = ? AND is_deleted = ?", callID, SoftDeleteStatusActive).
		First(&row).Error
	return row, err
}

func SelectSIPCallTurnsByCallID(db *gorm.DB, callID string) (SIPCall, error) {
	var row SIPCall
	err := db.Select("id", "call_id", "turns", "turn_count").
		Where("call_id = ?", callID).
		Order("id DESC").
		First(&row).Error
	return row, err
}
