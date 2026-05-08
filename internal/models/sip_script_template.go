package models

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SIPScriptTemplate stores reusable outbound dialog-flow scripts for campaigns.
type SIPScriptTemplate struct {
	BaseModel

	TenantID uint `json:"tenantId" gorm:"index;not null;default:0"`

	Name        string `json:"name" gorm:"size:128;not null;index"`
	ScriptID    string `json:"scriptId" gorm:"size:128;not null;index"`
	Version     string `json:"version" gorm:"size:64;index"`
	Description string `json:"description" gorm:"type:text"`
	Enabled     bool   `json:"enabled" gorm:"default:true;index"`

	ScriptSpec datatypes.JSON `json:"scriptSpec" gorm:"type:json;not null"`
}

func (SIPScriptTemplate) TableName() string {
	return constants.SIP_SCRIPT_TEMPLATE_TABLE_NAME
}

// ActiveSIPScriptTemplates limits to non–soft-deleted rows.
func ActiveSIPScriptTemplates(db *gorm.DB) *gorm.DB {
	return db.Model(&SIPScriptTemplate{})
}

// ListSIPScriptTemplatesPage lists active templates with optional filters.
func ListSIPScriptTemplatesPage(db *gorm.DB, tenantID uint, page, size int, scriptID, nameContains string) ([]SIPScriptTemplate, int64, error) {
	q := ActiveSIPScriptTemplates(db).Where("tenant_id = ?", tenantID)
	if s := strings.TrimSpace(scriptID); s != "" {
		q = q.Where("script_id = ?", s)
	}
	if name := strings.TrimSpace(nameContains); name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size
	var list []SIPScriptTemplate
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetActiveSIPScriptTemplateByID returns one active row by primary key.
func GetActiveSIPScriptTemplateByID(db *gorm.DB, id uint) (SIPScriptTemplate, error) {
	var row SIPScriptTemplate
	err := ActiveSIPScriptTemplates(db).Where("id = ?", id).First(&row).Error
	return row, err
}

// GetActiveSIPScriptTemplateForTenant returns one script template for tenant scope.
func GetActiveSIPScriptTemplateForTenant(db *gorm.DB, id uint, tenantID uint) (SIPScriptTemplate, error) {
	var row SIPScriptTemplate
	err := ActiveSIPScriptTemplates(db).Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error
	return row, err
}

// SoftDeleteSIPScriptTemplateByIDForTenant soft-deletes for tenant scope.
func SoftDeleteSIPScriptTemplateByIDForTenant(db *gorm.DB, id uint, tenantID uint, updateBy string) (int64, error) {
	updates := map[string]interface{}{}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	if len(updates) > 0 {
		if err := db.Model(&SIPScriptTemplate{}).Where("id = ? AND tenant_id = ?", id, tenantID).Updates(updates).Error; err != nil {
			return 0, err
		}
	}
	res := db.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&SIPScriptTemplate{})
	return res.RowsAffected, res.Error
}

// ParseScriptTemplateSpec validates JSON and returns bytes for GORM JSON column.
func ParseScriptTemplateSpec(raw string) (datatypes.JSON, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("scriptSpec is empty")
	}
	if !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("invalid scriptSpec JSON")
	}
	return datatypes.JSON(raw), nil
}

// RandomScriptTemplateID returns a unique script_* id for new templates when caller omits scriptId.
func RandomScriptTemplateID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("script_%d", time.Now().Unix())
	}
	return "script_" + strings.ToLower(hex.EncodeToString(buf))
}

// NewSIPScriptTemplateForCreate builds a row after name/scriptId/spec are normalized and validated.
func NewSIPScriptTemplateForCreate(name, scriptID, version, description string, enabled bool, spec datatypes.JSON) SIPScriptTemplate {
	return SIPScriptTemplate{
		Name:        name,
		ScriptID:    scriptID,
		Version:     version,
		Description: description,
		Enabled:     enabled,
		ScriptSpec:  spec,
	}
}

// BuildSIPScriptTemplateUpdates builds a GORM Updates map for PATCH semantics.
// Empty scriptSpecRaw leaves script_spec unchanged; empty scriptID keeps existing.ScriptID.
func BuildSIPScriptTemplateUpdates(existing SIPScriptTemplate, name, scriptID, version, description string, enabled *bool, scriptSpecRaw, updateBy string) (map[string]interface{}, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	updates := map[string]interface{}{
		"name":        name,
		"script_id":   strings.TrimSpace(scriptID),
		"version":     strings.TrimSpace(version),
		"description": strings.TrimSpace(description),
	}
	if strings.TrimSpace(scriptID) == "" {
		updates["script_id"] = existing.ScriptID
	}
	if enabled != nil {
		updates["enabled"] = *enabled
	}
	if strings.TrimSpace(scriptSpecRaw) != "" {
		spec, err := ParseScriptTemplateSpec(scriptSpecRaw)
		if err != nil {
			return nil, err
		}
		updates["script_spec"] = spec
	}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	return updates, nil
}

// SoftDeleteSIPScriptTemplateByID soft-deletes an active template; returns rows affected.
func SoftDeleteSIPScriptTemplateByID(db *gorm.DB, id uint, updateBy string) (int64, error) {
	updates := map[string]interface{}{}
	if updateBy != "" {
		updates["update_by"] = updateBy
	}
	if len(updates) > 0 {
		if err := db.Model(&SIPScriptTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return 0, err
		}
	}
	res := db.Where("id = ?", id).Delete(&SIPScriptTemplate{})
	return res.RowsAffected, res.Error
}

// ReloadSIPScriptTemplateByID refetches a row by id (e.g. after Updates).
func ReloadSIPScriptTemplateByID(db *gorm.DB, id uint) (SIPScriptTemplate, error) {
	var row SIPScriptTemplate
	err := db.Where("id = ?", id).First(&row).Error
	return row, err
}
