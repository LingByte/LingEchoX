package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/stores"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

type tenantUserCreateReq struct {
	Email       string `json:"email" binding:"required,email"`
	Phone       string `json:"phone"`
	Username    string `json:"username"`
	Password    string `json:"password"` // plain text; always hashed server-side
	DisplayName string `json:"displayName"`
	Status      string `json:"status"` // active | disabled | pending
	Source      string `json:"source"`
}

type tenantUserUpdateReq struct {
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
}

type tenantUserStatusReq struct {
	Status string `json:"status" binding:"required"` // active | disabled | pending
}

func (h *Handlers) listTenantUsers(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)

	list, total, err := models.ListTenantUsersPage(h.db, tenantID, page, size, c.Query("status"), c.Query("search"))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	pub := make([]gin.H, 0, len(list))
	for _, row := range list {
		pub = append(pub, h.tenantUserPublic(row))
	}
	response.Success(c, "success", gin.H{"list": pub, "total": total, "page": page, "size": size})
}

func (h *Handlers) getTenantUser(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetActiveTenantUserByID(h.db, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if row.TenantID != tenantID {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(row))
}

func (h *Handlers) createTenantUser(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var req tenantUserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}

	email := utils.TrimLower(req.Email)
	if email == "" {
		response.Fail(c, "email required", nil)
		return
	}

	// Check for duplicates
	exists, _ := models.CheckTenantUserEmailExists(h.db, tenantID, email, 0)
	if exists {
		response.Fail(c, "email already exists", nil)
		return
	}
	if req.Phone != "" {
		exists, _ = models.CheckTenantUserPhoneExists(h.db, tenantID, strings.TrimSpace(req.Phone), 0)
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
	}
	if req.Username != "" {
		exists, _ = models.CheckTenantUserUsernameExists(h.db, tenantID, strings.TrimSpace(req.Username), 0)
		if exists {
			response.Fail(c, "username already exists", nil)
			return
		}
	}

	pw := strings.TrimSpace(req.Password)
	if len(pw) < 8 {
		response.Fail(c, "password required (min 8 chars)", nil)
		return
	}
	hash, err := access.HashPassword(pw)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = models.TenantUserStatusActive
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = models.TenantUserSourceManual
	}

	user := &models.TenantUser{
		TenantID:     tenantID,
		Email:        email,
		Phone:        strings.TrimSpace(req.Phone),
		Username:     strings.TrimSpace(req.Username),
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Status:       status,
		Source:       source,
	}
	if op := acdOperator(c); op != "" {
		user.SetCreateInfo(op)
	}

	if err := models.CreateTenantUser(h.db, user); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(*user))
}

func (h *Handlers) updateTenantUser(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	var req tenantUserUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}

	// Get existing user to check tenant
	existing, err := models.GetActiveTenantUserByID(h.db, id)
	if err != nil {
		response.Fail(c, "user not found", nil)
		return
	}
	if existing.TenantID != tenantID {
		response.Fail(c, "user not found", nil)
		return
	}

	updates := make(map[string]any)
	if req.Email != "" {
		email := utils.TrimLower(req.Email)
		exists, _ := models.CheckTenantUserEmailExists(h.db, existing.TenantID, email, uint(id))
		if exists {
			response.Fail(c, "email already exists", nil)
			return
		}
		updates["email"] = email
	}
	if req.Phone != "" {
		phone := strings.TrimSpace(req.Phone)
		exists, _ := models.CheckTenantUserPhoneExists(h.db, existing.TenantID, phone, uint(id))
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
		updates["phone"] = phone
	}
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		exists, _ := models.CheckTenantUserUsernameExists(h.db, existing.TenantID, username, uint(id))
		if exists {
			response.Fail(c, "username already exists", nil)
			return
		}
		updates["username"] = username
	}
	if req.DisplayName != "" {
		updates["display_name"] = strings.TrimSpace(req.DisplayName)
	}
	if req.Status != "" {
		updates["status"] = strings.TrimSpace(req.Status)
	}

	if len(updates) == 0 {
		response.Fail(c, "no fields to update", nil)
		return
	}

	n, err := models.UpdateTenantUser(h.db, uint(id), updates, acdOperator(c))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if n == 0 {
		response.Fail(c, "user not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) updateTenantUserStatus(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	existing, err := models.GetActiveTenantUserByID(h.db, id)
	if err != nil || existing.TenantID != tenantID {
		response.Fail(c, "user not found", nil)
		return
	}

	var req tenantUserStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}

	status := strings.TrimSpace(req.Status)
	if status != models.TenantUserStatusActive && status != models.TenantUserStatusDisabled && status != models.TenantUserStatusPending {
		response.Fail(c, "invalid status", nil)
		return
	}

	n, err := models.UpdateTenantUserStatus(h.db, id, status, acdOperator(c))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if n == 0 {
		response.Fail(c, "user not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id, "status": status})
}

func (h *Handlers) deleteTenantUser(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	existing, getErr := models.GetActiveTenantUserByID(h.db, id)
	if getErr != nil || existing.TenantID != tenantID {
		response.Fail(c, "not found", nil)
		return
	}

	rows, err := models.SoftDeleteTenantUserByID(h.db, id, acdOperator(c))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if rows == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) restoreTenantUser(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	existing, getErr := models.GetTenantUserByID(h.db, id)
	if getErr != nil || existing.TenantID != tenantID {
		response.Fail(c, "not found", nil)
		return
	}

	rows, err := models.RestoreTenantUser(h.db, id, acdOperator(c))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if rows == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) getTenantUserStats(c *gin.Context) {
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	total, _ := models.CountTenantUsers(h.db, tenantID)
	active, _ := models.CountTenantUsersByStatus(h.db, tenantID, models.TenantUserStatusActive)
	disabled, _ := models.CountTenantUsersByStatus(h.db, tenantID, models.TenantUserStatusDisabled)
	pending, _ := models.CountTenantUsersByStatus(h.db, tenantID, models.TenantUserStatusPending)
	response.Success(c, "success", gin.H{
		"total":    total,
		"active":   active,
		"disabled": disabled,
		"pending":  pending,
	})
}

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

func (h *Handlers) currentTenantUser(c *gin.Context) (models.TenantUser, bool) {
	uid := middleware.AuthUserID(c)
	tid := middleware.AuthTenantID(c)
	if uid == 0 || tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return models.TenantUser{}, false
	}
	u, err := models.GetActiveTenantUserByID(h.db, uid)
	if err != nil || u.TenantID != tid {
		response.Fail(c, "unauthorized", nil)
		return models.TenantUser{}, false
	}
	return u, true
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
			"platformAdmin": platformAdminPublic(row),
		})
		return
	}

	u, ok := h.currentTenantUser(c)
	if !ok {
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
		"user":            h.tenantUserPublic(u),
		"tenant":          tenantPublic(tenant),
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
		response.Success(c, "success", platformAdminPublic(row))
		return
	}

	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	var req updateMeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	updates := map[string]any{}
	if v := strings.TrimSpace(req.DisplayName); v != "" {
		updates["display_name"] = v
	}
	if req.Phone != "" {
		phone := strings.TrimSpace(req.Phone)
		exists, _ := models.CheckTenantUserPhoneExists(h.db, u.TenantID, phone, u.ID)
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
		updates["phone"] = phone
	}
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		exists, _ := models.CheckTenantUserUsernameExists(h.db, u.TenantID, username, u.ID)
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
	if _, err := models.UpdateTenantUser(h.db, u.ID, updates, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(next))
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

	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	var req updateMyPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
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
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{"password_hash": hash}, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
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

func pickAvatarExt(contentType string) string {
	switch strings.ToLower(contentType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func (h *Handlers) uploadMeAvatar(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		response.Fail(c, "平台管理员不支持上传头像", nil)
		return
	}
	u, ok := h.currentTenantUser(c)
	if !ok {
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
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if len(body) > maxAvatarBytes {
		response.Fail(c, "图片不能超过 2MB", nil)
		return
	}
	ct := http.DetectContentType(body)
	ext := pickAvatarExt(ct)
	if ext == "" {
		response.Fail(c, "仅支持 JPEG、PNG、GIF、WebP", nil)
		return
	}

	key := path.Join("avatars", "t"+strconv.FormatUint(uint64(u.TenantID), 10), "u"+strconv.FormatUint(uint64(u.ID), 10)+ext)
	st := stores.Default()
	bucket := strings.TrimSpace(config.GlobalConfig.Services.Storage.Bucket)
	if err := st.Write(bucket, key, bytes.NewReader(body)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	proto := c.Request.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if c.Request.TLS != nil {
			proto = "https"
		}
	}
	url := stores.ResolveUploadPublicURL(st, bucket, key, config.GlobalConfig.Services.Storage.PublicBase, proto, c.Request.Host)
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{"avatar_url": url}, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"avatarUrl": url, "user": h.tenantUserPublic(next)})
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
	u, ok := h.currentTenantUser(c)
	if !ok {
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
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
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
	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	if u.TOTPEnabled {
		response.Fail(c, "两步验证已开启", nil)
		return
	}
	var req enableTotpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
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
	}, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(next))
}

func (h *Handlers) disableTotp(c *gin.Context) {
	if middleware.AuthPlatformAdminID(c) > 0 {
		response.Fail(c, "平台管理员不支持两步验证", nil)
		return
	}
	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	if !u.TOTPEnabled || strings.TrimSpace(u.TOTPSecret) == "" {
		response.Fail(c, "两步验证未开启", nil)
		return
	}
	var req disableTotpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
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
	}, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(next))
}
