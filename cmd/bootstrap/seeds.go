package bootstrap

import (
	"errors"

	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"gorm.io/gorm"
)

const KEY_SITE_NAME = "SITE_NAME"
const KEY_SITE_ADMIN = "SITE_ADMIN"
const KEY_SITE_URL = "SITE_URL"
const KEY_SITE_KEYWORDS = "SITE_KEYWORDS"
const KEY_SITE_DESCRIPTION = "SITE_DESCRIPTION"
const KEY_SITE_GA = "SITE_GA"

const KEY_SITE_LOGO_URL = "SITE_LOGO_URL"
const KEY_SITE_TERMS_URL = "SITE_TERMS_URL"
const KEY_USER_ACTIVATED = "USER_ACTIVATED"
const KEY_STORAGE_KIND = "STORAGE_KIND"

type SeedService struct {
	db *gorm.DB
}

func (s *SeedService) SeedAll() error {
	if err := s.seedConfigs(); err != nil {
		return err
	}
	return nil
}

func (s *SeedService) seedConfigs() error {
	defaults := []utils.Config{
		{Key: KEY_SITE_URL, Desc: "Site URL", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.URL != "" {
				return config.GlobalConfig.Server.URL
			}
			return "https://store.lingecho.com/admin/storage"
		}()},
		{Key: KEY_SITE_NAME, Desc: "Site Name", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.Name != "" {
				return config.GlobalConfig.Server.Name
			}
			return "LingEcho"
		}()},
		{Key: KEY_SITE_LOGO_URL, Desc: "Site Logo", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.Logo != "" {
				return config.GlobalConfig.Server.Logo
			}
			return "/static/img/favicon.png"
		}()},
		{Key: KEY_SITE_DESCRIPTION, Desc: "Site Description", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.Desc != "" {
				return config.GlobalConfig.Server.Desc
			}
			return "LingStorage - Intelligent Storage Service Platform"
		}()},
		{Key: KEY_SITE_TERMS_URL, Desc: "Terms of Service", Autoload: true, Public: true, Format: "text", Value: func() string {
			if config.GlobalConfig.Server.TermsURL != "" {
				return config.GlobalConfig.Server.TermsURL
			}
			return "https://store.lingecho.com/api/docs"
		}()},
	}
	for _, cfg := range defaults {
		var existingConfig utils.Config
		result := s.db.Where("`key` = ?", cfg.Key).First(&existingConfig)

		if result.Error != nil {
			if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return result.Error
			}
			if err := s.db.Create(&cfg).Error; err != nil {
				return err
			}
		} else {
			existingConfig.Value = cfg.Value
			existingConfig.Desc = cfg.Desc
			existingConfig.Autoload = cfg.Autoload
			existingConfig.Public = cfg.Public
			existingConfig.Format = cfg.Format
			if err := s.db.Save(&existingConfig).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
