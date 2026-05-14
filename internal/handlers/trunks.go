package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	TenantID uint `json:"tenantId"`
	// VoiceDialogWsURL 入局呼入语音对话网关（ws/wss）；空则平台默认。
	VoiceDialogWsURL string `json:"voiceDialogWsUrl"`
	// OutboundTrunkNumberID 当本号码作为「呼入 DID」需要对外发起呼叫（盲转/外呼回流）时，
	// 改用哪条 TrunkNumber 作为出局网关 + 主叫；0 表示用本号码自己。
	OutboundTrunkNumberID uint `json:"outboundTrunkNumberId"`
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
			if v, err := strconv.ParseUint(s, 10, 32); err == nil {
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
		"outbound_trunk_number_id": req.OutboundTrunkNumberID,
	}
	if err := h.db.Model(&models.TrunkNumber{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	row, _ := models.GetTrunkNumberByID(h.db, id)
	response.Success(c, "success", row)
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
