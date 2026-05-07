package bootstrap

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnsurePlatformAdminFromEnv inserts the first PlatformAdmin when env vars are set and table is empty.
func EnsurePlatformAdminFromEnv(db *gorm.DB) {
	if db == nil || config.GlobalConfig == nil {
		return
	}
	email := strings.TrimSpace(strings.ToLower(config.GlobalConfig.Features.PlatformAdminBootstrapEmail))
	password := strings.TrimSpace(config.GlobalConfig.Features.PlatformAdminBootstrapPassword)
	if email == "" || password == "" {
		return
	}
	n, err := models.CountPlatformAdmins(db)
	if err != nil {
		logger.Warn("platform admin bootstrap skipped (count)", zap.Error(err))
		return
	}
	if n > 0 {
		return
	}
	hash, err := access.HashPassword(password)
	if err != nil {
		logger.Warn("platform admin bootstrap hash failed", zap.Error(err))
		return
	}
	row := &models.PlatformAdmin{
		Email:        email,
		PasswordHash: hash,
		DisplayName:  "Bootstrap",
		Status:       models.PlatformAdminStatusActive,
	}
	row.SetCreateInfo("bootstrap")
	if err := db.Create(row).Error; err != nil {
		logger.Warn("platform admin bootstrap create failed", zap.Error(err))
		return
	}
	logger.Info("platform admin bootstrapped from env",
		zap.String("email", email),
		zap.Uint("id", row.ID))
}
