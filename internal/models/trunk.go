package models

import (
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

type Trunk struct {
	ID          uint           `json:"id" gorm:"primarykey"`
	CreatedAt   time.Time      `json:"createdAt" label:"创建时间"`
	UpdatedAt   time.Time      `json:"updatedAt" label:"更新时间"`
	DeletedAt   gorm.DeletedAt `json:"deletedAt" gorm:"index"`
	Name        string         `json:"name" gorm:"size:200" label:"线路名称"`
	Description string         `json:"description,omitempty" label:"备注"`
	Prefix      string         `json:"prefix"`
	LocalAddr   string         `json:"local_addr"`
	Numbers     []TrunkNumber  `json:"numbers" gorm:"foreignKey:TrunkID" label:"号码"`
	NumberNames []string       `json:"numberNames" gorm:"-" label:"号码名称"`
	ProviderId  uint           `json:"providerId"`
	Provider    string         `json:"-" label:"供应商"`
}

type TrunkNumber struct {
	ID               uint           `json:"id" gorm:"primarykey" label:"编号"`
	CreatedAt        time.Time      `json:"createdAt" label:"创建时间"`
	UpdatedAt        time.Time      `json:"updatedAt" label:"更新时间"`
	DeletedAt        gorm.DeletedAt `json:"deletedAt" gorm:"index"`
	TrunkID          uint           `json:"trunkId" label:"所属线路"`
	Trunk            Trunk          `json:"-" label:"所属线路"`
	Number           string         `json:"number" gorm:"size:200" label:"号码"`
	Prefix           string         `json:"prefix" gorm:"size:200" label:"前缀"`
	Description      string         `json:"description,omitempty" label:"备注"`
	Direction        string         `json:"direction" label:"呼叫用途"`
	Status           string         `json:"status" gorm:"size:64;index" label:"状态"`
	Concurrent       uint           `json:"concurrent" label:"呼出并发数"`
	CallInConcurrent uint           `json:"callInConcurrent" gorm:"column:call_in_concurrent;default:0" label:"呼入并发数"`
	IsTransferRelay  bool           `json:"isTransferRelay" gorm:"column:is_transfer_relay;default:0;comment:是否为转人工中继号码" label:"转人工中继号码"`
	EffectiveTime    *time.Time     `json:"effectiveTime" gorm:"column:effective_time;comment:生效时间" label:"生效时间"`
	ExpirationTime   *time.Time     `json:"expirationTime" gorm:"column:expiration_time;comment:失效时间" label:"失效时间"`
	ProviderId       uint           `json:"providerId"`
	Provider         string         `json:"-" label:"供应商"`
}

func (Trunk) TableName() string {
	return constants.SIP_TRUNK_TABLE_NAME
}

func (TrunkNumber) TableName() string {
	return constants.SIP_TRUNK_NUMBER_TABLE_NAME
}

// ListTrunksPage lists non-deleted trunks with optional name filter.
func ListTrunksPage(db *gorm.DB, page, size int, nameContains string) ([]Trunk, int64, error) {
	q := db.Model(&Trunk{})
	if name := strings.TrimSpace(nameContains); name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	offset := (page - 1) * size
	var list []Trunk
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetTrunkByID loads one trunk (with numbers); ErrRecordNotFound if missing or soft-deleted.
func GetTrunkByID(db *gorm.DB, id uint) (Trunk, error) {
	var row Trunk
	err := db.Preload("Numbers", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("id ASC")
	}).First(&row, id).Error
	return row, err
}

// GetTrunkByIDBare loads trunk row without numbers.
func GetTrunkByIDBare(db *gorm.DB, id uint) (Trunk, error) {
	var row Trunk
	err := db.First(&row, id).Error
	return row, err
}

// SoftDeleteTrunkCascade soft-deletes a trunk and its numbers.
func SoftDeleteTrunkCascade(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("trunk_id = ?", id).Delete(&TrunkNumber{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Trunk{}, id).Error
	})
}

// ListTrunkNumbersPage lists numbers; trunkID 0 means all trunks.
func ListTrunkNumbersPage(db *gorm.DB, trunkID uint, page, size int, numberContains string) ([]TrunkNumber, int64, error) {
	q := db.Model(&TrunkNumber{})
	if trunkID > 0 {
		q = q.Where("trunk_id = ?", trunkID)
	}
	if num := strings.TrimSpace(numberContains); num != "" {
		q = q.Where("`number` LIKE ?", "%"+num+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	offset := (page - 1) * size
	var list []TrunkNumber
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetTrunkNumberByID loads one trunk number row.
func GetTrunkNumberByID(db *gorm.DB, id uint) (TrunkNumber, error) {
	var row TrunkNumber
	err := db.First(&row, id).Error
	return row, err
}

// SoftDeleteTrunkNumberByID soft-deletes one number row.
func SoftDeleteTrunkNumberByID(db *gorm.DB, id uint) error {
	return db.Delete(&TrunkNumber{}, id).Error
}
