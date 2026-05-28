package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"fmt"
	"strings"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/voiceclone"
	"github.com/gin-gonic/gin"
)

type vcVolcSubmitAudioReq struct {
	SpeakerID string `form:"speakerId" binding:"required"`
	Language  string `form:"language" binding:"required"`
	TaskName  string `form:"taskName"`
}

type vcVolcQueryTaskReq struct {
	SpeakerID string `json:"speakerId" binding:"required"`
	TaskName  string `json:"taskName"`
}

type vcVolcQueryTaskResp struct {
	SpeakerID  string `json:"speakerId"`
	Status     int    `json:"status"` // 0=排队/未找到 1=训练中 2=成功 3=失败
	TrainVID   string `json:"trainVid"`
	AssetID    string `json:"assetId"`
	FailedDesc string `json:"failedDesc"`
}

// volcengineSubmitTrainingAudio uploads training audio for a Volcengine speaker_id.
func (h *Handlers) volcengineSubmitTrainingAudio(c *gin.Context) {
	tenantID, userID, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcVolcSubmitAudioReq
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	file, err := c.FormFile("audio")
	if err != nil {
		response.Fail(c, "获取音频文件失败", err.Error())
		return
	}
	src, err := file.Open()
	if err != nil {
		response.Fail(c, "打开音频文件失败", err.Error())
		return
	}
	defer src.Close()

	service, err := h.newVoiceCloneService(voiceclone.ProviderVolcengine)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderVolcengine, err), nil)
		return
	}
	if err := service.SubmitAudio(c.Request.Context(), &voiceclone.SubmitAudioRequest{
		TaskID:    req.SpeakerID,
		AudioFile: src,
		Language:  req.Language,
	}); err != nil {
		response.Fail(c, "提交音频失败", err.Error())
		return
	}
	h.saveVoiceCloneConfig("volcengine")

	taskName := strings.TrimSpace(req.TaskName)
	if taskName == "" {
		taskName = fmt.Sprintf("火山音色 %s", req.SpeakerID)
	}
	var task models.VoiceTrainingTask
	err = h.db.Where("tenant_id = ? AND task_id = ? AND provider = ?", tenantID, req.SpeakerID, "volcengine").First(&task).Error
	if err != nil {
		task = models.VoiceTrainingTask{
			TenantID:  tenantID,
			CreatedBy: userID,
			TaskID:    req.SpeakerID,
			TaskName:  taskName,
			Provider:  "volcengine",
			Language:  req.Language,
			Status:    models.VCTrainingStatusInProgress,
		}
		if err := h.db.Create(&task).Error; err != nil {
			response.Fail(c, "保存训练任务失败", err.Error())
			return
		}
	} else {
		task.Status = models.VCTrainingStatusInProgress
		task.Language = req.Language
		if err := h.db.Save(&task).Error; err != nil {
			response.Fail(c, "更新训练任务失败", err.Error())
			return
		}
	}

	response.Success(c, "提交音频成功", gin.H{
		"speakerId": req.SpeakerID,
		"message":   "音频已提交，请查询训练状态",
	})
}

// volcengineQueryTrainingTask polls Volcengine training status and upserts VoiceClone on success.
func (h *Handlers) volcengineQueryTrainingTask(c *gin.Context) {
	tenantID, userID, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcVolcQueryTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}

	service, err := h.newVoiceCloneService(voiceclone.ProviderVolcengine)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderVolcengine, err), nil)
		return
	}
	status, err := service.QueryTaskStatus(c.Request.Context(), req.SpeakerID)
	if err != nil {
		response.Fail(c, "查询任务状态失败", err.Error())
		return
	}

	var trainStatus int
	switch status.Status {
	case voiceclone.TrainingStatusInProgress:
		trainStatus = 1
	case voiceclone.TrainingStatusSuccess:
		trainStatus = 2
	case voiceclone.TrainingStatusFailed:
		trainStatus = 3
	case voiceclone.TrainingStatusQueued:
		trainStatus = 0
	default:
		trainStatus = 0
	}

	out := vcVolcQueryTaskResp{
		SpeakerID:  status.TaskID,
		Status:     trainStatus,
		TrainVID:   status.TrainVID,
		AssetID:    status.AssetID,
		FailedDesc: status.FailedDesc,
	}

	if trainStatus == 2 && status.AssetID != "" {
		taskName := strings.TrimSpace(req.TaskName)
		if taskName == "" {
			taskName = fmt.Sprintf("火山音色 %s", req.SpeakerID)
		}
		var task models.VoiceTrainingTask
		if err := h.db.Where("tenant_id = ? AND task_id = ? AND provider = ?", tenantID, req.SpeakerID, "volcengine").
			First(&task).Error; err != nil {
			task = models.VoiceTrainingTask{
				TenantID:  tenantID,
				CreatedBy: userID,
				TaskID:    req.SpeakerID,
				TaskName:  taskName,
				Provider:  "volcengine",
				Status:    models.VCTrainingStatusSuccess,
				AssetID:   status.AssetID,
				TrainVID:  status.TrainVID,
			}
			if err := h.db.Create(&task).Error; err != nil {
				response.Fail(c, "保存训练任务失败", err.Error())
				return
			}
		} else {
			task.Status = models.VCTrainingStatusSuccess
			task.AssetID = status.AssetID
			task.TrainVID = status.TrainVID
			if err := h.db.Save(&task).Error; err != nil {
				response.Fail(c, "更新训练任务失败", err.Error())
				return
			}
		}
		if err := h.upsertVoiceClone(c.Request.Context(), tenantID, userID, &task, status.AssetID, status.TrainVID, "volcengine"); err != nil {
			response.Fail(c, "创建音色记录失败", err.Error())
			return
		}
	}

	response.Success(c, "查询任务状态成功", out)
}

// volcengineSynthesizeByAssetID synthesizes with a Volcengine speaker_id (assetId) directly.
func (h *Handlers) volcengineSynthesizeByAssetID(c *gin.Context) {
	tenantID, userID, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req struct {
		AssetID  string `json:"assetId" binding:"required"`
		Text     string `json:"text" binding:"required"`
		Language string `json:"language"`
		Key      string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	if req.Language == "" {
		req.Language = models.VCLanguageChinese
	}
	key := req.Key
	if key == "" {
		key = fmt.Sprintf("volcengine/%s_%d", req.AssetID, len(req.Text))
	}

	service, err := h.newVoiceCloneService(voiceclone.ProviderVolcengine)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderVolcengine, err), nil)
		return
	}
	url, err := service.SynthesizeToStorage(c.Request.Context(), &voiceclone.SynthesizeRequest{
		AssetID:  req.AssetID,
		Text:     req.Text,
		Language: req.Language,
	}, key)
	if err != nil {
		response.Fail(c, "语音合成失败", err.Error())
		return
	}

	var clone models.VoiceClone
	_ = h.db.Where("tenant_id = ? AND asset_id = ? AND provider = ? AND is_active = ?",
		tenantID, req.AssetID, "volcengine", true).First(&clone).Error

	if clone.ID > 0 {
		synthesis := &models.VoiceSynthesis{
			TenantID:     tenantID,
			CreatedBy:    userID,
			VoiceCloneID: clone.ID,
			Text:         req.Text,
			Language:     req.Language,
			AudioURL:     url,
			Status:       "success",
		}
		if err := h.db.Create(synthesis).Error; err == nil {
			clone.IncrementUsage()
			_ = h.db.Save(&clone).Error
		}
		response.Success(c, "语音合成成功", synthesis)
		return
	}
	response.Success(c, "语音合成成功", gin.H{"audioUrl": url})
}
