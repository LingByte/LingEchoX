package models

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"errors"

	"github.com/LinByte/VoiceServer/internal/constants"
	"gorm.io/gorm"
)

// ErrInvalidOrgReference indicates an id list did not resolve to valid catalog rows.
var ErrInvalidOrgReference = errors.New("invalid organization reference")

// Permission is a global capability code (shared RBAC catalog across tenants).
type Permission struct {
	BaseModel

	Code        string `json:"code" gorm:"size:128;uniqueIndex;not null;comment:权限编码"`
	Name        string `json:"name" gorm:"size:256;not null;comment:权限名称"`
	Description string `json:"description,omitempty" gorm:"size:512;comment:描述"`
	Kind        string `json:"kind" gorm:"size:32;index;not null;default:menu;comment:类型(module/menu/button/api/data)"`
	ParentCode  string `json:"parentCode,omitempty" gorm:"size:128;index;comment:父级编码"`
	Resource    string `json:"resource,omitempty" gorm:"size:128;index;comment:资源标识"`
	Action      string `json:"action,omitempty" gorm:"size:64;index;comment:动作标识"`
}

func (Permission) TableName() string {
	return constants.PermissionTableName
}

// TenantRolePermission assigns permissions to a tenant role.
type TenantRolePermission struct {
	BaseModel

	RoleID       uint `json:"roleId" gorm:"index;not null;uniqueIndex:idx_role_permission;comment:角色ID"`
	PermissionID uint `json:"permissionId" gorm:"index;not null;uniqueIndex:idx_role_permission;comment:权限ID"`
}

func (TenantRolePermission) TableName() string {
	return constants.TenantRolePermissionTableName
}

// ListAllPermissions returns the global permission catalog (active rows).
func ListAllPermissions(db *gorm.DB) ([]Permission, error) {
	var rows []Permission
	err := db.
		Order(`CASE kind 
			WHEN '` + constants.PermissionKindModule + `' THEN 0 
			WHEN '` + constants.PermissionKindMenu + `' THEN 1 
			WHEN '` + constants.PermissionKindButton + `' THEN 2 
			WHEN '` + constants.PermissionKindAPI + `' THEN 3 
			WHEN '` + constants.PermissionKindData + `' THEN 4 
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
	{constants.PermAPISIPCallsRead, "通话记录查看", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPACDRead, "ACD 坐席查看", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPACDWrite, "ACD 坐席管理", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPScriptsRead, "话术模板查看", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPScriptsWrite, "话术模板管理", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPCampaignsRead, "外呼任务查看", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPCampaignsWrite, "外呼任务管理", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPISIPNumbersRead, "号码资源查看", constants.PermissionKindAPI, "api.sip"},
	{constants.PermAPITenantOrgRead, "组织架构查看", constants.PermissionKindAPI, "api.tenant"},
	{constants.PermAPITenantOrgWrite, "组织架构管理", constants.PermissionKindAPI, "api.tenant"},
	{constants.PermAPITenantUsersRead, "成员查看", constants.PermissionKindAPI, "api.tenant"},
	{constants.PermAPITenantUsersWrite, "成员管理", constants.PermissionKindAPI, "api.tenant"},
	{constants.PermAPICredentialsRead, "访问密钥查看", constants.PermissionKindAPI, "api.credentials"},
	{constants.PermAPICredentialsWrite, "访问密钥管理", constants.PermissionKindAPI, "api.credentials"},
	{constants.PermMenuWorkspaceOverview, "工作台菜单", constants.PermissionKindMenu, "menu.workspace"},
	{constants.PermMenuTelRecords, "通话记录菜单", constants.PermissionKindMenu, "menu.tel"},
	{constants.PermMenuTelWebseat, "Web 坐席菜单", constants.PermissionKindMenu, "menu.tel"},
	{constants.PermMenuResPool, "号码池菜单", constants.PermissionKindMenu, "menu.res"},
	{constants.PermMenuResOutbound, "外呼任务菜单", constants.PermissionKindMenu, "menu.res"},
	{constants.PermMenuResScript, "脚本管理菜单", constants.PermissionKindMenu, "menu.res"},
	{constants.PermMenuAccKeys, "访问管理菜单", constants.PermissionKindMenu, "menu.acc"},
	{constants.PermMenuOrgMembers, "成员管理菜单", constants.PermissionKindMenu, "menu.org"},
	{constants.PermMenuOrgDept, "部门菜单", constants.PermissionKindMenu, "menu.org"},
	{constants.PermMenuOrgRole, "角色与权限菜单", constants.PermissionKindMenu, "menu.org"},
}

// BackfillSystemTenantAdminPermissions ensures every system「管理员」role is bound to the
// full current catalog. Idempotent; only inserts missing pivot rows. Used to heal tenants
// created before new permission codes (e.g. menu.* codes) were introduced.
func BackfillSystemTenantAdminPermissions(db *gorm.DB, operator string) error {
	var permIDs []uint
	if err := db.Model(&Permission{}).Pluck("id", &permIDs).Error; err != nil {
		return err
	}
	if len(permIDs) == 0 {
		return nil
	}
	var adminRoleIDs []uint
	if err := db.Model(&TenantRole{}).
		Where("is_system = ? AND name = ?", true, constants.TenantAdminRoleName).
		Pluck("id", &adminRoleIDs).Error; err != nil {
		return err
	}
	for _, roleID := range adminRoleIDs {
		var bound []uint
		if err := db.Model(&TenantRolePermission{}).
			Where("role_id = ?", roleID).
			Pluck("permission_id", &bound).Error; err != nil {
			return err
		}
		have := make(map[uint]struct{}, len(bound))
		for _, id := range bound {
			have[id] = struct{}{}
		}
		for _, pid := range permIDs {
			if _, ok := have[pid]; ok {
				continue
			}
			rp := &TenantRolePermission{RoleID: roleID, PermissionID: pid}
			rp.SetCreateInfo(operator)
			if err := db.Create(rp).Error; err != nil {
				return err
			}
		}
	}
	return nil
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
