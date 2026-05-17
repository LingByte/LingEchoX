package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// registerPlatformTenantRoutes mounts /tenants — cross-tenant
// administration only available to the platform admin role.
// Distinct from /tenant-users (in-tenant user management) because
// these handlers can list and mutate ANY tenant's metadata.
func (h *Handlers) registerPlatformTenantRoutes(r *gin.RouterGroup) {
	g := r.Group("tenants")
	g.Use(middleware.RequirePlatformAdmin())
	{
		g.GET("", h.listTenants)
		g.GET("/:id", h.getTenant)
		g.POST("", h.createTenantPlatform)
		g.PUT("/:id", h.updateTenantPlatform)
		g.DELETE("/:id", h.deleteTenantPlatform)
	}
}
