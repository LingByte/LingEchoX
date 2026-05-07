package models

import (
	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

// TenantAdminRoleName is the default full-access role created on tenant registration.
const TenantAdminRoleName = "管理员"

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

// TenantUserRole assigns roles to a tenant user.
type TenantUserRole struct {
	BaseModel

	TenantUserID uint `json:"tenantUserId" gorm:"index;not null;uniqueIndex:idx_tenant_user_role"`
	RoleID       uint `json:"roleId" gorm:"index;not null;uniqueIndex:idx_tenant_user_role"`
}

func (TenantUserRole) TableName() string {
	return constants.TENANT_USER_ROLE_TABLE_NAME
}

// CreateTenantRole inserts a tenant-scoped role.
func CreateTenantRole(db *gorm.DB, r *TenantRole) error {
	return db.Create(r).Error
}

// GetTenantRoleByTenantAndName returns a role by tenant and display name.
func GetTenantRoleByTenantAndName(db *gorm.DB, tenantID uint, name string) (TenantRole, error) {
	var row TenantRole
	err := db.Where("tenant_id = ? AND name = ? AND is_deleted = ?", tenantID, name, SoftDeleteStatusActive).First(&row).Error
	return row, err
}

// CreateTenantUserRole binds a tenant user to a role.
func CreateTenantUserRole(db *gorm.DB, tur *TenantUserRole) error {
	return db.Create(tur).Error
}

// TenantUserHasRoleName reports whether the user has an active role with the given name.
func TenantUserHasRoleName(db *gorm.DB, tenantUserID uint, roleName string) (bool, error) {
	var roleIDs []uint
	if err := db.Model(&TenantUserRole{}).Where("tenant_user_id = ?", tenantUserID).Pluck("role_id", &roleIDs).Error; err != nil {
		return false, err
	}
	if len(roleIDs) == 0 {
		return false, nil
	}
	var n int64
	err := db.Model(&TenantRole{}).
		Where("id IN ? AND name = ? AND is_deleted = ?", roleIDs, roleName, SoftDeleteStatusActive).
		Count(&n).Error
	return n > 0, err
}
