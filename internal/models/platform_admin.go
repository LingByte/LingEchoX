package models

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"strings"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

const (
	PlatformAdminStatusActive   = "active"
	PlatformAdminStatusDisabled = "disabled"
)

// PlatformAdmin is a global operator (not under any tenant).
type PlatformAdmin struct {
	BaseModel

	Email        string `json:"email" gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string `json:"-" gorm:"size:255;not null;column:password_hash"`
	DisplayName  string `json:"displayName" gorm:"size:128"`
	Status       string `json:"status" gorm:"size:24;index;not null;default:active"`
}

func (PlatformAdmin) TableName() string {
	return constants.PLATFORM_ADMIN_TABLE_NAME
}

func GetActivePlatformAdminByEmail(db *gorm.DB, email string) (PlatformAdmin, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var row PlatformAdmin
	err := ActivePlatformAdmins(db).Where("email = ?", email).First(&row).Error
	return row, err
}

func ActivePlatformAdmins(db *gorm.DB) *gorm.DB {
	return db.Model(&PlatformAdmin{})
}

func CountPlatformAdmins(db *gorm.DB) (int64, error) {
	var n int64
	err := ActivePlatformAdmins(db).Count(&n).Error
	return n, err
}
