package models

import (
	"time"
)

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

// Assistant 表示一个自定义的 AI 助手
type Assistant struct {
	ID                   int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID               uint      `json:"userId" gorm:"index"`
	GroupID              *uint     `json:"groupId,omitempty" gorm:"index"` // 组织ID，如果设置则表示这是组织共享的助手
	Name                 string    `json:"name" gorm:"index"`
	Description          string    `json:"description"`
	Icon                 string    `json:"icon"`
	SystemPrompt         string    `json:"systemPrompt"`
	PersonaTag           string    `json:"personaTag"`
	Temperature          float32   `json:"temperature"`
	JsSourceID           string    `json:"jsSourceId" gorm:"index:idx_assistant_js_source"` // 关联的JS模板ID
	MaxTokens            int       `json:"maxTokens"`
	Language             string    `json:"language" gorm:"column:language"`                                     // 语言设置
	Speaker              string    `json:"speaker" gorm:"column:speaker"`                                       // 发音人ID
	VoiceCloneID         *int      `json:"voiceCloneId" gorm:"column:voice_clone_id"`                           // 训练音色ID（可选）
	TtsProvider          string    `json:"ttsProvider" gorm:"column:tts_provider"`                              // TTS提供商
	ApiKey               string    `json:"apiKey" gorm:"column:api_key"`                                        // API密钥
	ApiSecret            string    `json:"apiSecret" gorm:"column:api_secret"`                                  // API密钥
	LLMModel             string    `json:"llmModel" gorm:"column:llm_model"`                                    // LLM模型名称
	EnableGraphMemory    bool      `json:"enableGraphMemory" gorm:"column:enable_graph_memory;default:false"`   // 是否启用基于图数据库的长期记忆
	EnableVAD            bool      `json:"enableVAD" gorm:"column:enable_vad;default:true"`                     // 是否启用VAD（语音活动检测）用于打断TTS
	VADThreshold         float64   `json:"vadThreshold" gorm:"column:vad_threshold;default:500"`                // VAD阈值（RMS值，范围0-32768，默认500）
	VADConsecutiveFrames int       `json:"vadConsecutiveFrames" gorm:"column:vad_consecutive_frames;default:2"` // 需要连续超过阈值的帧数（默认2帧，约40ms）
	EnableJSONOutput     bool      `json:"enableJSONOutput" gorm:"column:enable_json_output;default:false"`     // 是否启用JSON格式化输出
	CreatedAt            time.Time `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt            time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}
