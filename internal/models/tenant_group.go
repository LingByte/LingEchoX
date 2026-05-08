package models

import (
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

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

// FirstTenantGroupForUser returns the first department (alphabetical) linked to the tenant user, if any.
func FirstTenantGroupForUser(db *gorm.DB, tenantUserID uint) (TenantGroup, error) {
	var g TenantGroup
	tg := constants.TENANT_GROUP_TABLE_NAME
	tugTbl := constants.TENANT_USER_GROUP_TABLE_NAME
	err := db.Model(&TenantGroup{}).
		Joins("INNER JOIN "+tugTbl+" AS tug ON tug.group_id = "+tg+".id AND tug.deleted_at IS NULL").
		Where("tug.tenant_user_id = ? AND "+tg+".deleted_at IS NULL", tenantUserID).
		Order(tg + ".name ASC").
		First(&g).Error
	return g, err
}

// ListTenantGroupsForTenant lists departments for a tenant.
func ListTenantGroupsForTenant(db *gorm.DB, tenantID uint) ([]TenantGroup, error) {
	var rows []TenantGroup
	err := db.Where("tenant_id = ?", tenantID).
		Order("name ASC").
		Find(&rows).Error
	return rows, err
}

// ListTenantGroupsForUser lists departments linked to a user (active memberships).
func ListTenantGroupsForUser(db *gorm.DB, tenantUserID uint) ([]TenantGroup, error) {
	tg := constants.TENANT_GROUP_TABLE_NAME
	tugTbl := constants.TENANT_USER_GROUP_TABLE_NAME
	var rows []TenantGroup
	err := db.Model(&TenantGroup{}).
		Joins("INNER JOIN "+tugTbl+" AS tug ON tug.group_id = "+tg+".id AND tug.deleted_at IS NULL").
		Where("tug.tenant_user_id = ? AND "+tg+".deleted_at IS NULL", tenantUserID).
		Order(tg + ".name ASC").
		Find(&rows).Error
	return rows, err
}

// CreateTenantGroupRecord persists a new tenant group.
func CreateTenantGroupRecord(db *gorm.DB, g *TenantGroup) error {
	return db.Create(g).Error
}

// ReplaceTenantUserGroups replaces group memberships for a tenant user.
func ReplaceTenantUserGroups(db *gorm.DB, tenantID, tenantUserID uint, groupIDs []uint, operator string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		groupIDs = dedupeUint(groupIDs)
		if len(groupIDs) > 0 {
			var n int64
			if err := tx.Model(&TenantGroup{}).
				Where("tenant_id = ? AND id IN ?", tenantID, groupIDs).
				Count(&n).Error; err != nil {
				return err
			}
			if int(n) != len(groupIDs) {
				return ErrInvalidOrgReference
			}
		}
		if err := tx.Unscoped().Where("tenant_user_id = ?", tenantUserID).Delete(&TenantUserGroup{}).Error; err != nil {
			return err
		}
		for _, gid := range groupIDs {
			tug := &TenantUserGroup{TenantUserID: tenantUserID, GroupID: gid}
			tug.SetCreateInfo(operator)
			if err := tx.Create(tug).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SoftDeleteTenantGroup soft-deletes a group and marks its memberships deleted.
func SoftDeleteTenantGroup(db *gorm.DB, tenantID, groupID uint, updateBy string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var g TenantGroup
		if err := tx.Where("id = ? AND tenant_id = ?", groupID, tenantID).
			First(&g).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&TenantUserGroup{}).
			Where("group_id = ?", groupID).
			Updates(map[string]any{"updated_at": now, "update_by": updateBy}).Error; err != nil {
			return err
		}
		if err := tx.Model(&TenantGroup{}).Where("id = ?", groupID).Updates(map[string]any{
			"updated_at": now,
			"update_by":  updateBy,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", groupID).Delete(&TenantUserGroup{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", groupID).Delete(&TenantGroup{}).Error
	})
}
