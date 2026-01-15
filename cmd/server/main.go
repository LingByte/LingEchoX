package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/LingByte/LingEchoX/cmd/bootstrap"
	"github.com/LingByte/LingEchoX/internal/listeners"
	"github.com/LingByte/LingEchoX/pkg/config"
	"github.com/LingByte/LingEchoX/pkg/logger"
	"github.com/LingByte/LingEchoX/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LingEchoX struct {
	db *gorm.DB
}

func NewLingEchoX(db *gorm.DB) *LingEchoX {
	return &LingEchoX{
		db: db,
	}
}

func main() {
	// 2. Parse Command Line Parameters
	addr := flag.String("port", "8080", "Port to listen on")
	mode := flag.String("mode", "", "running environment (development, test, production)")
	init := flag.Bool("init", false, "initialize database")
	initSQL := flag.String("init-sql", "", "path to database init .sql script (optional)")
	flag.Parse()
	os.Setenv("MODE", *mode)
	// 4. Load Global Configuration
	if err := config.Load(); err != nil {
		panic("config load failed: " + err.Error())
	}
	// 5. Load Log Configuration
	err := logger.Init(&config.GlobalConfig.Log, config.GlobalConfig.Mode)
	if err != nil {
		panic(err)
	}
	// 5. Print Banner
	if err := bootstrap.PrintBannerFromFile("banner.txt", config.GlobalConfig.ServerName); err != nil {
		log.Fatalf("unload banner: %v", err)
	}
	// 6. Print Configuration
	bootstrap.LogConfigInfo()
	// 7. Load Data Source
	db, err := bootstrap.SetupDatabase(os.Stdout, &bootstrap.Options{
		InitSQLPath: *initSQL, // Can be specified via --init-sql
		AutoMigrate: *init,    // Whether to migrate entities
		SeedNonProd: *init,    // Non-production default configuration
	})
	if err != nil {
		logger.Error("database setup failed", zap.Error(err))
		return
	}

	// 8. Load Base Configs
	if *addr == "" {
		*addr = config.GlobalConfig.Addr
	}
	if !strings.HasPrefix(*addr, ":") {
		*addr = ":" + *addr
	}

	var DBDriver = config.GlobalConfig.DBDriver
	if DBDriver == "" {
		DBDriver = "sqlite"
	}

	var DSN = config.GlobalConfig.DSN
	if DSN == "" {
		DSN = "file::memory:?cache=shared"
	}
	flag.StringVar(addr, "addr", *addr, "HTTP Serve address")
	flag.StringVar(&DBDriver, "db-driver", DBDriver, "database driver")
	flag.StringVar(&DSN, "dsn", DSN, "database source name")

	logger.Info("checked config -- addr: ", zap.String("addr", *addr))
	logger.Info("checked config -- db-driver: ", zap.String("db-driver", DBDriver), zap.String("dsn", DSN))
	logger.Info("checked config -- mode: ", zap.String("mode", config.GlobalConfig.Mode))
	// 9. Load Global Cache (new cache system)
	utils.InitGlobalCache(1024, 5*time.Minute)
	// 10. New App
	_ = NewLingEchoX(db)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()        // Use gin.New() instead of gin.Default() to avoid automatic redirects
	r.Use(gin.Recovery()) // Manually add Recovery middleware

	// Note: Templates are embedded as individual strings and used directly in email notifications
	// No need to load HTML templates for this API server

	// Disable automatic redirects to avoid CORS issues caused by 307 redirects
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// Set maximum memory limit for multipart forms (32MB)
	r.MaxMultipartMemory = 32 << 20 // 32 MB
	// 11. Init System Listeners
	listeners.InitSystemListeners()
	httpServer := &http.Server{
		Addr:           *addr,
		Handler:        r,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Check if SSL is enabled
	if config.GlobalConfig.SSLEnabled && listeners.IsSSLEnabled() {
		tlsConfig, err := listeners.GetTLSConfig()
		if err != nil {
			logger.Error("failed to get TLS config", zap.Error(err))
			return
		}

		if tlsConfig != nil {
			httpServer.TLSConfig = tlsConfig
			logger.Info("Starting HTTPS server", zap.String("addr", *addr))
			if err := httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTPS server run failed", zap.Error(err))
			}
		} else {
			logger.Warn("SSL enabled but TLS config is nil, falling back to HTTP")
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP server run failed", zap.Error(err))
			}
		}
	} else {
		logger.Info("Starting HTTP server Port is", zap.String("addr", *addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server run failed", zap.Error(err))
		}
	}
}
