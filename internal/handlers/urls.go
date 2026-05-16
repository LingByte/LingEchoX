package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/sipserver"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/constants"
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
	uploadDir := utils.GetEnv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	engine.Static("/uploads", uploadDir)
	r := engine.Group(config.GlobalConfig.Server.APIPrefix)
	// Register Global Singleton DB
	r.Use(middleware.InjectDB(h.db))
	h.registerTenantPublicRoutes(r)

	protected := r.Group("")
	protected.Use(middleware.RequireTenantJWTOrAKSK())
	h.registerSIPContactCenterRoutes(protected)
	h.registerTenantUserRoutes(protected)
	h.registerCredentialRoutes(protected)

	// WebSeat signaling is token-gated inside webseat package; keep it outside JWT/AKSK middleware
	// so browser WebSocket/HTTP calls can connect with ?token=... only.
	h.registerLingechoWebSeatRoutes(r)
	// Voice-dialog WebSocket also has its own (?token=...) gate inside voicedialog.WebSocketTokenOK,
	// and is dialed by SIP loopback (no JWT/AKSK). Keep it on the unprotected group.
	h.registerVoiceDialogRoutes(r)
}

func (h *Handlers) registerSIPContactCenterRoutes(r *gin.RouterGroup) {
	g := r.Group("sip-center")

	sipUsersRead := g.Group("")
	sipUsersRead.Use(middleware.RequirePlatformAdmin())
	{
		sipUsersRead.GET("/users", h.listSIPUsers)
		sipUsersRead.GET("/users/:id", h.getSIPUser)
	}
	sipUsersWrite := g.Group("")
	sipUsersWrite.Use(middleware.RequirePlatformAdmin())
	{
		sipUsersWrite.DELETE("/users/:id", h.deleteSIPUser)
	}

	callsRead := g.Group("")
	callsRead.Use(middleware.RequireTenantPermissionAll("api.sip.calls.read"))
	{
		callsRead.GET("/calls", h.listSIPCalls)
		callsRead.GET("/calls/:id", h.getSIPCall)
	}

	acdRead := g.Group("")
	acdRead.Use(middleware.RequireTenantPermissionAll("api.sip.acd.read"))
	{
		acdRead.GET("/acd-pool", h.listACDPoolTargets)
		acdRead.GET("/acd-pool/:id", h.getACDPoolTarget)
		acdRead.GET("/acd-dispatch-mode", h.getACDDispatchMode)
	}
	acdWrite := g.Group("")
	acdWrite.Use(middleware.RequireTenantPermissionAll("api.sip.acd.write"))
	{
		acdWrite.POST("/acd-pool", h.createACDPoolTarget)
		acdWrite.PUT("/acd-pool/:id", h.updateACDPoolTarget)
		acdWrite.DELETE("/acd-pool/:id", h.deleteACDPoolTarget)
		acdWrite.PUT("/acd-dispatch-mode", h.updateACDDispatchMode)
	}
	acdSeat := g.Group("")
	acdSeat.Use(middleware.RequireTenantPermissionAny("api.sip.acd.read", "api.sip.acd.write"))
	{
		acdSeat.POST("/acd-pool/web-seat/heartbeat", h.webSeatACDHeartbeat)
	}

	scriptsRead := g.Group("")
	scriptsRead.Use(middleware.RequireTenantPermissionAll("api.sip.scripts.read"))
	{
		scriptsRead.GET("/scripts", h.listSIPScriptTemplates)
		scriptsRead.GET("/scripts/:id", h.getSIPScriptTemplate)
	}
	scriptsWrite := g.Group("")
	scriptsWrite.Use(middleware.RequireTenantPermissionAll("api.sip.scripts.write"))
	{
		scriptsWrite.POST("/scripts", h.createSIPScriptTemplate)
		scriptsWrite.PUT("/scripts/:id", h.updateSIPScriptTemplate)
		scriptsWrite.DELETE("/scripts/:id", h.deleteSIPScriptTemplate)
	}

	campRead := g.Group("")
	campRead.Use(middleware.RequireTenantPermissionAll("api.sip.campaigns.read"))
	{
		campRead.GET("/campaigns", h.listSIPCampaigns)
		campRead.GET("/campaigns/:id/contacts", h.listSIPCampaignContacts)
		campRead.GET("/campaigns/metrics", h.getSIPCampaignMetrics)
		campRead.GET("/campaigns/worker-metrics", h.getSIPCampaignWorkerMetrics)
		campRead.GET("/campaigns/:id/logs", h.getSIPCampaignLogs)
	}
	campWrite := g.Group("")
	campWrite.Use(middleware.RequireTenantPermissionAll("api.sip.campaigns.write"))
	{
		campWrite.POST("/campaigns", h.createSIPCampaign)
		campWrite.POST("/campaigns/:id/contacts", h.addSIPCampaignContacts)
		campWrite.POST("/campaigns/:id/contacts/reset-suppressed", h.resetSIPCampaignSuppressedContacts)
		campWrite.POST("/campaigns/:id/start", h.startSIPCampaign)
		campWrite.POST("/campaigns/:id/pause", h.pauseSIPCampaign)
		campWrite.POST("/campaigns/:id/resume", h.resumeSIPCampaign)
		campWrite.POST("/campaigns/:id/stop", h.stopSIPCampaign)
		campWrite.DELETE("/campaigns/:id", h.deleteSIPCampaign)
	}

	// Trunk-numbers read: dual-mode (platform admin sees all, tenant user sees own).
	numRead := g.Group("")
	numRead.Use(middleware.RequireTenantPermissionAll("api.sip.numbers.read"))
	{
		numRead.GET("/trunk-numbers", h.listTrunkNumbers)
	}
	// Trunks CRUD + trunk-number detail/write: platform admin only.
	numAdmin := g.Group("")
	numAdmin.Use(middleware.RequirePlatformAdmin())
	{
		numAdmin.GET("/trunks", h.listTrunks)
		numAdmin.GET("/trunks/:id", h.getTrunk)
		numAdmin.POST("/trunks", h.createTrunk)
		numAdmin.PUT("/trunks/:id", h.updateTrunk)
		numAdmin.DELETE("/trunks/:id", h.deleteTrunk)
		numAdmin.GET("/trunk-numbers/:id", h.getTrunkNumber)
		numAdmin.POST("/trunk-numbers", h.createTrunkNumber)
		numAdmin.PUT("/trunk-numbers/:id", h.updateTrunkNumber)
		numAdmin.DELETE("/trunk-numbers/:id", h.deleteTrunkNumber)
		numAdmin.POST("/trunk-numbers/welcome-audio", h.uploadTrunkNumberAudio("welcome-audio"))
		numAdmin.POST("/trunk-numbers/transfer-ringing-audio", h.uploadTrunkNumberAudio("transfer-ringing-audio"))
	}

	h.registerTenantOrgRoutes(g)
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
	r.POST("/register", h.registerTenant)
	r.POST("/login", h.tenantLogin)
}

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
	tenantAdmin := r.Group("tenants")
	tenantAdmin.Use(middleware.RequirePlatformAdmin())
	{
		tenantAdmin.GET("", h.listTenants)
		tenantAdmin.GET("/:id", h.getTenant)
		tenantAdmin.POST("", h.createTenantPlatform)
		tenantAdmin.PUT("/:id", h.updateTenantPlatform)
		tenantAdmin.DELETE("/:id", h.deleteTenantPlatform)
	}
	r.GET("/me", h.getMe)
	// 个人资料：任意已登录租户用户 / 平台管理员均可修改，不纳入租户 RBAC 分配项。
	r.PUT("/me", h.updateMe)
	r.PUT("/me/password", h.updateMyPassword)
	r.POST("/me/avatar", h.uploadMeAvatar)
	r.POST("/me/totp/setup", h.setupTotp)
	r.POST("/me/totp/enable", h.enableTotp)
	r.POST("/me/totp/disable", h.disableTotp)
	r.POST("/auth/logout", h.logout)
}

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
