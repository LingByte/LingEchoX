package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
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
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	page, size := ginutil.QueryPage(c, 100)

	list, total, err := models.ListTenantUsersPage(h.db, tenantID, page, size, c.Query("status"), c.Query("search"))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	pub := make([]gin.H, 0, len(list))
	for _, row := range list {
		pub = append(pub, models.TenantUserPublic(h.db, row))
	}
	ginutil.PageSuccess(c, pub, total, page, size)
}

func (h *Handlers) getTenantUser(c *gin.Context) {
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
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
	response.Success(c, "success", models.TenantUserPublic(h.db, row))
}

func (h *Handlers) createTenantUser(c *gin.Context) {
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	var req tenantUserCreateReq
	if !ginutil.BindJSON(c, &req) {
		return
	}

	email := utils.TrimLower(req.Email)
	if email == "" {
		response.Fail(c, "email required", nil)
		return
	}

	// Check for duplicates
	exists, _ := models.CheckTenantUserEmailExists(h.db, email, 0)
	if exists {
		response.Fail(c, "email already exists", nil)
		return
	}
	if req.Phone != "" {
		exists, _ = models.CheckTenantUserPhoneExists(h.db, strings.TrimSpace(req.Phone), 0)
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
	}
	if req.Username != "" {
		exists, _ = models.CheckTenantUserUsernameExists(h.db, strings.TrimSpace(req.Username), 0)
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
		ginutil.WriteInternalError(c, err)
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = constants.TenantUserStatusActive
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = constants.TenantUserSourceManual
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
	if op := middleware.AuditOperator(c); op != "" {
		user.SetCreateInfo(op)
	}

	if err := models.CreateTenantUser(h.db, user); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", models.TenantUserPublic(h.db, *user))
}

func (h *Handlers) updateTenantUser(c *gin.Context) {
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}

	var req tenantUserUpdateReq
	if !ginutil.BindJSON(c, &req) {
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
		exists, _ := models.CheckTenantUserEmailExists(h.db, email, uint(id))
		if exists {
			response.Fail(c, "email already exists", nil)
			return
		}
		updates["email"] = email
	}
	if req.Phone != "" {
		phone := strings.TrimSpace(req.Phone)
		exists, _ := models.CheckTenantUserPhoneExists(h.db, phone, uint(id))
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
		updates["phone"] = phone
	}
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		exists, _ := models.CheckTenantUserUsernameExists(h.db, username, uint(id))
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

	n, err := models.UpdateTenantUser(h.db, uint(id), updates, middleware.AuditOperator(c))
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	if n == 0 {
		response.Fail(c, "user not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) updateTenantUserStatus(c *gin.Context) {
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}

	existing, err := models.GetActiveTenantUserByID(h.db, id)
	if err != nil || existing.TenantID != tenantID {
		response.Fail(c, "user not found", nil)
		return
	}

	var req tenantUserStatusReq
	if !ginutil.BindJSON(c, &req) {
		return
	}

	status := strings.TrimSpace(req.Status)
	if status != constants.TenantUserStatusActive && status != constants.TenantUserStatusDisabled && status != constants.TenantUserStatusPending {
		response.Fail(c, "invalid status", nil)
		return
	}

	n, err := models.UpdateTenantUserStatus(h.db, id, status, middleware.AuditOperator(c))
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	if n == 0 {
		response.Fail(c, "user not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id, "status": status})
}

func (h *Handlers) deleteTenantUser(c *gin.Context) {
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}

	existing, getErr := models.GetActiveTenantUserByID(h.db, id)
	if getErr != nil || existing.TenantID != tenantID {
		response.Fail(c, "not found", nil)
		return
	}

	rows, err := models.SoftDeleteTenantUserByID(h.db, id, middleware.AuditOperator(c))
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	if rows == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) restoreTenantUser(c *gin.Context) {
	tenantID, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}

	existing, getErr := models.GetTenantUserByID(h.db, id)
	if getErr != nil || existing.TenantID != tenantID {
		response.Fail(c, "not found", nil)
		return
	}

	rows, err := models.RestoreTenantUser(h.db, id, middleware.AuditOperator(c))
	if err != nil {
		ginutil.WriteInternalError(c, err)
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
	active, _ := models.CountTenantUsersByStatus(h.db, tenantID, constants.TenantUserStatusActive)
	disabled, _ := models.CountTenantUsersByStatus(h.db, tenantID, constants.TenantUserStatusDisabled)
	pending, _ := models.CountTenantUsersByStatus(h.db, tenantID, constants.TenantUserStatusPending)
	response.Success(c, "success", gin.H{
		"total":    total,
		"active":   active,
		"disabled": disabled,
		"pending":  pending,
	})
}
