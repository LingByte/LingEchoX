package main

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LinByte/VoiceServer/cmd/bootstrap"
	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/handlers"
	"github.com/LinByte/VoiceServer/internal/listeners"
	"github.com/LinByte/VoiceServer/internal/sipserver"
	"github.com/LinByte/VoiceServer/internal/tasks"
	"github.com/LinByte/VoiceServer/pkg/config"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/utils/backup"
	voiceMetrics "github.com/LinByte/VoiceServer/pkg/voice/metrics"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LingEchoApp struct {
	db       *gorm.DB
	handlers *handlers.Handlers
}

func NewLingEchoApp(db *gorm.DB) *LingEchoApp {
	return &LingEchoApp{
		db:       db,
		handlers: handlers.NewHandlers(db),
	}
}

func (app *LingEchoApp) RegisterRoutes(r *gin.Engine) {
	// Register system routes (with /api prefix)
	app.handlers.Register(r)
}

func main() {
	// 1. Parse command-line parameters
	init := flag.Bool("init", false, "run database schema migration at startup (default: true)")
	seed := flag.Bool("seed", false, "seed database")
	mode := flag.String("mode", "", "running environment (development, test, production)")
	initSQL := flag.String("init-sql", "", "optional tenant seed .sql (e.g. scripts/sql/init.sql); only runs when this flag is set")
	sipHost := flag.String("sip-host", "0.0.0.0", "embedded SIP UDP listen host")
	sipPort := flag.Int("sip-port", 6050, "embedded SIP UDP listen port")
	sipLocalIP := flag.String("sip-local-ip", "127.0.0.1", "Advertised IP for SDP c= AND for SIP Via/Contact when -sip-host is 0.0.0.0 (must be reachable by LAN phones for outbound/campaign; avoid 127.0.0.1)")
	flag.Parse()

	// 2. Set environment variables
	if *mode != "" {
		os.Setenv("MODE", *mode)
	}

	// 3. Load global configuration
	if err := config.Load(); err != nil {
		panic("config load failed: " + err.Error())
	}

	// 4. Print banner
	if err := bootstrap.PrintBannerFromFile("banner.txt", config.GlobalConfig.Server.Name); err != nil {
		log.Fatalf("unload banner: %v", err)
	}

	// 5. Initialize logger
	err := logger.Init(&config.GlobalConfig.Log, config.GlobalConfig.Server.Mode)
	if err != nil {
		panic(err)
	}

	// 6. Log configuration
	bootstrap.LogConfigInfo()
	// 7. Setup database
	db, err := bootstrap.SetupDatabase(os.Stdout, &bootstrap.Options{
		InitSQLPath: *initSQL,
		AutoMigrate: *init,
		SeedNonProd: *seed,
	})

	if err != nil {
		logger.Error("database setup failed", zap.Error(err))
		return
	}

	if err := bootstrap.InitializeKeyManager(); err != nil {
		logger.Error("key manager initialization failed", zap.Error(err))
		return
	}

	// 8. Resolve listen address
	var addr = config.GlobalConfig.Server.Addr
	if addr == "" {
		addr = ":8082"
	}

	// 9. Create application
	app := NewLingEchoApp(db)
	sipUserCleaner := tasks.NewSIPUserOnlineCleaner(db, time.Duration(utils.GetIntEnv("SIP_USER_ONLINE_SWEEP_SECONDS"))*time.Second)
	sipUserCleaner.Start()
	if config.GlobalConfig.Features.BackupEnabled {
		backup.StartBackupScheduler(db)
	}

	// 10. Initialize Gin engine
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()        // Use gin.New() instead of gin.Default() to avoid automatic redirects
	r.Use(gin.Recovery()) // Manually add Recovery middleware
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.MaxMultipartMemory = 32 << 20 // 32 MB

	// Cookie Register
	secret := utils.GetEnv(constants.ENV_SESSION_SECRET)
	if secret != "" {
		expireDays := utils.GetIntEnv(constants.ENV_SESSION_EXPIRE_DAYS)
		if expireDays <= 0 {
			expireDays = 7
		}
		r.Use(middleware.WithCookieSession(secret, int(expireDays)*24*3600))
	} else {
		r.Use(middleware.WithMemSession(utils.RandText(32)))
	}

	// Cors Handle Middleware
	r.Use(middleware.CorsMiddleware())

	// Request id (X-Reqid) for all HTTP requests — must run before access log + handlers.
	r.Use(middleware.RequestIDMiddleware())

	// HTTP access log (includes x-reqid)
	r.Use(middleware.LoggerMiddleware(logger.Lg))

	// 11. Register routes
	app.RegisterRoutes(r)

	// Expose the in-process metrics registry over Prometheus text
	// exposition. Default-deny via METRICS_ALLOWED_IPS — this used to
	// be wide-open and leaked deployment topology / live traffic
	// patterns to anyone who could reach the listener. Configure the
	// env to a comma list of IPs or CIDRs (e.g. "127.0.0.1,10.0.0.0/8")
	// or "*" if you front /metrics with mTLS / k8s NetworkPolicy.
	r.GET("/metrics", middleware.MetricsACL(), gin.WrapH(voiceMetrics.Handler()))
	// 12. Initialize system listeners
	listeners.InitSystemListeners()
	// 13. Emit system initialization signal
	utils.Sig().Emit(constants.SigInitSystemConfig, nil)

	// 14. Start embedded SIP stack
	var sipEmbedded *sipserver.Embedded
	se, err := sipserver.Start(sipserver.Config{
		Host:    *sipHost,
		Port:    *sipPort,
		LocalIP: *sipLocalIP,
		DB:      db,
	})
	if err != nil {
		logger.Fatal("embedded SIP stack failed to start", zap.Error(err))
	}
	sipEmbedded = se
	app.handlers.SetCampaignService(sipEmbedded.CampaignService())
	logger.Info("embedded SIP stack started",
		zap.String("sip_host", *sipHost),
		zap.Int("sip_port", *sipPort),
		zap.String("sip_local_ip", *sipLocalIP))

	// 15. Start HTTP/HTTPS server
	const httpLongRun = 45 * time.Minute
	httpServer := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    httpLongRun,
		WriteTimeout:   httpLongRun,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	shutdownAll := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		sipUserCleaner.Stop()
		if sipEmbedded != nil {
			sipEmbedded.Shutdown(ctx)
		}
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("HTTP server shutdown", zap.Error(err))
		}
	}

	if config.GlobalConfig.Server.SSLEnabled && listeners.IsSSLEnabled() {
		tlsConfig, err := listeners.GetTLSConfig()
		if err != nil {
			logger.Error("failed to get TLS config", zap.Error(err))
			return
		}
		if tlsConfig != nil {
			httpServer.TLSConfig = tlsConfig
		} else {
			logger.Warn("SSL enabled but TLS config is nil, falling back to HTTP")
		}
	}

	go func() {
		var err error
		if httpServer.TLSConfig != nil {
			logger.Info("Starting HTTPS server", zap.String("addr", addr))
			err = httpServer.ListenAndServeTLS("", "")
		} else {
			logger.Info("Starting HTTP server", zap.String("addr", addr))
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server run failed", zap.Error(err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutdown signal received")
	shutdownAll()
}
