package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
)

type tenantUserCreateReq struct {
	TenantID     uint   `json:"tenantId" binding:"required"`
	Email        string `json:"email" binding:"required,email"`
	Phone        string `json:"phone"`
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash" binding:"required"`
	DisplayName  string `json:"displayName"`
	Status       string `json:"status"` // active | disabled | pending
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
	tenantID, _ := strconv.ParseUint(c.Query("tenantId"), 10, 32)
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

	list, total, err := models.ListTenantUsersPage(h.db, uint(tenantID), page, size, c.Query("status"), c.Query("search"))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"list": list, "total": total, "page": page, "size": size})
}

func (h *Handlers) getTenantUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	row, err := models.GetActiveTenantUserByID(h.db, uint(id))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) createTenantUser(c *gin.Context) {
	var req tenantUserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		response.Fail(c, "email required", nil)
		return
	}

	// Check for duplicates
	exists, _ := models.CheckTenantUserEmailExists(h.db, req.TenantID, email, 0)
	if exists {
		response.Fail(c, "email already exists", nil)
		return
	}
	if req.Phone != "" {
		exists, _ = models.CheckTenantUserPhoneExists(h.db, req.TenantID, strings.TrimSpace(req.Phone), 0)
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
	}
	if req.Username != "" {
		exists, _ = models.CheckTenantUserUsernameExists(h.db, req.TenantID, strings.TrimSpace(req.Username), 0)
		if exists {
			response.Fail(c, "username already exists", nil)
			return
		}
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = models.TenantUserStatusActive
	}

	user := &models.TenantUser{
		TenantID:     req.TenantID,
		Email:        email,
		Phone:        strings.TrimSpace(req.Phone),
		Username:     strings.TrimSpace(req.Username),
		PasswordHash: req.PasswordHash,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Status:       status,
	}
	if op := acdOperator(c); op != "" {
		user.SetCreateInfo(op)
	}

	if err := models.CreateTenantUser(h.db, user); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", user)
}

func (h *Handlers) updateTenantUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	existing, err := models.GetActiveTenantUserByID(h.db, uint(id))
	if err != nil {
		response.Fail(c, "user not found", nil)
		return
	}

	updates := make(map[string]any)
	if req.Email != "" {
		email := strings.TrimSpace(req.Email)
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
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

	n, err := models.UpdateTenantUserStatus(h.db, uint(id), status, acdOperator(c))
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	rows, err := models.SoftDeleteTenantUserByID(h.db, uint(id), acdOperator(c))
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	rows, err := models.RestoreTenantUser(h.db, uint(id), acdOperator(c))
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
	tenantID, _ := strconv.ParseUint(c.Query("tenantId"), 10, 32)

	total, _ := models.CountTenantUsers(h.db, uint(tenantID))
	active, _ := models.CountTenantUsersByStatus(h.db, uint(tenantID), models.TenantUserStatusActive)
	disabled, _ := models.CountTenantUsersByStatus(h.db, uint(tenantID), models.TenantUserStatusDisabled)
	pending, _ := models.CountTenantUsersByStatus(h.db, uint(tenantID), models.TenantUserStatusPending)

	response.Success(c, "success", gin.H{
		"total":    total,
		"active":   active,
		"disabled": disabled,
		"pending":  pending,
	})
}
