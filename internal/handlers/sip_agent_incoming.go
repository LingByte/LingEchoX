package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	response.Success(c, "success", sipAgentIncomingPayload(row, snap))
}

func sipAgentIncomingPayload(row models.ACDPoolTarget, snap sipagentpoll.Snapshot) gin.H {
	return gin.H{
		"incoming":     snap.Incoming,
		"acdTargetId":  row.ID,
		"seatName":     strings.TrimSpace(row.Name),
		"targetValue":  strings.TrimSpace(row.TargetValue),
		"routeType":    row.RouteType,
		"callId":       snap.InboundCallID,
		"callerNumber": snap.CallerNumber,
		"phase":        snap.Phase,
		"since":        snap.Since,
	}
}

// parseACDTargetIDsQuery reads acdTargetIds as comma-separated decimal IDs (snowflake-safe).
func parseACDTargetIDsQuery(c *gin.Context) ([]uint, bool) {
	raw := strings.TrimSpace(c.Query("acdTargetIds"))
	if raw == "" {
		return nil, false
	}
	parts := strings.Split(raw, ",")
	out := make([]uint, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := utils.ParseID(p)
		if err != nil {
			return nil, true
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, false
}

// streamSIPAgentIncoming pushes seat ringing state over Server-Sent Events (SSE).
// Query: acdTargetIds=1,2,3 (required, comma-separated acd_pool_targets.id).
// Events: ready, snapshot, heartbeat. Poll GET /sip-agent/incoming remains available.
func (h *Handlers) streamSIPAgentIncoming(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	ids, bad := parseACDTargetIDsQuery(c)
	if bad {
		response.Fail(c, "invalid acdTargetIds", nil)
		return
	}
	if len(ids) == 0 {
		response.Fail(c, "acdTargetIds is required", nil)
		return
	}

	rows := make([]models.ACDPoolTarget, 0, len(ids))
	for _, acdID := range ids {
		row, ok, err := models.FindSIPACDPoolTargetForIncomingPoll(h.db, tid, acdID, "", "")
		if err != nil {
			response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
			return
		}
		if !ok || row.RouteType != constants.ACDPoolRouteTypeSIP {
			response.Fail(c, "not found", gin.H{"acdTargetId": acdID})
			return
		}
		rows = append(rows, row)
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeSSE := func(event string, payload any) {
		b, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	sendSnapshots := func() {
		for _, row := range rows {
			snap := sipagentpoll.ResolveSnapshot(row.ID, conversation.IsTransferInProgress)
			writeSSE("snapshot", sipAgentIncomingPayload(row, snap))
		}
	}

	sendSnapshots()
	idStrs := make([]string, 0, len(ids))
	for _, id := range ids {
		idStrs = append(idStrs, strconv.FormatUint(uint64(id), 10))
	}
	writeSSE("ready", gin.H{"acdTargetIds": idStrs})

	updates, cancel := sipagentpoll.SubscribeACDChanges(ids)
	defer cancel()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	rowByID := make(map[uint]models.ACDPoolTarget, len(rows))
	for _, row := range rows {
		rowByID[row.ID] = row
	}

	for {
		select {
		case <-ctx.Done():
			return
		case acdID, ok := <-updates:
			if !ok {
				return
			}
			row, found := rowByID[acdID]
			if !found {
				continue
			}
			snap := sipagentpoll.ResolveSnapshot(acdID, conversation.IsTransferInProgress)
			writeSSE("snapshot", sipAgentIncomingPayload(row, snap))
		case <-heartbeat.C:
			writeSSE("heartbeat", gin.H{"ts": time.Now().UTC().Format(time.RFC3339)})
		}
	}
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
