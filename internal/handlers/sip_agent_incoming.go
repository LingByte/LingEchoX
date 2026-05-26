package handlers

import (
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/sip/conversation"
	"github.com/LinByte/VoiceServer/pkg/sip/sipagentpoll"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
)

// parseACDTargetIDQuery reads acdTargetId (snowflake-safe decimal string).
func parseACDTargetIDQuery(c *gin.Context) (id uint, ok bool, invalid bool) {
	s := strings.TrimSpace(c.Query("acdTargetId"))
	if s == "" {
		return 0, false, false
	}
	id, err := utils.ParseID(s)
	if err != nil {
		return 0, false, true
	}
	return id, true, false
}

// pollSIPAgentIncoming answers whether a SIP ACD seat (acd_pool_targets, route_type=sip)
// currently has an inbound transfer ringing toward it.
//
// Query (at least one):
//   - acdTargetId — primary key of acd_pool_targets
//   - name — admin label on the pool row (exact match)
//   - targetValue — SIP username / dial target on the pool row (exact match)
func (h *Handlers) pollSIPAgentIncoming(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	acdID, hasID, badID := parseACDTargetIDQuery(c)
	name := strings.TrimSpace(c.Query("name"))
	targetValue := strings.TrimSpace(c.Query("targetValue"))
	if badID && name == "" && targetValue == "" {
		response.Fail(c, "invalid acdTargetId", nil)
		return
	}
	if !hasID && name == "" && targetValue == "" {
		response.Fail(c, "need acdTargetId, name, or targetValue", nil)
		return
	}

	row, ok, err := models.FindSIPACDPoolTargetForIncomingPoll(h.db, tid, acdID, name, targetValue)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if !ok || row.RouteType != constants.ACDPoolRouteTypeSIP {
		response.Fail(c, "not found", nil)
		return
	}

	snap := sipagentpoll.ResolveSnapshot(row.ID, conversation.IsTransferInProgress)
	out := gin.H{
		"incoming":      snap.Incoming,
		"acdTargetId":   row.ID,
		"seatName":      strings.TrimSpace(row.Name),
		"targetValue":   strings.TrimSpace(row.TargetValue),
		"routeType":     row.RouteType,
		"callId":        snap.InboundCallID,
		"callerNumber":  snap.CallerNumber,
		"phase":         snap.Phase,
		"since":         snap.Since,
	}
	response.Success(c, "success", out)
}

// listSIPAgentIncomingLogs returns persisted transfer-offer history for one SIP seat.
func (h *Handlers) listSIPAgentIncomingLogs(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	acdID, hasID, badID := parseACDTargetIDQuery(c)
	if !hasID || badID {
		response.Fail(c, "invalid acdTargetId", nil)
		return
	}
	if _, ok, err := models.FindSIPACDPoolTargetForIncomingPoll(h.db, tid, acdID, "", ""); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	} else if !ok {
		response.Fail(c, "not found", nil)
		return
	}
	page, size := ginutil.QueryPage(c, 20)
	list, total, err := models.ListSIPACDTransferOffersPage(c.Request.Context(), h.db, tid, acdID, page, size)
	if ginutil.WriteInternalError(c, err) {
		return
	}
	ginutil.PageSuccess(c, list, total, page, size)
}
