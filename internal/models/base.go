package models

import (
	"time"

	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

// BaseModel is embedded by tenant/RBAC/SIP console entities (not sip_calls / sip_users).
type BaseModel struct {
	ID        uint           `json:"id,string" gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time      `json:"createdAt" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time      `json:"updatedAt,omitempty" gorm:"autoUpdateTime;comment:更新时间"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	CreateBy  string         `json:"createBy,omitempty" gorm:"size:128;comment:创建人"`
	UpdateBy  string         `json:"updateBy,omitempty" gorm:"size:128;comment:更新人"`
	Remark    string         `json:"remark,omitempty" gorm:"size:128;comment:备注"`
}

func (m *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == 0 && utils.SnowflakeUtil != nil {
		m.ID = uint(utils.SnowflakeUtil.NextID())
	}
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = now
	}
	return nil
}

func (m *BaseModel) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}

func (m *BaseModel) SoftDelete(operator string) {
	m.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	m.UpdateBy = operator
	m.UpdatedAt = time.Now()
}

func (m *BaseModel) Restore(operator string) {
	m.DeletedAt = gorm.DeletedAt{}
	m.UpdateBy = operator
	m.UpdatedAt = time.Now()
}

func (m *BaseModel) SetCreateInfo(operator string) {
	m.CreateBy = operator
	m.UpdateBy = operator
}

func (m *BaseModel) SetUpdateInfo(operator string) {
	m.UpdateBy = operator
}
