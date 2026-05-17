package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// registerCredentialRoutes mounts /credentials for tenant-scoped
// API key (AK/SK) management. RequireHumanJWTUser blocks AK/SK
// callers — credentials must be managed via a human-authenticated
// session, never via another AK/SK (prevents key-rotation lockout
// scenarios and credential lateral movement).
func (h *Handlers) registerCredentialRoutes(r *gin.RouterGroup) {
	cr := r.Group("credentials")
	cr.Use(middleware.RequireHumanJWTUser())

	crRead := cr.Group("")
	crRead.Use(middleware.RequireTenantPermissionAll("api.credentials.read"))
	{
		crRead.GET("", h.listCredentials)
	}
	crWrite := cr.Group("")
	crWrite.Use(middleware.RequireTenantPermissionAll("api.credentials.write"))
	{
		crWrite.POST("", h.createCredential)
		crWrite.PUT("/:id", h.updateCredential)
		crWrite.POST("/:id/disable", h.disableCredential)
		crWrite.POST("/:id/enable", h.enableCredential)
		crWrite.DELETE("/:id", h.deleteCredential)
	}
}
