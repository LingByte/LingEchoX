package models

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LingByte/SoulNexus/pkg/constants"
	"gorm.io/gorm"
)

type CredentialRequest struct {
	Name        string         `json:"name"` // 应用名称 or 用途备注
	LLMProvider string         `json:"llmProvider"`
	LLMApiKey   string         `json:"llmApiKey"`
	LLMApiURL   string         `json:"llmApiUrl"`
	AsrConfig   ProviderConfig `json:"asrConfig"` // ASR配置,格式: {"provider": "qiniu", "apiKey": "...", "baseUrl": "..."} 或 {"provider": "qcloud", "appId": "...", "secretId": "...", "secretKey": "..."}
	TtsConfig   ProviderConfig `json:"ttsConfig"` // TTS配置
}

// ProviderConfig 提供商的灵活配置,支持任意键值对
type ProviderConfig map[string]interface{}

// CredentialStatus 凭证状态类型
type CredentialStatus string

const (
	// CredentialStatusActive 活跃状态
	CredentialStatusActive CredentialStatus = "active"
	// CredentialStatusBanned 已封禁
	CredentialStatusBanned CredentialStatus = "banned"
	// CredentialStatusSuspended 已暂停
	CredentialStatusSuspended CredentialStatus = "suspended"
	// CredentialStatusExpired 已过期
	CredentialStatusExpired CredentialStatus = "expired"
)

// Value 实现 driver.Valuer 接口
func (pc ProviderConfig) Value() (driver.Value, error) {
	if pc == nil || len(pc) == 0 {
		return nil, nil
	}
	return json.Marshal(pc)
}

// Scan 实现 sql.Scanner 接口
func (pc *ProviderConfig) Scan(value interface{}) error {
	if value == nil {
		*pc = make(ProviderConfig)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to convert value to []byte")
	}
	if len(bytes) == 0 {
		*pc = make(ProviderConfig)
		return nil
	}
	return json.Unmarshal(bytes, pc)
}

type Credential struct {
	BaseModel
	UserID       uint             `gorm:"index;" json:"userId"`
	Name         string           `json:"name"`                                                      // 应用名称 or 用途备注
	APIKey       string           `gorm:"uniqueIndex:idx_api_key,length:100;not null" json:"apiKey"` // 用于认证
	APISecret    string           `gorm:"not null" json:"apiSecret"`                                 // 用于签名校验
	Status       CredentialStatus `gorm:"type:varchar(20);default:'active'" json:"status"`           // 状态: active, banned, suspended
	BannedAt     *time.Time       `gorm:"index" json:"bannedAt"`                                     // 封禁时间
	BannedReason string           `gorm:"type:text" json:"bannedReason"`                             // 封禁原因
	BannedBy     *uint            `gorm:"index" json:"bannedBy"`                                     // 封禁操作者ID
	ExpiresAt    *time.Time       `gorm:"index" json:"expiresAt"`                                    // 过期时间
	LastUsedAt   *time.Time       `gorm:"index" json:"lastUsedAt"`                                   // 最后使用时间
	UsageCount   int64            `gorm:"default:0" json:"usageCount"`                               // 使用次数
	LLMProvider  string           `json:"llmProvider"`
	LLMApiKey    string           `json:"llmApiKey"`
	LLMApiURL    string           `json:"llmApiUrl"`
	AsrConfig    ProviderConfig   `json:"asrConfig" gorm:"type:json"`
	TtsConfig    ProviderConfig   `json:"ttsConfig" gorm:"type:json"`
}

// CredentialResponse 用于返回给前端的凭证信息
type CredentialResponse struct {
	ID           uint             `json:"id"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
	UserID       uint             `json:"userId"`
	Name         string           `json:"name"`
	Status       CredentialStatus `json:"status"`
	BannedAt     *time.Time       `json:"bannedAt"`
	BannedReason string           `json:"bannedReason"`
	ExpiresAt    *time.Time       `json:"expiresAt"`
	LastUsedAt   *time.Time       `json:"lastUsedAt"`
	UsageCount   int64            `json:"usageCount"`
	LLMProvider  string           `json:"llmProvider"`
	AsrProvider  string           `json:"asrProvider"`
	TtsProvider  string           `json:"ttsProvider"`
}

// ToResponse 将 Credential 转换为 CredentialResponse
func (uc *Credential) ToResponse() *CredentialResponse {
	asrProvider := ""
	if uc.AsrConfig != nil {
		if provider, ok := uc.AsrConfig["provider"].(string); ok {
			asrProvider = provider
		}
	}

	ttsProvider := ""
	if uc.TtsConfig != nil {
		if provider, ok := uc.TtsConfig["provider"].(string); ok {
			ttsProvider = provider
		}
	}

	return &CredentialResponse{
		ID:           uc.ID,
		CreatedAt:    uc.CreatedAt,
		UpdatedAt:    uc.UpdatedAt,
		UserID:       uc.UserID,
		Name:         uc.Name,
		Status:       uc.Status,
		BannedAt:     uc.BannedAt,
		BannedReason: uc.BannedReason,
		ExpiresAt:    uc.ExpiresAt,
		LastUsedAt:   uc.LastUsedAt,
		UsageCount:   uc.UsageCount,
		LLMProvider:  uc.LLMProvider,
		AsrProvider:  asrProvider,
		TtsProvider:  ttsProvider,
	}
}

// ToResponseList 将 Credential 列表转换为 CredentialResponse 列表
func ToResponseList(credentials []*Credential) []*CredentialResponse {
	responses := make([]*CredentialResponse, len(credentials))
	for i, cred := range credentials {
		responses[i] = cred.ToResponse()
	}
	return responses
}

func (uc *Credential) TableName() string {
	return constants.USER_CREDENTIAL_TABLE_NAME
}

// GetASRProvider 从AsrConfig获取provider
func (uc *Credential) GetASRProvider() string {
	if uc.AsrConfig != nil {
		if provider, ok := uc.AsrConfig["provider"].(string); ok {
			return provider
		}
	}
	return ""
}

// GetASRConfig 获取ASR配置值
func (uc *Credential) GetASRConfig(key string) interface{} {
	if uc.AsrConfig != nil {
		return uc.AsrConfig[key]
	}
	return nil
}

// GetASRConfigString 获取ASR配置字符串值
func (uc *Credential) GetASRConfigString(key string) string {
	if uc.AsrConfig != nil {
		if val, ok := uc.AsrConfig[key].(string); ok {
			return val
		}
	}
	return ""
}

// GetTTSProvider 从TtsConfig获取provider
func (uc *Credential) GetTTSProvider() string {
	if uc.TtsConfig != nil {
		if provider, ok := uc.TtsConfig["provider"].(string); ok {
			return provider
		}
	}
	return ""
}

// GetTTSConfig 获取TTS配置值
func (uc *Credential) GetTTSConfig(key string) interface{} {
	if uc.TtsConfig != nil {
		return uc.TtsConfig[key]
	}
	return nil
}

// GetTTSConfigString 获取TTS配置字符串值
func (uc *Credential) GetTTSConfigString(key string) string {
	if uc.TtsConfig != nil {
		if val, ok := uc.TtsConfig[key].(string); ok {
			return val
		}
	}
	return ""
}

// IsActive 检查凭证是否处于活跃状态
func (uc *Credential) IsActive() bool {
	if uc.Status != CredentialStatusActive {
		return false
	}

	// 检查是否过期
	if uc.ExpiresAt != nil && time.Now().After(*uc.ExpiresAt) {
		return false
	}

	return true
}

// IsBanned 检查凭证是否被封禁
func (uc *Credential) IsBanned() bool {
	return uc.Status == CredentialStatusBanned
}

// IsExpired 检查凭证是否过期
func (uc *Credential) IsExpired() bool {
	if uc.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*uc.ExpiresAt)
}

// UpdateLastUsed 更新最后使用时间和使用次数
func (uc *Credential) UpdateLastUsed(db *gorm.DB) error {
	now := time.Now()
	return db.Model(uc).Updates(map[string]interface{}{
		"last_used_at": now,
		"usage_count":  gorm.Expr("usage_count + 1"),
	}).Error
}

// BuildASRConfig 从请求中构建ASR配置
func (req *CredentialRequest) BuildASRConfig() ProviderConfig {
	// 如果已经提供了配置,直接返回
	if req.AsrConfig != nil && len(req.AsrConfig) > 0 {
		// 确保provider字段存在
		if _, ok := req.AsrConfig["provider"]; !ok {
			return nil // provider 是必需的
		}
		return req.AsrConfig
	}
	return nil
}

// BuildTTSConfig 从请求中构建TTS配置
func (req *CredentialRequest) BuildTTSConfig() ProviderConfig {
	// 如果已经提供了配置,直接返回
	if req.TtsConfig != nil && len(req.TtsConfig) > 0 {
		// 确保provider字段存在
		if _, ok := req.TtsConfig["provider"]; !ok {
			return nil // provider 是必需的
		}
		return req.TtsConfig
	}
	return nil
}

// GetUserCredentials 根据用户ID获取其所有的凭证信息
func GetUserCredentials(db *gorm.DB, userID uint) ([]*Credential, error) {
	var credentials []*Credential
	err := db.Where("user_id = ?", userID).Find(&credentials).Error
	if err != nil {
		return nil, err
	}
	return credentials, nil
}

func GetUserCredentialByApiSecretAndApiKey(db *gorm.DB, apiKey, apiSecret string) (*Credential, error) {
	var credential Credential
	result := db.Where("api_key = ? AND api_secret = ?", apiKey, apiSecret).First(&credential)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	return &credential, nil
}
