package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
)

type trunkWriteReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prefix      string `json:"prefix"`
	LocalAddr   string `json:"local_addr"`
	ProviderID  uint   `json:"providerId"`
}

type trunkNumberWriteReq struct {
	TrunkID          uint    `json:"trunkId"`
	Number           string  `json:"number"`
	Prefix           string  `json:"prefix"`
	Description      string  `json:"description"`
	Direction        string  `json:"direction"`
	Status           string  `json:"status"`
	Concurrent       uint    `json:"concurrent"`
	CallInConcurrent uint    `json:"callInConcurrent"`
	IsTransferRelay  bool    `json:"isTransferRelay"`
	EffectiveTime    *string `json:"effectiveTime"`
	ExpirationTime   *string `json:"expirationTime"`
	ProviderID       uint    `json:"providerId"`
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
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	list, total, err := models.ListTrunksPage(h.db, 0, page, size, c.Query("name"))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) createTrunk(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
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
	row := models.Trunk{
		TenantID:    0,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Prefix:      strings.TrimSpace(req.Prefix),
		LocalAddr:   strings.TrimSpace(req.LocalAddr),
		ProviderId:  req.ProviderID,
	}
	if err := h.db.Create(&row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) getTrunk(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetTrunkByID(h.db, uint(id))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) updateTrunk(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	if _, err := models.GetTrunkByIDBare(h.db, uint(id)); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	updates := map[string]any{
		"name":        name,
		"description": strings.TrimSpace(req.Description),
		"prefix":      strings.TrimSpace(req.Prefix),
		"local_addr":  strings.TrimSpace(req.LocalAddr),
		"provider_id": req.ProviderID,
	}
	if err := h.db.Model(&models.Trunk{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	row, _ := models.GetTrunkByID(h.db, uint(id))
	response.Success(c, "success", row)
}

func (h *Handlers) deleteTrunk(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if _, err := models.GetTrunkByIDBare(h.db, uint(id)); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if err := models.SoftDeleteTrunkCascade(h.db, uint(id)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) listTrunkNumbers(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	var trunkID uint
	if s := strings.TrimSpace(c.Query("trunkId")); s != "" {
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			response.Fail(c, "invalid trunkId", nil)
			return
		}
		trunkID = uint(v)
	}
	if trunkID > 0 {
		if _, err := models.GetTrunkByIDBare(h.db, trunkID); err != nil {
			response.Fail(c, "trunk not found", nil)
			return
		}
	}
	list, total, err := models.ListTrunkNumbersPage(h.db, 0, trunkID, page, size, c.Query("number"))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) createTrunkNumber(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
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
	row := models.TrunkNumber{
		TrunkID:          req.TrunkID,
		Number:           number,
		Prefix:           strings.TrimSpace(req.Prefix),
		Description:      strings.TrimSpace(req.Description),
		Direction:        strings.TrimSpace(req.Direction),
		Status:           strings.TrimSpace(req.Status),
		Concurrent:       req.Concurrent,
		CallInConcurrent: req.CallInConcurrent,
		IsTransferRelay:  req.IsTransferRelay,
		EffectiveTime:    eff,
		ExpirationTime:   exp,
		ProviderId:       req.ProviderID,
	}
	if err := h.db.Create(&row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) getTrunkNumber(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetTrunkNumberByID(h.db, uint(id))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) updateTrunkNumber(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	if _, err := models.GetTrunkNumberByID(h.db, uint(id)); err != nil {
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
	updates := map[string]any{
		"trunk_id":             req.TrunkID,
		"number":               number,
		"prefix":               strings.TrimSpace(req.Prefix),
		"description":          strings.TrimSpace(req.Description),
		"direction":            strings.TrimSpace(req.Direction),
		"status":               strings.TrimSpace(req.Status),
		"concurrent":           req.Concurrent,
		"call_in_concurrent":   req.CallInConcurrent,
		"is_transfer_relay":    req.IsTransferRelay,
		"effective_time":       eff,
		"expiration_time":      exp,
		"provider_id":          req.ProviderID,
	}
	if err := h.db.Model(&models.TrunkNumber{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	row, _ := models.GetTrunkNumberByID(h.db, uint(id))
	response.Success(c, "success", row)
}

func (h *Handlers) deleteTrunkNumber(c *gin.Context) {
	_, ok := requirePlatformAdmin(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if _, err := models.GetTrunkNumberByID(h.db, uint(id)); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if err := models.SoftDeleteTrunkNumberByID(h.db, uint(id)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}
