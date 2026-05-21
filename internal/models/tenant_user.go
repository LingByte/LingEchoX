package models

import (
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// TenantUser is a login identity scoped to exactly one tenant (SaaS member).
type TenantUser struct {
	BaseModel

	TenantID     uint       `json:"tenantId" gorm:"index;not null;comment:所属租户ID"`
	Email        string     `json:"email" gorm:"size:256;uniqueIndex:idx_global_tenant_user_email;not null;comment:登录邮箱"`
	Phone        string     `json:"phone" gorm:"size:32;comment:手机号"`
	Username     string     `json:"username" gorm:"size:128;comment:用户名"`
	PasswordHash string     `json:"-" gorm:"size:256;comment:密码哈希"`
	DisplayName  string     `json:"displayName" gorm:"size:128;comment:显示名"`
	AvatarURL    string     `json:"avatarUrl" gorm:"size:512;comment:头像URL"`
	Status       string     `json:"status" gorm:"size:32;index;not null;default:active;comment:账号状态"`
	LastLogin    *time.Time `json:"lastLogin,omitempty" gorm:"column:last_login_at;comment:最后登录时间"`
	LastLoginIP  string     `json:"-" gorm:"size:128;column:last_login_ip;comment:最后登录IP"`
	Source       string     `json:"source" gorm:"size:64;index;default:register;comment:来源"`
	LoginCount   int        `json:"loginCount" gorm:"default:0;comment:登录次数"`
	TOTPSecret   string     `json:"-" gorm:"size:128;column:totp_secret;comment:TOTP密钥"`
	TOTPEnabled  bool       `json:"totpEnabled" gorm:"column:totp_enabled;not null;default:0;comment:是否启用TOTP"`
}

// TenantUserPublic builds a JSON-safe tenant user map (Snowflake ids as strings).
func TenantUserPublic(db *gorm.DB, u TenantUser) map[string]any {
	out := map[string]any{
		"id":          strconv.FormatUint(uint64(u.ID), 10),
		"tenantId":    strconv.FormatUint(uint64(u.TenantID), 10),
		"email":       u.Email,
		"phone":       u.Phone,
		"username":    u.Username,
		"displayName": u.DisplayName,
		"avatarUrl":   u.AvatarURL,
		"status":      u.Status,
		"createdAt":   u.CreatedAt,
		"lastLogin":   u.LastLogin,
		"lastLoginIp": u.LastLoginIP,
		"source":      u.Source,
		"loginCount":  u.LoginCount,
		"totpEnabled": u.TOTPEnabled,
	}
	if gs, err := ListTenantGroupsForUser(db, u.ID); err == nil && len(gs) > 0 {
		gpub := make([]map[string]any, 0, len(gs))
		for _, g := range gs {
			gpub = append(gpub, map[string]any{"id": g.ID, "name": g.Name, "isDefault": g.IsDefault})
		}
		out["tenantGroups"] = gpub
		out["tenantGroup"] = map[string]any{"id": gs[0].ID, "name": gs[0].Name}
	}
	if roles, err := ListTenantRolesForUser(db, u.ID); err == nil && len(roles) > 0 {
		rpub := make([]map[string]any, 0, len(roles))
		for _, r := range roles {
			rpub = append(rpub, map[string]any{"id": r.ID, "name": r.Name, "isSystem": r.IsSystem})
		}
		out["roles"] = rpub
	}
	return out
}

func (TenantUser) TableName() string {
	return constants.TenantUserTableName
}

// ActiveTenantUsers is the non-deleted tenant user scope.
func ActiveTenantUsers(db *gorm.DB) *gorm.DB {
	return db.Model(&TenantUser{})
}

// ByTenantID scopes to a specific tenant.
func ByTenantID(db *gorm.DB, tenantID uint) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

// ListTenantUsersPage lists active tenant users with optional filters.
func ListTenantUsersPage(db *gorm.DB, tenantID uint, page, size int, status, search string) ([]TenantUser, int64, error) {
	q := ActiveTenantUsers(db)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if s := strings.TrimSpace(status); s != "" {
		q = q.Where("status = ?", s)
	}
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where("email LIKE ? OR username LIKE ? OR display_name LIKE ? OR phone LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []TenantUser
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetAuthenticatedTenantUser returns the active user when JWT user id matches tenant id.
func GetAuthenticatedTenantUser(db *gorm.DB, userID, tenantID uint) (TenantUser, error) {
	if userID == 0 || tenantID == 0 {
		return TenantUser{}, gorm.ErrRecordNotFound
	}
	u, err := GetActiveTenantUserByID(db, userID)
	if err != nil {
		return TenantUser{}, err
	}
	if u.TenantID != tenantID {
		return TenantUser{}, gorm.ErrRecordNotFound
	}
	return u, nil
}

// GetActiveTenantUserByID returns one active tenant user by primary key.
func GetActiveTenantUserByID(db *gorm.DB, id uint) (TenantUser, error) {
	var row TenantUser
	err := ActiveTenantUsers(db).Where("id = ?", id).First(&row).Error
	return row, err
}

// GetTenantUserByID returns a tenant user by ID (ignores soft-delete).
func GetTenantUserByID(db *gorm.DB, id uint) (TenantUser, error) {
	var row TenantUser
	err := db.First(&row, id).Error
	return row, err
}

// GetTenantUserByEmail returns a tenant user by email within a tenant.
func GetTenantUserByEmail(db *gorm.DB, tenantID uint, email string) (TenantUser, error) {
	var row TenantUser
	err := db.Where("tenant_id = ? AND email = ?", tenantID, email).First(&row).Error
	return row, err
}

// GetActiveTenantUserByEmail returns a non-deleted tenant user by email within a tenant.
func GetActiveTenantUserByEmail(db *gorm.DB, tenantID uint, email string) (TenantUser, error) {
	var row TenantUser
	err := ActiveTenantUsers(db).Where("tenant_id = ? AND email = ?", tenantID, strings.TrimSpace(email)).First(&row).Error
	return row, err
}

// GetActiveTenantUserByEmailGlobal returns a non-deleted tenant user by email (unique across the system).
func GetActiveTenantUserByEmailGlobal(db *gorm.DB, email string) (TenantUser, error) {
	var row TenantUser
	err := ActiveTenantUsers(db).Where("email = ?", utils.TrimLower(email)).First(&row).Error
	return row, err
}

// GetTenantUserByPhone returns a tenant user by phone within a tenant.
func GetTenantUserByPhone(db *gorm.DB, tenantID uint, phone string) (TenantUser, error) {
	var row TenantUser
	err := db.Where("tenant_id = ? AND phone = ?", tenantID, phone).First(&row).Error
	return row, err
}

// GetTenantUserByUsername returns a tenant user by username within a tenant.
func GetTenantUserByUsername(db *gorm.DB, tenantID uint, username string) (TenantUser, error) {
	var row TenantUser
	err := db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&row).Error
	return row, err
}

// CreateTenantUser creates a new tenant user.
func CreateTenantUser(db *gorm.DB, user *TenantUser) error {
	return db.Create(user).Error
}

// UpdateTenantUser updates a tenant user by ID.
func UpdateTenantUser(db *gorm.DB, id uint, updates map[string]any, updateBy string) (int64, error) {
	meta := BaseModel{}
	meta.SetUpdateInfo(updateBy)
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(updates)
	return res.RowsAffected, res.Error
}

// UpdateTenantUserStatus updates the status of a tenant user.
func UpdateTenantUserStatus(db *gorm.DB, id uint, status, updateBy string) (int64, error) {
	updates := map[string]any{"status": status}
	meta := BaseModel{}
	meta.SetUpdateInfo(updateBy)
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(updates)
	return res.RowsAffected, res.Error
}

// RecordTenantUserLogin sets last login time and IP and increments login_count.
func RecordTenantUserLogin(db *gorm.DB, id uint, ip string) error {
	now := time.Now()
	ip = strings.TrimSpace(ip)
	if len(ip) > 128 {
		ip = ip[:128]
	}
	return db.Model(&TenantUser{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": &now,
		"last_login_ip": ip,
		"login_count":   gorm.Expr("login_count + ?", 1),
	}).Error
}

// SoftDeleteTenantUserByID soft-deletes a tenant user by ID.
func SoftDeleteTenantUserByID(db *gorm.DB, id uint, updateBy string) (int64, error) {
	meta := BaseModel{}
	meta.SoftDelete(updateBy)
	u := map[string]any{
		"deleted_at": meta.DeletedAt,
		"updated_at": meta.UpdatedAt,
	}
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	res := db.Model(&TenantUser{}).Where("id = ?", id).Updates(u)
	return res.RowsAffected, res.Error
}

// RestoreTenantUser restores a soft-deleted tenant user.
func RestoreTenantUser(db *gorm.DB, id uint, updateBy string) (int64, error) {
	meta := BaseModel{}
	meta.Restore(updateBy)
	u := map[string]any{
		"deleted_at": nil,
		"updated_at": meta.UpdatedAt,
	}
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	res := db.Unscoped().Model(&TenantUser{}).Where("id = ?", id).Updates(u)
	return res.RowsAffected, res.Error
}

// CountTenantUsers counts total active users (optionally by tenant).
func CountTenantUsers(db *gorm.DB, tenantID uint) (int64, error) {
	q := ActiveTenantUsers(db)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var count int64
	err := q.Count(&count).Error
	return count, err
}

// CountTenantUsersByStatus counts users by status.
func CountTenantUsersByStatus(db *gorm.DB, tenantID uint, status string) (int64, error) {
	q := ActiveTenantUsers(db).Where("status = ?", status)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var count int64
	err := q.Count(&count).Error
	return count, err
}

// CheckTenantUserEmailExists checks if email is already used by an active user (globally unique).
func CheckTenantUserEmailExists(db *gorm.DB, email string, excludeID uint) (bool, error) {
	email = utils.TrimLower(email)
	q := ActiveTenantUsers(db).Where("email = ?", email)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// CheckTenantUserPhoneExists checks if phone is already used globally (excluding empty phone).
func CheckTenantUserPhoneExists(db *gorm.DB, phone string, excludeID uint) (bool, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return false, nil
	}
	q := ActiveTenantUsers(db).Where("phone = ?", phone)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// CheckTenantUserUsernameExists checks if username is already used globally (excluding blank).
func CheckTenantUserUsernameExists(db *gorm.DB, username string, excludeID uint) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, nil
	}
	q := ActiveTenantUsers(db).Where("username = ?", username)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}
