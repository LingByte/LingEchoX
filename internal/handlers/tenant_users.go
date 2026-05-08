package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
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
