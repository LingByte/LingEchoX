package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	if req.PermissionCodes == nil || len(req.PermissionCodes) == 0 {
		response.Fail(c, "permissionCodes 必填，至少指定一项权限（不可用空数组或省略）", nil)
		return
	}
	pcodes, err := utils.MarshalStringSliceJSON(req.PermissionCodes, nil)
	if err != nil {
		response.Fail(c, "invalid permissionCodes", nil)
		return
	}
	if strings.TrimSpace(req.AllowIP) == "" && !utils.GetBoolEnv(constants.ENVCredentialAllowEmptyAllowIP) {
		response.Fail(c, "allowIp 必填（逗号分隔客户端 IP）；开发环境可设 CREDENTIAL_ALLOW_EMPTY_ALLOW_IP=true", nil)
		return
	}
	row := &models.Credential{
		TenantID:        tenantID,
		Name:            strings.TrimSpace(req.Name),
		AccessKey:       accessKey,
		SecretKey:       secretKey,
		Status:          constants.CredentialStatusActive,
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
	page, size := ginutil.QueryPage(c, 100)
	statusFilter := strings.TrimSpace(c.Query("status"))
	nameFilter := strings.TrimSpace(c.Query("name"))

	q := h.db.Model(&models.Credential{}).Where("tenant_id = ?", tenantID)
	if statusFilter == constants.CredentialStatusActive || statusFilter == constants.CredentialStatusDisabled {
		q = q.Where("status = ?", statusFilter)
	}
	if nameFilter != "" {
		q = q.Where("name LIKE ?", "%"+nameFilter+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}

	var rows []models.Credential
	if err := q.
		Select("id", "tenant_id", "name", "access_key", "status", "allow_ip", "permission_codes", "created_at", "updated_at", "create_by").
		Order("id DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error; err != nil {
		ginutil.WriteInternalError(c, err)
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
	ginutil.PageSuccess(c, list, total, page, size)
}

// updateCredential 仅允许修改 name / allowIp；ak / sk / status 走专门的接口。
func (h *Handlers) updateCredential(c *gin.Context) {
	tenantID := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	row, err := models.GetCredentialByIDForTenant(h.db, id, tenantID)
	if ginutil.WriteGORMError(c, err, "not found") {
		return
	}

	var req credentialUpdateReq
	if !ginutil.BindJSON(c, &req) {
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
		if len(req.PermissionCodes) == 0 {
			response.Fail(c, "permissionCodes 不能为空数组", nil)
			return
		}
		pcodes, marshalErr := utils.MarshalStringSliceJSON(req.PermissionCodes, nil)
		if marshalErr != nil {
			response.Fail(c, "invalid permissionCodes", nil)
			return
		}
		updates["permission_codes"] = pcodes
	}
	if req.AllowIP != nil && strings.TrimSpace(*req.AllowIP) == "" && !utils.GetBoolEnv(constants.ENVCredentialAllowEmptyAllowIP) {
		response.Fail(c, "allowIp 不能为空", nil)
		return
	}
	if ginutil.WriteInternalError(c, h.db.Model(&models.Credential{}).Where("id = ?", row.ID).Updates(updates).Error) {
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID})
}

func (h *Handlers) disableCredential(c *gin.Context) {
	patchCredentialStatus(h, c, constants.CredentialStatusDisabled)
}

func (h *Handlers) enableCredential(c *gin.Context) {
	patchCredentialStatus(h, c, constants.CredentialStatusActive)
}

func patchCredentialStatus(h *Handlers, c *gin.Context, target string) {
	tenantID := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	row, err := models.GetCredentialByIDForTenant(h.db, id, tenantID)
	if ginutil.WriteGORMError(c, err, "not found") {
		return
	}
	if row.Status == target {
		response.Success(c, "success", gin.H{"id": row.ID, "status": row.Status})
		return
	}
	if ginutil.WriteInternalError(c, models.UpdateCredentialStatus(h.db, &row, target, middleware.AuthEmail(c))) {
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID, "status": target})
}

// deleteCredential 软删除：deleted_at 非空后，AKSK 查询自然不可见，删除后立即吊销。
func (h *Handlers) deleteCredential(c *gin.Context) {
	tenantID := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	row, err := models.GetCredentialByIDForTenant(h.db, id, tenantID)
	if ginutil.WriteGORMError(c, err, "not found") {
		return
	}
	meta := models.BaseModel{}
	meta.SoftDelete(middleware.AuthEmail(c))
	if ginutil.WriteInternalError(c, h.db.Model(&models.Credential{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"status":     constants.CredentialStatusDisabled,
			"update_by":  meta.UpdateBy,
			"updated_at": meta.UpdatedAt,
			"deleted_at": meta.DeletedAt,
		}).Error) {
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID})
}
