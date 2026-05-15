package models

import (
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// Tenant is one SaaS customer organization (multi-tenant root).
type Tenant struct {
	BaseModel
	Name         string `json:"name" gorm:"size:128;index;not null"`
	Slug         string `json:"slug" gorm:"size:64;uniqueIndex;not null"`
	Description  string `json:"description,omitempty" gorm:"size:512"`
	Status       string `json:"status" gorm:"size:24;index;not null;default:active"` // active | suspended
	ContactEmail string `json:"contactEmail" gorm:"size:128;index"`
	MaxUserCount int    `json:"maxUserCount" gorm:"default:5"`
}

func (Tenant) TableName() string {
	return constants.TENANT_TABLE_NAME
}

// CreateTenant inserts a tenant row.
func CreateTenant(db *gorm.DB, t *Tenant) error {
	return db.Create(t).Error
}

// GetActiveTenantByID returns an active tenant by primary key.
func GetActiveTenantByID(db *gorm.DB, id uint) (Tenant, error) {
	var row Tenant
	err := db.Where("id = ?", id).First(&row).Error
	return row, err
}

// TenantSlugTaken reports whether slug is already used by an active tenant.
func TenantSlugTaken(db *gorm.DB, slug string) (bool, error) {
	var n int64
	err := db.Model(&Tenant{}).Where("slug = ?", slug).Count(&n).Error
	return n > 0, err
}

// ListTenantsPage lists active tenants (platform admin).
func ListTenantsPage(db *gorm.DB, page, size int, search string) ([]Tenant, int64, error) {
	q := db.Model(&Tenant{})
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + s + "%"
		q = q.Where("name LIKE ? OR slug LIKE ?", like, like)
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
	if size > 500 {
		size = 500
	}
	offset := (page - 1) * size
	var list []Tenant
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// UpdateActiveTenant patches name / description / status for an active tenant.
func UpdateActiveTenant(db *gorm.DB, id uint, name, description, status, contactEmail string, maxUserCount int, updateBy string) error {
	meta := BaseModel{}
	meta.SetUpdateInfo(updateBy)
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	if !utils.IsEmpty(name) {
		updates["name"] = strings.TrimSpace(name)
	}
	if description != "" {
		updates["description"] = strings.TrimSpace(description)
	}
	status = utils.Trim(status)
	if status != "" {
		updates["status"] = status
	}
	if contactEmail != "" {
		updates["contact_email"] = strings.TrimSpace(contactEmail)
	}
	if maxUserCount > 0 {
		updates["max_user_count"] = maxUserCount
	}
	if len(updates) <= 2 { // only UpdatedAt / update_by
		return nil
	}
	return db.Model(&Tenant{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// SoftDeleteTenant soft-deletes one tenant row (platform ops).
func SoftDeleteTenant(db *gorm.DB, id uint, updateBy string) error {
	meta := BaseModel{}
	meta.SoftDelete(updateBy)
	updates := map[string]any{
		"updated_at": meta.UpdatedAt,
		"deleted_at": meta.DeletedAt,
	}
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	return db.Model(&Tenant{}).Where("id = ?", id).Updates(updates).Error
}
