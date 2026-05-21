package models

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

// PlatformAdmin is a global operator (not under any tenant).
type PlatformAdmin struct {
	BaseModel

	Email        string `json:"email" gorm:"size:255;uniqueIndex;not null;comment:登录邮箱"`
	PasswordHash string `json:"-" gorm:"size:255;not null;column:password_hash;comment:密码哈希"`
	DisplayName  string `json:"displayName" gorm:"size:128;comment:显示名"`
	Status       string `json:"status" gorm:"size:24;index;not null;default:active;comment:账号状态"`
}

func (PlatformAdmin) TableName() string {
	return constants.PlatformAdminTableName
}

func GetActivePlatformAdminByEmail(db *gorm.DB, email string) (PlatformAdmin, error) {
	email = utils.TrimLower(email)
	var row PlatformAdmin
	err := ActivePlatformAdmins(db).Where("email = ?", email).First(&row).Error
	return row, err
}

func ActivePlatformAdmins(db *gorm.DB) *gorm.DB {
	return db.Model(&PlatformAdmin{}).Where("status = ?", constants.PlatformAdminStatusActive)
}

func CountPlatformAdmins(db *gorm.DB) (int64, error) {
	var n int64
	err := ActivePlatformAdmins(db).Count(&n).Error
	return n, err
}

// PlatformAdminPublic builds API JSON for a platform admin row.
func PlatformAdminPublic(a PlatformAdmin) map[string]any {
	return map[string]any{
		"id":          a.ID,
		"email":       a.Email,
		"displayName": a.DisplayName,
		"status":      a.Status,
	}
}
