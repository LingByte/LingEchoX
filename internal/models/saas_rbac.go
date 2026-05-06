package models

import (
	"github.com/LingByte/SoulNexus/pkg/constants"
)

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

// TenantRole is a named role within one tenant.
type TenantRole struct {
	BaseModel

	TenantID    uint   `json:"tenantId" gorm:"index;not null"`
	Name        string `json:"name" gorm:"size:128;index;not null"`
	Description string `json:"description,omitempty" gorm:"size:512"`
	IsSystem    bool   `json:"isSystem" gorm:"not null;default:0"`
}

func (TenantRole) TableName() string {
	return constants.TENANT_ROLE_TABLE_NAME
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

// TenantUserRole assigns roles to a tenant user.
type TenantUserRole struct {
	BaseModel

	TenantUserID uint `json:"tenantUserId" gorm:"index;not null;uniqueIndex:idx_tenant_user_role"`
	RoleID       uint `json:"roleId" gorm:"index;not null;uniqueIndex:idx_tenant_user_role"`
}

func (TenantUserRole) TableName() string {
	return constants.TENANT_USER_ROLE_TABLE_NAME
}
