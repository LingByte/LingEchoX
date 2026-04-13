package handlers

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handlers struct {
	db *gorm.DB
}

func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{
		db: db,
	}
}

func (h *Handlers) Register(engine *gin.Engine) {
	r := engine.Group("/api")
	h.RegisterCredentialRoutes(r)
	h.registerVoiceDialogueRoutes(r)
	h.registerOpenAPIRoutes(r)
}
