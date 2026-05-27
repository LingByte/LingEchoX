package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"time"

	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/sipserver"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
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
	engine.Use(middleware.LocaleMiddleware())
	engine.GET("/.well-known/jwks.json", h.JWKSHandler)
	uploadDir := utils.GetEnv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	engine.Use(middleware.UploadsACL())
	engine.Static("/uploads", uploadDir)
	r := engine.Group(config.GlobalConfig.Server.APIPrefix)
	r.Use(middleware.InjectDB(h.db))
	h.registerTenantPublicRoutes(r)
	protected := r.Group("")
	protected.Use(middleware.RequireTenantJWTOrAKSK())
	h.registerSIPContactCenterRoutes(protected)
	h.registerTenantUserRoutes(protected)
	h.registerPlatformTenantRoutes(protected)
	h.registerPlatformAdminRoutes(protected)
	h.registerPlatformSystemRoutes(protected)
	h.registerAccountRoutes(protected)
	h.registerCredentialRoutes(protected)
	h.registerLingechoWebSeatRoutes(r)
	h.registerVoiceDialogRoutes(r)
}

// registerCredentialRoutes mounts /credentials for tenant-scoped
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

// registerAccountRoutes: /me and logout — any authenticated tenant user or platform admin (no tenant RBAC codes).
func (h *Handlers) registerAccountRoutes(r *gin.RouterGroup) {
	r.GET("/me", h.getMe)
	r.PUT("/me", h.updateMe)
	r.PUT("/me/password", h.updateMyPassword)
	r.POST("/me/avatar", h.uploadMeAvatar)
	r.POST("/me/totp/setup", h.setupTotp)
	r.POST("/me/totp/enable", h.enableTotp)
	r.POST("/me/totp/disable", h.disableTotp)
	r.POST("/auth/logout", h.logout)
}

// registerVoiceDialogRoutes mounts the WebSocket endpoint that the
func (h *Handlers) registerVoiceDialogRoutes(r *gin.RouterGroup) {
	g := r.Group(constants.LingechoVoiceDialogPathPrefix)
	g.GET("/ws", voiceDialogWebSocket)
}

// registerLingechoWebSeatRoutes mounts the browser-agent (web-seat)
func (h *Handlers) registerLingechoWebSeatRoutes(r *gin.RouterGroup) {
	g := r.Group(constants.LingechoWebSeatPathPrefix)
	{
		g.POST("/join", gin.WrapF(webseat.JoinHTTP))
		g.POST("/hangup", gin.WrapF(webseat.HangupHTTP))
		g.POST("/reject", gin.WrapF(webseat.RejectHTTP))
		g.GET("/ws", gin.WrapF(webseat.WebSocketHTTP))
		g.GET("/status/:callId", h.lingechoWebSeatStatus)
	}
}

// registerTenantUserRoutes mounts /tenant-users for in-tenant user
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

func (h *Handlers) registerTenantPublicRoutes(r *gin.RouterGroup) {
	// Throttle credential-bearing endpoints by client IP to make
	// credential stuffing / brute force noisy and slow. Limits are
	// intentionally generous (10 req / 5 min, burst 10) so a human
	// retrying a typo or a shared NAT with multiple users is not
	// affected, but a botnet hammering one IP gets 429s quickly.
	authLimit := middleware.AuthRateLimiter(10, 5*time.Minute, 10)
	r.POST("/register", authLimit, h.registerTenant)
	r.POST("/login", authLimit, h.tenantLogin)
}

// registerPlatformTenantRoutes mounts /tenants — cross-tenant
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

// registerPlatformSystemRoutes mounts /system/* read-only ops endpoints
// for platform admins. See handlers/system_status.go for the data source.
//
// We intentionally separate this from /metrics:
//   - /metrics speaks Prometheus text (counters/gauges/histograms) and
//     is for time-series scrapers; it has its own IP ACL.
//   - /system/status is JSON, includes runtime.MemStats + disk-cache
//     stats, and is meant for interactive ops dashboards / human eyes.
func (h *Handlers) registerPlatformSystemRoutes(r *gin.RouterGroup) {
	g := r.Group("system")
	g.Use(middleware.RequirePlatformAdmin())
	{
		g.GET("/status", h.getSystemStatus)
	}
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
