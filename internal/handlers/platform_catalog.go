package handlers

import (
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/gin-gonic/gin"
)

// listTenants returns tenant organizations for platform admin (e.g. assign trunk numbers).
func (h *Handlers) listTenants(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	page, size := ginutil.QueryPage(c, 500)
	list, total, err := models.ListTenantsPage(h.db, page, size, strings.TrimSpace(c.Query("search")))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	rows := make([]gin.H, len(list))
	for i := range list {
		rows[i] = models.TenantPublic(list[i])
	}
	ginutil.PageSuccess(c, rows, total, page, size)
}
