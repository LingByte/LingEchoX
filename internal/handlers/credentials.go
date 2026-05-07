package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type credentialCreateReq struct {
	Name    string `json:"name"`
	AllowIP string `json:"allowIp"` // 逗号分隔，可为空表示不限制
}

// createCredential issues AK/SK for the tenant (human JWT only).
func (h *Handlers) createCredential(c *gin.Context) {
	if middleware.AuthUserID(c) == 0 {
		response.Fail(c, "forbidden", nil)
		return
	}
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var req credentialCreateReq
	_ = c.ShouldBindJSON(&req)

	skBytes := make([]byte, 32)
	if _, err := rand.Read(skBytes); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	secretKey := hex.EncodeToString(skBytes)
	accessKey := "ak_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	row := &models.Credential{
		TenantID:  tenantID,
		Name:      strings.TrimSpace(req.Name),
		AccessKey: accessKey,
		SecretKey: secretKey,
		Status:    models.CredentialStatusActive,
		AllowIP:   strings.TrimSpace(req.AllowIP),
	}
	row.SetCreateInfo(middleware.AuthEmail(c))
	if row.Name == "" {
		row.Name = "Integration"
	}
	if err := h.db.Create(row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	response.Success(c, "success", gin.H{
		"id":        row.ID,
		"tenantId":  row.TenantID,
		"name":      row.Name,
		"accessKey": row.AccessKey,
		"secretKey": secretKey,
		"allowIp":   row.AllowIP,
		"status":    row.Status,
		"notice":    "secretKey is shown once; store it safely",
	})
}

func (h *Handlers) listCredentials(c *gin.Context) {
	if middleware.AuthUserID(c) == 0 {
		response.Fail(c, "forbidden", nil)
		return
	}
	tenantID := middleware.AuthTenantID(c)
	if tenantID == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}

	var rows []models.Credential
	if err := h.db.Model(&models.Credential{}).
		Select("id", "tenant_id", "name", "access_key", "status", "allow_ip", "created_at", "updated_at", "create_by").
		Where("tenant_id = ? AND is_deleted = ?", tenantID, models.SoftDeleteStatusActive).
		Order("id DESC").
		Find(&rows).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": rows})
}
