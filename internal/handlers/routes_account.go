package handlers

import "github.com/gin-gonic/gin"

// registerAccountRoutes: /me and logout — any authenticated tenant user or platform admin (no tenant RBAC codes).
func (h *Handlers) registerAccountRoutes(r *gin.RouterGroup) {
	r.GET("/me", h.getMe)
	r.PUT("/me", h.updateMe)
	r.PUT("/me/password", h.updateMyPassword)
	r.POST("/me/avatar", h.uploadMeAvatar)
	r.POST("/me/totp/setup", h.setupTotp)
	r.POST("/me/totp/enable", h.enableTotp)
	r.POST("/me/totp/disable", h.disableTotp)
	r.POST("/auth/logout", h.logout)
}
