package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// registerSIPContactCenterRoutes mounts everything under /sip-center.
// Authorization model:
//   - SIP user CRUD / trunk CRUD / trunk-number write   → platform admin only.
//   - Call records / ACD pool / scripts / campaigns / trunk-numbers read
//     → tenant RBAC permission codes (api.sip.*).
// The split is deliberate: tenants manage their own dial flows but
// can't enumerate other tenants' SIP credentials or trunk wiring.
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
	// Web-seat heartbeat needs EITHER read OR write because the
	// browser agent has its own non-tenant token but still wants to
	// keep the seat alive.
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

	// Trunk-numbers read: dual-mode (platform admin sees all, tenant
	// user sees own). The handler itself decides which subset to
	// expose based on the JWT subject type.
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
		// Trunk audio uploads (multipart/form-data, field "file").
		// Returns {url,key,size}; frontend then PUTs the URL back to
		// the matching trunk-number field. Two endpoints share one
		// handler factory, only the spool prefix differs.
		numAdmin.POST("/trunk-numbers/welcome-audio", h.uploadTrunkNumberAudio("welcome-audio"))
		numAdmin.POST("/trunk-numbers/transfer-ringing-audio", h.uploadTrunkNumberAudio("transfer-ringing-audio"))
	}

	h.registerTenantOrgRoutes(g)
}
