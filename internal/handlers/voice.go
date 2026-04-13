package handlers

// Copyright (c) 2026 LingByte
// SPDX-License-Identifier: MIT

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/voice"
	"github.com/LingByte/SoulNexus/pkg/voice/constants"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var voiceUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024, // 1MB读缓冲区，支持大音频数据
	WriteBufferSize: 1024 * 1024, // 1MB写缓冲区，支持大音频数据
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// registerVoiceDialogueRoutes 注册音色训练路由
func (h *Handlers) registerVoiceDialogueRoutes(r *gin.RouterGroup) {
	voice := r.Group("/voice")
	voice.GET("/hardware/v1/", h.HandleHardwareWebSocketVoice)
	voice.GET("/web/v1/", h.HandleWebSocketVoice)
}

// HandleHardwareWebSocketVoice 处理硬件WebSocket语音连接
// 从Header中获取Device-Id（MAC地址），查询设备绑定的助手，动态获取配置
func (h *Handlers) HandleHardwareWebSocketVoice(c *gin.Context) {
	// 从Header获取Device-Id（MAC地址），与xiaozhi-esp32兼容
	deviceID := c.GetHeader("Device-Id")
	if deviceID == "" {
		// 如果Header中没有，尝试从URL查询参数获取（xiaozhi-esp32兼容）
		deviceID = c.Query("device-id")
	}

	logger.Info("硬件WebSocket连接请求",
		zap.String("deviceID", deviceID),
		zap.String("path", c.Request.URL.Path),
		zap.String("remoteAddr", c.Request.RemoteAddr),
		zap.String("userAgent", c.Request.UserAgent()))

	if deviceID == "" {
		logger.Warn("硬件WebSocket连接缺少Device-Id参数",
			zap.String("path", c.Request.URL.Path),
			zap.String("headers", fmt.Sprintf("%v", c.Request.Header)))
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "缺少Device-Id参数",
			"data": nil,
		})
		c.Abort()
		return
	}

	// 根据Device-Id查询设备
	device, err := models.GetDeviceByMacAddress(h.db, deviceID)
	if err != nil || device == nil {
		// 设备不存在或未激活
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "设备未激活，请先激活设备",
			"data": nil,
		})
		c.Abort()
		return
	}

	// 检查设备是否绑定了助手
	if device.AssistantID == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "设备未绑定助手",
			"data": nil,
		})
		c.Abort()
		return
	}

	assistantID := *device.AssistantID

	// 获取助手配置
	var assistant models.Assistant
	if err := h.db.Where("id = ?", assistantID).First(&assistant).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "获取助手配置失败: " + err.Error(),
			"data": nil,
		})
		c.Abort()
		return
	}
	if assistant.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "助手不存在",
			"data": nil,
		})
		c.Abort()
		return
	}

	// 使用 Assistant 的 ApiKey 和 ApiSecret 获取用户凭证
	if assistant.ApiKey == "" || assistant.ApiSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "助手未配置API凭证",
			"data": nil,
		})
		c.Abort()
		return
	}

	cred, err := models.GetUserCredentialByApiSecretAndApiKey(h.db, assistant.ApiKey, assistant.ApiSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取凭证失败: " + err.Error(),
			"data": nil,
		})
		c.Abort()
		return
	}
	if cred == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "无效的API凭证",
			"data": nil,
		})
		c.Abort()
		return
	}

	// 升级为WebSocket连接
	logger.Info("准备升级WebSocket连接",
		zap.String("deviceID", deviceID),
		zap.Int64("assistantID", int64(assistantID)))
	conn, err := voiceUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WebSocket升级失败",
			zap.String("deviceID", deviceID),
			zap.Error(err))
		return
	}
	logger.Info("WebSocket连接已建立",
		zap.String("deviceID", deviceID),
		zap.Int64("assistantID", int64(assistantID)))

	// 使用助手配置中的参数
	language := assistant.Language
	if language == "" {
		language = "zh-cn"
	}
	speaker := assistant.Speaker
	if speaker == "" {
		speaker = "502007"
	}
	systemPrompt := assistant.SystemPrompt
	temperature := assistant.Temperature

	// Get LLM model from assistant, fallback to default
	llmModel := assistant.LLMModel
	if llmModel == "" {
		llmModel = "deepseek-v3.1" // Default model
	}

	// 创建WebSocket处理器
	handler := voice.NewHardwareHandler(h.db, logger.Lg)

	ctx := context.Background()

	// 使用常量中的VAD配置（覆盖数据库值）
	// 数据库中的值太低，导致Barge-in过于敏感
	vadThreshold := constants.DefaultVADThreshold
	vadConsecutiveFrames := constants.DefaultVADConsecutiveFrames

	handler.HandlerHardwareWebsocket(ctx, &voice.HardwareOptions{
		Conn:                 conn,
		AssistantID:          assistantID,
		DeviceID:             &device.ID,
		Language:             language,
		Speaker:              speaker,
		Temperature:          float64(temperature),
		SystemPrompt:         systemPrompt,
		KnowledgeKey:         "",
		UserID:               device.UserID,
		MacAddress:           device.MacAddress,
		LLMModel:             llmModel,
		Credential:           cred,
		EnableVAD:            assistant.EnableVAD,
		VADThreshold:         vadThreshold,
		VADConsecutiveFrames: vadConsecutiveFrames,
		VoiceCloneID:         assistant.VoiceCloneID,
	})
}

// HandleWebSocketVoice Web WebSocket 语音连接。
// 鉴权方式：Header `X-API-Key` + `X-API-Secret`，并通过 `assistant_id` 指定对话助手。
func (h *Handlers) HandleWebSocketVoice(c *gin.Context) {
	assistantIDStr := c.Query("assistant_id")
	if assistantIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "缺少 assistant_id 参数",
			"data": nil,
		})
		c.Abort()
		return
	}
	assistantIDInt, err := strconv.ParseUint(assistantIDStr, 10, 64)
	if err != nil || assistantIDInt == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "assistant_id 参数无效",
			"data": nil,
		})
		c.Abort()
		return
	}

	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		apiKey = c.GetHeader("API-Key")
	}
	if apiKey == "" {
		apiKey = c.Query("api_key")
	}
	apiSecret := c.GetHeader("X-API-Secret")
	if apiSecret == "" {
		apiSecret = c.GetHeader("API-Secret")
	}
	if apiSecret == "" {
		apiSecret = c.Query("api_secret")
	}
	if apiKey == "" || apiSecret == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 500,
			"msg":  "缺少 API 鉴权头",
			"data": nil,
		})
		c.Abort()
		return
	}

	credential, err := models.GetUserCredentialByApiSecretAndApiKey(h.db, apiKey, apiSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "查询凭证失败: " + err.Error(),
			"data": nil,
		})
		c.Abort()
		return
	}
	if credential == nil || !credential.IsActive() || credential.IsBanned() || credential.IsExpired() {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 500,
			"msg":  "无效或不可用的 API 凭证",
			"data": nil,
		})
		c.Abort()
		return
	}

	var assistant models.Assistant
	if err := h.db.Where("id = ? AND user_id = ?", assistantIDInt, credential.UserID).First(&assistant).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 500,
			"msg":  "助手不存在或无访问权限",
			"data": nil,
		})
		c.Abort()
		return
	}

	conn, err := voiceUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("网页端 WebSocket 升级失败", zap.Error(err))
		return
	}

	language := assistant.Language
	if language == "" {
		language = "zh-cn"
	}
	speaker := assistant.Speaker
	if speaker == "" {
		speaker = "502007"
	}
	llmModel := assistant.LLMModel
	if llmModel == "" {
		llmModel = "deepseek-v3.1"
	}
	vadThreshold := constants.DefaultVADThreshold
	vadConsecutiveFrames := constants.DefaultVADConsecutiveFrames
	handler := voice.NewHardwareHandler(h.db, logger.Lg)
	handler.HandlerHardwareWebsocket(context.Background(), &voice.HardwareOptions{
		Conn:                 conn,
		AssistantID:          uint(assistant.ID),
		Credential:           credential,
		Language:             language,
		Speaker:              speaker,
		Temperature:          float64(assistant.Temperature),
		SystemPrompt:         assistant.SystemPrompt,
		KnowledgeKey:         "",
		UserID:               credential.UserID,
		LLMModel:             llmModel,
		MaxLLMToken:          assistant.MaxTokens,
		EnableVAD:            assistant.EnableVAD,
		VADThreshold:         vadThreshold,
		VADConsecutiveFrames: vadConsecutiveFrames,
		VoiceCloneID:         assistant.VoiceCloneID,
	})
}
