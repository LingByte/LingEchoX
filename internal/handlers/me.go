package handlers

import (
	"net/http"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

type updateMeReq struct {
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
	Username    string `json:"username"`
}

type updateMyPasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=128"`
}

func (h *Handlers) currentTenantUser(c *gin.Context) (models.TenantUser, bool) {
	uid := middleware.AuthUserID(c)
	tid := middleware.AuthTenantID(c)
	if uid == 0 || tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return models.TenantUser{}, false
	}
	u, err := models.GetActiveTenantUserByID(h.db, uid)
	if err != nil || u.TenantID != tid {
		response.Fail(c, "unauthorized", nil)
		return models.TenantUser{}, false
	}
	return u, true
}

func (h *Handlers) getMe(c *gin.Context) {
	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	var tenant models.Tenant
	if err := h.db.Where("id = ? AND is_deleted = ?", u.TenantID, models.SoftDeleteStatusActive).First(&tenant).Error; err != nil {
		response.Fail(c, "tenant not found", nil)
		return
	}
	response.Success(c, "success", gin.H{
		"user":   tenantUserPublic(u),
		"tenant": tenantPublic(tenant),
	})
}

func (h *Handlers) updateMe(c *gin.Context) {
	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	var req updateMeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	updates := map[string]any{}
	if v := strings.TrimSpace(req.DisplayName); v != "" {
		updates["display_name"] = v
	}
	if req.Phone != "" {
		phone := strings.TrimSpace(req.Phone)
		exists, _ := models.CheckTenantUserPhoneExists(h.db, u.TenantID, phone, u.ID)
		if exists {
			response.Fail(c, "phone already exists", nil)
			return
		}
		updates["phone"] = phone
	}
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		exists, _ := models.CheckTenantUserUsernameExists(h.db, u.TenantID, username, u.ID)
		if exists {
			response.Fail(c, "username already exists", nil)
			return
		}
		updates["username"] = username
	}
	if len(updates) == 0 {
		response.Fail(c, "no fields to update", nil)
		return
	}
	if _, err := models.UpdateTenantUser(h.db, u.ID, updates, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", tenantUserPublic(next))
}

func (h *Handlers) updateMyPassword(c *gin.Context) {
	u, ok := h.currentTenantUser(c)
	if !ok {
		return
	}
	var req updateMyPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	if !access.CheckPassword(u.PasswordHash, req.OldPassword) {
		response.Fail(c, "old password incorrect", nil)
		return
	}
	if req.OldPassword == req.NewPassword {
		response.Fail(c, "new password must differ from old password", nil)
		return
	}
	hash, err := access.HashPassword(req.NewPassword)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	if _, err := models.UpdateTenantUser(h.db, u.ID, map[string]any{"password_hash": hash}, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": u.ID})
}

func (h *Handlers) logout(c *gin.Context) {
	response.Success(c, "success", gin.H{"loggedOut": true})
}
