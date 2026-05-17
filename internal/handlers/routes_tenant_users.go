package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// registerTenantUserRoutes mounts /tenant-users for in-tenant user
// administration. The /me + /auth/logout + TOTP routes live in
// routes_account.go because they apply to any authenticated principal
// (platform admin or tenant user) with no RBAC code required.
func (h *Handlers) registerTenantUserRoutes(r *gin.RouterGroup) {
	g := r.Group("tenant-users")
	tuRead := g.Group("")
	tuRead.Use(middleware.RequireTenantPermissionAll("api.tenant_users.read"))
	{
		tuRead.GET("", h.listTenantUsers)
		tuRead.GET("/stats", h.getTenantUserStats)
		tuRead.GET("/:id", h.getTenantUser)
	}
	tuWrite := g.Group("")
	tuWrite.Use(middleware.RequireTenantPermissionAll("api.tenant_users.write"))
	{
		tuWrite.POST("", h.createTenantUser)
		tuWrite.PUT("/:id", h.updateTenantUser)
		tuWrite.PUT("/:id/status", h.updateTenantUserStatus)
		tuWrite.DELETE("/:id", h.deleteTenantUser)
		tuWrite.POST("/:id/restore", h.restoreTenantUser)
	}
}
