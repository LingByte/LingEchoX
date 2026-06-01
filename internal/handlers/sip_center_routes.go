package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// registerSIPContactCenterRoutes mounts everything under /sip-center.
// Authorization model:
//   - SIP user CRUD / trunk CRUD / trunk-number write → platform admin only.
//   - Call records / ACD pool / scripts / campaigns / trunk-numbers read
//     → tenant RBAC permission codes (api.sip.*).
func (h *Handlers) registerSIPContactCenterRoutes(r *gin.RouterGroup) {
	g := r.Group("sip-center")
	h.registerSIPCenterUsersRoutes(g)
	h.registerSIPCenterCallsRoutes(g)
	h.registerSIPCenterACDRoutes(g)
	h.registerSIPCenterScriptsRoutes(g)
	h.registerSIPCenterCampaignsRoutes(g)
	h.registerSIPCenterNumbersRoutes(g)
	h.registerTenantOrgRoutes(g)
}

func (h *Handlers) registerSIPCenterUsersRoutes(g *gin.RouterGroup) {
	read := g.Group("")
	read.Use(middleware.RequirePlatformAdmin())
	{
		read.GET("/users", h.listSIPUsers)
		read.GET("/users/:id", h.getSIPUser)
	}
	write := g.Group("")
	write.Use(middleware.RequirePlatformAdmin())
	{
		write.DELETE("/users/:id", h.deleteSIPUser)
	}
}

func (h *Handlers) registerSIPCenterCallsRoutes(g *gin.RouterGroup) {
	read := g.Group("")
	read.Use(middleware.RequireTenantPermissionAll("api.sip.calls.read"))
	{
		read.GET("/calls", h.listSIPCalls)
		read.GET("/calls/:id", h.getSIPCall)
	}
}

func (h *Handlers) registerSIPCenterACDRoutes(g *gin.RouterGroup) {
	read := g.Group("")
	read.Use(middleware.RequireTenantPermissionAll("api.sip.acd.read"))
	{
		read.GET("/acd-pool", h.listACDPoolTargets)
		read.GET("/acd-pool/:id", h.getACDPoolTarget)
		read.GET("/acd-dispatch-mode", h.getACDDispatchMode)
		read.GET("/sip-agent/incoming", h.pollSIPAgentIncoming)
		read.GET("/sip-agent/incoming/stream", h.streamSIPAgentIncoming)
		read.GET("/sip-agent/incoming/logs", h.listSIPAgentIncomingLogs)
	}
	write := g.Group("")
	write.Use(middleware.RequireTenantPermissionAll("api.sip.acd.write"))
	{
		write.POST("/acd-pool", h.createACDPoolTarget)
		write.PUT("/acd-pool/:id", h.updateACDPoolTarget)
		write.DELETE("/acd-pool/:id", h.deleteACDPoolTarget)
		write.PUT("/acd-dispatch-mode", h.updateACDDispatchMode)
	}
	// Web-seat heartbeat: read OR write (browser agent token).
	seat := g.Group("")
	seat.Use(middleware.RequireTenantPermissionAny("api.sip.acd.read", "api.sip.acd.write"))
	{
		seat.POST("/acd-pool/web-seat/heartbeat", h.webSeatACDHeartbeat)
	}
}

func (h *Handlers) registerSIPCenterScriptsRoutes(g *gin.RouterGroup) {
	read := g.Group("")
	read.Use(middleware.RequireTenantPermissionAll("api.sip.scripts.read"))
	{
		read.GET("/scripts", h.listSIPScriptTemplates)
		read.GET("/scripts/:id", h.getSIPScriptTemplate)
	}
	write := g.Group("")
	write.Use(middleware.RequireTenantPermissionAll("api.sip.scripts.write"))
	{
		write.POST("/scripts", h.createSIPScriptTemplate)
		write.PUT("/scripts/:id", h.updateSIPScriptTemplate)
		write.DELETE("/scripts/:id", h.deleteSIPScriptTemplate)
	}
}

func (h *Handlers) registerSIPCenterCampaignsRoutes(g *gin.RouterGroup) {
	read := g.Group("")
	read.Use(middleware.RequireTenantPermissionAll("api.sip.campaigns.read"))
	{
		read.GET("/campaigns", h.listSIPCampaigns)
		read.GET("/campaigns/:id/contacts", h.listSIPCampaignContacts)
		read.GET("/campaigns/metrics", h.getSIPCampaignMetrics)
		read.GET("/campaigns/worker-metrics", h.getSIPCampaignWorkerMetrics)
		read.GET("/campaigns/:id/logs", h.getSIPCampaignLogs)
	}
	write := g.Group("")
	write.Use(middleware.RequireTenantPermissionAll("api.sip.campaigns.write"))
	{
		write.POST("/campaigns", h.createSIPCampaign)
		write.POST("/campaigns/:id/contacts", h.addSIPCampaignContacts)
		write.POST("/campaigns/:id/contacts/reset-suppressed", h.resetSIPCampaignSuppressedContacts)
		write.POST("/campaigns/:id/start", h.startSIPCampaign)
		write.POST("/campaigns/:id/pause", h.pauseSIPCampaign)
		write.POST("/campaigns/:id/resume", h.resumeSIPCampaign)
		write.POST("/campaigns/:id/stop", h.stopSIPCampaign)
		write.DELETE("/campaigns/:id", h.deleteSIPCampaign)
	}
}

func (h *Handlers) registerSIPCenterNumbersRoutes(g *gin.RouterGroup) {
	read := g.Group("")
	read.Use(middleware.RequireTenantPermissionAll("api.sip.numbers.read"))
	{
		read.GET("/trunk-numbers", h.listTrunkNumbers)
	}
	admin := g.Group("")
	admin.Use(middleware.RequirePlatformAdmin())
	{
		admin.GET("/trunks", h.listTrunks)
		admin.GET("/trunks/:id", h.getTrunk)
		admin.POST("/trunks", h.createTrunk)
		admin.PUT("/trunks/:id", h.updateTrunk)
		admin.DELETE("/trunks/:id", h.deleteTrunk)
		admin.GET("/trunk-numbers/:id", h.getTrunkNumber)
		admin.POST("/trunk-numbers", h.createTrunkNumber)
		admin.PUT("/trunk-numbers/:id", h.updateTrunkNumber)
		admin.DELETE("/trunk-numbers/:id", h.deleteTrunkNumber)
		admin.POST("/trunk-numbers/welcome-audio", h.uploadTrunkNumberAudio("welcome-audio"))
		admin.POST("/trunk-numbers/transfer-ringing-audio", h.uploadTrunkNumberAudio("transfer-ringing-audio"))
	}
}
