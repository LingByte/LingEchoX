package handlers

import (
	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/LingByte/SoulNexus/pkg/sip/voicedialog"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) registerVoiceDialogRoutes(r *gin.RouterGroup) {
	g := r.Group(constants.LingechoVoiceDialogPathPrefix)
	g.GET("/ws", voiceDialogWebSocket)
}

// voiceDialogWebSocket handles Lingecho voice-dialog WebSocket upgrade (GET …/ws).
// Upgrade and hub wiring are implemented in pkg/sip/voicedialog.WebSocketHTTP.
func voiceDialogWebSocket(c *gin.Context) {
	voicedialog.WebSocketHTTP(c.Writer, c.Request)
}
