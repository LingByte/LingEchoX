package handlers

import (
	"context"
	"fmt"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) startSIPCampaign(c *gin.Context) {
	changeSIPCampaignStatus(h, c, constants.SIPCampaignStatusRunning, false)
}

func (h *Handlers) pauseSIPCampaign(c *gin.Context) {
	changeSIPCampaignStatus(h, c, constants.SIPCampaignStatusPaused, true)
}

func (h *Handlers) resumeSIPCampaign(c *gin.Context) {
	changeSIPCampaignStatus(h, c, constants.SIPCampaignStatusRunning, false)
}

func (h *Handlers) stopSIPCampaign(c *gin.Context) {
	changeSIPCampaignStatus(h, c, constants.SIPCampaignStatusDone, true)
}

func changeSIPCampaignStatus(h *Handlers, c *gin.Context, status string, cancelQueued bool) {
	tid := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	n, err := models.UpdateActiveSIPCampaignStatusForTenant(h.db, id, tid, status, middleware.AuditOperator(c))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	if n == 0 {
		response.Fail(c, "campaign not found", nil)
		return
	}
	if cancelQueued && h.campaignSvc != nil {
		if _, err := h.campaignSvc.CancelCampaignQueuedTasks(context.Background(), id); err != nil {
			ginutil.WriteInternalError(c, err)
			return
		}
	}
	models.LogSIPCampaignEvent(h.db, id, 0, 0, "", "", "campaign", "info", fmt.Sprintf(
		"campaign status changed to %s campaign_id=%d (running=enqueues dial worker; paused/done stops new dials)",
		status, id,
	))
	response.Success(c, "success", nil)
}
