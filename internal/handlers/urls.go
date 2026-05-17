package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/sipserver"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handlers struct {
	db          *gorm.DB
	campaignSvc *sipserver.CampaignService
}

func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// SetCampaignService wires the embedded SIP outbound worker (optional). Call after sipserver.Start.
func (h *Handlers) SetCampaignService(svc *sipserver.CampaignService) {
	if h == nil {
		return
	}
	h.campaignSvc = svc
}

func (h *Handlers) Register(engine *gin.Engine) {
	engine.GET("/.well-known/jwks.json", h.JWKSHandler)
	uploadDir := utils.GetEnv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	engine.Static("/uploads", uploadDir)

	r := engine.Group(config.GlobalConfig.Server.APIPrefix)
	r.Use(middleware.InjectDB(h.db))

	h.registerTenantPublicRoutes(r)

	protected := r.Group("")
	protected.Use(middleware.RequireTenantJWTOrAKSK())
	h.registerSIPContactCenterRoutes(protected)
	h.registerTenantUserRoutes(protected)
	h.registerPlatformTenantRoutes(protected)
	h.registerAccountRoutes(protected)
	h.registerCredentialRoutes(protected)

	h.registerLingechoWebSeatRoutes(r)
	h.registerVoiceDialogRoutes(r)
}

// JWKSHandler returns the JSON Web Key Set (JWKS) endpoint.
func (h *Handlers) JWKSHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	c.Header("Cache-Control", "public, max-age=3600")
	if bootstrap.GlobalKeyManager == nil {
		c.JSON(500, gin.H{"error": "key manager not initialized"})
		return
	}
	jwksJSON, err := bootstrap.GlobalKeyManager.GetJWKSJSON()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to generate JWKS"})
		return
	}
	c.String(200, jwksJSON)
}
