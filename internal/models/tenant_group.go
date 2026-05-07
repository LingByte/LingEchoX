package models

import "github.com/LingByte/SoulNexus/pkg/constants"

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// TenantGroup is a team / department within a tenant. Mark IsDefault for the bucket new users join by default.
type TenantGroup struct {
	BaseModel

	TenantID  uint   `json:"tenantId" gorm:"index;not null"`
	Name      string `json:"name" gorm:"size:128;index;not null"`
	IsDefault bool   `json:"isDefault" gorm:"index;not null;default:0"`
}

func (TenantGroup) TableName() string {
	return constants.TENANT_GROUP_TABLE_NAME
}

// TenantUserGroup links users to groups (many-to-many).
type TenantUserGroup struct {
	BaseModel

	TenantUserID uint `json:"tenantUserId" gorm:"index;not null;uniqueIndex:idx_user_group"`
	GroupID      uint `json:"groupId" gorm:"index;not null;uniqueIndex:idx_user_group"`
}

func (TenantUserGroup) TableName() string {
	return constants.TENANT_USER_GROUP_TABLE_NAME
}
