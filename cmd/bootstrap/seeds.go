package bootstrap

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultPlatformAdminEmail       = "admin@lingecho.com"
	defaultPlatformAdminPassword    = "admin123"
	defaultPlatformAdminDisplayName = "Platform Admin"
)

type SeedService struct {
	db *gorm.DB
}

func (s *SeedService) SeedAll() error {
	if err := s.seedConfigs(); err != nil {
		return err
	}
	if err := s.seedPermissions(); err != nil {
		return err
	}
	if err := s.seedPlatformAdmin(); err != nil {
		return err
	}
	return nil
}

func (s *SeedService) seedPermissions() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := models.SyncPermissionCatalog(s.db); err != nil {
		return err
	}
	// Heal pre-existing tenants whose system「管理员」role was bound before new
	// permission codes (e.g. menu.* sidebar codes) were added to the catalog.
	return models.BackfillSystemTenantAdminPermissions(s.db, "seed")
}

func (s *SeedService) seedConfigs() error {
	apiPrefix := config.GlobalConfig.Server.APIPrefix
	defaults := []utils.Config{
		{Key: constants.KEY_SITE_URL, Desc: "Site URL", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.URL != "" {
				return config.GlobalConfig.Server.URL
			}
			return "https://lingecho.com"
		}()},
		{Key: constants.KEY_SITE_NAME, Desc: "Site Name", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.Name != "" {
				return config.GlobalConfig.Server.Name
			}
			return "SoulNexus"
		}()},
		{Key: constants.KEY_SITE_LOGO_URL, Desc: "Site Logo", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.Logo != "" {
				return config.GlobalConfig.Server.Logo
			}
			return "/static/img/favicon.png"
		}()},
		{Key: constants.KEY_SITE_DESCRIPTION, Desc: "Site Description", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.Desc != "" {
				return config.GlobalConfig.Server.Desc
			}
			return "SoulNexus - Intelligent Voice Customer Service Platform"
		}()},
		{Key: constants.KEY_SITE_TERMS_URL, Desc: "Terms of Service", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.TermsURL != "" {
				return config.GlobalConfig.Server.TermsURL
			}
			return "https://lingecho.com"
		}()},
		{Key: constants.KEY_SITE_SIGNIN_URL, Desc: "Sign In Page", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/login"},
		{Key: constants.KEY_SITE_FAVICON_URL, Desc: "Favicon URL", Autoload: true, Public: true, Format: "text", Value: "/static/img/favicon.png"},
		{Key: constants.KEY_SITE_SIGNUP_URL, Desc: "Sign Up Page", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/register"},
		{Key: constants.KEY_SITE_LOGOUT_URL, Desc: "Logout Page", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/logout"},
		{Key: constants.KEY_SITE_RESET_PASSWORD_URL, Desc: "Reset Password Page", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/reset-password"},
		{Key: constants.KEY_SITE_SIGNIN_API, Desc: "Sign In API", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/login"},
		{Key: constants.KEY_SITE_SIGNUP_API, Desc: "Sign Up API", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/register"},
		{Key: constants.KEY_SITE_RESET_PASSWORD_DONE_API, Desc: "Reset Password API", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/auth/reset-password-done"},
		{Key: constants.KEY_SITE_LOGIN_NEXT, Desc: "Login Redirect Page", Autoload: true, Public: true, Format: "text", Value: apiPrefix + "/admin/"},
		{Key: constants.KEY_SITE_USER_ID_TYPE, Desc: "User ID Type", Autoload: true, Public: true, Format: "text", Value: "email"},
	}
	for _, cfg := range defaults {
		var count int64
		err := s.db.Model(&utils.Config{}).Where("`key` = ?", cfg.Key).Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			if err := s.db.Create(&cfg).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// seedPlatformAdmin 在 platform_admins 表为空时插入一个默认平台管理员账号。
// 与 seedConfigs 一样是幂等的：表里已经有任何一条记录时直接跳过，不会覆盖运营改过的密码。
func (s *SeedService) seedPlatformAdmin() error {
	if s == nil || s.db == nil {
		return nil
	}
	n, err := models.CountPlatformAdmins(s.db)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := access.HashPassword(defaultPlatformAdminPassword)
	if err != nil {
		return err
	}
	row := &models.PlatformAdmin{
		Email:        defaultPlatformAdminEmail,
		PasswordHash: hash,
		DisplayName:  defaultPlatformAdminDisplayName,
		Status:       models.PlatformAdminStatusActive,
	}
	row.SetCreateInfo("seed")
	if err := s.db.Create(row).Error; err != nil {
		return err
	}
	logger.Info("seeded default platform admin (change password after first login)",
		zap.String("email", row.Email),
		zap.Uint("id", row.ID),
	)
	return nil
}
