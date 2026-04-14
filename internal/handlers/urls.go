package handlers

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"net/http"
	"time"

	"github.com/LingByte/SoulNexus/internal/sfu"
	"github.com/LingByte/SoulNexus/internal/sipserver"
	"github.com/LingByte/SoulNexus/pkg/config"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/rtcsfu"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Handlers struct {
	db                   *gorm.DB
	campaignSvc          *sipserver.CampaignService
	rtcsfu               *rtcsfu.ControlPlane
	sfuEng               *sfu.Engine
	p2p                  *sfu.P2PBroker
	signalCheckOrigin    func(*http.Request) bool
	rtcsfuICEClientJSON  []byte
	rtcsfuWSMaxReadBytes int64
}

func NewHandlers(db *gorm.DB) *Handlers {
	h := &Handlers{db: db}
	cfg := config.GlobalConfig.RTCSFU
	h.signalCheckOrigin = BuildRTCSFUSignalOriginChecker(config.GlobalConfig.Server.Mode, cfg.SignalAllowedOrigins)
	h.rtcsfuWSMaxReadBytes = int64(cfg.WSMaxMessageBytes)
	if h.rtcsfuWSMaxReadBytes <= 0 {
		h.rtcsfuWSMaxReadBytes = 786432
	}

	iceServers, iceClientJSON, err := sfu.ParseICEServersJSON(cfg.ICEServersJSON)
	if err != nil {
		logger.Warn("RTCSFU ICE config invalid, using defaults", zap.Error(err))
		iceServers, iceClientJSON, _ = sfu.ParseICEServersJSON(sfu.DefaultICEServersJSON)
	}
	h.rtcsfuICEClientJSON = iceClientJSON

	if cfg.Enabled {
		h.sfuEng = sfu.NewEngine(sfu.Options{
			ICEServers:      iceServers,
			MaxRooms:        cfg.MaxRooms,
			MaxPeersPerRoom: cfg.MaxPeersPerRoom,
			WSReadTimeout:   time.Duration(cfg.WSReadTimeoutSec) * time.Second,
			WSPingInterval:  time.Duration(cfg.WSPingIntervalSec) * time.Second,
		})
		h.p2p = sfu.NewP2PBroker()
		logger.Info("RTCSFU Pion SFU engine started",
			zap.Int("max_rooms", cfg.MaxRooms),
			zap.Int("max_peers_per_room", cfg.MaxPeersPerRoom),
		)
		if cfg.NodesJSON != "" {
			nodes, err := rtcsfu.ParseNodesJSON([]byte(cfg.NodesJSON))
			if err != nil {
				logger.Warn("RTCSFU routing disabled: invalid RTCSFU_NODES", zap.Error(err))
			} else if len(nodes) == 0 {
				logger.Warn("RTCSFU routing disabled: RTCSFU_NODES parsed to empty list")
			} else {
				h.rtcsfu = rtcsfu.NewControlPlane(nodes, cfg.ReplicaStaleSeconds)
				logger.Info("RTCSFU control plane initialized", zap.Int("nodes", len(nodes)))
			}
		}
	}
	return h
}

// SetCampaignService wires the embedded SIP outbound worker (optional). Call after sipserver.Start
// so Gin routes can expose dial-side counters (e.g. GET .../sip-center/campaigns/worker-metrics).
func (h *Handlers) SetCampaignService(svc *sipserver.CampaignService) {
	if h == nil {
		return
	}
	h.campaignSvc = svc
}

func (h *Handlers) Register(engine *gin.Engine) {
	engine.StaticFile("/rtcsfu_demo.html", "static/rtcsfu_demo.html")
	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r := engine.Group("/api")
	h.RegisterCredentialRoutes(r)
	h.registerVoiceDialogueRoutes(r)
	h.registerOpenAPIRoutes(r)
	h.registerRTCSFURoutes(r)

	// Register Global Singleton DB
	r.Use(middleware.InjectDB(h.db))
	h.registerSIPContactCenterRoutes(r)
	h.registerLingechoWebSeatRoutes(r)
}
