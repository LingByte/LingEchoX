package handlers

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/stores"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

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
