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
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
			prevDash = false
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
	out := strings.Trim(b.String(), "-")
	out = strings.TrimSuffix(strings.TrimPrefix(out, "-"), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
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

func tenantUserPublic(u models.TenantUser) gin.H {
	return gin.H{
		"id":          u.ID,
		"tenantId":    u.TenantID,
		"email":       u.Email,
		"phone":       u.Phone,
		"username":    u.Username,
		"displayName": u.DisplayName,
		"status":      u.Status,
	}
}

func tenantPublic(t models.Tenant) gin.H {
	return gin.H{
		"id":     t.ID,
		"name":   t.Name,
		"slug":   t.Slug,
		"status": t.Status,
	}
}

// registerTenant creates a tenant, default admin role, first admin user, and returns JWT.
func (h *Handlers) registerTenant(c *gin.Context) {
	var req tenantRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "请求参数无效", err.Error())
		return
	}

	slugBase := baseSlugFromCompanyName(req.CompanyName)

	if bootstrap.GlobalKeyManager == nil {
		response.Fail(c, "服务未就绪：JWT 密钥未初始化", nil)
		return
	}

	hash, err := access.HashPassword(req.AdminPassword)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.AdminEmail))

	takenMail, mailErr := models.CheckTenantUserEmailExists(h.db, 0, email, 0)
	if mailErr != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, mailErr)
		return
	}
	if takenMail {
		response.Fail(c, "该邮箱已被注册", nil)
		return
	}

	display := strings.TrimSpace(req.AdminDisplayName)
	if display == "" {
		display = strings.Split(email, "@")[0]
	}

	var tenant models.Tenant
	var user models.TenantUser
	var role models.TenantRole

	err = h.db.Transaction(func(tx *gorm.DB) error {
		slug, err := allocateUniqueTenantSlug(tx, slugBase)
		if err != nil {
			return err
		}
		t := &models.Tenant{
			Name:        strings.TrimSpace(req.CompanyName),
			Slug:        slug,
			Description: strings.TrimSpace(req.TenantDescription),
			Status:      "active",
		}
		if err := models.CreateTenant(tx, t); err != nil {
			return err
		}
		tenant = *t

		roleRow := &models.TenantRole{
			TenantID:    tenant.ID,
			Name:        models.TenantAdminRoleName,
			Description: "组织管理员，注册时自动创建",
			IsSystem:    true,
		}
		if err := models.CreateTenantRole(tx, roleRow); err != nil {
			return err
		}
		role = *roleRow

		u := &models.TenantUser{
			TenantID:     tenant.ID,
			Email:        email,
			PasswordHash: hash,
			DisplayName:  display,
			Status:       models.TenantUserStatusActive,
		}
		if err := models.CreateTenantUser(tx, u); err != nil {
			return err
		}
		user = *u

		tur := &models.TenantUserRole{
			TenantUserID: user.ID,
			RoleID:       role.ID,
		}
		return models.CreateTenantUserRole(tx, tur)
	})
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	token, err := issueTenantAccessToken(h.db, user, tenant)
	if err != nil {
		response.Fail(c, "签发登录凭证失败", nil)
		return
	}

	response.Success(c, "success", gin.H{
		"principal":   "tenant",
		"token":       token,
		"expiresIn":   int(tenantAccessTokenTTL.Seconds()),
		"tenant":      tenantPublic(tenant),
		"user":        tenantUserPublic(user),
		"roleCreated": models.TenantAdminRoleName,
		"roleId":      role.ID,
	})
}
