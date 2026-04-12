package config

import (
	"log"
	"os"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/utils"
)

var GlobalConfig *Config

type Config struct {
	MachineID int64            `env:"MACHINE_ID"`
	Server    ServerConfig     `mapstructure:"server"`
	Database  DatabaseConfig   `mapstructure:"database"`
	Log       logger.LogConfig `mapstructure:"log"`
}

// ServerConfig server configuration
type ServerConfig struct {
	Name        string `env:"SERVER_NAME"`
	Desc        string `env:"SERVER_DESC"`
	URL         string `env:"SERVER_URL"`
	Logo        string `env:"SERVER_LOGO"`
	TermsURL    string `env:"SERVER_TERMS_URL"`
	Addr        string `env:"ADDR"`
	Mode        string `env:"MODE"`
	SSLEnabled  bool   `env:"SSL_ENABLED"`
	SSLCertFile string `env:"SSL_CERT_FILE"`
	SSLKeyFile  string `env:"SSL_KEY_FILE"`
}

// DatabaseConfig database configuration
type DatabaseConfig struct {
	Driver string `env:"DB_DRIVER"`
	DSN    string `env:"DSN"`
}

func Load() error {
	// 1. Load .env file based on environment (don't error if it doesn't exist, use default values)
	env := os.Getenv("MODE")
	err := utils.LoadEnv(env)
	if err != nil {
		// Only log when .env file doesn't exist, don't affect startup
		log.Printf("Note: .env file not found or failed to load: %v (using default values)", err)
	}

	// 2. Load global configuration
	GlobalConfig = &Config{
		MachineID: utils.GetIntEnv("MACHINE_ID"),
		Server: ServerConfig{
			Name:        utils.GetStringOrDefault("SERVER_NAME", ""),
			Desc:        utils.GetStringOrDefault("SERVER_DESC", ""),
			URL:         utils.GetStringOrDefault("SERVER_URL", ""),
			Logo:        utils.GetStringOrDefault("SERVER_LOGO", ""),
			TermsURL:    utils.GetStringOrDefault("SERVER_TERMS_URL", ""),
			Addr:        utils.GetStringOrDefault("ADDR", ":8082"),
			Mode:        utils.GetStringOrDefault("MODE", "development"),
			SSLEnabled:  utils.GetBoolOrDefault("SSL_ENABLED", false),
			SSLCertFile: utils.GetStringOrDefault("SSL_CERT_FILE", ""),
			SSLKeyFile:  utils.GetStringOrDefault("SSL_KEY_FILE", ""),
		},
		Database: DatabaseConfig{
			Driver: utils.GetStringOrDefault("DB_DRIVER", "sqlite"),
			DSN:    utils.GetStringOrDefault("DSN", "./ling.db"),
		},
		Log: logger.LogConfig{
			Level:      utils.GetStringOrDefault("LOG_LEVEL", "info"),
			Filename:   utils.GetStringOrDefault("LOG_FILENAME", "./logs/app.log"),
			MaxSize:    utils.GetIntOrDefault("LOG_MAX_SIZE", 100),
			MaxAge:     utils.GetIntOrDefault("LOG_MAX_AGE", 30),
			MaxBackups: utils.GetIntOrDefault("LOG_MAX_BACKUPS", 5),
			Daily:      utils.GetBoolOrDefault("LOG_DAILY", true),
		},
	}
	return nil
}
