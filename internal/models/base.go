package models

import (
	"time"

	"github.com/LinByte/VoiceServer/pkg/utils"
	"gorm.io/gorm"
)

type BaseModel struct {
	ID        uint           `json:"id,string" gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time      `json:"createdAt" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time      `json:"updatedAt,omitempty" gorm:"autoUpdateTime;comment:更新时间"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	CreateBy  string         `json:"createBy,omitempty" gorm:"size:128;comment:创建人"`
	UpdateBy  string         `json:"updateBy,omitempty" gorm:"size:128;comment:更新人"`
	Remark    string         `json:"remark,omitempty" gorm:"size:128;comment:备注"`
}

// BeforeCreate GORM hook: assigns a Snowflake ID when the caller did not
// supply one, and stamps creation/update timestamps.
//
// The catalog uses `autoIncrement:false` (see BaseModel.ID) because Snowflake
// IDs are mandatory across services (they're embedded in JWTs, exported to
// JS clients as strings, and survive logical replication / sharding).
// Historically callers were expected to assign IDs themselves, but in
// practice every code path skipped that step — which made any model.Create
// with ID=0 fail with `Duplicate entry '0' for key 'PRIMARY'` on the second
// row because MySQL stored the literal zero from the first row instead of
// auto-incrementing. Assigning here closes the gap once for every model.
//
// Callers that explicitly set m.ID (e.g. importing fixed-ID seed data) are
// unaffected: the hook only fills zero values.
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

// BeforeUpdate GORM hook: automatically set update time before updating
func (m *BaseModel) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}

// IsSoftDeleted checks if the record is soft deleted
func (m *BaseModel) IsSoftDeleted() bool {
	return !m.DeletedAt.Time.IsZero()
}

// SoftDelete performs soft deletion
func (m *BaseModel) SoftDelete(operator string) {
	m.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	m.UpdateBy = operator
	m.UpdatedAt = time.Now()
}

// Restore restores a soft deleted record
func (m *BaseModel) Restore(operator string) {
	m.DeletedAt = gorm.DeletedAt{}
	m.UpdateBy = operator
	m.UpdatedAt = time.Now()
}

// SetCreateInfo sets creation information
func (m *BaseModel) SetCreateInfo(operator string) {
	m.CreateBy = operator
	m.UpdateBy = operator
}

// SetUpdateInfo sets update information
func (m *BaseModel) SetUpdateInfo(operator string) {
	m.UpdateBy = operator
}

// GetCreatedAtString returns formatted creation time string
func (m *BaseModel) GetCreatedAtString() string {
	return m.CreatedAt.Format("2006-01-02 15:04:05")
}

// GetUpdatedAtString returns formatted update time string
func (m *BaseModel) GetUpdatedAtString() string {
	if m.UpdatedAt.IsZero() {
		return ""
	}
	return m.UpdatedAt.Format("2006-01-02 15:04:05")
}

// GetCreatedAtUnix returns creation time as Unix timestamp
func (m *BaseModel) GetCreatedAtUnix() int64 {
	return m.CreatedAt.Unix()
}

// GetUpdatedAtUnix returns update time as Unix timestamp
func (m *BaseModel) GetUpdatedAtUnix() int64 {
	if m.UpdatedAt.IsZero() {
		return 0
	}
	return m.UpdatedAt.Unix()
}
