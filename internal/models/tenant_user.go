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
)

// TenantUser is a login identity scoped to exactly one tenant (SaaS member).
type TenantUser struct {
	BaseModel

	TenantID     uint       `json:"tenantId" gorm:"uniqueIndex:idx_tenant_user_email;index;not null"`
	Email        string     `json:"email" gorm:"size:256;uniqueIndex:idx_tenant_user_email"`
	Phone        string     `json:"phone" gorm:"size:32;uniqueIndex:idx_tenant_phone"`
	Username     string     `json:"username" gorm:"size:128;index"`
	PasswordHash string     `json:"-" gorm:"size:256"`
	DisplayName  string     `json:"displayName" gorm:"size:128"`
	Status       string     `json:"status" gorm:"size:24;index;not null;default:active"` // active | disabled | pending
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
}

func (TenantUser) TableName() string {
	return constants.TENANT_USER_TABLE_NAME
}

// ActiveTenantUsers is the non-deleted tenant user scope.
func ActiveTenantUsers(db *gorm.DB) *gorm.DB {
	return db.Model(&TenantUser{}).Where("is_deleted = ?", SoftDeleteStatusActive)
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
	res := db.Model(&TenantUser{}).Where("id = ? AND is_deleted = ?", id, SoftDeleteStatusActive).Updates(updates)
	return res.RowsAffected, res.Error
}

// UpdateTenantUserStatus updates the status of a tenant user.
func UpdateTenantUserStatus(db *gorm.DB, id uint, status, updateBy string) (int64, error) {
	updates := map[string]any{"status": status}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ? AND is_deleted = ?", id, SoftDeleteStatusActive).Updates(updates)
	return res.RowsAffected, res.Error
}

// UpdateTenantUserLastLogin updates the last login time.
func UpdateTenantUserLastLogin(db *gorm.DB, id uint) error {
	now := time.Now()
	return db.Model(&TenantUser{}).Where("id = ?", id).Update("last_login_at", &now).Error
}

// SoftDeleteTenantUserByID soft-deletes a tenant user by ID.
func SoftDeleteTenantUserByID(db *gorm.DB, id uint, updateBy string) (int64, error) {
	u := map[string]any{"is_deleted": SoftDeleteStatusDeleted}
	if updateBy != "" {
		u["update_by"] = updateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(u)
	return res.RowsAffected, res.Error
}

// RestoreTenantUser restores a soft-deleted tenant user.
func RestoreTenantUser(db *gorm.DB, id uint, updateBy string) (int64, error) {
	u := map[string]any{"is_deleted": SoftDeleteStatusActive}
	if updateBy != "" {
		u["update_by"] = updateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(u)
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

// CheckTenantUserEmailExists checks if email exists in tenant (excluding optional user ID).
func CheckTenantUserEmailExists(db *gorm.DB, tenantID uint, email string, excludeID uint) (bool, error) {
	q := db.Model(&TenantUser{}).Where("tenant_id = ? AND email = ? AND is_deleted = ?", tenantID, email, SoftDeleteStatusActive)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// CheckTenantUserPhoneExists checks if phone exists in tenant (excluding optional user ID).
func CheckTenantUserPhoneExists(db *gorm.DB, tenantID uint, phone string, excludeID uint) (bool, error) {
	q := db.Model(&TenantUser{}).Where("tenant_id = ? AND phone = ? AND is_deleted = ?", tenantID, phone, SoftDeleteStatusActive)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// CheckTenantUserUsernameExists checks if username exists in tenant (excluding optional user ID).
func CheckTenantUserUsernameExists(db *gorm.DB, tenantID uint, username string, excludeID uint) (bool, error) {
	q := db.Model(&TenantUser{}).Where("tenant_id = ? AND username = ? AND is_deleted = ?", tenantID, username, SoftDeleteStatusActive)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}
