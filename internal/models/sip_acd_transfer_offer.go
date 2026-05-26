package models

import (
	"context"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"gorm.io/gorm"
)

// SIP ACD transfer-offer phases (sip_acd_transfer_offers.phase).
const (
	SIPACDTransferOfferPhaseRinging    = "ringing"
	SIPACDTransferOfferPhaseConnected  = "connected"
	SIPACDTransferOfferPhaseCancelled  = "cancelled"
	SIPACDTransferOfferPhaseFailed     = "failed"
	SIPACDTransferOfferPhaseSuperseded = "superseded"
)

// SIPACDTransferOffer records one SIP-seat transfer ring attempt (audit / poll fallback).
type SIPACDTransferOffer struct {
	BaseModel
	TenantID        uint       `json:"tenantId" gorm:"index;not null;default:0"`
	InboundCallID   string     `json:"inboundCallId" gorm:"column:inbound_call_id;size:128;index;not null"`
	OutboundCallID  string     `json:"outboundCallId,omitempty" gorm:"column:outbound_call_id;size:128;index"`
	ACDPoolTargetID uint       `json:"acdPoolTargetId" gorm:"column:acd_pool_target_id;index;not null"`
	TrunkNumberID   uint       `json:"trunkNumberId,omitempty" gorm:"column:trunk_number_id;index;default:0"`
	CallerNumber    string     `json:"callerNumber,omitempty" gorm:"column:caller_number;size:64;index"`
	Phase           string     `json:"phase" gorm:"size:24;index;not null"`
	StartedAt       time.Time  `json:"startedAt" gorm:"column:started_at;index;not null"`
	EndedAt         *time.Time `json:"endedAt,omitempty" gorm:"column:ended_at;index"`
}

func (SIPACDTransferOffer) TableName() string {
	return constants.SIPACDTransferOfferTableName
}

// StartSIPACDTransferOffer closes any open ringing row for the inbound leg and inserts a new ringing offer.
func StartSIPACDTransferOffer(ctx context.Context, db *gorm.DB, acdTargetID uint, inboundCallID, callerNumber string) (*SIPACDTransferOffer, error) {
	if db == nil || acdTargetID == 0 {
		return nil, nil
	}
	inboundCallID = strings.TrimSpace(inboundCallID)
	if inboundCallID == "" {
		return nil, nil
	}
	var acd ACDPoolTarget
	if err := ActiveACDPoolTargets(db).Where("id = ?", acdTargetID).First(&acd).Error; err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_ = db.WithContext(ctx).Model(&SIPACDTransferOffer{}).
		Where("inbound_call_id = ? AND phase = ? AND ended_at IS NULL", inboundCallID, SIPACDTransferOfferPhaseRinging).
		Updates(map[string]any{
			"phase":    SIPACDTransferOfferPhaseSuperseded,
			"ended_at": now,
		}).Error

	row := &SIPACDTransferOffer{
		TenantID:        acd.TenantID,
		InboundCallID:   inboundCallID,
		ACDPoolTargetID: acdTargetID,
		TrunkNumberID:   acd.TrunkNumberID,
		CallerNumber:    strings.TrimSpace(callerNumber),
		Phase:           SIPACDTransferOfferPhaseRinging,
		StartedAt:       now,
	}
	if err := db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// FinishSIPACDTransferOffersByInbound ends open offers for an inbound Call-ID.
func FinishSIPACDTransferOffersByInbound(ctx context.Context, db *gorm.DB, inboundCallID, phase string) error {
	if db == nil {
		return nil
	}
	inboundCallID = strings.TrimSpace(inboundCallID)
	phase = strings.TrimSpace(phase)
	if inboundCallID == "" || phase == "" {
		return nil
	}
	now := time.Now().UTC()
	return db.WithContext(ctx).Model(&SIPACDTransferOffer{}).
		Where("inbound_call_id = ? AND ended_at IS NULL", inboundCallID).
		Updates(map[string]any{"phase": phase, "ended_at": now}).Error
}

// FinishSIPACDTransferOffersByACD ends open offers for one ACD row (e.g. before reassignment).
func FinishSIPACDTransferOffersByACD(ctx context.Context, db *gorm.DB, acdTargetID uint, phase string) error {
	if db == nil || acdTargetID == 0 {
		return nil
	}
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return nil
	}
	now := time.Now().UTC()
	return db.WithContext(ctx).Model(&SIPACDTransferOffer{}).
		Where("acd_pool_target_id = ? AND ended_at IS NULL", acdTargetID).
		Updates(map[string]any{"phase": phase, "ended_at": now}).Error
}

// ActiveSIPACDTransferOffer returns the latest open ringing offer for a seat, if any.
func ActiveSIPACDTransferOffer(ctx context.Context, db *gorm.DB, acdTargetID uint) (SIPACDTransferOffer, bool, error) {
	if db == nil || acdTargetID == 0 {
		return SIPACDTransferOffer{}, false, nil
	}
	var row SIPACDTransferOffer
	err := db.WithContext(ctx).Model(&SIPACDTransferOffer{}).
		Where("acd_pool_target_id = ? AND phase = ? AND ended_at IS NULL", acdTargetID, SIPACDTransferOfferPhaseRinging).
		Order("started_at DESC").
		First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return SIPACDTransferOffer{}, false, nil
	}
	if err != nil {
		return SIPACDTransferOffer{}, false, err
	}
	return row, true, nil
}

// ListSIPACDTransferOffersPage lists offer history for one ACD seat (tenant-scoped).
func ListSIPACDTransferOffersPage(ctx context.Context, db *gorm.DB, tenantID, acdTargetID uint, page, size int) ([]SIPACDTransferOffer, int64, error) {
	if db == nil || tenantID == 0 || acdTargetID == 0 {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := db.WithContext(ctx).Model(&SIPACDTransferOffer{}).
		Where("tenant_id = ? AND acd_pool_target_id = ?", tenantID, acdTargetID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []SIPACDTransferOffer
	err := q.Order("started_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&list).Error
	return list, total, err
}
