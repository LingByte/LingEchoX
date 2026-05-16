package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/stores"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/welcomeaudio"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxWelcomeWAVBytes 上限与 pkg/welcomeaudio.MaxBytes 对齐（16MiB）；
// 上传端点先在 HTTP 层硬截断，避免恶意客户端在内存里囤大 multipart payload。
const maxWelcomeWAVBytes = 16 << 20

// trunkWriteReq 没有 providerCode 字段：供应商编码由后端在 BeforeCreate 钩子中生成，
// 前端即便提交也会被丢弃，更新接口不支持修改 provider_code。
type trunkWriteReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prefix      string `json:"prefix"`
	LocalAddr   string `json:"local_addr"`
}

type trunkNumberWriteReq struct {
	TrunkID           uint    `json:"trunkId"`
	Number            string  `json:"number"`
	CallerDisplayName string  `json:"callerDisplayName"`
	Prefix            string  `json:"prefix"`
	Description       string  `json:"description"`
	Direction         string  `json:"direction"`
	Status            string  `json:"status"`
	Concurrent        uint    `json:"concurrent"`
	CallInConcurrent  uint    `json:"callInConcurrent"`
	IsTransferRelay   bool    `json:"isTransferRelay"`
	EffectiveTime     *string `json:"effectiveTime"`
	ExpirationTime    *string `json:"expirationTime"`
	// TenantID 平台将号码分配给租户；0 表示待分配池。
	// 因 Tenant.ID 使用 Snowflake（>2^53），JSON 必须以字符串编解码以防止 JS Number 精度丢失。
	TenantID uint `json:"tenantId,string"`
	// VoiceDialogWsURL 入局呼入语音对话网关（ws/wss）；空则平台默认。
	VoiceDialogWsURL string `json:"voiceDialogWsUrl"`
	// OutboundTrunkNumberID 当本号码作为「呼入 DID」需要对外发起呼叫（盲转/外呼回流）时，
	// 改用哪条 TrunkNumber 作为出局网关 + 主叫；0 表示用本号码自己。
	OutboundTrunkNumberID uint `json:"outboundTrunkNumberId"`
	// WelcomeAudioUrl 入局欢迎语音频 URL（http/https）。空则按 SIP_WELCOME_WAV_PATH
	// env / scripts/welcome.wav 兜底；兜底不存在则跳过欢迎语阶段。写入时后端会做
	// 可达性 + WAV magic 双重校验，避免落库非音频或外链失效 URL。
	WelcomeAudioUrl string `json:"welcomeAudioUrl"`
	// TransferRingingUrl 转人工/转接阶段播放给主叫的回铃 WAV URL（http/https）。
	// 空则回退 SIP_TRANSFER_RINGING_WAV_PATH env / scripts/ringing.wav。
	// 与 WelcomeAudioUrl 共用同一套校验逻辑（welcomeaudio.ValidateURL）。
	TransferRingingUrl string `json:"transferRingingUrl"`
}

// normalizeTrunkNumberAudioURL 处理写入路径上任何「中继音频 URL」字段
// （welcomeAudioUrl / transferRingingUrl 共用）：空值直接返回（合法，走
// fallback）；非空时调用 welcomeaudio.ValidateURL 做 scheme / 可达性 /
// RIFF-WAVE magic 三重校验。
//
// 校验在 5 秒 deadline 内完成（管理后台交互预算），失败时把底层错误原样
// 透传给前端，便于运营定位是配置错（scheme 不对）还是网络错（CDN 504）。
// fieldName 仅用于错误信息提示，方便用户在浏览器看到具体字段名。
func normalizeTrunkNumberAudioURL(ctx context.Context, fieldName, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if err := welcomeaudio.ValidateURL(ctx, s); err != nil {
		return "", fmt.Errorf("%s invalid: %w", fieldName, err)
	}
	return s, nil
}

// validateOutboundTrunkNumberBinding 校验外呼号码绑定：必须与本号码同租户、direction 允许外呼、
// 且不指向本号码自身（避免循环）。tenantID == 0（平台号池）时禁止绑定。
func validateOutboundTrunkNumberBinding(db *gorm.DB, selfID, outboundID, tenantID uint) error {
	if outboundID == 0 {
		return nil
	}
	if tenantID == 0 {
		return fmt.Errorf("outboundTrunkNumberId 需先把本号码分配给某个租户")
	}
	if outboundID == selfID {
		return fmt.Errorf("outboundTrunkNumberId 不能指向号码自身")
	}
	var n models.TrunkNumber
	if err := db.Where("id = ? AND tenant_id = ?", outboundID, tenantID).First(&n).Error; err != nil {
		return fmt.Errorf("outboundTrunkNumberId 不属于同一租户或不存在")
	}
	dir := strings.ToLower(strings.TrimSpace(n.Direction))
	if dir != "outbound" && dir != "both" && dir != "all" {
		return fmt.Errorf("outboundTrunkNumberId 对应号码 direction 必须是 outbound/both/all")
	}
	return nil
}

func normalizeTrunkNumberVoiceDialogWs(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") {
		return "", fmt.Errorf("voiceDialogWsUrl must be ws:// or wss:// with host")
	}
	return s, nil
}

func parseOptionalRFC3339(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (h *Handlers) listTrunks(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)
	list, total, err := models.ListTrunksPage(h.db, 0, page, size, c.Query("name"))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) createTrunk(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	var req trunkWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		response.Fail(c, "name required", nil)
		return
	}
	row := models.Trunk{
		TenantID:    0,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Prefix:      strings.TrimSpace(req.Prefix),
		LocalAddr:   strings.TrimSpace(req.LocalAddr),
	}
	if err := h.db.Create(&row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) getTrunk(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetTrunkByID(h.db, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) updateTrunk(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	var req trunkWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		response.Fail(c, "name required", nil)
		return
	}
	if _, err := models.GetTrunkByIDBare(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	updates := map[string]any{
		"name":        name,
		"description": strings.TrimSpace(req.Description),
		"prefix":      strings.TrimSpace(req.Prefix),
		"local_addr":  strings.TrimSpace(req.LocalAddr),
	}
	if err := h.db.Model(&models.Trunk{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	row, _ := models.GetTrunkByID(h.db, id)
	response.Success(c, "success", row)
}

func (h *Handlers) deleteTrunk(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if _, err := models.GetTrunkByIDBare(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if err := models.SoftDeleteTrunkCascade(h.db, id); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) listTrunkNumbers(c *gin.Context) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)
	var trunkID uint
	if s := strings.TrimSpace(c.Query("trunkId")); s != "" {
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			response.Fail(c, "invalid trunkId", nil)
			return
		}
		trunkID = uint(v)
	}

	if middleware.AuthPlatformAdminID(c) > 0 {
		if trunkID > 0 {
			if _, err := models.GetTrunkByIDBare(h.db, trunkID); err != nil {
				response.Fail(c, "trunk not found", nil)
				return
			}
		}
		var tenantFilter uint
		if s := strings.TrimSpace(c.Query("tenantId")); s != "" {
			// Tenant.ID is a Snowflake (>2^32), so parse as 64-bit; truncating to
			// 32-bit silently dropped the filter and showed all tenants' numbers.
			if v, err := strconv.ParseUint(s, 10, 64); err == nil {
				tenantFilter = uint(v)
			}
		}
		list, total, err := models.ListTrunkNumbersPage(h.db, tenantFilter, trunkID, page, size, c.Query("number"))
		if err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
		response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
		return
	}

	tid := middleware.CurrentTenantID(c)
	list, total, err := models.ListTrunkNumbersForTenant(h.db, tid, page, size, c.Query("number"))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) createTrunkNumber(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	var req trunkNumberWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	number := strings.TrimSpace(req.Number)
	if req.TrunkID == 0 || number == "" {
		response.Fail(c, "trunkId and number required", nil)
		return
	}
	if _, err := models.GetTrunkByIDBare(h.db, req.TrunkID); err != nil {
		response.Fail(c, "trunk not found", nil)
		return
	}
	eff, err := parseOptionalRFC3339(req.EffectiveTime)
	if err != nil {
		response.Fail(c, "invalid effectiveTime", err.Error())
		return
	}
	exp, err := parseOptionalRFC3339(req.ExpirationTime)
	if err != nil {
		response.Fail(c, "invalid expirationTime", err.Error())
		return
	}
	voiceWS, err := normalizeTrunkNumberVoiceDialogWs(req.VoiceDialogWsURL)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	welcomeURL, err := normalizeTrunkNumberAudioURL(c.Request.Context(), "welcomeAudioUrl", req.WelcomeAudioUrl)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	ringingURL, err := normalizeTrunkNumberAudioURL(c.Request.Context(), "transferRingingUrl", req.TransferRingingUrl)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	if err := validateOutboundTrunkNumberBinding(h.db, 0, req.OutboundTrunkNumberID, req.TenantID); err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	row := models.TrunkNumber{
		TrunkID:               req.TrunkID,
		TenantID:              req.TenantID,
		Number:                number,
		CallerDisplayName:     strings.TrimSpace(req.CallerDisplayName),
		Prefix:                strings.TrimSpace(req.Prefix),
		Description:           strings.TrimSpace(req.Description),
		Direction:             strings.TrimSpace(req.Direction),
		Status:                strings.TrimSpace(req.Status),
		Concurrent:            req.Concurrent,
		CallInConcurrent:      req.CallInConcurrent,
		IsTransferRelay:       req.IsTransferRelay,
		EffectiveTime:         eff,
		ExpirationTime:        exp,
		VoiceDialogWSURL:      voiceWS,
		WelcomeAudioURL:       welcomeURL,
		TransferRingingURL:    ringingURL,
		OutboundTrunkNumberID: req.OutboundTrunkNumberID,
	}
	if err := h.db.Create(&row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) getTrunkNumber(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetTrunkNumberByID(h.db, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) updateTrunkNumber(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	var req trunkNumberWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	number := strings.TrimSpace(req.Number)
	if req.TrunkID == 0 || number == "" {
		response.Fail(c, "trunkId and number required", nil)
		return
	}
	if _, err := models.GetTrunkNumberByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if _, err := models.GetTrunkByIDBare(h.db, req.TrunkID); err != nil {
		response.Fail(c, "trunk not found", nil)
		return
	}
	eff, err := parseOptionalRFC3339(req.EffectiveTime)
	if err != nil {
		response.Fail(c, "invalid effectiveTime", err.Error())
		return
	}
	exp, err := parseOptionalRFC3339(req.ExpirationTime)
	if err != nil {
		response.Fail(c, "invalid expirationTime", err.Error())
		return
	}
	voiceWS, err := normalizeTrunkNumberVoiceDialogWs(req.VoiceDialogWsURL)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	welcomeURL, err := normalizeTrunkNumberAudioURL(c.Request.Context(), "welcomeAudioUrl", req.WelcomeAudioUrl)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	ringingURL, err := normalizeTrunkNumberAudioURL(c.Request.Context(), "transferRingingUrl", req.TransferRingingUrl)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	if err := validateOutboundTrunkNumberBinding(h.db, id, req.OutboundTrunkNumberID, req.TenantID); err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	updates := map[string]any{
		"trunk_id":                 req.TrunkID,
		"tenant_id":                req.TenantID,
		"number":                   number,
		"caller_display_name":      strings.TrimSpace(req.CallerDisplayName),
		"prefix":                   strings.TrimSpace(req.Prefix),
		"description":              strings.TrimSpace(req.Description),
		"direction":                strings.TrimSpace(req.Direction),
		"status":                   strings.TrimSpace(req.Status),
		"concurrent":               req.Concurrent,
		"call_in_concurrent":       req.CallInConcurrent,
		"is_transfer_relay":        req.IsTransferRelay,
		"effective_time":           eff,
		"expiration_time":          exp,
		"voice_dialog_ws_url":      voiceWS,
		"welcome_audio_url":        welcomeURL,
		"transfer_ringing_url":     ringingURL,
		"outbound_trunk_number_id": req.OutboundTrunkNumberID,
	}
	if err := h.db.Model(&models.TrunkNumber{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	row, _ := models.GetTrunkNumberByID(h.db, id)
	response.Success(c, "success", row)
}

// uploadTrunkNumberAudio 返回一个绑定到具体「音频用途」的 Gin handler：
// kind 只影响 stores 落盘目录前缀（welcome-audio / transfer-ringing-audio），
// 校验逻辑、HTTP 限流、平台管理员权限要求完全相同。
//
// 此端点只接受 RIFF/WAVE 文件 —— 文件签名校验（ValidateBytes）+ HTTP
// 层 16MiB 硬截断。文件名扩展名只是给 stores 落盘留参考，校验唯一权威
// 来源是 WAV magic。
//
// 平台管理员权限（路由层 middleware 已限制）；上传成功的 URL 不绑定到
// 任何号码 —— 后续 PUT /trunk-numbers/:id 把 URL 写到相应字段时才会做
// "二次校验"，从而上传 + 配置可以分两步操作。
func (h *Handlers) uploadTrunkNumberAudio(kind string) gin.HandlerFunc {
	// 把 kind 校验提前到注册期（构造时），避免每个请求都做字符串比较。
	switch kind {
	case "welcome-audio", "transfer-ringing-audio":
	default:
		panic(fmt.Sprintf("uploadTrunkNumberAudio: unsupported kind %q", kind))
	}
	return func(c *gin.Context) {
		h.handleTrunkNumberAudioUpload(c, kind)
	}
}

func (h *Handlers) handleTrunkNumberAudioUpload(c *gin.Context, kind string) {
	// Platform admin only (enforced by route middleware in handlers/urls.go).
	_ = middleware.AuthPlatformAdminID // keep import in case future hardening checks role explicitly
	fh, err := c.FormFile("file")
	if err != nil || fh == nil {
		response.Fail(c, "请选择 WAV 文件", nil)
		return
	}
	if fh.Size > 0 && fh.Size > maxWelcomeWAVBytes {
		response.Fail(c, fmt.Sprintf("WAV 文件不能超过 %d MiB", maxWelcomeWAVBytes>>20), nil)
		return
	}
	src, err := fh.Open()
	if err != nil {
		response.Fail(c, "无法读取文件", nil)
		return
	}
	defer src.Close()
	body, err := io.ReadAll(io.LimitReader(src, maxWelcomeWAVBytes+1))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if len(body) > maxWelcomeWAVBytes {
		response.Fail(c, fmt.Sprintf("WAV 文件不能超过 %d MiB", maxWelcomeWAVBytes>>20), nil)
		return
	}
	if err := welcomeaudio.ValidateBytes(body); err != nil {
		response.Fail(c, "仅支持 WAV (RIFF/WAVE) 文件", err.Error())
		return
	}
	// Key 按 "<kind>/<yyyymm>/<时间戳>.wav" 分目录便于后续生命周期管理
	// (S3 lifecycle / 本地按月归档清理)。kind 已在 uploadTrunkNumberAudio
	// 注册期校验过白名单，这里直接拼接安全。
	now := time.Now().UTC()
	key := path.Join(
		kind,
		fmt.Sprintf("%04d%02d", now.Year(), int(now.Month())),
		strings.ReplaceAll(strconv.FormatInt(now.UnixNano(), 36), "-", "")+".wav",
	)
	st := stores.Default()
	if err := st.Write(key, bytes.NewReader(body)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	// URL 解析顺序与 tenant_users.uploadMeAvatar 保持一致：
	//   1) stores.PublicObjectURL 给的绝对 URL（含 STORAGE_PUBLIC_BASE_URL 兜底）
	//   2) 落到 /uploads/<key> 由网关回源
	u := strings.TrimSpace(stores.PublicObjectURL(st, key))
	if lower := strings.ToLower(u); !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		proto := c.Request.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			proto = "http"
			if c.Request.TLS != nil {
				proto = "https"
			}
		}
		if host := strings.TrimSpace(c.Request.Host); host != "" {
			u = proto + "://" + host + "/uploads/" + strings.TrimPrefix(key, "/")
		} else {
			u = "/uploads/" + strings.TrimPrefix(key, "/")
		}
	}
	response.Success(c, "success", gin.H{"url": u, "key": key, "size": len(body)})
}

func (h *Handlers) deleteTrunkNumber(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	id, err := utils.ParseID(c.Param("id"))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if _, err := models.GetTrunkNumberByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if err := models.SoftDeleteTrunkNumberByID(h.db, id); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}
