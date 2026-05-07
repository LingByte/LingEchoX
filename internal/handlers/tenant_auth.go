package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"strings"

	"github.com/LingByte/SoulNexus/cmd/bootstrap"
	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

type tenantLoginReq struct {
	TenantSlug string `json:"tenantSlug" binding:"required"`
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
}

// tenantLogin authenticates a TenantUser within a tenant (identified by slug).
func (h *Handlers) tenantLogin(c *gin.Context) {
	var req tenantLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "请求参数无效", err.Error())
		return
	}

	slug := normalizeTenantSlug(req.TenantSlug)
	if !validTenantSlug(slug) {
		response.Fail(c, "无效的组织标识", nil)
		return
	}

	if bootstrap.GlobalKeyManager == nil {
		response.Fail(c, "服务未就绪：JWT 密钥未初始化", nil)
		return
	}

	tenant, err := models.GetTenantBySlug(h.db, slug)
	if err != nil {
		response.Fail(c, "组织不存在或已被停用", nil)
		return
	}
	if tenant.Status != "" && tenant.Status != "active" {
		response.Fail(c, "组织已暂停服务", nil)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	user, err := models.GetActiveTenantUserByEmail(h.db, tenant.ID, email)
	if err != nil {
		response.Fail(c, "邮箱或密码错误", nil)
		return
	}
	if user.Status != models.TenantUserStatusActive {
		response.Fail(c, "账号不可用", nil)
		return
	}
	if !access.CheckPassword(user.PasswordHash, req.Password) {
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
		"token":     token,
		"expiresIn": int(tenantAccessTokenTTL.Seconds()),
		"tenant":    tenantPublic(tenant),
		"user":      tenantUserPublic(user),
	})
}
