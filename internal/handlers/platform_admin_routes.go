package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) registerPlatformAdminRoutes(r *gin.RouterGroup) {
	g := r.Group("platform-admins")
	g.Use(middleware.RequirePlatformAdmin())
	{
		g.GET("", h.listPlatformAdmins)
		g.GET("/:id", h.getPlatformAdmin)
		g.POST("", h.createPlatformAdmin)
		g.PUT("/:id", h.updatePlatformAdmin)
		g.PUT("/:id/status", h.updatePlatformAdminStatus)
		g.PUT("/:id/password", h.resetPlatformAdminPassword)
		g.DELETE("/:id", h.deletePlatformAdmin)
	}
}
