package models

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"time"

	"gorm.io/gorm"
)

// Voice training / clone models (tenant-scoped; adapted from SoulNexus voice_training).

type VoiceTrainingTask struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	TenantID      uint           `json:"tenantId" gorm:"index;not null"`
	CreatedBy     uint           `json:"createdBy" gorm:"index"`
	TaskID        string         `json:"taskId" gorm:"uniqueIndex:idx_vc_task_id,length:100;not null"`
	Provider      string         `json:"provider" gorm:"default:'xunfei';index"`
	TaskName      string         `json:"taskName" gorm:"not null"`
	Sex           int            `json:"sex" gorm:"default:1"`
	AgeGroup      int            `json:"ageGroup" gorm:"default:2"`
	Language      string         `json:"language" gorm:"default:'zh'"`
	Status        int            `json:"status" gorm:"default:-1"`
	TextID        int64          `json:"textId" gorm:"default:5001"`
	TextSegID     int64          `json:"textSegId"`
	AudioURL      string         `json:"audioUrl"`
	AudioDuration float64        `json:"audioDuration"`
	AudioSize     int64          `json:"audioSize"`
	TrainVID      string         `json:"trainVid"`
	AssetID       string         `json:"assetId"`
	FailedReason  string         `json:"failedReason"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

type VoiceClone struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	TenantID         uint           `json:"tenantId" gorm:"uniqueIndex:idx_vc_tenant_asset,priority:1;index;not null"`
	CreatedBy        uint           `json:"createdBy" gorm:"index"`
	TrainingTaskID   uint           `json:"trainingTaskId" gorm:"index"`
	Provider         string         `json:"provider" gorm:"default:'xunfei';index"`
	AssetID          string         `json:"assetId" gorm:"uniqueIndex:idx_vc_tenant_asset,priority:2,length:100;not null"`
	TrainVID         string         `json:"trainVid"`
	VoiceName        string         `json:"voiceName" gorm:"not null"`
	VoiceDescription string         `json:"voiceDescription"`
	IsActive         bool           `json:"isActive" gorm:"default:true"`
	UsageCount       int            `json:"usageCount" gorm:"default:0"`
	LastUsedAt       *time.Time     `json:"lastUsedAt"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

type VoiceSynthesis struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	TenantID      uint           `json:"tenantId" gorm:"index;not null"`
	CreatedBy     uint           `json:"createdBy" gorm:"index"`
	VoiceCloneID  uint           `json:"voiceCloneId" gorm:"not null;index"`
	Text          string         `json:"text" gorm:"not null"`
	Language      string         `json:"language" gorm:"default:'zh'"`
	AudioURL      string         `json:"audioUrl"`
	AudioDuration float64        `json:"audioDuration"`
	AudioSize     int64          `json:"audioSize"`
	Status        string         `json:"status" gorm:"default:'success'"`
	ErrorMessage  string         `json:"errorMessage"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

type VoiceTrainingText struct {
	ID           uint                       `json:"id" gorm:"primaryKey"`
	TextID       int64                      `json:"textId" gorm:"uniqueIndex:idx_vc_text_id;not null"`
	TextName     string                     `json:"textName" gorm:"not null"`
	Language     string                     `json:"language" gorm:"default:'zh'"`
	IsActive     bool                       `json:"isActive" gorm:"default:true"`
	CreatedAt    time.Time                  `json:"createdAt"`
	UpdatedAt    time.Time                  `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt             `json:"-" gorm:"index"`
	TextSegments []VoiceTrainingTextSegment `json:"textSegments" gorm:"-"`
}

func (VoiceTrainingText) TableName() string { return "voice_training_texts" }

type VoiceTrainingTextSegment struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	TextID    uint           `json:"textId" gorm:"not null;index"`
	SegID     string         `json:"segId" gorm:"not null"`
	SegText   string         `json:"segText" gorm:"not null"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (VoiceTrainingTextSegment) TableName() string { return "voice_training_text_segments" }

const (
	VCTrainingStatusInProgress = -1
	VCTrainingStatusFailed     = 0
	VCTrainingStatusSuccess    = 1
	VCTrainingStatusQueued     = 2

	VCSexMale   = 1
	VCSexFemale = 2

	VCAgeGroupChild   = 1
	VCAgeGroupYouth   = 2
	VCAgeGroupMiddle  = 3
	VCAgeGroupElderly = 4

	VCLanguageChinese = "zh"
)

func (v *VoiceClone) IncrementUsage() {
	v.UsageCount++
	now := time.Now()
	v.LastUsedAt = &now
}

func (v *VoiceClone) IsAvailable() bool {
	return v.IsActive && v.AssetID != ""
}

func GetVoiceCloneByID(db *gorm.DB, tenantID uint, voiceCloneID uint) (*VoiceClone, error) {
	var voiceClone VoiceClone
	q := db.Where("id = ?", voiceCloneID)
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.First(&voiceClone).Error; err != nil {
		return nil, err
	}
	return &voiceClone, nil
}
