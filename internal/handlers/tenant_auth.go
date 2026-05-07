package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"errors"
	"strings"

	"github.com/LingByte/SoulNexus/cmd/bootstrap"
	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const jwtRolePlatformSuper = "platform_super"

type tenantLoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// tenantLogin authenticates a tenant user or a platform admin using the same email + password endpoint.
func (h *Handlers) tenantLogin(c *gin.Context) {
	var req tenantLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "请求参数无效", err.Error())
		return
	}

	if bootstrap.GlobalKeyManager == nil {
		response.Fail(c, "服务未就绪：JWT 密钥未初始化", nil)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	user, err := models.GetActiveTenantUserByEmailGlobal(h.db, email)
	if err == nil {
		h.finishTenantLogin(c, req.Password, user)
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		response.AbortWithStatusJSON(c, 500, err)
		return
	}

	adm, err := models.GetActivePlatformAdminByEmail(h.db, email)
	if err != nil {
		response.Fail(c, "邮箱或密码错误", nil)
		return
	}
	if adm.Status != "" && adm.Status != models.PlatformAdminStatusActive {
		response.Fail(c, "账号不可用", nil)
		return
	}
	if !access.CheckPassword(adm.PasswordHash, req.Password) {
		response.Fail(c, "邮箱或密码错误", nil)
		return
	}

	token, err := access.SignPlatformAccessTokenWithKey(access.PlatformPayload{
		AdminID: adm.ID,
		Email:   adm.Email,
		Role:    jwtRolePlatformSuper,
	}, bootstrap.GlobalKeyManager, tenantAccessTokenTTL)
	if err != nil {
		response.Fail(c, "签发登录凭证失败", nil)
		return
	}

	response.Success(c, "success", gin.H{
		"principal":     "platform",
		"token":         token,
		"expiresIn":     int(tenantAccessTokenTTL.Seconds()),
		"platformAdmin": platformAdminPublic(adm),
	})
}

func (h *Handlers) finishTenantLogin(c *gin.Context, password string, user models.TenantUser) {
	tenant, err := models.GetActiveTenantByID(h.db, user.TenantID)
	if err != nil {
		response.Fail(c, "组织不存在或已被停用", nil)
		return
	}
	if tenant.Status != "" && tenant.Status != "active" {
		response.Fail(c, "组织已暂停服务", nil)
		return
	}
	if user.Status != models.TenantUserStatusActive {
		response.Fail(c, "账号不可用", nil)
		return
	}
	if !access.CheckPassword(user.PasswordHash, password) {
		response.Fail(c, "邮箱或密码错误", nil)
		return
	}

	_ = models.UpdateTenantUserLastLogin(h.db, user.ID)

	token, err := issueTenantAccessToken(h.db, user, tenant)
	if err != nil {
		response.Fail(c, "签发登录凭证失败", nil)
		return
	}

	response.Success(c, "success", gin.H{
		"principal": "tenant",
		"token":     token,
		"expiresIn": int(tenantAccessTokenTTL.Seconds()),
		"tenant":    tenantPublic(tenant),
		"user":      tenantUserPublic(user),
	})
}

func platformAdminPublic(a models.PlatformAdmin) gin.H {
	return gin.H{
		"id":          a.ID,
		"email":       a.Email,
		"displayName": a.DisplayName,
		"status":      a.Status,
	}
}
