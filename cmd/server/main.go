package main

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"flag"
	"log"
	"os"

	"github.com/LingByte/SoulNexus/cmd/bootstrap"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LingEchoXApp struct {
	db *gorm.DB
}

func NewLingEchoXApp(db *gorm.DB) *LingEchoXApp {
	return &LingEchoXApp{
		db: db,
	}
}

func main() {
	// 1. Parse Command Line Parameters
	mode := flag.String("mode", "", "running environment (development, test, production)")
	init := flag.Bool("init", false, "initialize database")
	initSQL := flag.String("init-sql", "", "path to database init .sql script (optional)")
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

	NewLingEchoXApp(db)
}
