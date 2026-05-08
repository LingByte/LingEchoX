package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type credentialCreateReq struct {
	Name            string   `json:"name"`
	AllowIP         string   `json:"allowIp"` // 逗号分隔，可为空表示不限制
	PermissionCodes []string `json:"permissionCodes"`
}

type credentialUpdateReq struct {
	Name            *string  `json:"name"`
	AllowIP         *string  `json:"allowIp"`
	PermissionCodes []string `json:"permissionCodes"`
}

func marshalCredentialPermissionCodes(codes []string) (string, error) {
	if len(codes) == 0 {
		codes = []string{"*"}
	}
	b, err := json.Marshal(codes)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// findCredentialForTenant 命中时返回行；未命中时已写过响应，调用方直接 return。
func (h *Handlers) findCredentialForTenant(c *gin.Context, tenantID uint) (*models.Credential, bool) {
	id, idErr := utils.ParseID(c.Param("id"))
	if idErr != nil {
		response.Fail(c, "invalid id", nil)
		return nil, false
	}
	var row models.Credential
	if err := h.db.Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Fail(c, "not found", nil)
			return nil, false
		}
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return nil, false
	}
	return &row, true
}

// createCredential issues AK/SK for the tenant (human JWT only).
func (h *Handlers) createCredential(c *gin.Context) {
	tenantID := middleware.CurrentTenantID(c)
	var req credentialCreateReq
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.Fail(c, "invalid body", err.Error())
		return
	}

	skBytes := make([]byte, 32)
	if _, err := rand.Read(skBytes); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	secretKey := hex.EncodeToString(skBytes)
	accessKey := "ak_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var pcodes string
	var err error
	if req.PermissionCodes != nil && len(req.PermissionCodes) == 0 {
		pcodes = "[]"
	} else {
		pcodes, err = marshalCredentialPermissionCodes(req.PermissionCodes)
	}
	if err != nil {
		response.Fail(c, "invalid permissionCodes", nil)
		return
	}
	row := &models.Credential{
		TenantID:        tenantID,
		Name:            strings.TrimSpace(req.Name),
		AccessKey:       accessKey,
		SecretKey:       secretKey,
		Status:          models.CredentialStatusActive,
		AllowIP:         strings.TrimSpace(req.AllowIP),
		PermissionCodes: pcodes,
	}
	row.SetCreateInfo(middleware.AuthEmail(c))
	if row.Name == "" {
		row.Name = "Integration"
	}
	if creErr := h.db.Create(row).Error; creErr != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, creErr)
		return
	}

	var pc []string
	_ = json.Unmarshal([]byte(row.PermissionCodes), &pc)
	response.Success(c, "success", gin.H{
		"id":              row.ID,
		"tenantId":        row.TenantID,
		"name":            row.Name,
		"accessKey":       row.AccessKey,
		"secretKey":       secretKey, // 仅本次响应返回，后续不再可读
		"allowIp":         row.AllowIP,
		"permissionCodes": pc,
		"status":          row.Status,
		"createdAt":       row.CreatedAt,
		"notice":          "secretKey is shown once; store it safely",
	})
}

func (h *Handlers) listCredentials(c *gin.Context) {
	tenantID := middleware.CurrentTenantID(c)

	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 100)
	statusFilter := strings.TrimSpace(c.Query("status"))
	nameFilter := strings.TrimSpace(c.Query("name"))

	q := h.db.Model(&models.Credential{}).
		Where("tenant_id = ?", tenantID)
	if statusFilter == models.CredentialStatusActive || statusFilter == models.CredentialStatusDisabled {
		q = q.Where("status = ?", statusFilter)
	}
	if nameFilter != "" {
		q = q.Where("name LIKE ?", "%"+nameFilter+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}

	var rows []models.Credential
	if err := q.
		Select("id", "tenant_id", "name", "access_key", "status", "allow_ip", "permission_codes", "created_at", "updated_at", "create_by").
		Order("id DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		var pc []string
		if strings.TrimSpace(row.PermissionCodes) != "" {
			_ = json.Unmarshal([]byte(row.PermissionCodes), &pc)
		}
		list = append(list, gin.H{
			"id":              row.ID,
			"tenantId":        row.TenantID,
			"name":            row.Name,
			"accessKey":       row.AccessKey,
			"status":          row.Status,
			"allowIp":         row.AllowIP,
			"permissionCodes": pc,
			"createdAt":       row.CreatedAt,
			"updatedAt":       row.UpdatedAt,
			"createBy":        row.CreateBy,
		})
	}
	response.Success(c, "success", gin.H{
		"list":  list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// updateCredential 仅允许修改 name / allowIp；ak / sk / status 走专门的接口。
func (h *Handlers) updateCredential(c *gin.Context) {
	tenantID := middleware.CurrentTenantID(c)
	row, ok := h.findCredentialForTenant(c, tenantID)
	if !ok {
		return
	}

	var req credentialUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	meta := models.BaseModel{}
	meta.SetUpdateInfo(middleware.AuthEmail(c))
	updates := map[string]any{}
	if meta.UpdateBy != "" {
		updates["update_by"] = meta.UpdateBy
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			response.Fail(c, "name cannot be empty", nil)
			return
		}
		updates["name"] = name
	}
	if req.AllowIP != nil {
		updates["allow_ip"] = strings.TrimSpace(*req.AllowIP)
	}
	if req.PermissionCodes != nil {
		var pcodes string
		var err error
		if len(req.PermissionCodes) == 0 {
			pcodes = "[]"
		} else {
			pcodes, err = marshalCredentialPermissionCodes(req.PermissionCodes)
		}
		if err != nil {
			response.Fail(c, "invalid permissionCodes", nil)
			return
		}
		updates["permission_codes"] = pcodes
	}
	if err := h.db.Model(&models.Credential{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID})
}

func (h *Handlers) setCredentialStatus(c *gin.Context, target string) {
	tenantID := middleware.CurrentTenantID(c)
	row, ok := h.findCredentialForTenant(c, tenantID)
	if !ok {
		return
	}
	if row.Status == target {
		response.Success(c, "success", gin.H{"id": row.ID, "status": row.Status})
		return
	}
	if err := h.db.Model(&models.Credential{}).
		Where("id = ?", row.ID).
		Updates(func() map[string]any {
			meta := models.BaseModel{}
			meta.SetUpdateInfo(middleware.AuthEmail(c))
			u := map[string]any{"status": target}
			if meta.UpdateBy != "" {
				u["update_by"] = meta.UpdateBy
			}
			return u
		}()).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID, "status": target})
}

// disableCredential 立即禁用一个 AK/SK，禁用后所有签名请求都会失败。
func (h *Handlers) disableCredential(c *gin.Context) {
	h.setCredentialStatus(c, models.CredentialStatusDisabled)
}

// enableCredential 恢复一个被禁用的 AK/SK。
func (h *Handlers) enableCredential(c *gin.Context) {
	h.setCredentialStatus(c, models.CredentialStatusActive)
}

// deleteCredential 软删除：deleted_at 非空后，AKSK 查询自然不可见，删除后立即吊销。
func (h *Handlers) deleteCredential(c *gin.Context) {
	tenantID := middleware.CurrentTenantID(c)
	row, ok := h.findCredentialForTenant(c, tenantID)
	if !ok {
		return
	}
	meta := models.BaseModel{}
	meta.SoftDelete(middleware.AuthEmail(c))
	if err := h.db.Model(&models.Credential{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"status":     models.CredentialStatusDisabled,
			"update_by":  meta.UpdateBy,
			"updated_at": meta.UpdatedAt,
			"deleted_at": meta.DeletedAt,
		}).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID})
}
