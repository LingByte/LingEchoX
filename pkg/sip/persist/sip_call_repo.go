package persist

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

func ActiveSIPCalls(db *gorm.DB) *gorm.DB {
	return db.Model(&SIPCall{}).Where("is_deleted = ?", SoftDeleteStatusActive)
}

func ListSIPCallsPage(db *gorm.DB, page, size int, callID, state string) ([]SIPCall, int64, error) {
	q := ActiveSIPCalls(db)
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
