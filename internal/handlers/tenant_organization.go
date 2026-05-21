package handlers

import (
	"errors"
	"strings"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) registerTenantOrgRoutes(g *gin.RouterGroup) {
	org := g.Group("tenant-org")
	read := org.Group("")
	read.Use(middleware.RequireTenantPermissionAll(constants.PermAPITenantOrgRead))
	{
		read.GET("/permissions", h.listOrgPermissions)
		read.GET("/groups", h.listOrgGroups)
		read.GET("/roles", h.listOrgRoles)
		read.GET("/roles/:id", h.getOrgRole)
	}
	write := org.Group("")
	write.Use(middleware.RequireTenantPermissionAll(constants.PermAPITenantOrgWrite))
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
	if _, ok := ginutil.RequireAuthTenant(c); !ok {
		return
	}
	rows, err := models.ListAllPermissions(h.db)
	if err != nil {
		ginutil.WriteInternalError(c, err)
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
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	rows, err := models.ListTenantGroupsForTenant(h.db, tid)
	if err != nil {
		ginutil.WriteInternalError(c, err)
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
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	var req orgGroupWriteReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	op := middleware.AuditOperator(c)
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
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"id": g.ID, "name": g.Name, "isDefault": g.IsDefault})
}

func (h *Handlers) updateOrgGroup(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	var req orgGroupWriteReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	var row models.TenantGroup
	if err := h.db.Where("id = ? AND tenant_id = ?", id, tid).
		First(&row).Error; err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	op := middleware.AuditOperator(c)
	txErr := h.db.Transaction(func(tx *gorm.DB) error {
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
		}
		meta := models.BaseModel{}
		meta.SetUpdateInfo(op)
		if meta.UpdateBy != "" {
			u["update_by"] = meta.UpdateBy
		}
		return tx.Model(&models.TenantGroup{}).Where("id = ?", row.ID).Updates(u).Error
	})
	if txErr != nil {
		ginutil.WriteInternalError(c, txErr)
		return
	}
	response.Success(c, "success", gin.H{"id": row.ID, "name": name, "isDefault": req.IsDefault})
}

func (h *Handlers) deleteOrgGroup(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	if err := models.SoftDeleteTenantGroup(h.db, tid, id, middleware.AuditOperator(c)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Fail(c, "not found", nil)
			return
		}
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}

func (h *Handlers) listOrgRoles(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	rows, err := models.ListTenantRolesByTenant(h.db, tid)
	if err != nil {
		ginutil.WriteInternalError(c, err)
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
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	permIDs, err := models.ListPermissionIDsForRole(h.db, r.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
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
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	var req orgRoleCreateReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	r := &models.TenantRole{
		TenantID:    tid,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		IsSystem:    false,
	}
	r.SetCreateInfo(middleware.AuditOperator(c))
	if err := models.CreateTenantRole(h.db, r); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"id": r.ID, "name": r.Name, "description": r.Description, "isSystem": false})
}

type orgRoleUpdateReq struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description"`
}

func (h *Handlers) updateOrgRole(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	var req orgRoleUpdateReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if r.IsSystem && strings.TrimSpace(req.Name) != r.Name {
		response.Fail(c, "系统角色不可改名", nil)
		return
	}
	op := middleware.AuditOperator(c)
	u := map[string]any{
		"name":        strings.TrimSpace(req.Name),
		"description": strings.TrimSpace(req.Description),
	}
	meta := models.BaseModel{}
	meta.SetUpdateInfo(op)
	if meta.UpdateBy != "" {
		u["update_by"] = meta.UpdateBy
	}
	if err := h.db.Model(&models.TenantRole{}).Where("id = ?", r.ID).Updates(u).Error; err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"id": r.ID})
}

func (h *Handlers) deleteOrgRole(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if r.IsSystem {
		response.Fail(c, "系统角色不可删除", nil)
		return
	}
	if err := models.SoftDeleteTenantRole(h.db, tid, r.ID, middleware.AuditOperator(c)); err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"id": r.ID})
}

type orgRolePermissionsReq struct {
	PermissionIDs []uint `json:"permissionIds"`
}

func (h *Handlers) putOrgRolePermissions(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	r, err := models.GetTenantRoleScoped(h.db, tid, id)
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}
	if r.IsSystem && r.Name == constants.TenantAdminRoleName {
		response.Fail(c, "系统「管理员」角色固定拥有全部权限，不可在此修改", nil)
		return
	}
	var req orgRolePermissionsReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	if err := models.ReplaceTenantRolePermissions(h.db, id, req.PermissionIDs, middleware.AuditOperator(c)); err != nil {
		if errors.Is(err, models.ErrInvalidOrgReference) {
			response.Fail(c, "无效的权限 id", nil)
			return
		}
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", gin.H{"roleId": id})
}

type orgUserRolesReq struct {
	RoleIDs []uint `json:"roleIds"`
}

func (h *Handlers) putOrgTenantUserRoles(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	uid, ok := ginutil.ParamID(c, "userId")
	if !ok {
		return
	}
	u, err := models.GetActiveTenantUserByID(h.db, uid)
	if err != nil || u.TenantID != tid {
		response.Fail(c, "not found", nil)
		return
	}
	var req orgUserRolesReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	if err := models.ReplaceTenantUserRoles(h.db, tid, u.ID, req.RoleIDs, middleware.AuditOperator(c)); err != nil {
		if errors.Is(err, models.ErrInvalidOrgReference) {
			response.Fail(c, "无效的角色 id", nil)
			return
		}
		ginutil.WriteInternalError(c, err)
		return
	}
	middleware.InvalidatePermissionCache(u.ID)
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", models.TenantUserPublic(h.db, next))
}

type orgUserGroupsReq struct {
	GroupIDs []uint `json:"groupIds"`
}

func (h *Handlers) putOrgTenantUserGroups(c *gin.Context) {
	tid, ok := ginutil.RequireAuthTenant(c)
	if !ok {
		return
	}
	uid, ok := ginutil.ParamID(c, "userId")
	if !ok {
		return
	}
	u, err := models.GetActiveTenantUserByID(h.db, uid)
	if err != nil || u.TenantID != tid {
		response.Fail(c, "not found", nil)
		return
	}
	var req orgUserGroupsReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	if err := models.ReplaceTenantUserGroups(h.db, tid, u.ID, req.GroupIDs, middleware.AuditOperator(c)); err != nil {
		if errors.Is(err, models.ErrInvalidOrgReference) {
			response.Fail(c, "无效的部门 id", nil)
			return
		}
		ginutil.WriteInternalError(c, err)
		return
	}
	next, err := models.GetActiveTenantUserByID(h.db, u.ID)
	if err != nil {
		ginutil.WriteInternalError(c, err)
		return
	}
	response.Success(c, "success", models.TenantUserPublic(h.db, next))
}
