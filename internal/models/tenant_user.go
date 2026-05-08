package models

import (
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

const (
	TenantUserStatusActive   = "active"
	TenantUserStatusDisabled = "disabled"
	TenantUserStatusPending  = "pending"

	TenantUserSourceRegister = "register"
	TenantUserSourceManual   = "manual"
)

// TenantUser is a login identity scoped to exactly one tenant (SaaS member).
type TenantUser struct {
	BaseModel

	TenantID     uint       `json:"tenantId" gorm:"index;not null"`
	Email        string     `json:"email" gorm:"size:256;uniqueIndex:idx_global_tenant_user_email;not null"`
	Phone        string     `json:"phone" gorm:"size:32"`
	Username     string     `json:"username" gorm:"size:128"`
	PasswordHash string     `json:"-" gorm:"size:256"`
	DisplayName  string     `json:"displayName" gorm:"size:128"`
	AvatarURL    string     `json:"avatarUrl" gorm:"size:512"`
	Status       string     `json:"status" gorm:"size:32;index;not null;default:active"` // active | disabled | pending
	LastLogin    *time.Time `json:"lastLogin,omitempty" gorm:"column:last_login_at"`
	LastLoginIP  string     `json:"-" gorm:"size:128;column:last_login_ip"`
	Source       string     `json:"source" gorm:"size:64;index;default:register"`
	LoginCount   int        `json:"loginCount" gorm:"default:0"`
	TOTPSecret   string     `json:"-" gorm:"size:128;column:totp_secret"`
	TOTPEnabled  bool       `json:"totpEnabled" gorm:"column:totp_enabled;not null;default:0"`
}

func (TenantUser) TableName() string {
	return constants.TENANT_USER_TABLE_NAME
}

// ActiveTenantUsers is the non-deleted tenant user scope.
func ActiveTenantUsers(db *gorm.DB) *gorm.DB {
	return db.Model(&TenantUser{})
}

// ByTenantID scopes to a specific tenant.
func ByTenantID(db *gorm.DB, tenantID uint) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

// ListTenantUsersPage lists active tenant users with optional filters.
func ListTenantUsersPage(db *gorm.DB, tenantID uint, page, size int, status, search string) ([]TenantUser, int64, error) {
	q := ActiveTenantUsers(db)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if s := strings.TrimSpace(status); s != "" {
		q = q.Where("status = ?", s)
	}
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where("email LIKE ? OR username LIKE ? OR display_name LIKE ? OR phone LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []TenantUser
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetActiveTenantUserByID returns one active tenant user by primary key.
func GetActiveTenantUserByID(db *gorm.DB, id uint) (TenantUser, error) {
	var row TenantUser
	err := ActiveTenantUsers(db).Where("id = ?", id).First(&row).Error
	return row, err
}

// GetTenantUserByID returns a tenant user by ID (ignores soft-delete).
func GetTenantUserByID(db *gorm.DB, id uint) (TenantUser, error) {
	var row TenantUser
	err := db.First(&row, id).Error
	return row, err
}

// GetTenantUserByEmail returns a tenant user by email within a tenant.
func GetTenantUserByEmail(db *gorm.DB, tenantID uint, email string) (TenantUser, error) {
	var row TenantUser
	err := db.Where("tenant_id = ? AND email = ?", tenantID, email).First(&row).Error
	return row, err
}

// GetActiveTenantUserByEmail returns a non-deleted tenant user by email within a tenant.
func GetActiveTenantUserByEmail(db *gorm.DB, tenantID uint, email string) (TenantUser, error) {
	var row TenantUser
	err := ActiveTenantUsers(db).Where("tenant_id = ? AND email = ?", tenantID, strings.TrimSpace(email)).First(&row).Error
	return row, err
}

// GetActiveTenantUserByEmailGlobal returns a non-deleted tenant user by email (unique across the system).
func GetActiveTenantUserByEmailGlobal(db *gorm.DB, email string) (TenantUser, error) {
	var row TenantUser
	err := ActiveTenantUsers(db).Where("email = ?", strings.TrimSpace(strings.ToLower(email))).First(&row).Error
	return row, err
}

// GetTenantUserByPhone returns a tenant user by phone within a tenant.
func GetTenantUserByPhone(db *gorm.DB, tenantID uint, phone string) (TenantUser, error) {
	var row TenantUser
	err := db.Where("tenant_id = ? AND phone = ?", tenantID, phone).First(&row).Error
	return row, err
}

// GetTenantUserByUsername returns a tenant user by username within a tenant.
func GetTenantUserByUsername(db *gorm.DB, tenantID uint, username string) (TenantUser, error) {
	var row TenantUser
	err := db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&row).Error
	return row, err
}

// CreateTenantUser creates a new tenant user.
func CreateTenantUser(db *gorm.DB, user *TenantUser) error {
	return db.Create(user).Error
}

// UpdateTenantUser updates a tenant user by ID.
func UpdateTenantUser(db *gorm.DB, id uint, updates map[string]any, updateBy string) (int64, error) {
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(updates)
	return res.RowsAffected, res.Error
}

// UpdateTenantUserStatus updates the status of a tenant user.
func UpdateTenantUserStatus(db *gorm.DB, id uint, status, updateBy string) (int64, error) {
	updates := map[string]any{"status": status}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(updates)
	return res.RowsAffected, res.Error
}

// RecordTenantUserLogin sets last login time and IP and increments login_count.
func RecordTenantUserLogin(db *gorm.DB, id uint, ip string) error {
	now := time.Now()
	ip = strings.TrimSpace(ip)
	if len(ip) > 128 {
		ip = ip[:128]
	}
	return db.Model(&TenantUser{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": &now,
		"last_login_ip": ip,
		"login_count":   gorm.Expr("login_count + ?", 1),
	}).Error
}

// SoftDeleteTenantUserByID soft-deletes a tenant user by ID.
func SoftDeleteTenantUserByID(db *gorm.DB, id uint, updateBy string) (int64, error) {
	u := map[string]any{}
	if updateBy != "" {
		u["update_by"] = updateBy
	}
	if len(u) > 0 {
		if err := db.Model(&TenantUser{}).Where("id = ?", id).Updates(u).Error; err != nil {
			return 0, err
		}
	}
	res := db.Where("id = ?", id).Delete(&TenantUser{})
	return res.RowsAffected, res.Error
}

// RestoreTenantUser restores a soft-deleted tenant user.
func RestoreTenantUser(db *gorm.DB, id uint, updateBy string) (int64, error) {
	u := map[string]any{"deleted_at": nil}
	if updateBy != "" {
		u["update_by"] = updateBy
	}
	res := db.Unscoped().Model(&TenantUser{}).Where("id = ?", id).Updates(u)
	return res.RowsAffected, res.Error
}

// CountTenantUsers counts total active users (optionally by tenant).
func CountTenantUsers(db *gorm.DB, tenantID uint) (int64, error) {
	q := ActiveTenantUsers(db)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var count int64
	err := q.Count(&count).Error
	return count, err
}

// CountTenantUsersByStatus counts users by status.
func CountTenantUsersByStatus(db *gorm.DB, tenantID uint, status string) (int64, error) {
	q := ActiveTenantUsers(db).Where("status = ?", status)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var count int64
	err := q.Count(&count).Error
	return count, err
}

// CheckTenantUserEmailExists checks if email is already used by an active user (globally unique).
func CheckTenantUserEmailExists(db *gorm.DB, _ uint, email string, excludeID uint) (bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	q := ActiveTenantUsers(db).Where("email = ?", email)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// CheckTenantUserPhoneExists checks if phone is already used globally (excluding empty phone).
func CheckTenantUserPhoneExists(db *gorm.DB, _ uint, phone string, excludeID uint) (bool, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return false, nil
	}
	q := ActiveTenantUsers(db).Where("phone = ?", phone)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// CheckTenantUserUsernameExists checks if username is already used globally (excluding blank).
func CheckTenantUserUsernameExists(db *gorm.DB, _ uint, username string, excludeID uint) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, nil
	}
	q := ActiveTenantUsers(db).Where("username = ?", username)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}
