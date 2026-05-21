package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"errors"
	"strings"

	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tenantLoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	TotpCode string `json:"totpCode"`
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

	email := utils.TrimLower(req.Email)
	user, err := models.GetActiveTenantUserByEmailGlobal(h.db, email)
	if err == nil {
		tenant, terr := models.GetActiveTenantByID(h.db, user.TenantID)
		if terr != nil {
			response.Fail(c, "组织不存在或已被停用", nil)
			return
		}
		if tenant.Status != "" && tenant.Status != "active" {
			response.Fail(c, "组织已暂停服务", nil)
			return
		}
		if user.Status != constants.TenantUserStatusActive {
			response.Fail(c, "账号不可用", nil)
			return
		}
		if !access.CheckPassword(user.PasswordHash, req.Password) {
			response.Fail(c, "邮箱或密码错误", nil)
			return
		}
		if user.TOTPEnabled && strings.TrimSpace(user.TOTPSecret) != "" {
			if !access.ValidateTOTP(req.TotpCode, user.TOTPSecret) {
				if strings.TrimSpace(req.TotpCode) == "" {
					response.Fail(c, "需要两步验证码", gin.H{"needsTotp": true})
				} else {
					response.Fail(c, "两步验证码错误", gin.H{"needsTotp": true})
				}
				return
			}
		}
		_ = models.RecordTenantUserLogin(h.db, user.ID, c.ClientIP())
		token, terr := signTenantAccessToken(h.db, user, tenant)
		if terr != nil {
			response.Fail(c, "签发登录凭证失败", nil)
			return
		}
		codes, _ := models.ListEffectivePermissionCodesForTenantUser(h.db, user.ID)
		response.Success(c, "success", gin.H{
			"principal":       "tenant",
			"token":           token,
			"expiresIn":       int(constants.TenantAccessTokenTTL.Seconds()),
			"tenant":          models.TenantPublic(tenant),
			"user":            models.TenantUserPublic(h.db, user),
			"permissionCodes": codes,
		})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		response.AbortWithStatusJSON(c, 500, err)
		return
	}

	adm, err := models.GetActivePlatformAdminByEmail(h.db, email)
	if err != nil {
		response.Fail(c, "邮箱或密码错误", nil)
		return
	}
	if !access.CheckPassword(adm.PasswordHash, req.Password) {
		response.Fail(c, "邮箱或密码错误", nil)
		return
	}

	token, err := access.SignPlatformAccessTokenWithKey(access.PlatformPayload{
		AdminID: adm.ID,
		Email:   adm.Email,
		Role:    constants.JWTRolePlatformSuper,
	}, bootstrap.GlobalKeyManager, constants.TenantAccessTokenTTL)
	if err != nil {
		response.Fail(c, "签发登录凭证失败", nil)
		return
	}

	response.Success(c, "success", gin.H{
		"principal":     "platform",
		"token":         token,
		"expiresIn":     int(constants.TenantAccessTokenTTL.Seconds()),
		"platformAdmin": models.PlatformAdminPublic(adm),
	})
}
