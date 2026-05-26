package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformAdminCreateReq struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8,max=128"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
}

type platformAdminUpdateReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type platformAdminStatusReq struct {
	Status string `json:"status" binding:"required"`
}

type platformAdminPasswordReq struct {
	Password string `json:"password" binding:"required,min=8,max=128"`
}

func (h *Handlers) listPlatformAdmins(c *gin.Context) {
	page, size := ginutil.QueryPage(c, 100)
	list, total, err := models.ListPlatformAdminsPage(h.db, page, size, c.Query("search"))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, row := range list {
		out = append(out, models.PlatformAdminPublic(row))
	}
	ginutil.PageSuccess(c, out, total, page, size)
}

func (h *Handlers) getPlatformAdmin(c *gin.Context) {
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	row, err := models.GetPlatformAdminByID(h.db, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", models.PlatformAdminPublic(row))
}

func (h *Handlers) createPlatformAdmin(c *gin.Context) {
	var req platformAdminCreateReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	email := utils.TrimLower(req.Email)
	var existing models.PlatformAdmin
	if err := h.db.Where("email = ?", email).First(&existing).Error; err == nil {
		response.Fail(c, "email already exists", nil)
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	hash, err := access.HashPassword(req.Password)
	if err != nil {
		response.Fail(c, "hash password failed", nil)
		return
	}
	status := models.NormalizePlatformAdminStatus(req.Status)
	if status == "" {
		status = constants.PlatformAdminStatusActive
	}
	row := &models.PlatformAdmin{
		Email:        email,
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Status:       status,
	}
	if op := middleware.AuditOperator(c); op != "" {
		row.SetCreateInfo(op)
	}
	if err := h.db.Create(row).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", models.PlatformAdminPublic(*row))
}

func (h *Handlers) updatePlatformAdmin(c *gin.Context) {
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	if _, err := models.GetPlatformAdminByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	var req platformAdminUpdateReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	n, err := models.UpdatePlatformAdminProfile(h.db, id, req.Email, req.DisplayName, middleware.AuditOperator(c))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	if n == 0 {
		response.Fail(c, "no fields to update", nil)
		return
	}
	row, _ := models.GetPlatformAdminByID(h.db, id)
	response.Success(c, "success", models.PlatformAdminPublic(row))
}

func (h *Handlers) updatePlatformAdminStatus(c *gin.Context) {
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	selfID := middleware.AuthPlatformAdminID(c)
	var req platformAdminStatusReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	status := models.NormalizePlatformAdminStatus(req.Status)
	if status == "" {
		response.Fail(c, "status must be active or disabled", nil)
		return
	}
	if status == constants.PlatformAdminStatusDisabled {
		if selfID > 0 && selfID == id {
			response.Fail(c, "cannot disable your own account", nil)
			return
		}
		if err := models.EnsureNotLastActivePlatformAdmin(h.db, id); err != nil {
			if errors.Is(err, models.ErrLastActivePlatformAdmin) {
				response.Fail(c, "cannot disable the last active platform admin", nil)
				return
			}
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
	}
	n, err := models.UpdatePlatformAdminStatus(h.db, id, status, middleware.AuditOperator(c))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	if n == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id, "status": status})
}

func (h *Handlers) resetPlatformAdminPassword(c *gin.Context) {
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	if _, err := models.GetPlatformAdminByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	var req platformAdminPasswordReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	hash, err := access.HashPassword(req.Password)
	if err != nil {
		response.Fail(c, "hash password failed", nil)
		return
	}
	if err := models.UpdatePlatformAdminPassword(h.db, id, hash); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) deletePlatformAdmin(c *gin.Context) {
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	if selfID := middleware.AuthPlatformAdminID(c); selfID > 0 && selfID == id {
		response.Fail(c, "cannot delete your own account", nil)
		return
	}
	if _, err := models.GetPlatformAdminByID(h.db, id); err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if err := models.EnsureNotLastActivePlatformAdmin(h.db, id); err != nil {
		if errors.Is(err, models.ErrLastActivePlatformAdmin) {
			response.Fail(c, "cannot delete the last active platform admin", nil)
			return
		}
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	n, err := models.SoftDeletePlatformAdmin(h.db, id, middleware.AuditOperator(c))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	if n == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}
