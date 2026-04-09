package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handlers struct {
	db                *gorm.DB
	ipLocationService *utils.IPLocationService
}

func NewHandlers(db *gorm.DB) *Handlers {
	// Initialize IP geolocation service
	ipLocationService := utils.NewIPLocationService(logger.Lg)
	return &Handlers{
		db:                db,
		ipLocationService: ipLocationService,
	}
}

func (h *Handlers) Register(engine *gin.Engine) {

	r := engine.Group(config.GlobalConfig.Server.APIPrefix)

	// Register Global Singleton DB
	r.Use(middleware.InjectDB(h.db))
	h.registerSIPContactCenterRoutes(r)
}
