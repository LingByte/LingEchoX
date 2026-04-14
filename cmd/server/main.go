package main

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LingByte/SoulNexus/cmd/bootstrap"
	"github.com/LingByte/SoulNexus/internal/handlers"
	"github.com/LingByte/SoulNexus/internal/listeners"
	"github.com/LingByte/SoulNexus/internal/rtcsfu_replica"
	"github.com/LingByte/SoulNexus/internal/sipserver"
	"github.com/LingByte/SoulNexus/internal/tasks"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LingEchoXApp struct {
	db       *gorm.DB
	handlers *handlers.Handlers
}

func NewLingEchoXApp(db *gorm.DB) *LingEchoXApp {
	return &LingEchoXApp{
		db:       db,
		handlers: handlers.NewHandlers(db),
	}
}

func (app *LingEchoXApp) RegisterRoutes(r *gin.Engine) {
	app.handlers.Register(r)
}

func main() {
	// 1. Parse Command Line Parameters
	mode := flag.String("mode", "", "running environment (development, test, production)")
	init := flag.Bool("init", false, "initialize database")
	initSQL := flag.String("init-sql", "", "path to database init .sql script (optional)")
	sipHost := flag.String("sip-host", "0.0.0.0", "embedded SIP UDP listen host")
	sipPort := flag.Int("sip-port", 5060, "embedded SIP UDP listen port")
	sipLocalIP := flag.String("sip-local-ip", "127.0.0.1", "SDP c= line IP (RTP reachable from SIP peers)")
	flag.Parse()

	// 2. Set Environment Variables
	if *mode != "" {
		os.Setenv("APP_ENV", *mode)
	}

	// 3. Load Global Configuration
	if err := config.Load(); err != nil {
		panic("config load failed: " + err.Error())
	}

	// 4. Load Log Configuration
	err := logger.Init(&config.GlobalConfig.Log, config.GlobalConfig.Server.Mode)
	if err != nil {
		panic(err)
	}

	// 5. Print Banner
	if err := bootstrap.PrintBannerFromFile("banner.txt", config.GlobalConfig.Server.Name); err != nil {
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
	var addr = config.GlobalConfig.Server.Addr
	if addr == "" {
		addr = ":7075"
	}

	var DBDriver = config.GlobalConfig.Database.Driver
	if DBDriver == "" {
		DBDriver = "sqlite"
	}

	var DSN = config.GlobalConfig.Database.DSN
	if DSN == "" {
		DSN = "file::memory:?cache=shared"
	}
	flag.StringVar(&addr, "addr", addr, "HTTP Serve address")
	flag.StringVar(&DBDriver, "db-driver", DBDriver, "database driver")
	flag.StringVar(&DSN, "dsn", DSN, "database source name")

	logger.Info("checked config -- addr: ", zap.String("addr", addr))
	logger.Info("checked config -- db-driver: ", zap.String("db-driver", DBDriver), zap.String("dsn", DSN))
	logger.Info("checked config -- mode: ", zap.String("mode", config.GlobalConfig.Server.Mode))

	app := NewLingEchoXApp(db)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()        // Use gin.New() instead of gin.Default() to avoid automatic redirects
	r.Use(gin.Recovery()) // Manually add Recovery middleware

	// Disable automatic redirects to avoid CORS issues caused by 307 redirects
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// Set maximum memory limit for multipart forms (32MB)
	r.MaxMultipartMemory = 32 << 20 // 32 MB
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

	// 13. Cors Handle Middleware
	r.Use(middleware.CorsMiddleware())

	// 14. Logger Handle Middleware
	r.Use(middleware.LoggerMiddleware(zap.L()))
	middleware.SetRateLimiterConfig(middleware.RateLimiterConfig{
		Rate:        "1000-M", // Global: 1000 requests per minute
		Identifier:  "ip",
		AddHeaders:  true,
		DenyStatus:  429,
		DenyMessage: "请求过于频繁，请稍后再试 / Requests too frequent, please try again later",
		PerRouteRates: map[string]string{
			// 存储相关接口限流
			"/api/public/upload":     "100-M", // 上传接口：100次/分钟
			"/api/storage/buckets":   "200-M", // 存储桶列表：200次/分钟
			"/api/storage/buckets/*": "300-M", // 存储桶操作：300次/分钟
			"/api/public/files/*":    "500-M", // 文件操作：500次/分钟
			"/api/public/buckets":    "200-M", // 公共存储桶接口：200次/分钟
			"/api/public/buckets/*":  "300-M", // 公共存储桶操作：300次/分钟

			// 认证相关接口限流
			"/api/auth/login":          "20-M", // 登录：20次/分钟
			"/api/auth/register":       "10-M", // 注册：10次/分钟
			"/api/auth/reset-password": "10-M", // 重置密码：10次/分钟

			// 配置管理接口限流
			"/api/configs":   "100-M", // 配置读取：100次/分钟
			"/api/configs/*": "50-M",  // 配置修改：50次/分钟

			// 用户管理接口限流
			"/api/users":   "100-M", // 用户列表：100次/分钟
			"/api/users/*": "200-M", // 用户操作：200次/分钟

			"/api/rtcsfu/v1/join":                      "120-M", // SFU 房间分配
			"/api/rtcsfu/v1/token":                     "30-M",  // 签发 join_token
			"/api/rtcsfu/v1/admin/reload-nodes":        "30-M",  // 运维：热更新节点列表
			"/api/rtcsfu/v1/admin/nodes/register":      "20-M",  // 副节点注册
			"/api/rtcsfu/v1/admin/nodes/replica/touch": "60-M",  // 轻量心跳
			"/api/rtcsfu/v1/admin/nodes":               "60-M",  // 节点列表
		},
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/static/",
			"/uploads/",
			"/media/",
			"/admin",   // 管理后台静态资源
			"/admin/*", // 管理后台路由
			"/api/rtcsfu/v1/signal",
			"/api/rtcsfu/v1/p2p",
			"/api/rtcsfu/v1/ready",
		},
	})
	r.Use(middleware.RateLimiterMiddleware())

	// 16. Register Routes
	app.RegisterRoutes(r)
	sipUserCleaner := tasks.NewSIPUserOnlineCleaner(db, time.Duration(utils.GetIntEnv("SIP_USER_ONLINE_SWEEP_SECONDS"))*time.Second)
	sipUserCleaner.Start()
	// 17. Initialize System Listeners
	listeners.InitSystemListeners()
	go rtcsfu_replica.RunAnnouncer()

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
	// 18. Start HTTP/HTTPS Server
	httpServer := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
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
