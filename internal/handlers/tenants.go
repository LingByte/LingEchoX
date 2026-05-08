package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/LingByte/SoulNexus/cmd/bootstrap"
	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"github.com/gin-gonic/gin"
	pinyinLib "github.com/mozillazg/go-pinyin"
	"gorm.io/gorm"
)

const (
	tenantAccessTokenTTL = 24 * time.Hour
	jwtRoleTenantAdmin   = "tenant_admin"
	jwtRoleTenantMember  = "tenant_member"
)

type tenantRegisterReq struct {
	CompanyName       string `json:"companyName" binding:"required,min=2,max=128"`
	AdminEmail        string `json:"adminEmail" binding:"required,email"`
	AdminPassword     string `json:"adminPassword" binding:"required,min=8,max=128"`
	AdminDisplayName  string `json:"adminDisplayName"`
	TenantDescription string `json:"tenantDescription"`
	MaxUserCount      int    `json:"maxUserCount"`
}

func baseSlugFromCompanyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tenant"
	}
	args := pinyinLib.NewArgs()
	segs := pinyinLib.LazyPinyin(name, args)
	raw := strings.Join(segs, "-")
	slug := normalizeTenantSlug(raw)
	if slug == "" {
		slug = normalizeTenantSlug(name)
	}
	if len(slug) < 2 {
		slug = "tenant"
	}
	if len(slug) > 62 {
		slug = strings.TrimRight(strings.TrimSpace(slug[:62]), "-")
	}
	return slug
}

func allocateUniqueTenantSlug(db *gorm.DB, base string) (string, error) {
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
		if !validTenantSlug(candidate) {
			continue
		}
		ok, err := models.TenantSlugTaken(db, candidate)
		if err != nil {
			return "", err
		}
		if !ok {
			return candidate, nil
		}
	}
	return "", errors.New("could not allocate unique tenant slug")
}

func normalizeTenantSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if b.Len() > 0 && !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func validTenantSlug(slug string) bool {
	if len(slug) < 2 || len(slug) > 64 {
		return false
	}
	for i, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			continue
		case r == '-':
			if i == 0 || i == len(slug)-1 {
				return false
			}
			continue
		default:
			return false
		}
	}
	return true
}

func tenantJWTDisplayRole(db *gorm.DB, user models.TenantUser) string {
	if ok, _ := models.TenantUserHasRoleName(db, user.ID, models.TenantAdminRoleName); ok {
		return jwtRoleTenantAdmin
	}
	return jwtRoleTenantMember
}

func tenantLoginPayload(db *gorm.DB, user models.TenantUser, tenant models.Tenant) access.AccessPayload {
	return access.AccessPayload{
		UserID:     user.ID,
		TenantID:   tenant.ID,
		TenantSlug: tenant.Slug,
		Email:      user.Email,
		Role:       tenantJWTDisplayRole(db, user),
	}
}

func issueTenantAccessToken(db *gorm.DB, user models.TenantUser, tenant models.Tenant) (string, error) {
	if bootstrap.GlobalKeyManager == nil {
		return "", errors.New("jwt key manager not initialized")
	}
	p := tenantLoginPayload(db, user, tenant)
	return access.SignAccessTokenWithKey(p, bootstrap.GlobalKeyManager, tenantAccessTokenTTL)
}

func (h *Handlers) tenantUserPublic(u models.TenantUser) gin.H {
	out := gin.H{
		"id":          u.ID,
		"tenantId":    u.TenantID,
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
	if gs, err := models.ListTenantGroupsForUser(h.db, u.ID); err == nil && len(gs) > 0 {
		gpub := make([]gin.H, 0, len(gs))
		for _, g := range gs {
			gpub = append(gpub, gin.H{"id": g.ID, "name": g.Name, "isDefault": g.IsDefault})
		}
		out["tenantGroups"] = gpub
		out["tenantGroup"] = gin.H{"id": gs[0].ID, "name": gs[0].Name}
	}
	if roles, err := models.ListTenantRolesForUser(h.db, u.ID); err == nil && len(roles) > 0 {
		rpub := make([]gin.H, 0, len(roles))
		for _, r := range roles {
			rpub = append(rpub, gin.H{"id": r.ID, "name": r.Name, "isSystem": r.IsSystem})
		}
		out["roles"] = rpub
	}
	return out
}

func tenantPublic(t models.Tenant) gin.H {
	return gin.H{
		"id":           t.ID,
		"name":         t.Name,
		"slug":         t.Slug,
		"description":  t.Description,
		"status":       t.Status,
		"contactEmail": t.ContactEmail,
		"maxUserCount": t.MaxUserCount,
		"createdAt":    t.CreatedAt,
	}
}

// provisionTenantWithAdmin creates tenant, system「管理员」role with full catalog permissions, first admin user, and role binding.
func provisionTenantWithAdmin(db *gorm.DB, req tenantRegisterReq, passwordHash string, attachTag string) (tenant models.Tenant, user models.TenantUser, role models.TenantRole, err error) {
	email := utils.TrimLower(req.AdminEmail)
	display := strings.TrimSpace(req.AdminDisplayName)
	if display == "" {
		display = strings.Split(email, "@")[0]
	}
	slugBase := baseSlugFromCompanyName(req.CompanyName)
	err = db.Transaction(func(tx *gorm.DB) error {
		slug, e := allocateUniqueTenantSlug(tx, slugBase)
		if e != nil {
			return e
		}
		t := &models.Tenant{
			Name:         strings.TrimSpace(req.CompanyName),
			Slug:         slug,
			Description:  strings.TrimSpace(req.TenantDescription),
			Status:       "active",
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
		if e := models.CreateTenant(tx, t); e != nil {
			return e
		}
		tenant = *t

		roleRow := &models.TenantRole{
			TenantID:    tenant.ID,
			Name:        models.TenantAdminRoleName,
			Description: "组织管理员，注册时自动创建",
			IsSystem:    true,
		}
		roleRow.SetCreateInfo(attachTag)
		if e := models.CreateTenantRole(tx, roleRow); e != nil {
			return e
		}
		role = *roleRow

		u := &models.TenantUser{
			TenantID:     tenant.ID,
			Email:        email,
			PasswordHash: passwordHash,
			DisplayName:  display,
			Status:       models.TenantUserStatusActive,
			Source:       models.TenantUserSourceRegister,
		}
		u.SetCreateInfo(attachTag)
		if e := models.CreateTenantUser(tx, u); e != nil {
			return e
		}
		user = *u

		tur := &models.TenantUserRole{
			TenantUserID: user.ID,
			RoleID:       role.ID,
		}
		tur.SetCreateInfo(attachTag)
		if e := models.CreateTenantUserRole(tx, tur); e != nil {
			return e
		}
		return models.AttachAllPermissionsToRole(tx, role.ID, attachTag)
	})
	return tenant, user, role, err
}

// registerTenant creates a tenant, default admin role, first admin user, and returns JWT.
func (h *Handlers) registerTenant(c *gin.Context) {
	var req tenantRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "请求参数无效", err.Error())
		return
	}

	if bootstrap.GlobalKeyManager == nil {
		response.Fail(c, "服务未就绪：JWT 密钥未初始化", nil)
		return
	}

	hash, err := access.HashPassword(req.AdminPassword)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	email := utils.TrimLower(req.AdminEmail)

	takenMail, mailErr := models.CheckTenantUserEmailExists(h.db, 0, email, 0)
	if mailErr != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, mailErr)
		return
	}
	if takenMail {
		response.Fail(c, "该邮箱已被注册", nil)
		return
	}

	tenant, user, role, err := provisionTenantWithAdmin(h.db, req, hash, "register")
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	_ = models.RecordTenantUserLogin(h.db, user.ID, c.ClientIP())

	token, err := issueTenantAccessToken(h.db, user, tenant)
	if err != nil {
		response.Fail(c, "签发登录凭证失败", nil)
		return
	}

	pc, _ := models.ListEffectivePermissionCodesForTenantUser(h.db, user.ID)
	response.Success(c, "success", gin.H{
		"principal":       "tenant",
		"token":           token,
		"expiresIn":       int(tenantAccessTokenTTL.Seconds()),
		"tenant":          tenantPublic(tenant),
		"user":            h.tenantUserPublic(user),
		"permissionCodes": pc,
		"roleCreated":     models.TenantAdminRoleName,
		"roleId":          role.ID,
	})
}

type tenantPlatformUpdateReq struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	ContactEmail string `json:"contactEmail"`
	MaxUserCount int    `json:"maxUserCount"`
}

func (h *Handlers) getTenant(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	t, err := models.GetActiveTenantByID(h.db, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"tenant": tenantPublic(t)})
}

// createTenantPlatform provisions a tenant (platform console); same payload as public register but returns JSON without issuing tenant JWT.
func (h *Handlers) createTenantPlatform(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	var req tenantRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "请求参数无效", err.Error())
		return
	}
	hash, err := access.HashPassword(req.AdminPassword)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	email := utils.TrimLower(req.AdminEmail)
	takenMail, mailErr := models.CheckTenantUserEmailExists(h.db, 0, email, 0)
	if mailErr != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, mailErr)
		return
	}
	if takenMail {
		response.Fail(c, "该邮箱已被注册", nil)
		return
	}
	tenant, user, role, err := provisionTenantWithAdmin(h.db, req, hash, "platform")
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{
		"tenant":    tenantPublic(tenant),
		"adminUser": h.tenantUserPublic(user),
		"roleId":    role.ID,
	})
}

func (h *Handlers) updateTenantPlatform(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if _, err := models.GetActiveTenantByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	var req tenantPlatformUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	st := strings.TrimSpace(req.Status)
	if st != "" && st != "active" && st != "suspended" {
		response.Fail(c, "invalid status", nil)
		return
	}
	if req.ContactEmail != "" && !utils.IsEmail(req.ContactEmail) {
		response.Fail(c, "invalid contactEmail", nil)
		return
	}
	op := "platform"
	if err := models.UpdateActiveTenant(
		h.db,
		id,
		strings.TrimSpace(req.Name),
		req.Description,
		st,
		req.ContactEmail,
		req.MaxUserCount,
		op,
	); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	t, err := models.GetActiveTenantByID(h.db, id)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"tenant": tenantPublic(t)})
}

func (h *Handlers) deleteTenantPlatform(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if _, err := models.GetActiveTenantByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if err := models.SoftDeleteTenant(h.db, id, "platform"); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}
