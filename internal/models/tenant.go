package models

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	pinyinLib "github.com/mozillazg/go-pinyin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// Tenant is one SaaS customer organization (multi-tenant root).
type Tenant struct {
	BaseModel
	Name         string         `json:"name" gorm:"size:128;index;not null;comment:租户名称"`
	Slug         string         `json:"slug" gorm:"size:64;uniqueIndex;not null;comment:租户标识"`
	Description  string         `json:"description,omitempty" gorm:"size:512;comment:描述"`
	Status       string         `json:"status" gorm:"size:24;index;not null;default:active;comment:租户状态"`
	ContactEmail string         `json:"contactEmail" gorm:"size:128;index;comment:联系邮箱"`
	MaxUserCount int            `json:"maxUserCount" gorm:"default:5;comment:最大成员数"`
	AsrConfig    datatypes.JSON `json:"asrConfig,omitempty" gorm:"column:asr_config;comment:ASR配置JSON"`
	TtsConfig    datatypes.JSON `json:"ttsConfig,omitempty" gorm:"column:tts_config;comment:TTS配置JSON"`
	LlmConfig    datatypes.JSON `json:"llmConfig,omitempty" gorm:"column:llm_config;comment:LLM配置JSON"`
}

func (Tenant) TableName() string {
	return constants.TENANT_TABLE_NAME
}

// TenantProvisionInput is the payload for tenant self-register or platform provisioning.
type TenantProvisionInput struct {
	CompanyName       string
	AdminEmail        string
	AdminDisplayName  string
	TenantDescription string
	MaxUserCount      int
}

// TenantPublic builds a JSON-safe tenant map (Snowflake id as string for JS).
func TenantPublic(t Tenant) map[string]any {
	return map[string]any{
		"id":           strconv.FormatUint(uint64(t.ID), 10),
		"name":         t.Name,
		"slug":         t.Slug,
		"description":  t.Description,
		"status":       t.Status,
		"contactEmail": t.ContactEmail,
		"maxUserCount": t.MaxUserCount,
		"createdAt":    t.CreatedAt,
	}
}

// TenantPlatformDetail includes per-tenant AI JSON (platform admin APIs only).
func TenantPlatformDetail(t Tenant) map[string]any {
	h := TenantPublic(t)
	h["asrConfig"] = utils.JSONValueFromBytes(t.AsrConfig)
	h["ttsConfig"] = utils.JSONValueFromBytes(t.TtsConfig)
	h["llmConfig"] = utils.JSONValueFromBytes(t.LlmConfig)
	return h
}

// BaseSlugFromCompanyName derives a slug base from company name (pinyin + normalization).
func BaseSlugFromCompanyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tenant"
	}
	args := pinyinLib.NewArgs()
	segs := pinyinLib.LazyPinyin(name, args)
	raw := strings.Join(segs, "-")
	slug := utils.NormalizeTenantSlug(raw)
	if slug == "" {
		slug = utils.NormalizeTenantSlug(name)
	}
	if len(slug) < 2 {
		slug = "tenant"
	}
	if len(slug) > 62 {
		slug = strings.TrimRight(strings.TrimSpace(slug[:62]), "-")
	}
	return slug
}

// AllocateUniqueTenantSlug picks an unused slug by appending a random two-digit suffix.
func AllocateUniqueTenantSlug(db *gorm.DB, base string) (string, error) {
	base = strings.Trim(base, "-")
	if base == "" {
		base = "tenant"
	}
	for attempts := 0; attempts < 80; attempts++ {
		nBig, err := rand.Int(rand.Reader, big.NewInt(100))
		if err != nil {
			return "", err
		}
		suffix := fmt.Sprintf("%02d", nBig.Int64())
		candidate := base + suffix
		if len(candidate) > 64 {
			trunc := base[:64-len(suffix)]
			trunc = strings.TrimRight(strings.TrimSpace(trunc), "-")
			if len(trunc) < 2 {
				trunc = "te"
			}
			candidate = trunc + suffix
		}
		if len(candidate) < 2 || len(candidate) > 64 {
			continue
		}
		if !utils.ValidTenantSlug(candidate) {
			continue
		}
		ok, err := TenantSlugTaken(db, candidate)
		if err != nil {
			return "", err
		}
		if !ok {
			return candidate, nil
		}
	}
	return "", errors.New("could not allocate unique tenant slug")
}

// ProvisionTenantWithAdmin creates tenant, system admin role, first admin user, and full permission bindings.
func ProvisionTenantWithAdmin(db *gorm.DB, req TenantProvisionInput, passwordHash string, attachTag string) (tenant Tenant, user TenantUser, role TenantRole, err error) {
	email := utils.TrimLower(req.AdminEmail)
	display := strings.TrimSpace(req.AdminDisplayName)
	if display == "" {
		display = strings.Split(email, "@")[0]
	}
	slugBase := BaseSlugFromCompanyName(req.CompanyName)
	err = db.Transaction(func(tx *gorm.DB) error {
		slug, e := AllocateUniqueTenantSlug(tx, slugBase)
		if e != nil {
			return e
		}
		t := &Tenant{
			Name:         strings.TrimSpace(req.CompanyName),
			Slug:         slug,
			Description:  strings.TrimSpace(req.TenantDescription),
			Status:       constants.TenantStatusActive,
			ContactEmail: email,
			MaxUserCount: req.MaxUserCount,
		}
		t.SetCreateInfo(attachTag)
		if t.MaxUserCount <= 0 {
			t.MaxUserCount = 5
		}
		if !utils.IsEmail(t.ContactEmail) {
			return errors.New("invalid contact email")
		}
		if e := CreateTenant(tx, t); e != nil {
			return e
		}
		tenant = *t

		roleRow := &TenantRole{
			TenantID:    tenant.ID,
			Name:        TenantAdminRoleName,
			Description: "组织管理员，注册时自动创建",
			IsSystem:    true,
		}
		roleRow.SetCreateInfo(attachTag)
		if e := CreateTenantRole(tx, roleRow); e != nil {
			return e
		}
		role = *roleRow

		u := &TenantUser{
			TenantID:     tenant.ID,
			Email:        email,
			PasswordHash: passwordHash,
			DisplayName:  display,
			Status:       TenantUserStatusActive,
			Source:       TenantUserSourceRegister,
		}
		u.SetCreateInfo(attachTag)
		if e := CreateTenantUser(tx, u); e != nil {
			return e
		}
		user = *u

		tur := &TenantUserRole{
			TenantUserID: user.ID,
			RoleID:       role.ID,
		}
		tur.SetCreateInfo(attachTag)
		if e := CreateTenantUserRole(tx, tur); e != nil {
			return e
		}
		return AttachAllPermissionsToRole(tx, role.ID, attachTag)
	})
	return tenant, user, role, err
}

// CreateTenant inserts a tenant row.
func CreateTenant(db *gorm.DB, t *Tenant) error {
	return db.Create(t).Error
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
func UpdateActiveTenant(db *gorm.DB, id uint, name, description, status, contactEmail string, maxUserCount int, updateBy string) error {
	meta := BaseModel{}
	meta.SetUpdateInfo(updateBy)
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	if !utils.IsEmpty(name) {
		updates["name"] = strings.TrimSpace(name)
	}
	if description != "" {
		updates["description"] = strings.TrimSpace(description)
	}
	status = utils.Trim(status)
	if status != "" {
		updates["status"] = status
	}
	if contactEmail != "" {
		updates["contact_email"] = strings.TrimSpace(contactEmail)
	}
	if maxUserCount > 0 {
		updates["max_user_count"] = maxUserCount
	}
	if len(updates) <= 1 {
		return nil
	}
	return db.Model(&Tenant{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// SoftDeleteTenant soft-deletes one tenant row (platform ops).
func SoftDeleteTenant(db *gorm.DB, id uint, updateBy string) error {
	meta := BaseModel{}
	meta.SoftDelete(updateBy)
	updates := map[string]any{
		"updated_at": meta.UpdatedAt,
		"deleted_at": meta.DeletedAt,
	}
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	return db.Model(&Tenant{}).Where("id = ?", id).Updates(updates).Error
}

// PatchTenantAIConfigJSON updates optional AI config columns.
func PatchTenantAIConfigJSON(db *gorm.DB, id uint, asr, tts, llm *json.RawMessage, updateBy string) error {
	patch := map[string]any{
		"updated_at": time.Now(),
		"update_by":  updateBy,
	}
	if asr != nil {
		patch["asr_config"] = datatypes.JSON(utils.CloneRawMessage(*asr))
	}
	if tts != nil {
		patch["tts_config"] = datatypes.JSON(utils.CloneRawMessage(*tts))
	}
	if llm != nil {
		patch["llm_config"] = datatypes.JSON(utils.CloneRawMessage(*llm))
	}
	if len(patch) <= 1 {
		return nil
	}
	return db.Model(&Tenant{}).Where("id = ?", id).Updates(patch).Error
}
