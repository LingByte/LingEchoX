package handlers

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"strconv"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterCredentialRoutes 注册凭证相关的路由
func (h *Handlers) RegisterCredentialRoutes(r *gin.RouterGroup) {
	credentialAPI := r.Group("/credentials")
	credentialAPI.Use(middleware.SignVerifyMiddleware(h.db))
	{
		credentialAPI.GET("/current", h.GetCurrentCredentialInfo)
		credentialAPI.GET("/list", h.GetUserCredentialsList)
		credentialAPI.GET("/:id", h.GetCredentialByID)
		credentialAPI.GET("/stats", h.GetCredentialStats)
		credentialAPI.PUT("/:id/name", h.UpdateCredentialName)
		credentialAPI.GET("/:id/status", h.GetCredentialStatus)
	}
}

// GetCurrentCredentialInfo 获取当前凭证信息
func (h *Handlers) GetCurrentCredentialInfo(c *gin.Context) {
	credential, err := middleware.GetCredentialFromContext(c, h.db)
	if err != nil {
		response.Fail(c, "Failed to get credential from context", nil)
		return
	}

	response.Success(c, "Get credential info successfully", credential.ToResponse())
}

// GetUserCredentialsList 获取当前用户的所有凭证列表
func (h *Handlers) GetUserCredentialsList(c *gin.Context) {
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Fail(c, "User ID not found in context", nil)
		return
	}

	credentials, err := models.GetUserCredentials(h.db, userID)
	if err != nil {
		response.Fail(c, "Failed to get user credentials", nil)
		return
	}

	responses := models.ToResponseList(credentials)
	response.Success(c, "Get credentials list successfully", gin.H{
		"credentials": responses,
		"count":       len(responses),
	})
}

// GetCredentialByID 根据ID获取凭证详细信息
func (h *Handlers) GetCredentialByID(c *gin.Context) {
	// 获取路径参数中的ID
	idStr := c.Param("id")
	credentialID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.FailWithCode(c, 400, "Invalid credential ID", nil)
		return
	}

	// 获取当前用户ID
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Fail(c, "User ID not found in context", nil)
		return
	}

	// 查询凭证
	var credential models.Credential
	err = h.db.Where("id = ? AND user_id = ?", credentialID, userID).First(&credential).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			response.FailWithCode(c, 404, "Credential not found or access denied", nil)
		} else {
			response.Fail(c, "Database error", nil)
		}
		return
	}

	response.Success(c, "Get credential successfully", credential.ToResponse())
}

// GetCredentialStats 获取凭证使用统计
func (h *Handlers) GetCredentialStats(c *gin.Context) {
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Fail(c, "User ID not found in context", nil)
		return
	}

	var stats struct {
		TotalCredentials int64 `json:"total_credentials"`
		ActiveCount      int64 `json:"active_count"`
		BannedCount      int64 `json:"banned_count"`
		SuspendedCount   int64 `json:"suspended_count"`
		ExpiredCount     int64 `json:"expired_count"`
		TotalUsage       int64 `json:"total_usage"`
	}

	// 统计总数
	h.db.Model(&models.Credential{}).Where("user_id = ?", userID).Count(&stats.TotalCredentials)

	// 统计各状态数量
	h.db.Model(&models.Credential{}).Where("user_id = ? AND status = ?", userID, models.CredentialStatusActive).Count(&stats.ActiveCount)
	h.db.Model(&models.Credential{}).Where("user_id = ? AND status = ?", userID, models.CredentialStatusBanned).Count(&stats.BannedCount)
	h.db.Model(&models.Credential{}).Where("user_id = ? AND status = ?", userID, models.CredentialStatusSuspended).Count(&stats.SuspendedCount)
	// 统计过期数量（状态为active但已过期）
	h.db.Model(&models.Credential{}).Where("user_id = ? AND expires_at IS NOT NULL AND expires_at < ?", userID, gorm.Expr("NOW()")).Count(&stats.ExpiredCount)
	// 统计总使用次数
	h.db.Model(&models.Credential{}).Where("user_id = ?", userID).Select("COALESCE(SUM(usage_count), 0)").Scan(&stats.TotalUsage)

	response.Success(c, "Get credential stats successfully", stats)
}

// UpdateCredentialName 更新凭证名称
func (h *Handlers) UpdateCredentialName(c *gin.Context) {
	// 获取路径参数中的ID
	idStr := c.Param("id")
	credentialID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.FailWithCode(c, 400, "Invalid credential ID", nil)
		return
	}

	// 获取当前用户ID
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Fail(c, "User ID not found in context", nil)
		return
	}

	// 解析请求体
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithCode(c, 400, "Invalid request body: "+err.Error(), nil)
		return
	}

	// 更新凭证名称
	result := h.db.Model(&models.Credential{}).
		Where("id = ? AND user_id = ?", credentialID, userID).
		Update("name", req.Name)

	if result.Error != nil {
		response.Fail(c, "Failed to update credential name", nil)
		return
	}

	if result.RowsAffected == 0 {
		response.FailWithCode(c, 404, "Credential not found or access denied", nil)
		return
	}

	response.Success(c, "Credential name updated successfully", nil)
}

// GetCredentialStatus 获取凭证状态
func (h *Handlers) GetCredentialStatus(c *gin.Context) {
	// 获取路径参数中的ID
	idStr := c.Param("id")
	credentialID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.FailWithCode(c, 400, "Invalid credential ID", nil)
		return
	}

	// 获取当前用户ID
	userID, exists := middleware.GetUserIDFromContext(c)
	if !exists {
		response.Fail(c, "User ID not found in context", nil)
		return
	}

	// 查询凭证
	var credential models.Credential
	err = h.db.Select("id", "status", "banned_at", "banned_reason", "expires_at", "last_used_at", "usage_count").
		Where("id = ? AND user_id = ?", credentialID, userID).
		First(&credential).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			response.FailWithCode(c, 404, "Credential not found or access denied", nil)
		} else {
			response.Fail(c, "Database error", nil)
		}
		return
	}

	statusInfo := gin.H{
		"id":         credential.ID,
		"status":     credential.Status,
		"is_active":  credential.IsActive(),
		"is_banned":  credential.IsBanned(),
		"is_expired": credential.IsExpired(),
	}

	if credential.BannedAt != nil {
		statusInfo["banned_at"] = credential.BannedAt
	}
	if credential.BannedReason != "" {
		statusInfo["banned_reason"] = credential.BannedReason
	}
	if credential.ExpiresAt != nil {
		statusInfo["expires_at"] = credential.ExpiresAt
	}
	if credential.LastUsedAt != nil {
		statusInfo["last_used_at"] = credential.LastUsedAt
	}

	statusInfo["usage_count"] = credential.UsageCount

	response.Success(c, "Get credential status successfully", gin.H{
		"status": statusInfo,
	})
}
