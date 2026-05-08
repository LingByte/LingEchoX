package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/middleware"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) registerTenantOrgRoutes(g *gin.RouterGroup) {
	org := g.Group("tenant-org")
	read := org.Group("")
	read.Use(middleware.RequireTenantPermissionAll("api.tenant_org.read"))
	{
		read.GET("/permissions", h.listOrgPermissions)
		read.GET("/groups", h.listOrgGroups)
		read.GET("/roles", h.listOrgRoles)
		read.GET("/roles/:id", h.getOrgRole)
	}
	write := org.Group("")
	write.Use(middleware.RequireTenantPermissionAll("api.tenant_org.write"))
	{
		write.POST("/groups", h.createOrgGroup)
		write.PUT("/groups/:id", h.updateOrgGroup)
		write.DELETE("/groups/:id", h.deleteOrgGroup)

		write.POST("/roles", h.createOrgRole)
		write.PUT("/roles/:id", h.updateOrgRole)
		write.DELETE("/roles/:id", h.deleteOrgRole)
		write.PUT("/roles/:id/permissions", h.putOrgRolePermissions)

		write.PUT("/users/:userId/roles", h.putOrgTenantUserRoles)
		write.PUT("/users/:userId/groups", h.putOrgTenantUserGroups)
	}
}

func (h *Handlers) listOrgPermissions(c *gin.Context) {
	if middleware.AuthTenantID(c) == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	rows, err := models.ListAllPermissions(h.db)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	pub := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		pub = append(pub, gin.H{
			"id":          p.ID,
			"code":        p.Code,
			"name":        p.Name,
			"description": p.Description,
			"kind":        p.Kind,
			"parentCode":  p.ParentCode,
			"resource":    p.Resource,
			"action":      p.Action,
		})
	}
	response.Success(c, "success", gin.H{"list": pub})
}

func (h *Handlers) listOrgGroups(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	rows, err := models.ListTenantGroupsForTenant(h.db, tid)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	pub := make([]gin.H, 0, len(rows))
	for _, g := range rows {
		pub = append(pub, gin.H{"id": g.ID, "name": g.Name, "isDefault": g.IsDefault})
	}
	response.Success(c, "success", gin.H{"list": pub})
}

type orgGroupWriteReq struct {
	Name      string `json:"name" binding:"required,min=1,max=128"`
	IsDefault bool   `json:"isDefault"`
}

func (h *Handlers) createOrgGroup(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var req orgGroupWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	op := acdOperator(c)
	g := &models.TenantGroup{TenantID: tid, Name: name, IsDefault: req.IsDefault}
	g.SetCreateInfo(op)
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&models.TenantGroup{}).
				Where("tenant_id = ?", tid).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return models.CreateTenantGroupRecord(tx, g)
	})
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": g.ID, "name": g.Name, "isDefault": g.IsDefault})
}

func (h *Handlers) updateOrgGroup(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	var req orgGroupWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	var row models.TenantGroup
	if err := h.db.Where("id = ? AND tenant_id = ?", uint(id64), tid).
		First(&row).Error; err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	op := acdOperator(c)
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&models.TenantGroup{}).
				Where("tenant_id = ?", tid).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		u := map[string]any{
			"name":       name,
			"is_default": req.IsDefault,
			"update_by":  op,
		}
		return tx.Model(&models.TenantGroup{}).Where("id = ?", row.ID).Updates(u).Error
	})
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID, "name": name, "isDefault": req.IsDefault})
}

func (h *Handlers) deleteOrgGroup(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	if err := models.SoftDeleteTenantGroup(h.db, tid, uint(id64), acdOperator(c)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Fail(c, "not found", nil)
			return
		}
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": uint(id64)})
}

func (h *Handlers) listOrgRoles(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	rows, err := models.ListTenantRolesByTenant(h.db, tid)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	pub := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		pub = append(pub, gin.H{
			"id":          r.ID,
			"name":        r.Name,
			"description": r.Description,
			"isSystem":    r.IsSystem,
		})
	}
	response.Success(c, "success", gin.H{"list": pub})
}

func (h *Handlers) getOrgRole(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, uint(id64))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	permIDs, err := models.ListPermissionIDsForRole(h.db, r.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{
		"id":            r.ID,
		"name":          r.Name,
		"description":   r.Description,
		"isSystem":      r.IsSystem,
		"permissionIds": permIDs,
	})
}

type orgRoleCreateReq struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description"`
}

func (h *Handlers) createOrgRole(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	var req orgRoleCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	r := &models.TenantRole{
		TenantID:    tid,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		IsSystem:    false,
	}
	r.SetCreateInfo(acdOperator(c))
	if err := models.CreateTenantRole(h.db, r); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": r.ID, "name": r.Name, "description": r.Description, "isSystem": false})
}

type orgRoleUpdateReq struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description"`
}

func (h *Handlers) updateOrgRole(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	var req orgRoleUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, uint(id64))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if r.IsSystem && strings.TrimSpace(req.Name) != r.Name {
		response.Fail(c, "系统角色不可改名", nil)
		return
	}
	op := acdOperator(c)
	u := map[string]any{
		"name":        strings.TrimSpace(req.Name),
		"description": strings.TrimSpace(req.Description),
		"update_by":   op,
	}
	if err := h.db.Model(&models.TenantRole{}).Where("id = ?", r.ID).Updates(u).Error; err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": r.ID})
}

func (h *Handlers) deleteOrgRole(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, uint(id64))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if r.IsSystem {
		response.Fail(c, "系统角色不可删除", nil)
		return
	}
	if err := models.SoftDeleteTenantRole(h.db, tid, r.ID, acdOperator(c)); err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"id": r.ID})
}

type orgRolePermissionsReq struct {
	PermissionIDs []uint `json:"permissionIds"`
}

func (h *Handlers) putOrgRolePermissions(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, uint(id64))
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if r.IsSystem && r.Name == models.TenantAdminRoleName {
		response.Fail(c, "系统「管理员」角色固定拥有全部权限，不可在此修改", nil)
		return
	}
	var req orgRolePermissionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	if err := models.ReplaceTenantRolePermissions(h.db, uint(id64), req.PermissionIDs, acdOperator(c)); err != nil {
		if errors.Is(err, models.ErrInvalidOrgReference) {
			response.Fail(c, "无效的权限 id", nil)
			return
		}
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", gin.H{"roleId": uint(id64)})
}

type orgUserRolesReq struct {
	RoleIDs []uint `json:"roleIds"`
}

func (h *Handlers) putOrgTenantUserRoles(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	uid64, err := strconv.ParseUint(c.Param("userId"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid user id", nil)
		return
	}
	u, err := models.GetActiveTenantUserByID(h.db, uint(uid64))
	if err != nil || u.TenantID != tid {
		response.Fail(c, "not found", nil)
		return
	}
	var req orgUserRolesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	if err := models.ReplaceTenantUserRoles(h.db, tid, u.ID, req.RoleIDs, acdOperator(c)); err != nil {
		if errors.Is(err, models.ErrInvalidOrgReference) {
			response.Fail(c, "无效的角色 id", nil)
			return
		}
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(next))
}

type orgUserGroupsReq struct {
	GroupIDs []uint `json:"groupIds"`
}

func (h *Handlers) putOrgTenantUserGroups(c *gin.Context) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return
	}
	uid64, err := strconv.ParseUint(c.Param("userId"), 10, 32)
	if err != nil {
		response.Fail(c, "invalid user id", nil)
		return
	}
	u, err := models.GetActiveTenantUserByID(h.db, uint(uid64))
	if err != nil || u.TenantID != tid {
		response.Fail(c, "not found", nil)
		return
	}
	var req orgUserGroupsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return
	}
	if err := models.ReplaceTenantUserGroups(h.db, tid, u.ID, req.GroupIDs, acdOperator(c)); err != nil {
		if errors.Is(err, models.ErrInvalidOrgReference) {
			response.Fail(c, "无效的部门 id", nil)
			return
		}
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
		return
	}
	response.Success(c, "success", h.tenantUserPublic(next))
}
