package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
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

	r := engine.Group(config.GlobalConfig.Server.APIPrefix)

	// Register Global Singleton DB
	r.Use(middleware.InjectDB(h.db))
	h.registerSIPContactCenterRoutes(r)
	h.registerLingechoWebSeatRoutes(r)
	h.registerVoiceDialogRoutes(r)
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
