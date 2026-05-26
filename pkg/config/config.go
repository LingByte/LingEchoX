package config

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"go.uber.org/zap"
)

// Config main configuration structure
type Config struct {
	MachineID  int64            `env:"MACHINE_ID"`
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Log        logger.LogConfig `mapstructure:"log"`
	Auth       AuthConfig       `mapstructure:"auth"`
	Services   ServicesConfig   `mapstructure:"services"`
	Features   FeaturesConfig   `mapstructure:"features"`
	Middleware MiddlewareConfig `mapstructure:"middleware"`
	SIP        SIPConfig        `mapstructure:"sip"`
	JWT        JWTConfig        `mapstructure:"jwt"`
}

// JWTConfig JWT related configuration
type JWTConfig struct {
	Algorithm    string `env:"JWT_ALGORITHM"`
	KeyFile      string `env:"JWT_KEY_FILE"`
	RotationDays int    `env:"JWT_ROTATION_DAYS"`
	KeepOldKeys  int    `env:"JWT_KEEP_OLD_KEYS"`
}

type SIPConfig struct {
	SIPPort            int     `env:"SIP_PORT"`
	SIPVADBargeIn      bool    `env:"SIP_VAD_BARGE_IN"`
	SIPVADThreshold    float64 `env:"SIP_VAD_THRESHOLD"`
	SIPVADConsecFrames int     `env:"SIP_VAD_CONSEC_FRAMES"`
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
	DocsPrefix  string `env:"DOCS_PREFIX"`
	APIPrefix   string `env:"API_PREFIX"`
	SSLEnabled  bool   `env:"SSL_ENABLED"`
	SSLCertFile string `env:"SSL_CERT_FILE"`
	SSLKeyFile  string `env:"SSL_KEY_FILE"`
}

// DatabaseConfig database configuration
type DatabaseConfig struct {
	Driver string `env:"DB_DRIVER"`
	DSN    string `env:"DSN"`
}

// AuthConfig authentication configuration
type AuthConfig struct {
	Header           string `env:"AUTH_HEADER"`
	SessionSecret    string `env:"SESSION_SECRET"`
	SecretExpireDays string `env:"SESSION_EXPIRE_DAYS"`
	APISecretKey     string `env:"API_SECRET_KEY"`
}

// ServicesConfig services configuration
type ServicesConfig struct {
	LLM LLMConfig `mapstructure:"llm"`
}

// LLMConfig LLM service configuration
type LLMConfig struct {
	APIKey  string `env:"LLM_API_KEY"`
	BaseURL string `env:"LLM_BASE_URL"`
	Model   string `env:"LLM_MODEL"`
}

// FeaturesConfig feature flags configuration
type FeaturesConfig struct {
	BackupEnabled  bool   `env:"BACKUP_ENABLED"`
	BackupPath     string `env:"BACKUP_PATH"`
	BackupSchedule string `env:"BACKUP_SCHEDULE"`
}

// MiddlewareConfig middleware configuration
type MiddlewareConfig struct {
	// Rate limiting configuration
	RateLimit RateLimiterConfig
	// Timeout configuration
	Timeout TimeoutConfig
	// Circuit breaker configuration
	CircuitBreaker CircuitBreakerConfig
	// Whether to enable each middleware
	EnableRateLimit      bool `env:"ENABLE_RATE_LIMIT"`
	EnableTimeout        bool `env:"ENABLE_TIMEOUT"`
	EnableCircuitBreaker bool `env:"ENABLE_CIRCUIT_BREAKER"`
	EnableOperationLog   bool `env:"ENABLE_OPERATION_LOG"`
}

// RateLimiterConfig rate limiting configuration
type RateLimiterConfig struct {
	GlobalRPS    int           `env:"RATE_LIMIT_GLOBAL_RPS"`   // Global requests per second
	GlobalBurst  int           `env:"RATE_LIMIT_GLOBAL_BURST"` // Global burst requests
	GlobalWindow time.Duration // Global time window
	UserRPS      int           `env:"RATE_LIMIT_USER_RPS"`   // User requests per second
	UserBurst    int           `env:"RATE_LIMIT_USER_BURST"` // User burst requests
	UserWindow   time.Duration // User time window
	IPRPS        int           `env:"RATE_LIMIT_IP_RPS"`   // IP requests per second
	IPBurst      int           `env:"RATE_LIMIT_IP_BURST"` // IP burst requests
	IPWindow     time.Duration // IP time window
}

// TimeoutConfig timeout configuration
type TimeoutConfig struct {
	DefaultTimeout   time.Duration `env:"DEFAULT_TIMEOUT"`
	FallbackResponse interface{}
}

// CircuitBreakerConfig circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold      int           `env:"CIRCUIT_BREAKER_FAILURE_THRESHOLD"`
	SuccessThreshold      int           `env:"CIRCUIT_BREAKER_SUCCESS_THRESHOLD"`
	Timeout               time.Duration `env:"CIRCUIT_BREAKER_TIMEOUT"`
	OpenTimeout           time.Duration `env:"CIRCUIT_BREAKER_OPEN_TIMEOUT"`
	MaxConcurrentRequests int           `env:"CIRCUIT_BREAKER_MAX_CONCURRENT"`
}

var GlobalConfig *Config

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
			Name:        getStringOrDefault("SERVER_NAME", ""),
			Desc:        getStringOrDefault("SERVER_DESC", ""),
			URL:         getStringOrDefault("SERVER_URL", ""),
			Logo:        getStringOrDefault("SERVER_LOGO", ""),
			TermsURL:    getStringOrDefault("SERVER_TERMS_URL", ""),
			Addr:        getStringOrDefault("ADDR", ":8082"),
			Mode:        getStringOrDefault("MODE", "development"),
			DocsPrefix:  getStringOrDefault("DOCS_PREFIX", "/api/docs"),
			APIPrefix:   getStringOrDefault("API_PREFIX", "/api"),
			SSLEnabled:  getBoolOrDefault("SSL_ENABLED", false),
			SSLCertFile: getStringOrDefault("SSL_CERT_FILE", ""),
			SSLKeyFile:  getStringOrDefault("SSL_KEY_FILE", ""),
		},
		Database: DatabaseConfig{
			Driver: getStringOrDefault("DB_DRIVER", "sqlite"),
			DSN:    getStringOrDefault("DSN", "./ling.db"),
		},
		Log: logger.LogConfig{
			Level:      getStringOrDefault("LOG_LEVEL", "info"),
			Filename:   getStringOrDefault("LOG_FILENAME", "./logs/app.log"),
			MaxSize:    getIntOrDefault("LOG_MAX_SIZE", 100),
			MaxAge:     getIntOrDefault("LOG_MAX_AGE", 30),
			MaxBackups: getIntOrDefault("LOG_MAX_BACKUPS", 5),
			Daily:      getBoolOrDefault("LOG_DAILY", true),
		},
		Auth: AuthConfig{
			Header:           getStringOrDefault("AUTH_HEADER", "Authorization"),
			SessionSecret:    getStringOrDefault("SESSION_SECRET", generateDefaultSessionSecret()),
			SecretExpireDays: getStringOrDefault("SESSION_EXPIRE_DAYS", "7"),
			APISecretKey:     getStringOrDefault("API_SECRET_KEY", generateDefaultSessionSecret()),
		},
		Services: ServicesConfig{
			LLM: LLMConfig{
				APIKey:  getStringOrDefault("LLM_API_KEY", ""),
				BaseURL: getStringOrDefault("LLM_BASE_URL", "https://api.openai.com/v1"),
				Model:   getStringOrDefault("LLM_MODEL", "gpt-3.5-turbo"),
			},
		},
		Features: FeaturesConfig{
			BackupEnabled:  getBoolOrDefault("BACKUP_ENABLED", false),
			BackupPath:     getStringOrDefault("BACKUP_PATH", "./backups"),
			BackupSchedule: getStringOrDefault("BACKUP_SCHEDULE", "0 2 * * *"),
		},
		Middleware: loadMiddlewareConfig(),
		SIP: SIPConfig{
			SIPVADBargeIn:      getBoolOrDefault("SIPVADBargeIn", true),
			SIPVADThreshold:    getFloatOrDefault("SIP_VAD_THRESHOLD", 3200.0),
			SIPVADConsecFrames: getIntOrDefault("SIP_VAD_CONSEC_FRAMES", 4),
		},
		JWT: JWTConfig{
			Algorithm:    getStringOrDefault("JWT_ALGORITHM", "RS256"),
			KeyFile:      getStringOrDefault("JWT_KEY_FILE", "./keys/jwks.json"),
			RotationDays: getIntOrDefault("JWT_ROTATION_DAYS", 30),
			KeepOldKeys:  getIntOrDefault("JWT_KEEP_OLD_KEYS", 2),
		},
	}
	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate database configuration
	if c.Database.DSN == "" {
		return errors.New("database DSN is required")
	}

	// Validate server configuration
	if c.Server.Addr == "" {
		return errors.New("server address is required")
	}

	return nil
}

// getStringOrDefault gets environment variable value, returns default if empty
func getStringOrDefault(key, defaultValue string) string {
	value := utils.GetEnv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getBoolOrDefault gets boolean environment variable value, returns default if empty
func getBoolOrDefault(key string, defaultValue bool) bool {
	value := utils.GetEnv(key)
	if value == "" {
		return defaultValue
	}
	return utils.GetBoolEnv(key)
}

// getIntOrDefault gets integer environment variable value, returns default if empty
func getIntOrDefault(key string, defaultValue int) int {
	value := utils.GetIntEnv(key)
	if value == 0 {
		return defaultValue
	}
	return int(value)
}

// getFloatOrDefault gets float environment variable value, returns default if empty
func getFloatOrDefault(key string, defaultValue float64) float64 {
	value := utils.GetEnv(key)
	if value == "" {
		return defaultValue
	}
	// 简单的字符串到float64转换
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return defaultValue
}

// parseDuration parses duration string with default fallback
func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// generateDefaultSessionSecret generates default session secret (for development only)
func generateDefaultSessionSecret() string {
	if secret := utils.GetEnv("SESSION_SECRET"); secret != "" {
		return secret
	}
	return "default-secret-key-change-in-production-" + utils.RandText(16)
}

// loadMiddlewareConfig loads middleware configuration
func loadMiddlewareConfig() MiddlewareConfig {
	mode := getStringOrDefault("MODE", "development")
	var defaultConfig MiddlewareConfig

	if mode == "production" {
		defaultConfig = MiddlewareConfig{
			RateLimit: RateLimiterConfig{
				GlobalRPS:    2000,
				GlobalBurst:  4000,
				GlobalWindow: time.Minute,
				UserRPS:      200,
				UserBurst:    400,
				UserWindow:   time.Minute,
				IPRPS:        100,
				IPBurst:      200,
				IPWindow:     time.Minute,
			},
			Timeout: TimeoutConfig{
				DefaultTimeout: 30 * time.Second,
				FallbackResponse: map[string]interface{}{
					"error":   "service_unavailable",
					"message": "Service temporarily unavailable, please try again later",
					"code":    503,
				},
			},
			CircuitBreaker: CircuitBreakerConfig{
				FailureThreshold:      3,
				SuccessThreshold:      2,
				Timeout:               30 * time.Second,
				OpenTimeout:           30 * time.Second,
				MaxConcurrentRequests: 200,
			},
			EnableRateLimit:      true,
			EnableTimeout:        true,
			EnableCircuitBreaker: true,
			EnableOperationLog:   true,
		}
	} else {
		defaultConfig = MiddlewareConfig{
			RateLimit: RateLimiterConfig{
				GlobalRPS:    10000,
				GlobalBurst:  20000,
				GlobalWindow: time.Minute,
				UserRPS:      1000,
				UserBurst:    2000,
				UserWindow:   time.Minute,
				IPRPS:        500,
				IPBurst:      1000,
				IPWindow:     time.Minute,
			},
			Timeout: TimeoutConfig{
				DefaultTimeout: 60 * time.Second,
				FallbackResponse: map[string]interface{}{
					"error":   "service_unavailable",
					"message": "Service temporarily unavailable, please try again later",
					"code":    503,
				},
			},
			CircuitBreaker: CircuitBreakerConfig{
				FailureThreshold:      10,
				SuccessThreshold:      5,
				Timeout:               60 * time.Second,
				OpenTimeout:           60 * time.Second,
				MaxConcurrentRequests: 1000,
			},
			EnableRateLimit:      true,
			EnableTimeout:        true,
			EnableCircuitBreaker: false,
			EnableOperationLog:   true,
		}
	}
	return MiddlewareConfig{
		RateLimit: RateLimiterConfig{
			GlobalRPS:    getIntOrDefault("RATE_LIMIT_GLOBAL_RPS", defaultConfig.RateLimit.GlobalRPS),
			GlobalBurst:  getIntOrDefault("RATE_LIMIT_GLOBAL_BURST", defaultConfig.RateLimit.GlobalBurst),
			GlobalWindow: parseDuration(getStringOrDefault("RATE_LIMIT_GLOBAL_WINDOW", "1m"), defaultConfig.RateLimit.GlobalWindow),
			UserRPS:      getIntOrDefault("RATE_LIMIT_USER_RPS", defaultConfig.RateLimit.UserRPS),
			UserBurst:    getIntOrDefault("RATE_LIMIT_USER_BURST", defaultConfig.RateLimit.UserBurst),
			UserWindow:   parseDuration(getStringOrDefault("RATE_LIMIT_USER_WINDOW", "1m"), defaultConfig.RateLimit.UserWindow),
			IPRPS:        getIntOrDefault("RATE_LIMIT_IP_RPS", defaultConfig.RateLimit.IPRPS),
			IPBurst:      getIntOrDefault("RATE_LIMIT_IP_BURST", defaultConfig.RateLimit.IPBurst),
			IPWindow:     parseDuration(getStringOrDefault("RATE_LIMIT_IP_WINDOW", "1m"), defaultConfig.RateLimit.IPWindow),
		},
		Timeout: TimeoutConfig{
			DefaultTimeout:   parseDuration(getStringOrDefault("DEFAULT_TIMEOUT", "30s"), defaultConfig.Timeout.DefaultTimeout),
			FallbackResponse: defaultConfig.Timeout.FallbackResponse,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold:      getIntOrDefault("CIRCUIT_BREAKER_FAILURE_THRESHOLD", defaultConfig.CircuitBreaker.FailureThreshold),
			SuccessThreshold:      getIntOrDefault("CIRCUIT_BREAKER_SUCCESS_THRESHOLD", defaultConfig.CircuitBreaker.SuccessThreshold),
			Timeout:               parseDuration(getStringOrDefault("CIRCUIT_BREAKER_TIMEOUT", "30s"), defaultConfig.CircuitBreaker.Timeout),
			OpenTimeout:           parseDuration(getStringOrDefault("CIRCUIT_BREAKER_OPEN_TIMEOUT", "30s"), defaultConfig.CircuitBreaker.OpenTimeout),
			MaxConcurrentRequests: getIntOrDefault("CIRCUIT_BREAKER_MAX_CONCURRENT", defaultConfig.CircuitBreaker.MaxConcurrentRequests),
		},
		EnableRateLimit:      getBoolOrDefault("ENABLE_RATE_LIMIT", defaultConfig.EnableRateLimit),
		EnableTimeout:        getBoolOrDefault("ENABLE_TIMEOUT", defaultConfig.EnableTimeout),
		EnableCircuitBreaker: getBoolOrDefault("ENABLE_CIRCUIT_BREAKER", defaultConfig.EnableCircuitBreaker),
		EnableOperationLog:   getBoolOrDefault("ENABLE_OPERATION_LOG", defaultConfig.EnableOperationLog),
	}
}

// SIPDialEnv holds SIP dial fields parsed from SIP_TRANSFER_* environment variables.
// Callers map this to pkg/sip/outbound.DialTarget at the SIP boundary so this package does not import outbound
// (outbound HTTP helpers live in the same module and would create an import cycle).
type SIPDialEnv struct {
	RequestURI    string
	SignalingAddr string
	WebSeat       bool
}

// CallerIdentityFromEnv reads SIP_CALLER_ID / SIP_CALLER_DISPLAY_NAME for outbound INVITE From/Contact.
// User is the SIP URI user part; displayName is optional (empty → From has no quoted display-name).
func CallerIdentityFromEnv() (user, displayName string) {
	user = utils.GetEnv(constants.EnvSIPCallerID)
	displayName = utils.GetEnv(constants.EnvSIPCallerDisplayName)
	return user, displayName
}

// RegisterPasswordFromEnv returns SIP_PASSWORD when set (trimmed). Empty means REGISTER is open
// (no shared secret). Non-empty means clients must send matching X-SIP-Register-Password on REGISTER.
func RegisterPasswordFromEnv() string {
	return utils.GetEnv(constants.EnvSIPRegisterPassword)
}

// TransferDialTargetFromEnv reads SIP_TRANSFER_* (agent extension for blind transfer dial).
func TransferDialTargetFromEnv() (t SIPDialEnv, ok bool) {
	sig := utils.GetEnv(constants.EnvSIPTransferSigAddr)
	num := utils.GetEnv(constants.EnvSIPTransferNumber)
	if strings.EqualFold(num, "web") {
		return SIPDialEnv{WebSeat: true}, true
	}
	host := utils.GetEnv(constants.EnvSIPTransferHost)
	if num == "" || host == "" {
		return SIPDialEnv{}, false
	}

	port := 50400
	ps := utils.GetEnv(constants.EnvSIPTransferPort)
	if ps != "" {
		if p, err := strconv.Atoi(ps); err == nil && p > 0 && p < 65536 {
			port = p
			logger.Info("parse ture", zap.Int("port", port))
		} else {
			logger.Error("parse error", zap.Error(err))
		}
	}
	t.RequestURI = fmt.Sprintf("sip:%s@%s:%d", num, host, port)
	if sig == "" {
		t.SignalingAddr = fmt.Sprintf("%s:%d", host, port)
	} else {
		t.SignalingAddr = sig
	}
	return t, true
}

func MediaMaxSecondsFromEnv() int {
	const def = 512
	const minQ = 64
	const maxQ = 2048
	s := utils.GetEnv("SIP_MEDIA_MAX_SECONDS")
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < minQ || n > maxQ {
		return def
	}
	return n
}

func MediaTxQueueSizeFromEnv() int {
	s := utils.GetEnv("SIP_MEDIA_TX_QUEUE_SIZE")
	if s == "" {
		return 3600
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 3600
	}
	return n
}
