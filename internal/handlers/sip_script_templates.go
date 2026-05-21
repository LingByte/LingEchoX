package handlers

import (
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/ginutil"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/gin-gonic/gin"
)

type sipScriptTemplateWriteReq struct {
	Name        string `json:"name"`
	ScriptID    string `json:"scriptId"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
	ScriptSpec  string `json:"scriptSpec"`
}

func (h *Handlers) listSIPScriptTemplates(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	page, size := ginutil.QueryPage(c, 100)
	list, total, err := models.ListSIPScriptTemplatesPage(h.db, tid, page, size, c.Query("scriptId"), c.Query("name"))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	ginutil.PageSuccess(c, list, total, page, size)
}

func (h *Handlers) getSIPScriptTemplate(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	row, err := models.GetActiveSIPScriptTemplateForTenant(h.db, id, tid)
	if ginutil.WriteGORMError(c, err, "not found") {
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) createSIPScriptTemplate(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	var req sipScriptTemplateWriteReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	spec, err := models.ParseScriptTemplateSpec(req.ScriptSpec)
	if err != nil {
		response.Fail(c, "invalid scriptSpec JSON", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		response.Fail(c, "name required", nil)
		return
	}
	scriptID := strings.TrimSpace(req.ScriptID)
	if scriptID == "" {
		scriptID = models.RandomScriptTemplateID()
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row := models.NewSIPScriptTemplateForCreate(
		name,
		scriptID,
		strings.TrimSpace(req.Version),
		strings.TrimSpace(req.Description),
		enabled,
		spec,
	)
	row.TenantID = tid
	if op := middleware.AuditOperator(c); op != "" {
		row.SetCreateInfo(op)
	}
	if ginutil.WriteInternalError(c, h.db.Create(&row).Error) {
		return
	}
	response.Success(c, "success", row)
}

func (h *Handlers) updateSIPScriptTemplate(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	var req sipScriptTemplateWriteReq
	if !ginutil.BindJSON(c, &req) {
		return
	}
	row, err := models.GetActiveSIPScriptTemplateForTenant(h.db, id, tid)
	if ginutil.WriteGORMError(c, err, "not found") {
		return
	}
	updates, err := models.BuildSIPScriptTemplateUpdates(
		row,
		req.Name,
		req.ScriptID,
		req.Version,
		req.Description,
		req.Enabled,
		req.ScriptSpec,
		middleware.AuditOperator(c),
	)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	if ginutil.WriteInternalError(c, h.db.Model(&row).Updates(updates).Error) {
		return
	}
	row, _ = models.ReloadSIPScriptTemplateByID(h.db, id)
	response.Success(c, "success", row)
}

func (h *Handlers) deleteSIPScriptTemplate(c *gin.Context) {
	tid := middleware.CurrentTenantID(c)
	id, ok := ginutil.ParamID(c, "id")
	if !ok {
		return
	}
	n, err := models.SoftDeleteSIPScriptTemplateByIDForTenant(h.db, id, tid, middleware.AuditOperator(c))
	if ginutil.WriteInternalError(c, err) {
		return
	}
	if n == 0 {
		response.Fail(c, "not found", nil)
		return
	}
	response.Success(c, "success", gin.H{"id": id})
}
