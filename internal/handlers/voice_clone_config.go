package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"encoding/json"
	"fmt"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/LinByte/VoiceServer/pkg/voiceclone"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) getVoiceCloneConfig(provider string) map[string]interface{} {
	var configKey string
	envConfig := map[string]interface{}{}

	switch provider {
	case "xunfei":
		configKey = constants.KEYVoiceCloneXunfeiConfig
		envConfig = map[string]interface{}{
			"app_id":          utils.GetEnv("XUNFEI_APP_ID"),
			"api_key":         utils.GetEnv("XUNFEI_API_KEY"),
			"base_url":        utils.GetEnv("XUNFEI_BASE_URL"),
			"ws_app_id":       utils.GetEnv("XUNFEI_WS_APP_ID"),
			"ws_api_key":      utils.GetEnv("XUNFEI_WS_API_KEY"),
			"ws_api_secret":   utils.GetEnv("XUNFEI_WS_API_SECRET"),
			"engine_version":  utils.GetEnv("XUNFEI_CLONE_ENGINE_VERSION"),
			"vcn":             utils.GetEnv("XUNFEI_CLONE_VCN"),
		}
		if envConfig["base_url"] == "" {
			envConfig["base_url"] = "http://opentrain.xfyousheng.com"
		}
		if t := utils.GetIntEnv("XUNFEI_TIMEOUT"); t > 0 {
			envConfig["timeout"] = t
		}
	case "volcengine":
		configKey = constants.KEYVoiceCloneVolcengineConfig
		envConfig = map[string]interface{}{
			"app_id":          utils.GetEnv("VOLCENGINE_CLONE_APP_ID"),
			"token":           utils.GetEnv("VOLCENGINE_CLONE_TOKEN"),
			"cluster":         utils.GetEnv("VOLCENGINE_CLONE_CLUSTER"),
			"voice_type":      utils.GetEnv("VOLCENGINE_CLONE_VOICE_TYPE"),
			"encoding":        utils.GetEnv("VOLCENGINE_CLONE_ENCODING"),
			"frame_duration":  utils.GetEnv("VOLCENGINE_CLONE_FRAME_DURATION"),
		}
		if envConfig["cluster"] == "" {
			envConfig["cluster"] = "volcano_icl"
		}
		if sampleRate := utils.GetIntEnv("VOLCENGINE_CLONE_SAMPLE_RATE"); sampleRate > 0 {
			envConfig["sample_rate"] = sampleRate
		}
		if bitDepth := utils.GetIntEnv("VOLCENGINE_CLONE_BIT_DEPTH"); bitDepth > 0 {
			envConfig["bit_depth"] = bitDepth
		}
		if channels := utils.GetIntEnv("VOLCENGINE_CLONE_CHANNELS"); channels > 0 {
			envConfig["channels"] = channels
		}
		if speedRatio := utils.GetFloatEnv("VOLCENGINE_CLONE_SPEED_RATIO"); speedRatio > 0 {
			envConfig["speed_ratio"] = speedRatio
		}
		if trainingTimes := utils.GetIntEnv("VOLCENGINE_CLONE_TRAINING_TIMES"); trainingTimes > 0 {
			envConfig["training_times"] = trainingTimes
		}
	default:
		return nil
	}

	if dbStr := utils.GetValue(h.db, configKey); dbStr != "" {
		var dbConfig map[string]interface{}
		if err := json.Unmarshal([]byte(dbStr), &dbConfig); err == nil && voiceCloneConfigValid(provider, dbConfig) {
			return dbConfig
		}
	}
	if voiceCloneConfigValid(provider, envConfig) {
		return envConfig
	}
	return nil
}

func voiceCloneConfigValid(provider string, config map[string]interface{}) bool {
	if config == nil {
		return false
	}
	switch provider {
	case "xunfei":
		appID, _ := config["app_id"].(string)
		apiKey, _ := config["api_key"].(string)
		return appID != "" && apiKey != ""
	case "volcengine":
		token, _ := config["token"].(string)
		return token != ""
	default:
		return false
	}
}

func (h *Handlers) newVoiceCloneService(provider voiceclone.Provider) (voiceclone.VoiceCloneService, error) {
	p := string(provider)
	cfgMap := h.getVoiceCloneConfig(p)
	if cfgMap != nil {
		factory := voiceclone.NewFactory()
		return factory.CreateService(&voiceclone.Config{
			Provider: provider,
			Options:  cfgMap,
		})
	}
	return voiceclone.NewFactory().CreateServiceFromEnv(provider)
}

func (h *Handlers) voiceProviderConfigured(provider string) bool {
	return h.getVoiceCloneConfig(provider) != nil
}

func (h *Handlers) getVoiceCloneCapabilities(c *gin.Context) {
	if _, _, ok := vcTenantScope(c); !ok {
		return
	}
	response.Success(c, "获取音色克隆能力成功", gin.H{
		"xunfei": gin.H{
			"configured": h.voiceProviderConfigured("xunfei"),
			"provider":   "xunfei",
			"label":      "讯飞一句话复刻（多风格）",
		},
		"volcengine": gin.H{
			"configured": h.voiceProviderConfigured("volcengine"),
			"provider":   "volcengine",
			"label":      "火山引擎",
		},
	})
}

// saveVoiceCloneConfig persists env snapshot when a provider call succeeds.
func (h *Handlers) saveVoiceCloneConfig(provider string) {
	cfg := h.getVoiceCloneConfig(provider)
	if cfg == nil {
		// Build from env only for persistence attempt
		switch provider {
		case "xunfei":
			cfg = map[string]interface{}{
				"app_id":         utils.GetEnv("XUNFEI_APP_ID"),
				"api_key":        utils.GetEnv("XUNFEI_API_KEY"),
				"base_url":       utils.GetEnv("XUNFEI_BASE_URL"),
				"ws_app_id":      utils.GetEnv("XUNFEI_WS_APP_ID"),
				"ws_api_key":     utils.GetEnv("XUNFEI_WS_API_KEY"),
				"ws_api_secret":  utils.GetEnv("XUNFEI_WS_API_SECRET"),
				"engine_version": utils.GetEnv("XUNFEI_CLONE_ENGINE_VERSION"),
				"vcn":            utils.GetEnv("XUNFEI_CLONE_VCN"),
			}
			if cfg["base_url"] == "" {
				cfg["base_url"] = "http://opentrain.xfyousheng.com"
			}
		case "volcengine":
			cfg = map[string]interface{}{
				"app_id":     utils.GetEnv("VOLCENGINE_CLONE_APP_ID"),
				"token":      utils.GetEnv("VOLCENGINE_CLONE_TOKEN"),
				"cluster":    utils.GetEnv("VOLCENGINE_CLONE_CLUSTER"),
				"voice_type": utils.GetEnv("VOLCENGINE_CLONE_VOICE_TYPE"),
			}
			if cfg["cluster"] == "" {
				cfg["cluster"] = "volcano_icl"
			}
		default:
			return
		}
	}
	if !voiceCloneConfigValid(provider, cfg) {
		return
	}
	var configKey string
	switch provider {
	case "xunfei":
		configKey = constants.KEYVoiceCloneXunfeiConfig
	case "volcengine":
		configKey = constants.KEYVoiceCloneVolcengineConfig
	default:
		return
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	utils.SetValue(h.db, configKey, string(raw), "json", true, true)
}

func voiceCloneServiceInitErr(provider voiceclone.Provider, err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("初始化%s服务失败: %v", provider, err)
}
