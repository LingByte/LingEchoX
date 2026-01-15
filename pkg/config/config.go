package config

import (
	"log"
	"time"

	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/LingByte/LingEchoX/pkg/utils"
)

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port         int           `json:"port"`
	Host         string        `json:"host"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

var GlobalConfig *Config

// Config System  common config
type Config struct {
	Server      ServerConfig     // Server configuration
	Log         logger.LogConfig // Log configuration
	MachineID   int64            `env:"MACHINE_ID"`
	DBDriver    string           `env:"DB_DRIVER"`
	DSN         string           `env:"DSN"`
	Addr        string           `env:"ADDR"`
	Mode        string           `env:"MODE"`
	ServerName  string           `env:"SERVER_NAME"`
	SSLEnabled  bool             `env:"SSL_ENABLED"`
	SSLCertFile string           `env:"SSL_CERT_FILE"`
	SSLKeyFile  string           `env:"SSL_KEY_FILE"`
}

func Load() error {
	// 1. 根据环境加载 .env 文件（如果不存在也不报错，使用默认值）s
	mode := utils.GetStringOrDefault("MODE", "development")
	err := utils.LoadEnv(mode)
	if err != nil {
		// .env文件不存在时只记录日志，不影响启动s
		log.Printf("Note: .env file not found or failed to load: %v (using default values)", err)
	}
	// 2. 加载全局配置（所有配置都有默认值，确保无.env文件也能启动）
	GlobalConfig = &Config{
		Server: ServerConfig{
			Port:         int(utils.GetIntEnv("PORT")),
			Host:         utils.GetEnv("HOST"),
			ReadTimeout:  time.Duration(utils.GetIntEnv("READ_TIMEOUT")) * time.Second,
			WriteTimeout: time.Duration(utils.GetIntEnv("WRITE_TIMEOUT")) * time.Second,
			IdleTimeout:  time.Duration(utils.GetIntEnv("IDLE_TIMEOUT")) * time.Second,
		},
		Log: logger.LogConfig{
			Level:      utils.GetStringOrDefault("LOG_LEVEL", "info"),
			Filename:   utils.GetStringOrDefault("LOG_FILENAME", "./logs/app.log"),
			MaxSize:    utils.GetIntOrDefault("LOG_MAX_SIZE", 100),
			MaxAge:     utils.GetIntOrDefault("LOG_MAX_AGE", 30),
			MaxBackups: utils.GetIntOrDefault("LOG_MAX_BACKUPS", 5),
			Daily:      utils.GetBoolOrDefault("LOG_DAILY", true),
		},
		Mode:        mode,
		MachineID:   utils.GetIntEnv("MACHINE_ID"),
		DBDriver:    utils.GetStringOrDefault("DB_DRIVER", "sqlite"),
		DSN:         utils.GetStringOrDefault("DSN", "./ling.db"),
		Addr:        utils.GetStringOrDefault("ADDR", ":7072"),
		ServerName:  utils.GetStringOrDefault("SERVER_NAME", ""),
		SSLEnabled:  utils.GetBoolOrDefault("SSL_ENABLED", false),
		SSLCertFile: utils.GetStringOrDefault("SSL_CERT_FILE", ""),
		SSLKeyFile:  utils.GetStringOrDefault("SSL_KEY_FILE", ""),
	}
	return nil
}
