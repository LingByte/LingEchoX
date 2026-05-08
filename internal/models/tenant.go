package models

import (
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// Tenant is one SaaS customer organization (multi-tenant root).
type Tenant struct {
	BaseModel
	Name        string `json:"name" gorm:"size:128;index;not null"`
	Slug        string `json:"slug" gorm:"size:64;uniqueIndex;not null"`
	Description string `json:"description,omitempty" gorm:"size:512"`
	Status      string `json:"status" gorm:"size:24;index;not null;default:active"` // active | suspended
}

func (Tenant) TableName() string {
	return constants.TENANT_TABLE_NAME
}

// CreateTenant inserts a tenant row.
func CreateTenant(db *gorm.DB, t *Tenant) error {
	return db.Create(t).Error
}

// GetTenantBySlug returns an active (non-deleted) tenant by slug.
func GetTenantBySlug(db *gorm.DB, slug string) (Tenant, error) {
	var row Tenant
	err := db.Where("slug = ?", slug).First(&row).Error
	return row, err
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
func UpdateActiveTenant(db *gorm.DB, id uint, name, description, status, updateBy string) error {
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	if strings.TrimSpace(name) != "" {
		updates["name"] = strings.TrimSpace(name)
	}
	if description != "" {
		updates["description"] = strings.TrimSpace(description)
	}
	st := strings.TrimSpace(status)
	if st != "" {
		updates["status"] = st
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
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	if err := db.Model(&Tenant{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	return db.Where("id = ?", id).Delete(&Tenant{}).Error
}
