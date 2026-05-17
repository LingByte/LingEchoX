package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
	"github.com/gin-gonic/gin"
)

// registerLingechoWebSeatRoutes mounts the browser-agent (web-seat)
// signaling endpoints. These are intentionally NOT JWT/AKSK-gated:
// the webseat package owns a per-call short-lived token (?token=...)
// that authorises the WebSocket and HTTP control endpoints. Mounting
// these inside the JWT group would block the browser-side flow.
func (h *Handlers) registerLingechoWebSeatRoutes(r *gin.RouterGroup) {
	g := r.Group(constants.LingechoWebSeatPathPrefix)
	{
		g.POST("/join", gin.WrapF(webseat.JoinHTTP))
		g.POST("/hangup", gin.WrapF(webseat.HangupHTTP))
		g.POST("/reject", gin.WrapF(webseat.RejectHTTP))
		g.GET("/ws", gin.WrapF(webseat.WebSocketHTTP))
		g.GET("/status/:callId", h.lingechoWebSeatStatus)
	}
}
