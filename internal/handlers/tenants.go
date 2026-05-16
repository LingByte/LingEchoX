package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tenantRegisterReq struct {
	CompanyName       string `json:"companyName" binding:"required,min=2,max=128"`
	AdminEmail        string `json:"adminEmail" binding:"required,email"`
	AdminPassword     string `json:"adminPassword" binding:"required,min=8,max=128"`
	AdminDisplayName  string `json:"adminDisplayName"`
	TenantDescription string `json:"tenantDescription"`
	MaxUserCount      int    `json:"maxUserCount"`
}

func signTenantAccessToken(db *gorm.DB, user models.TenantUser, tenant models.Tenant) (string, error) {
	if bootstrap.GlobalKeyManager == nil {
		return "", errors.New("jwt key manager not initialized")
	}
	role := constants.JWTRoleTenantMember
	if ok, _ := models.TenantUserHasRoleName(db, user.ID, models.TenantAdminRoleName); ok {
		role = constants.JWTRoleTenantAdmin
	}
	p := access.AccessPayload{
		UserID:     user.ID,
		TenantID:   tenant.ID,
		TenantSlug: tenant.Slug,
		Email:      user.Email,
		Role:       role,
	}
	return access.SignAccessTokenWithKey(p, bootstrap.GlobalKeyManager, constants.TenantAccessTokenTTL)
}

// registerTenant creates a tenant, default admin role, first admin user, and returns JWT.
func (h *Handlers) registerTenant(c *gin.Context) {
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
	if !utils.IsEmail(req.AdminEmail) {
		response.Fail(c, "邮箱格式错误", nil)
		return
	}
	takenMail, mailErr := models.CheckTenantUserEmailExists(h.db, req.AdminEmail, 0)
	if mailErr != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, mailErr)
		return
	}
	if takenMail {
		response.Fail(c, "该邮箱已被注册", nil)
		return
	}

	tenant, user, role, err := models.ProvisionTenantWithAdmin(h.db, models.TenantProvisionInput{
		CompanyName: req.CompanyName, AdminEmail: req.AdminEmail, AdminDisplayName: req.AdminDisplayName,
		TenantDescription: req.TenantDescription, MaxUserCount: req.MaxUserCount,
	}, hash, "register")
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	_ = models.RecordTenantUserLogin(h.db, user.ID, c.ClientIP())

	token, err := signTenantAccessToken(h.db, user, tenant)
	if err != nil {
		response.Fail(c, "签发登录凭证失败", nil)
		return
	}

	pc, _ := models.ListEffectivePermissionCodesForTenantUser(h.db, user.ID)
	response.Success(c, "success", gin.H{
		"principal":       "tenant",
		"token":           token,
		"expiresIn":       int(constants.TenantAccessTokenTTL.Seconds()),
		"tenant":          models.TenantPublic(tenant),
		"user":            models.TenantUserPublic(h.db, user),
		"permissionCodes": pc,
		"roleCreated":     models.TenantAdminRoleName,
		"roleId":          role.ID,
	})
}

type tenantPlatformUpdateReq struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Status       string           `json:"status"`
	ContactEmail string           `json:"contactEmail"`
	MaxUserCount int              `json:"maxUserCount"`
	AsrConfig    *json.RawMessage `json:"asrConfig"`
	TtsConfig    *json.RawMessage `json:"ttsConfig"`
	LlmConfig    *json.RawMessage `json:"llmConfig"`
}

func (h *Handlers) getTenant(c *gin.Context) {
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
	response.Success(c, "success", gin.H{"tenant": models.TenantPlatformDetail(t)})
}

func (h *Handlers) createTenantPlatform(c *gin.Context) {
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
	takenMail, mailErr := models.CheckTenantUserEmailExists(h.db, req.AdminEmail, 0)
	if mailErr != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, mailErr)
		return
	}
	if takenMail {
		response.Fail(c, "该邮箱已被注册", nil)
		return
	}
	tenant, user, role, err := models.ProvisionTenantWithAdmin(h.db, models.TenantProvisionInput{
		CompanyName: req.CompanyName, AdminEmail: req.AdminEmail, AdminDisplayName: req.AdminDisplayName,
		TenantDescription: req.TenantDescription, MaxUserCount: req.MaxUserCount,
	}, hash, "platform")
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{
		"tenant":    models.TenantPlatformDetail(tenant),
		"adminUser": models.TenantUserPublic(h.db, user),
		"roleId":    role.ID,
	})
}

func (h *Handlers) updateTenantPlatform(c *gin.Context) {
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
	if st != "" && st != constants.TenantStatusActive && st != constants.TenantStatusSuspended {
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
	if req.AsrConfig != nil || req.TtsConfig != nil || req.LlmConfig != nil {
		if err := models.PatchTenantAIConfigJSON(h.db, id, req.AsrConfig, req.TtsConfig, req.LlmConfig, op); err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
	}
	t, err := models.GetActiveTenantByID(h.db, id)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"tenant": models.TenantPlatformDetail(t)})
}

func (h *Handlers) deleteTenantPlatform(c *gin.Context) {
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
