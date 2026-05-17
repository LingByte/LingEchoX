package handlers

import "github.com/gin-gonic/gin"

func (h *Handlers) registerTenantPublicRoutes(r *gin.RouterGroup) {
	r.POST("/register", h.registerTenant)
	r.POST("/login", h.tenantLogin)
}
