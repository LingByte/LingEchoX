package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/gin-gonic/gin"
)

// registerVoiceDialogRoutes mounts the WebSocket endpoint that the
// SIP-loopback voice-dialog flow connects to. Like the web-seat path
// it has its own (?token=...) gate inside voicedialog.WebSocketTokenOK,
// and the SIP loopback never carries a JWT — keep this route on the
// unprotected group.
func (h *Handlers) registerVoiceDialogRoutes(r *gin.RouterGroup) {
	g := r.Group(constants.LingechoVoiceDialogPathPrefix)
	g.GET("/ws", voiceDialogWebSocket)
}
