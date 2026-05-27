package models

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"errors"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

var ErrPlatformAdminNotFound = errors.New("platform admin not found")
var ErrLastActivePlatformAdmin = errors.New("cannot disable or remove the last active platform admin")

// PlatformAdmin is a global operator (not under any tenant).
type PlatformAdmin struct {
	BaseModel

	Email        string `json:"email" gorm:"size:255;uniqueIndex;not null;comment:登录邮箱"`
	PasswordHash string `json:"-" gorm:"size:255;not null;column:password_hash;comment:密码哈希"`
	DisplayName  string `json:"displayName" gorm:"size:128;comment:显示名"`
	Status       string `json:"status" gorm:"size:24;index;not null;default:active;comment:账号状态"`
	TOTPSecret   string `json:"-" gorm:"size:128;column:totp_secret;comment:TOTP密钥"`
	TOTPEnabled  bool   `json:"totpEnabled" gorm:"column:totp_enabled;not null;default:0;comment:是否启用TOTP"`
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
		"totpEnabled": a.TOTPEnabled,
		"createdAt":   a.CreatedAt,
		"updatedAt":   a.UpdatedAt,
	}
}

func NormalizePlatformAdminStatus(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case constants.PlatformAdminStatusActive, constants.PlatformAdminStatusDisabled:
		return s
	default:
		return ""
	}
}

func GetPlatformAdminByID(db *gorm.DB, id uint) (PlatformAdmin, error) {
	var row PlatformAdmin
	err := db.Where("id = ?", id).First(&row).Error
	return row, err
}

func ListPlatformAdminsPage(db *gorm.DB, page, size int, search string) ([]PlatformAdmin, int64, error) {
	if db == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := db.Model(&PlatformAdmin{})
	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("email LIKE ? OR display_name LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []PlatformAdmin
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

func UpdatePlatformAdminStatus(db *gorm.DB, id uint, status, operator string) (int64, error) {
	status = NormalizePlatformAdminStatus(status)
	if status == "" || id == 0 {
		return 0, nil
	}
	updates := map[string]any{"status": status}
	if operator != "" {
		updates["update_by"] = operator
	}
	res := db.Model(&PlatformAdmin{}).Where("id = ?", id).Updates(updates)
	return res.RowsAffected, res.Error
}

func UpdatePlatformAdminProfile(db *gorm.DB, id uint, email, displayName, operator string) (int64, error) {
	if id == 0 {
		return 0, nil
	}
	updates := map[string]any{}
	if email = utils.TrimLower(email); email != "" {
		updates["email"] = email
	}
	if displayName = strings.TrimSpace(displayName); displayName != "" {
		updates["display_name"] = displayName
	}
	if len(updates) == 0 {
		return 0, nil
	}
	if operator != "" {
		updates["update_by"] = operator
	}
	res := db.Model(&PlatformAdmin{}).Where("id = ?", id).Updates(updates)
	return res.RowsAffected, res.Error
}

func UpdatePlatformAdminPassword(db *gorm.DB, id uint, passwordHash string) error {
	if id == 0 || passwordHash == "" {
		return nil
	}
	return db.Model(&PlatformAdmin{}).Where("id = ?", id).Update("password_hash", passwordHash).Error
}

// EnsureNotLastActivePlatformAdmin returns ErrLastActivePlatformAdmin when disabling/deleting
// would leave zero active platform admins.
func EnsureNotLastActivePlatformAdmin(db *gorm.DB, targetID uint) error {
	row, err := GetPlatformAdminByID(db, targetID)
	if err != nil {
		return err
	}
	if row.Status != constants.PlatformAdminStatusActive {
		return nil
	}
	n, err := CountPlatformAdmins(db)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastActivePlatformAdmin
	}
	return nil
}

func SoftDeletePlatformAdmin(db *gorm.DB, id uint, operator string) (int64, error) {
	var row PlatformAdmin
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		return 0, err
	}
	if err := EnsureNotLastActivePlatformAdmin(db, id); err != nil {
		return 0, err
	}
	row.SoftDelete(operator)
	if err := db.Save(&row).Error; err != nil {
		return 0, err
	}
	return 1, nil
}
