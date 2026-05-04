package handlers

import (
	"net/http"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/sip/webseat"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) lingechoWebSeatStatus(c *gin.Context) {
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
