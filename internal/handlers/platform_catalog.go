package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
)

// listTenants returns tenant organizations for platform admin (e.g. assign trunk numbers).
func (h *Handlers) listTenants(c *gin.Context) {
	// Platform admin only (enforced by route middleware).
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	page, size := utils.NormalizePage(p, s, 500)
	list, total, err := models.ListTenantsPage(h.db, page, size, strings.TrimSpace(c.Query("search")))
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	rows := make([]gin.H, len(list))
	for i := range list {
		rows[i] = tenantPublic(list[i])
	}
	response.Success(c, "success", gin.H{"list": rows, "total": total, "page": page, "size": size})
}
