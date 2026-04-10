package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LingByte/SoulNexus/internal/sipserver"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/middleware"
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
}
