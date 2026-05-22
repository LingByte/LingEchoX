package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// /me — self-service handlers for the currently authenticated principal
// (tenant user OR platform admin). Sibling file tenant_users.go owns
// the admin-facing CRUD on the same table. Split to keep each file
// focused on one concern.
//
// Endpoints in this file:
//   - GET    /me                 → getMe
//   - PUT    /me                 → updateMe
//   - PUT    /me/password        → updateMyPassword
//   - POST   /me/avatar          → uploadMeAvatar
//   - POST   /me/totp/setup      → setupTotp
//   - POST   /me/totp/enable     → enableTotp
//   - POST   /me/totp/disable    → disableTotp
//   - POST   /logout             → logout

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/stores"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

// ──────────────────────────────────────────────────────────────────────────────
// /me — Current User Profile & Security
// ──────────────────────────────────────────────────────────────────────────────

type updateMeReq struct {
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
	Username    string `json:"username"`
}

type updateMyPasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=128"`
}

func (h *Handlers) getMe(c *gin.Context) {
	if aid := middleware.AuthPlatformAdminID(c); aid > 0 {
		var row models.PlatformAdmin
		if err := h.db.Where("id = ?", aid).First(&row).Error; err != nil {
			response.Fail(c, "not found", nil)
			return
		}
		response.Success(c, "success", gin.H{
			"principal":     "platform",
			"platformAdmin": models.PlatformAdminPublic(row),
		})
		return
	}

	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var tenant models.Tenant
	if err := h.db.Where("id = ?", u.TenantID).First(&tenant).Error; err != nil {
		response.Fail(c, "tenant not found", nil)
		return
	}
	codes, _ := models.ListEffectivePermissionCodesForTenantUser(h.db, u.ID)
	response.Success(c, "success", gin.H{
		"principal":       "tenant",
		"user":            models.TenantUserPublic(h.db, u),
		"tenant":          models.TenantPublic(tenant),
		"permissionCodes": codes,
	})
}

func (h *Handlers) updateMe(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		aid := middleware.AuthPlatformAdminID(c)
		var req updateMeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, "invalid body", err.Error())
			return
		}
		updates := map[string]any{}
		if v := strings.TrimSpace(req.DisplayName); v != "" {
			updates["display_name"] = v
		}
		if len(updates) == 0 {
			response.Fail(c, "no fields to update", nil)
			return
		}
		if err := h.db.Model(&models.PlatformAdmin{}).
			Where("id = ?", aid).
			Updates(updates).Error; err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
		var row models.PlatformAdmin
		if err := h.db.Where("id = ?", aid).First(&row).Error; err != nil {
			response.Fail(c, "not found", nil)
			return
		}
		response.Success(c, "success", models.PlatformAdminPublic(row))
		return
	}

	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var req updateMeReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	updates := map[string]any{}
	if v := strings.TrimSpace(req.DisplayName); v != "" {
		updates["display_name"] = v
	}
	if req.Phone != "" {
		phone := strings.TrimSpace(req.Phone)
		exists, _ := models.CheckTenantUserPhoneExists(h.db, phone, u.ID)
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
		updates["phone"] = phone
	}
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		exists, _ := models.CheckTenantUserUsernameExists(h.db, username, u.ID)
		if exists {
			response.Fail(c, "username already exists", nil)
			return
		}
		updates["username"] = username
	}
	if len(updates) == 0 {
		response.Fail(c, "no fields to update", nil)
		return
	}
	if _, err := models.UpdateTenantUser(h.db, u.ID, updates, middleware.AuditOperator(c)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", models.TenantUserPublic(h.db, next))
}

func (h *Handlers) updateMyPassword(c *gin.Context) {
	if aid := middleware.AuthPlatformAdminID(c); aid > 0 {
		var req updateMyPasswordReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, "invalid body", err.Error())
			return
		}
		var row models.PlatformAdmin
		if err := h.db.Where("id = ?", aid).First(&row).Error; err != nil {
			response.Fail(c, "not found", nil)
			return
		}
		if !access.CheckPassword(row.PasswordHash, req.OldPassword) {
			response.Fail(c, "old password incorrect", nil)
			return
		}
		if req.OldPassword == req.NewPassword {
			response.Fail(c, "new password must differ from old password", nil)
			return
		}
		hash, err := access.HashPassword(req.NewPassword)
		if err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
		if err := h.db.Model(&models.PlatformAdmin{}).Where("id = ?", aid).
			Update("password_hash", hash).Error; err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
		response.Success(c, "success", gin.H{"id": aid})
		return
	}

	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var req updateMyPasswordReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	if !access.CheckPassword(u.PasswordHash, req.OldPassword) {
		response.Fail(c, "old password incorrect", nil)
		return
	}
	if req.OldPassword == req.NewPassword {
		response.Fail(c, "new password must differ from old password", nil)
		return
	}
	hash, err := access.HashPassword(req.NewPassword)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{"password_hash": hash}, middleware.AuditOperator(c)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"id": u.ID})
}

func (h *Handlers) logout(c *gin.Context) {
	response.Success(c, "success", gin.H{"loggedOut": true})
}

// ──────────────────────────────────────────────────────────────────────────────
// Avatar & TOTP
// ──────────────────────────────────────────────────────────────────────────────

const maxAvatarBytes = 2 << 20

func (h *Handlers) uploadMeAvatar(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		response.Fail(c, "平台管理员不支持上传头像", nil)
		return
	}
	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	fh, err := c.FormFile("file")
	if err != nil || fh == nil {
		response.Fail(c, "请选择图片文件", nil)
		return
	}
	src, err := fh.Open()
	if err != nil {
		response.Fail(c, "无法读取文件", nil)
		return
	}
	defer src.Close()

	body, err := io.ReadAll(io.LimitReader(src, maxAvatarBytes+1))
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	if len(body) > maxAvatarBytes {
		response.Fail(c, "图片不能超过 2MB", nil)
		return
	}
	ct := http.DetectContentType(body)
	ext := utils.PickImageExtFromContentType(ct)
	if ext == "" {
		response.Fail(c, "仅支持 JPEG、PNG、GIF、WebP", nil)
		return
	}

	key := path.Join("avatars", "t"+strconv.FormatUint(uint64(u.TenantID), 10), "u"+strconv.FormatUint(uint64(u.ID), 10)+ext)
	st := stores.Default()
	if err := st.Write(key, bytes.NewReader(body)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	// 头像可下载 URL 解析顺序：
	//   1) 后端自带绝对 URL（含 STORAGE_PUBLIC_BASE_URL 兜底）；
	//   2) 落到 /uploads/<key> 由网关回源。
	url := ginutil.UploadURL(c, key)
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{"avatar_url": url}, middleware.AuditOperator(c)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"avatarUrl": url, "user": models.TenantUserPublic(h.db, next)})
}

type enableTotpReq struct {
	Secret string `json:"secret" binding:"required"`
	Code   string `json:"code" binding:"required"`
}

type disableTotpReq struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

func (h *Handlers) setupTotp(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		response.Fail(c, "平台管理员不支持两步验证", nil)
		return
	}
	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	if u.TOTPEnabled {
		response.Fail(c, "已开启两步验证，请先关闭后再重新绑定", nil)
		return
	}
	issuer := strings.TrimSpace(config.GlobalConfig.Server.Name)
	if issuer == "" {
		issuer = access.DefaultTOTPIssuer
	}
	account := strings.TrimSpace(u.Email)
	if account == "" {
		account = strconv.FormatUint(uint64(u.ID), 10)
	}
	setup, err := access.GenerateTOTPSetup(issuer, account, 0)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{
		"secret":    setup.Secret,
		"url":       setup.URL,
		"qrDataUrl": setup.QRDataURL,
	})
}

func (h *Handlers) enableTotp(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		response.Fail(c, "平台管理员不支持两步验证", nil)
		return
	}
	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	if u.TOTPEnabled {
		response.Fail(c, "两步验证已开启", nil)
		return
	}
	var req enableTotpReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	if !access.ValidateTOTP(req.Code, req.Secret) {
		response.Fail(c, "验证码无效", nil)
		return
	}
	secret := strings.TrimSpace(req.Secret)
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{
		"totp_secret":  secret,
		"totp_enabled": true,
	}, middleware.AuditOperator(c)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", models.TenantUserPublic(h.db, next))
}

func (h *Handlers) disableTotp(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		response.Fail(c, "平台管理员不支持两步验证", nil)
		return
	}
	u, err := models.GetAuthenticatedTenantUser(h.db, middleware.AuthUserID(c), middleware.AuthTenantID(c))
	if err != nil {
		response.Fail(c, "unauthorized", nil)
		return
	}
	if !u.TOTPEnabled || strings.TrimSpace(u.TOTPSecret) == "" {
		response.Fail(c, "两步验证未开启", nil)
		return
	}
	var req disableTotpReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	if !access.CheckPassword(u.PasswordHash, req.Password) {
		response.Fail(c, "密码错误", nil)
		return
	}
	if !access.ValidateTOTP(req.Code, u.TOTPSecret) {
		response.Fail(c, "验证码无效", nil)
		return
	}
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{
		"totp_secret":  "",
		"totp_enabled": false,
	}, middleware.AuditOperator(c)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", models.TenantUserPublic(h.db, next))
}
