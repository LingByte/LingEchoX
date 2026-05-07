package models

import "github.com/LingByte/SoulNexus/pkg/constants"

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// Permission is a global capability code (shared RBAC catalog across tenants).
type Permission struct {
	BaseModel

	Code        string `json:"code" gorm:"size:128;uniqueIndex;not null"`
	Name        string `json:"name" gorm:"size:256;not null"`
	Description string `json:"description,omitempty" gorm:"size:512"`
	Resource    string `json:"resource,omitempty" gorm:"size:128;index"`
	Action      string `json:"action,omitempty" gorm:"size:64;index"`
}

func (Permission) TableName() string {
	return constants.PERMISSION_TABLE_NAME
}

// TenantRolePermission assigns permissions to a tenant role.
type TenantRolePermission struct {
	BaseModel

	RoleID       uint `json:"roleId" gorm:"index;not null;uniqueIndex:idx_role_permission"`
	PermissionID uint `json:"permissionId" gorm:"index;not null;uniqueIndex:idx_role_permission"`
}

func (TenantRolePermission) TableName() string {
	return constants.TENANT_ROLE_PERMISSION_TABLE_NAME
}
