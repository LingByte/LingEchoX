package config

import (
	"log"
	"os"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/LingByte/lingstorage-sdk-go"
)

var GlobalConfig *Config
var GlobalStore *lingstorage.Client

type Config struct {
	MachineID int64            `env:"MACHINE_ID"`
	Server    ServerConfig     `mapstructure:"server"`
	Database  DatabaseConfig   `mapstructure:"database"`
	Log       logger.LogConfig `mapstructure:"log"`
	Services  ServicesConfig   `mapstructure:"services"`
	RTCSFU    RTCSFUConfig     `mapstructure:"rtcsfu"`
}

// RTCSFUConfig enables the hybrid multi-SFU control-plane HTTP API (pkg/rtcsfu) and embedded Pion SFU.
type RTCSFUConfig struct {
	Enabled   bool   `env:"RTCSFU_ENABLED"`
	NodesJSON string `env:"RTCSFU_NODES"`   // JSON array, see pkg/rtcsfu.ParseNodesJSON
	APIKey    string `env:"RTCSFU_API_KEY"` // optional; if set, join requires header X-RTCSFU-Key

	ICEServersJSON       string `env:"RTCSFU_ICE_SERVERS_JSON"`       // browser-style JSON array; empty = default STUN
	MaxRooms             int    `env:"RTCSFU_MAX_ROOMS"`              // 0 = unlimited
	MaxPeersPerRoom      int    `env:"RTCSFU_MAX_PEERS_PER_ROOM"`     // 0 = unlimited
	WSReadTimeoutSec     int    `env:"RTCSFU_WS_READ_TIMEOUT_SEC"`    // WebSocket read deadline (ping/pong refresh)
	WSPingIntervalSec    int    `env:"RTCSFU_WS_PING_INTERVAL_SEC"`   // 0 = disable WS ping
	WSMaxMessageBytes    int    `env:"RTCSFU_WS_MAX_MESSAGE_BYTES"`   // signaling frame size cap
	SignalAllowedOrigins string `env:"RTCSFU_SIGNAL_ALLOWED_ORIGINS"` // comma-separated Origin allowlist; empty: prod=same-host only
	SignalRequireAuth    bool   `env:"RTCSFU_SIGNAL_REQUIRE_AUTH"`    // if true, signal WS requires same auth as join when API key is set

	JoinTokenSecret            string `env:"RTCSFU_JOIN_TOKEN_SECRET"`              // HMAC secret for short-lived room tokens (optional)
	JoinTokenDefaultTTLSeconds int    `env:"RTCSFU_JOIN_TOKEN_DEFAULT_TTL_SECONDS"` // mint default when ttl_sec omitted
	JoinTokenMaxTTLSeconds     int    `env:"RTCSFU_JOIN_TOKEN_MAX_TTL_SECONDS"`     // cap mint ttl; 0 = use default cap in handler

	// ClusterRole is informational + gates some admin APIs: primary | replica | standalone (default standalone).
	ClusterRole string `env:"RTCSFU_CLUSTER_ROLE"`

	// Replica → primary registration (HTTP). Auth: X-RTCSFU-Key must match primary RTCSFU_API_KEY.
	PrimaryBaseURL      string `env:"RTCSFU_PRIMARY_BASE_URL"`      // e.g. https://primary.example.com:7075
	ReplicaSelfJSON     string `env:"RTCSFU_REPLICA_SELF_JSON"`     // one node object or array (same shape as RTCSFU_NODES items)
	ReplicaHeartbeatSec int    `env:"RTCSFU_REPLICA_HEARTBEAT_SEC"` // interval for re-register; 0 = default 30
	// ReplicaHeartbeatMode: register (full JSON each tick) or touch (POST .../replica/touch after one initial register).
	ReplicaHeartbeatMode string `env:"RTCSFU_REPLICA_HEARTBEAT_MODE"`

	ReplicaMTLSClientCAFile       string `env:"RTCSFU_REPLICA_MTLS_CA_FILE"`              // PEM CA to verify primary server cert
	ReplicaMTLSClientCertFile     string `env:"RTCSFU_REPLICA_MTLS_CERT_FILE"`            // client cert for mTLS
	ReplicaMTLSClientKeyFile      string `env:"RTCSFU_REPLICA_MTLS_KEY_FILE"`             // client key
	ReplicaMTLSInsecureSkipVerify bool   `env:"RTCSFU_REPLICA_MTLS_INSECURE_SKIP_VERIFY"` // dev only

	// Primary: if last_seen older than this many seconds, replica is treated as unhealthy for routing (0 = off).
	ReplicaStaleSeconds int `env:"RTCSFU_REPLICA_STALE_SECONDS"`
	// Primary: if set, POST .../replica/touch must include valid HMAC (Bearer or JSON touch_token) bound to id.
	ReplicaTouchHMACSecret string `env:"RTCSFU_REPLICA_TOUCH_HMAC_SECRET"`
	// Replica: TTL for minted touch tokens (seconds); 0 = default 120.
	ReplicaTouchTokenTTLSeconds int `env:"RTCSFU_REPLICA_TOUCH_TOKEN_TTL_SECONDS"`
}

// ServerConfig server configuration
type ServerConfig struct {
	Name         string `env:"SERVER_NAME"`
	Desc         string `env:"SERVER_DESC"`
	URL          string `env:"SERVER_URL"`
	Logo         string `env:"SERVER_LOGO"`
	TermsURL     string `env:"SERVER_TERMS_URL"`
	Addr         string `env:"ADDR"`
	Mode         string `env:"MODE"`
	APISecretKey string `env:"API_SECRET_KEY"`
	SSLEnabled   bool   `env:"SSL_ENABLED"`
	SSLCertFile  string `env:"SSL_CERT_FILE"`
	SSLKeyFile   string `env:"SSL_KEY_FILE"`
}

// DatabaseConfig database configuration
type DatabaseConfig struct {
	Driver string `env:"DB_DRIVER"`
	DSN    string `env:"DSN"`
}

// ServicesConfig services configuration
type ServicesConfig struct {
	Storage StorageConfig `mapstructure:"storage"`
}

// StorageConfig storage configuration
type StorageConfig struct {
	BaseURL   string `env:"LINGSTORAGE_BASE_URL"`
	APIKey    string `env:"LINGSTORAGE_API_KEY"`
	APISecret string `env:"LINGSTORAGE_API_SECRET"`
	Bucket    string `env:"LINGSTORAGE_BUCKET"`
}

func Load() error {
	// 1. Load .env file based on the environment (don't error if it doesn't exist, use default values)
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
			Name:         utils.GetStringOrDefault("SERVER_NAME", ""),
			Desc:         utils.GetStringOrDefault("SERVER_DESC", ""),
			URL:          utils.GetStringOrDefault("SERVER_URL", ""),
			Logo:         utils.GetStringOrDefault("SERVER_LOGO", ""),
			TermsURL:     utils.GetStringOrDefault("SERVER_TERMS_URL", ""),
			Addr:         utils.GetStringOrDefault("ADDR", ":7070"),
			Mode:         utils.GetStringOrDefault("MODE", "development"),
			APISecretKey: utils.GetStringOrDefault("API_SECRET_KEY", ""),
			SSLEnabled:   utils.GetBoolOrDefault("SSL_ENABLED", false),
			SSLCertFile:  utils.GetStringOrDefault("SSL_CERT_FILE", ""),
			SSLKeyFile:   utils.GetStringOrDefault("SSL_KEY_FILE", ""),
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
		Services: ServicesConfig{
			Storage: StorageConfig{
				BaseURL:   utils.GetStringOrDefault("LINGSTORAGE_BASE_URL", "https://api.lingstorage.com"),
				APIKey:    utils.GetStringOrDefault("LINGSTORAGE_API_KEY", ""),
				APISecret: utils.GetStringOrDefault("LINGSTORAGE_API_SECRET", ""),
				Bucket:    utils.GetStringOrDefault("LINGSTORAGE_BUCKET", "default"),
			},
		},
		RTCSFU: RTCSFUConfig{
			Enabled:                       utils.GetBoolOrDefault("RTCSFU_ENABLED", false),
			NodesJSON:                     utils.GetStringOrDefault("RTCSFU_NODES", ""),
			APIKey:                        utils.GetStringOrDefault("RTCSFU_API_KEY", ""),
			ICEServersJSON:                utils.GetStringOrDefault("RTCSFU_ICE_SERVERS_JSON", ""),
			MaxRooms:                      utils.GetIntOrDefault("RTCSFU_MAX_ROOMS", 5000),
			MaxPeersPerRoom:               utils.GetIntOrDefault("RTCSFU_MAX_PEERS_PER_ROOM", 50),
			WSReadTimeoutSec:              utils.GetIntOrDefault("RTCSFU_WS_READ_TIMEOUT_SEC", 70),
			WSPingIntervalSec:             utils.GetIntOrDefault("RTCSFU_WS_PING_INTERVAL_SEC", 25),
			WSMaxMessageBytes:             utils.GetIntOrDefault("RTCSFU_WS_MAX_MESSAGE_BYTES", 786432),
			SignalAllowedOrigins:          utils.GetStringOrDefault("RTCSFU_SIGNAL_ALLOWED_ORIGINS", ""),
			SignalRequireAuth:             utils.GetBoolOrDefault("RTCSFU_SIGNAL_REQUIRE_AUTH", false),
			JoinTokenSecret:               utils.GetStringOrDefault("RTCSFU_JOIN_TOKEN_SECRET", ""),
			JoinTokenDefaultTTLSeconds:    utils.GetIntOrDefault("RTCSFU_JOIN_TOKEN_DEFAULT_TTL_SECONDS", 3600),
			JoinTokenMaxTTLSeconds:        utils.GetIntOrDefault("RTCSFU_JOIN_TOKEN_MAX_TTL_SECONDS", 0),
			ClusterRole:                   utils.GetStringOrDefault("RTCSFU_CLUSTER_ROLE", "standalone"),
			PrimaryBaseURL:                utils.GetStringOrDefault("RTCSFU_PRIMARY_BASE_URL", ""),
			ReplicaSelfJSON:               utils.GetStringOrDefault("RTCSFU_REPLICA_SELF_JSON", ""),
			ReplicaHeartbeatSec:           utils.GetIntOrDefault("RTCSFU_REPLICA_HEARTBEAT_SEC", 30),
			ReplicaHeartbeatMode:          utils.GetStringOrDefault("RTCSFU_REPLICA_HEARTBEAT_MODE", "register"),
			ReplicaMTLSClientCAFile:       utils.GetStringOrDefault("RTCSFU_REPLICA_MTLS_CA_FILE", ""),
			ReplicaMTLSClientCertFile:     utils.GetStringOrDefault("RTCSFU_REPLICA_MTLS_CERT_FILE", ""),
			ReplicaMTLSClientKeyFile:      utils.GetStringOrDefault("RTCSFU_REPLICA_MTLS_KEY_FILE", ""),
			ReplicaMTLSInsecureSkipVerify: utils.GetBoolOrDefault("RTCSFU_REPLICA_MTLS_INSECURE_SKIP_VERIFY", false),
			ReplicaStaleSeconds:           utils.GetIntOrDefault("RTCSFU_REPLICA_STALE_SECONDS", 0),
			ReplicaTouchHMACSecret:        utils.GetStringOrDefault("RTCSFU_REPLICA_TOUCH_HMAC_SECRET", ""),
			ReplicaTouchTokenTTLSeconds:   utils.GetIntOrDefault("RTCSFU_REPLICA_TOUCH_TOKEN_TTL_SECONDS", 0),
		},
	}
	GlobalStore = lingstorage.NewClient(&lingstorage.Config{
		BaseURL:   GlobalConfig.Services.Storage.BaseURL,
		APIKey:    GlobalConfig.Services.Storage.APIKey,
		APISecret: GlobalConfig.Services.Storage.APISecret,
	})
	return nil
}
