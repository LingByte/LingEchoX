package models

import (
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
	err := db.Where("slug = ? AND is_deleted = ?", slug, SoftDeleteStatusActive).First(&row).Error
	return row, err
}

// TenantSlugTaken reports whether slug is already used by an active tenant.
func TenantSlugTaken(db *gorm.DB, slug string) (bool, error) {
	var n int64
	err := db.Model(&Tenant{}).Where("slug = ? AND is_deleted = ?", slug, SoftDeleteStatusActive).Count(&n).Error
	return n > 0, err
}
