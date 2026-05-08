package models

import (
	"errors"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

// ErrInvalidOrgReference indicates an id list did not resolve to valid catalog rows.
var ErrInvalidOrgReference = errors.New("invalid organization reference")

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

const (
	PermissionKindModule = "module"
	PermissionKindMenu   = "menu"
	PermissionKindButton = "button"
	PermissionKindAPI    = "api"
	PermissionKindData   = "data"
)

// Permission is a global capability code (shared RBAC catalog across tenants).
type Permission struct {
	BaseModel

	Code        string `json:"code" gorm:"size:128;uniqueIndex;not null"`
	Name        string `json:"name" gorm:"size:256;not null"`
	Description string `json:"description,omitempty" gorm:"size:512"`
	// Kind: module | menu | button | api | data（模块折叠树 / 菜单 / 按钮 / 接口 / 数据范围）
	Kind       string `json:"kind" gorm:"size:32;index;not null;default:menu"`
	ParentCode string `json:"parentCode,omitempty" gorm:"size:128;index"`
	Resource   string `json:"resource,omitempty" gorm:"size:128;index"`
	Action     string `json:"action,omitempty" gorm:"size:64;index"`
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

// ListAllPermissions returns the global permission catalog (active rows).
func ListAllPermissions(db *gorm.DB) ([]Permission, error) {
	var rows []Permission
	err := db.
		Order(`CASE kind 
			WHEN '` + PermissionKindModule + `' THEN 0 
			WHEN '` + PermissionKindMenu + `' THEN 1 
			WHEN '` + PermissionKindButton + `' THEN 2 
			WHEN '` + PermissionKindAPI + `' THEN 3 
			WHEN '` + PermissionKindData + `' THEN 4 
			ELSE 5 END, parent_code ASC, code ASC`).
		Find(&rows).Error
	return rows, err
}

// ListPermissionIDsForRole returns permission ids bound to a role.
func ListPermissionIDsForRole(db *gorm.DB, roleID uint) ([]uint, error) {
	var ids []uint
	err := db.Model(&TenantRolePermission{}).
		Where("role_id = ?", roleID).
		Pluck("permission_id", &ids).Error
	return ids, err
}

// ReplaceTenantRolePermissions replaces all permission bindings for a role (hard reset pivot rows).
func ReplaceTenantRolePermissions(db *gorm.DB, roleID uint, permissionIDs []uint, operator string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if len(permissionIDs) > 0 {
			var n int64
			if err := tx.Model(&Permission{}).
				Where("id IN ?", permissionIDs).
				Count(&n).Error; err != nil {
				return err
			}
			if int(n) != len(dedupeUint(permissionIDs)) {
				return ErrInvalidOrgReference
			}
		}
		if err := tx.Unscoped().Where("role_id = ?", roleID).Delete(&TenantRolePermission{}).Error; err != nil {
			return err
		}
		for _, pid := range dedupeUint(permissionIDs) {
			rp := &TenantRolePermission{RoleID: roleID, PermissionID: pid}
			rp.SetCreateInfo(operator)
			if err := tx.Create(rp).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AttachAllPermissionsToRole binds every active catalog permission to the role (tenant admin bootstrap).
func AttachAllPermissionsToRole(tx *gorm.DB, roleID uint, operator string) error {
	var ids []uint
	if err := tx.Model(&Permission{}).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, pid := range ids {
		rp := &TenantRolePermission{RoleID: roleID, PermissionID: pid}
		rp.SetCreateInfo(operator)
		if err := tx.Create(rp).Error; err != nil {
			return err
		}
	}
	return nil
}

// builtinPermissions defines the default RBAC permission catalog seeded on startup.
var builtinPermissions = []struct {
	Code       string
	Name       string
	Kind       string
	ParentCode string
}{
	// SIP contact center
	{"api.sip.calls.read", "通话记录查看", PermissionKindAPI, "api.sip"},
	{"api.sip.acd.read", "ACD 坐席查看", PermissionKindAPI, "api.sip"},
	{"api.sip.acd.write", "ACD 坐席管理", PermissionKindAPI, "api.sip"},
	{"api.sip.scripts.read", "话术模板查看", PermissionKindAPI, "api.sip"},
	{"api.sip.scripts.write", "话术模板管理", PermissionKindAPI, "api.sip"},
	{"api.sip.campaigns.read", "外呼任务查看", PermissionKindAPI, "api.sip"},
	{"api.sip.campaigns.write", "外呼任务管理", PermissionKindAPI, "api.sip"},
	{"api.sip.numbers.read", "号码资源查看", PermissionKindAPI, "api.sip"},
	// Tenant organization
	{"api.tenant_org.read", "组织架构查看", PermissionKindAPI, "api.tenant"},
	{"api.tenant_org.write", "组织架构管理", PermissionKindAPI, "api.tenant"},
	{"api.tenant_users.read", "成员查看", PermissionKindAPI, "api.tenant"},
	{"api.tenant_users.write", "成员管理", PermissionKindAPI, "api.tenant"},
	// Credentials
	{"api.credentials.read", "访问密钥查看", PermissionKindAPI, "api.credentials"},
	{"api.credentials.write", "访问密钥管理", PermissionKindAPI, "api.credentials"},
}

// SyncPermissionCatalog upserts the built-in permission rows; existing rows are updated, missing ones created.
func SyncPermissionCatalog(db *gorm.DB) error {
	for _, p := range builtinPermissions {
		var existing Permission
		err := db.Where("code = ?", p.Code).First(&existing).Error
		if err == nil {
			// Update name / kind / parent if changed.
			db.Model(&existing).Updates(map[string]any{
				"name":        p.Name,
				"kind":        p.Kind,
				"parent_code": p.ParentCode,
			})
			continue
		}
		row := Permission{
			Code:       p.Code,
			Name:       p.Name,
			Kind:       p.Kind,
			ParentCode: p.ParentCode,
		}
		row.SetCreateInfo("system")
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func dedupeUint(ids []uint) []uint {
	seen := map[uint]struct{}{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
