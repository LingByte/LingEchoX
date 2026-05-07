package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LingByte/SoulNexus/cmd/bootstrap"
	"github.com/LingByte/SoulNexus/internal/sipserver"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/sip/webseat"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handlers struct {
	db          *gorm.DB
	campaignSvc *sipserver.CampaignService
}

func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{
		db: db,
	}
}

// SetCampaignService wires the embedded SIP outbound worker (optional). Call after sipserver.Start
// so Gin routes can expose dial-side counters (e.g. GET .../sip-center/campaigns/worker-metrics).
func (h *Handlers) SetCampaignService(svc *sipserver.CampaignService) {
	if h == nil {
		return
	}
	h.campaignSvc = svc
}

func (h *Handlers) Register(engine *gin.Engine) {
	engine.GET("/.well-known/jwks.json", h.JWKSHandler)
	r := engine.Group(config.GlobalConfig.Server.APIPrefix)
	// Register Global Singleton DB
	r.Use(middleware.InjectDB(h.db))
	h.registerTenantPublicRoutes(r)

	protected := r.Group("")
	protected.Use(middleware.RequireTenantJWTOrAKSK())
	h.registerSIPContactCenterRoutes(protected)
	h.registerVoiceDialogRoutes(protected)
	h.registerTenantUserRoutes(protected)
	h.registerCredentialRoutes(protected)

	// WebSeat signaling is token-gated inside webseat package; keep it outside JWT/AKSK middleware
	// so browser WebSocket/HTTP calls can connect with ?token=... only.
	h.registerLingechoWebSeatRoutes(r)
}

func (h *Handlers) registerSIPContactCenterRoutes(r *gin.RouterGroup) {
	g := r.Group("sip-center")
	{
		g.GET("/users", h.listSIPUsers)
		g.GET("/users/:id", h.getSIPUser)
		g.DELETE("/users/:id", h.deleteSIPUser)
		g.GET("/calls", h.listSIPCalls)
		g.GET("/calls/:id", h.getSIPCall)
		g.GET("/acd-pool", h.listACDPoolTargets)
		g.POST("/acd-pool/web-seat/heartbeat", h.webSeatACDHeartbeat)
		g.GET("/acd-pool/:id", h.getACDPoolTarget)
		g.POST("/acd-pool", h.createACDPoolTarget)
		g.PUT("/acd-pool/:id", h.updateACDPoolTarget)
		g.DELETE("/acd-pool/:id", h.deleteACDPoolTarget)
		g.GET("/scripts", h.listSIPScriptTemplates)
		g.GET("/scripts/:id", h.getSIPScriptTemplate)
		g.POST("/scripts", h.createSIPScriptTemplate)
		g.PUT("/scripts/:id", h.updateSIPScriptTemplate)
		g.DELETE("/scripts/:id", h.deleteSIPScriptTemplate)
		g.POST("/campaigns", h.createSIPCampaign)
		g.GET("/campaigns", h.listSIPCampaigns)
		g.POST("/campaigns/:id/contacts", h.addSIPCampaignContacts)
		g.GET("/campaigns/:id/contacts", h.listSIPCampaignContacts)
		g.POST("/campaigns/:id/contacts/reset-suppressed", h.resetSIPCampaignSuppressedContacts)
		g.POST("/campaigns/:id/start", h.startSIPCampaign)
		g.POST("/campaigns/:id/pause", h.pauseSIPCampaign)
		g.POST("/campaigns/:id/resume", h.resumeSIPCampaign)
		g.POST("/campaigns/:id/stop", h.stopSIPCampaign)
		g.DELETE("/campaigns/:id", h.deleteSIPCampaign)
		g.GET("/campaigns/metrics", h.getSIPCampaignMetrics)
		g.GET("/campaigns/worker-metrics", h.getSIPCampaignWorkerMetrics)
		g.GET("/campaigns/:id/logs", h.getSIPCampaignLogs)
		g.GET("/trunks", h.listTrunks)
		g.POST("/trunks", h.createTrunk)
		g.GET("/trunks/:id", h.getTrunk)
		g.PUT("/trunks/:id", h.updateTrunk)
		g.DELETE("/trunks/:id", h.deleteTrunk)
		g.GET("/trunk-numbers", h.listTrunkNumbers)
		g.POST("/trunk-numbers", h.createTrunkNumber)
		g.GET("/trunk-numbers/:id", h.getTrunkNumber)
		g.PUT("/trunk-numbers/:id", h.updateTrunkNumber)
		g.DELETE("/trunk-numbers/:id", h.deleteTrunkNumber)
	}
}

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

func (h *Handlers) registerVoiceDialogRoutes(r *gin.RouterGroup) {
	g := r.Group(constants.LingechoVoiceDialogPathPrefix)
	g.GET("/ws", voiceDialogWebSocket)
}

func (h *Handlers) registerTenantPublicRoutes(r *gin.RouterGroup) {
	r.POST("/tenants/register", h.registerTenant)
	r.POST("/auth/tenant-login", h.tenantLogin)
	r.POST("/register", h.registerTenant)
	r.POST("/login", h.tenantLogin)
}

func (h *Handlers) registerTenantUserRoutes(r *gin.RouterGroup) {
	g := r.Group("tenant-users")
	{
		g.GET("", h.listTenantUsers)
		g.POST("", h.createTenantUser)
		g.GET("/:id", h.getTenantUser)
		g.PUT("/:id", h.updateTenantUser)
		g.PUT("/:id/status", h.updateTenantUserStatus)
		g.DELETE("/:id", h.deleteTenantUser)
		g.POST("/:id/restore", h.restoreTenantUser)
		g.GET("/stats", h.getTenantUserStats)
	}
	r.GET("/me", h.getMe)
	r.PUT("/me", h.updateMe)
	r.PUT("/me/password", h.updateMyPassword)
	r.POST("/auth/logout", h.logout)
}

func (h *Handlers) registerCredentialRoutes(r *gin.RouterGroup) {
	g := r.Group("credentials")
	{
		g.POST("", h.createCredential)
		g.GET("", h.listCredentials)
	}
}

// JWKSHandler returns the JSON Web Key Set (JWKS) endpoint
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
