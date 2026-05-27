package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"errors"
	"strings"

	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/i18n"
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
		response.FailI18n(c, i18n.KeyInvalidParams, err.Error())
		return
	}

	if bootstrap.GlobalKeyManager == nil {
		response.FailI18n(c, i18n.KeyAuthJWTNotReady, nil)
		return
	}

	email := utils.TrimLower(req.Email)
	user, err := models.GetActiveTenantUserByEmailGlobal(h.db, email)
	if err == nil {
		tenant, terr := models.GetActiveTenantByID(h.db, user.TenantID)
		if terr != nil {
			response.FailI18n(c, i18n.KeyTenantNotFound, nil)
			return
		}
		if tenant.Status != "" && tenant.Status != "active" {
			response.FailI18n(c, i18n.KeyTenantSuspended, nil)
			return
		}
		if user.Status != constants.TenantUserStatusActive {
			response.FailI18n(c, i18n.KeyTenantUserUnavailable, nil)
			return
		}
		if !access.CheckPassword(user.PasswordHash, req.Password) {
			response.FailI18n(c, i18n.KeyAuthInvalidCredentials, nil)
			return
		}
		if user.TOTPEnabled && strings.TrimSpace(user.TOTPSecret) != "" {
			if !access.ValidateTOTP(req.TotpCode, user.TOTPSecret) {
				if strings.TrimSpace(req.TotpCode) == "" {
					response.FailI18n(c, i18n.KeyAuthNeedsTotp, gin.H{"needsTotp": true})
				} else {
					response.FailI18n(c, i18n.KeyAuthInvalidTotp, gin.H{"needsTotp": true})
				}
				return
			}
		}
		_ = models.RecordTenantUserLogin(h.db, user.ID, c.ClientIP())
		token, terr := signTenantAccessToken(h.db, user, tenant)
		if terr != nil {
			response.FailI18n(c, i18n.KeyTenantSignTokenFailed, nil)
			return
		}
		codes, _ := models.ListEffectivePermissionCodesForTenantUser(h.db, user.ID)
		response.SuccessI18n(c, i18n.KeySuccess, gin.H{
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
		response.FailI18n(c, i18n.KeyAuthInvalidCredentials, nil)
		return
	}
	if !access.CheckPassword(adm.PasswordHash, req.Password) {
		response.FailI18n(c, i18n.KeyAuthInvalidCredentials, nil)
		return
	}
	if adm.TOTPEnabled && strings.TrimSpace(adm.TOTPSecret) != "" {
		if !access.ValidateTOTP(req.TotpCode, adm.TOTPSecret) {
			if strings.TrimSpace(req.TotpCode) == "" {
				response.FailI18n(c, i18n.KeyAuthNeedsTotp, gin.H{"needsTotp": true})
			} else {
				response.FailI18n(c, i18n.KeyAuthInvalidTotp, gin.H{"needsTotp": true})
			}
			return
		}
	}

	token, err := access.SignPlatformAccessTokenWithKey(access.PlatformPayload{
		AdminID: adm.ID,
		Email:   adm.Email,
		Role:    constants.JWTRolePlatformSuper,
	}, bootstrap.GlobalKeyManager, constants.TenantAccessTokenTTL)
	if err != nil {
		response.FailI18n(c, i18n.KeyTenantSignTokenFailed, nil)
		return
	}

	response.SuccessI18n(c, i18n.KeySuccess, gin.H{
		"principal":     "platform",
		"token":         token,
		"expiresIn":     int(constants.TenantAccessTokenTTL.Seconds()),
		"platformAdmin": models.PlatformAdminPublic(adm),
	})
}
