package handlers

import (
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/sip/webseat"
	"github.com/gin-gonic/gin"
)

// lingechoWebSeatStatus reports whether a Call-ID is in webseat
// pending/active state. Even though it returns just one boolean, the
// endpoint is gated by the same WebSeat token used by /join /hangup
// /reject — leaving it open lets a probe enumerate which Call-IDs
// are live (useful as a precursor to a hangup/join attack and as a
// general traffic-pattern leak).
func (h *Handlers) lingechoWebSeatStatus(c *gin.Context) {
	if !webseat.HTTPTokenOK(c.Request) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callID := strings.TrimSpace(c.Param("callId"))
	if callID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"call_id":           callID,
		"pending_or_active": webseat.IsPendingOrActive(callID),
	})
}
