package handlers

import (
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
)

func requireTenant(c *gin.Context) (tenantID uint, ok bool) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return 0, false
	}
	return tid, true
}

func tenantOwns(rowTenantID, reqTenant uint) bool {
	return rowTenantID == reqTenant
}

func requirePlatformAdmin(c *gin.Context) (adminID uint, ok bool) {
	aid := middleware.AuthPlatformAdminID(c)
	if aid == 0 {
		response.Fail(c, "forbidden", nil)
		return 0, false
	}
	return aid, true
}
