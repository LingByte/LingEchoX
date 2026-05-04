package handlers

import (
	"net/http"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/sip/voicedialog"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// voiceDialogWebSocket upgrades GET …/ws to WebSocket (?token= [&call_id=]).
// HTTP concerns (method, status bodies, token gate) stay here; dialog protocol + SIP gateway wiring remain in pkg/sip/voicedialog.
func voiceDialogWebSocket(c *gin.Context) {
	w, r := c.Writer, c.Request
	remote := r.RemoteAddr

	if !voicedialog.WebSocketHubReady() {
		logger.Warn("voicedialog ws: upgrade refused (hub not initialized)", zap.String("remote", remote))
		http.Error(w, "voice dialog not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		logger.Warn("voicedialog ws: wrong method", zap.String("remote", remote), zap.String("method", r.Method))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !voicedialog.WebSocketTokenOK(r) {
		logger.Warn("voicedialog ws: upgrade refused (token)", zap.String("remote", remote))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callID := strings.TrimSpace(r.URL.Query().Get("call_id"))
	conn, err := voicedialog.UpgradeVoiceDialogWebSocket(w, r)
	if err != nil {
		logger.Warn("voicedialog ws: Upgrade failed",
			zap.String("remote", remote),
			zap.String(voicedialog.KeyCallID, callID),
			zap.Error(err),
		)
		return
	}
	voicedialog.ServeVoiceDialogWebSocket(conn, callID)
}
