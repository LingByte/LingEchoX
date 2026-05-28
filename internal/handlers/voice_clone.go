package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/voiceclone"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) registerVoiceCloneRoutes(r *gin.RouterGroup) {
	voice := r.Group("/voice")
	voice.Use(middleware.RequireHumanJWTUser())

	voiceRead := voice.Group("")
	voiceRead.Use(middleware.RequireTenantPermissionAll(constants.PermAPIVoiceRead))
	{
		voiceRead.GET("/capabilities", h.getVoiceCloneCapabilities)
		voiceRead.GET("/training-texts", h.getVoiceTrainingTexts)
		voiceRead.GET("/clones", h.listVoiceClones)
		voiceRead.GET("/clones/:id", h.getVoiceClone)
		voiceRead.GET("/synthesis/history", h.getVoiceSynthesisHistory)
	}

	voiceWrite := voice.Group("")
	voiceWrite.Use(middleware.RequireTenantPermissionAll(constants.PermAPIVoiceWrite))
	{
		// 讯飞训练链路
		voiceWrite.POST("/xunfei/training/create", h.createVoiceTrainingTask)
		voiceWrite.POST("/xunfei/training/submit-audio", h.submitVoiceTrainingAudio)
		voiceWrite.POST("/xunfei/training/query", h.queryVoiceTrainingTask)
		// 兼容旧路径
		voiceWrite.POST("/training/create", h.createVoiceTrainingTask)
		voiceWrite.POST("/training/submit-audio", h.submitVoiceTrainingAudio)
		voiceWrite.POST("/training/query", h.queryVoiceTrainingTask)

		// 火山引擎训练 / 合成
		voiceWrite.POST("/volcengine/task/submit-audio", h.volcengineSubmitTrainingAudio)
		voiceWrite.POST("/volcengine/task/query", h.volcengineQueryTrainingTask)
		voiceWrite.POST("/volcengine/synthesize", h.volcengineSynthesizeByAssetID)

		voiceWrite.POST("/clones/update", h.updateVoiceClone)
		voiceWrite.POST("/clones/delete", h.deleteVoiceClone)
		voiceWrite.POST("/synthesize", h.synthesizeWithVoiceClone)
		voiceWrite.POST("/synthesis/delete", h.deleteVoiceSynthesisRecord)
	}
}

type vcCreateTrainingTaskReq struct {
	TaskName string `json:"taskName" binding:"required"`
	Sex      int    `json:"sex"`
	AgeGroup int    `json:"ageGroup"`
	Language string `json:"language"`
}

type vcSubmitAudioReq struct {
	TaskID    string `form:"taskId" binding:"required"`
	TextSegID int64  `form:"textSegId" binding:"required"`
}

type vcQueryTaskReq struct {
	TaskID string `json:"taskId" binding:"required"`
}

type vcSynthesizeReq struct {
	VoiceCloneID uint   `json:"voiceCloneId" binding:"required"`
	Text         string `json:"text" binding:"required"`
	Language     string `json:"language"`
	StorageKey   string `json:"storageKey"`
}

type vcUpdateCloneReq struct {
	ID               uint   `json:"id" binding:"required"`
	VoiceName        string `json:"voiceName" binding:"required"`
	VoiceDescription string `json:"voiceDescription"`
}

type vcSynthesisHistoryItem struct {
	ID           uint   `json:"id"`
	VoiceCloneID uint   `json:"voiceCloneId"`
	Text         string `json:"text"`
	Language     string `json:"language"`
	AudioURL     string `json:"audioUrl"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	Provider     string `json:"provider"`
}

func vcTenantScope(c *gin.Context) (tenantID, userID uint, ok bool) {
	tenantID = middleware.CurrentTenantID(c)
	userID = middleware.AuthUserID(c)
	if tenantID == 0 || userID == 0 {
		response.Fail(c, "需要租户登录", nil)
		return 0, 0, false
	}
	return tenantID, userID, true
}

func (h *Handlers) createVoiceTrainingTask(c *gin.Context) {
	tenantID, userID, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcCreateTrainingTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	if req.Sex == 0 {
		req.Sex = models.VCSexMale
	}
	if req.AgeGroup == 0 {
		req.AgeGroup = models.VCAgeGroupYouth
	}
	if req.Language == "" {
		req.Language = models.VCLanguageChinese
	}

	service, err := h.newVoiceCloneService(voiceclone.ProviderXunfei)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderXunfei, err), nil)
		return
	}
	createResp, err := service.CreateTask(c.Request.Context(), &voiceclone.CreateTaskRequest{
		TaskName:     req.TaskName,
		Sex:          req.Sex,
		AgeGroup:     req.AgeGroup,
		Language:     req.Language,
		ResourceType: 12,
	})
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "训练次数") || strings.Contains(errMsg, "quota") {
			response.Fail(c, "训练次数不足", "讯飞 TTS 训练配额已用完，请联系管理员")
			return
		}
		response.Fail(c, "创建训练任务失败", errMsg)
		return
	}

	h.saveVoiceCloneConfig("xunfei")

	task := &models.VoiceTrainingTask{
		TenantID:  tenantID,
		CreatedBy: userID,
		TaskID:    createResp.TaskID,
		Provider:  "xunfei",
		TaskName:  req.TaskName,
		Sex:       req.Sex,
		AgeGroup:  req.AgeGroup,
		Language:  req.Language,
		Status:    models.VCTrainingStatusQueued,
		TextID:    5001,
	}
	if err := h.db.Create(task).Error; err != nil {
		response.Fail(c, "保存训练任务失败", err.Error())
		return
	}
	response.Success(c, "创建训练任务成功", task)
}

func (h *Handlers) submitVoiceTrainingAudio(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcSubmitAudioReq
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
	audioData, err := io.ReadAll(src)
	if err != nil {
		response.Fail(c, "读取音频文件失败", err.Error())
		return
	}
	if len(audioData) == 0 {
		response.Fail(c, "音频文件为空", nil)
		return
	}

	var task models.VoiceTrainingTask
	if err := h.db.Where("tenant_id = ? AND task_id = ? AND provider = ?", tenantID, req.TaskID, "xunfei").
		First(&task).Error; err != nil {
		response.Fail(c, "训练任务不存在", err.Error())
		return
	}

	service, err := h.newVoiceCloneService(voiceclone.ProviderXunfei)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderXunfei, err), nil)
		return
	}
	if err := service.SubmitAudio(c.Request.Context(), &voiceclone.SubmitAudioRequest{
		TaskID:    task.TaskID,
		TextID:    task.TextID,
		TextSegID: req.TextSegID,
		AudioFile: bytes.NewReader(audioData),
		Language:  task.Language,
	}); err != nil {
		response.Fail(c, "提交音频失败", err.Error())
		return
	}

	task.Status = models.VCTrainingStatusInProgress
	task.TextSegID = req.TextSegID
	if err := h.db.Save(&task).Error; err != nil {
		response.Fail(c, "更新任务状态失败", err.Error())
		return
	}
	response.Success(c, "提交音频成功", nil)
}

func (h *Handlers) queryVoiceTrainingTask(c *gin.Context) {
	tenantID, userID, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcQueryTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}

	var task models.VoiceTrainingTask
	if err := h.db.Where("tenant_id = ? AND task_id = ? AND provider = ?", tenantID, req.TaskID, "xunfei").
		First(&task).Error; err != nil {
		response.Fail(c, "训练任务不存在", err.Error())
		return
	}

	service, err := h.newVoiceCloneService(voiceclone.ProviderXunfei)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderXunfei, err), nil)
		return
	}
	status, err := service.QueryTaskStatus(c.Request.Context(), task.TaskID)
	if err != nil {
		response.Fail(c, "查询任务状态失败", err.Error())
		return
	}

	var trainStatus int
	switch status.Status {
	case voiceclone.TrainingStatusInProgress:
		trainStatus = models.VCTrainingStatusInProgress
	case voiceclone.TrainingStatusSuccess:
		trainStatus = models.VCTrainingStatusSuccess
	case voiceclone.TrainingStatusFailed:
		trainStatus = models.VCTrainingStatusFailed
	case voiceclone.TrainingStatusQueued:
		trainStatus = models.VCTrainingStatusQueued
	default:
		trainStatus = models.VCTrainingStatusInProgress
	}

	task.Status = trainStatus
	task.TrainVID = status.TrainVID
	task.AssetID = status.AssetID
	task.FailedReason = status.FailedDesc
	if err := h.db.Save(&task).Error; err != nil {
		response.Fail(c, "更新任务状态失败", err.Error())
		return
	}

	if trainStatus == models.VCTrainingStatusSuccess && status.AssetID != "" {
		if err := h.upsertVoiceClone(c.Request.Context(), tenantID, userID, &task, status.AssetID, status.TrainVID, "xunfei"); err != nil {
			response.Fail(c, "创建音色记录失败", err.Error())
			return
		}
	}
	response.Success(c, "查询任务状态成功", task)
}

func (h *Handlers) listVoiceClones(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var clones []models.VoiceClone
	q := h.db.Where("tenant_id = ? AND is_active = ?", tenantID, true)
	if provider := c.Query("provider"); provider != "" {
		q = q.Where("provider = ?", provider)
	}
	if err := q.Order("created_at DESC").Find(&clones).Error; err != nil {
		response.Fail(c, "获取音色列表失败", err.Error())
		return
	}
	response.Success(c, "获取音色列表成功", clones)
}

func (h *Handlers) getVoiceClone(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	cloneID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, "音色ID格式错误", err.Error())
		return
	}
	var clone models.VoiceClone
	if err := h.db.Where("id = ? AND tenant_id = ? AND is_active = ?", uint(cloneID), tenantID, true).
		First(&clone).Error; err != nil {
		response.Fail(c, "音色不存在", err.Error())
		return
	}
	response.Success(c, "获取音色信息成功", clone)
}

func (h *Handlers) synthesizeWithVoiceClone(c *gin.Context) {
	tenantID, userID, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcSynthesizeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	if req.Language == "" {
		req.Language = models.VCLanguageChinese
	}
	if req.StorageKey == "" {
		ts := time.Now().Format("20060102_150405")
		req.StorageKey = fmt.Sprintf("voice_synthesis/%d_%s_%d.mp3", req.VoiceCloneID, ts, len(req.Text))
	}

	var clone models.VoiceClone
	if err := h.db.Where("id = ? AND tenant_id = ? AND is_active = ?", req.VoiceCloneID, tenantID, true).
		First(&clone).Error; err != nil {
		response.Fail(c, "音色不存在", err.Error())
		return
	}
	if clone.AssetID == "" {
		response.Fail(c, "音色未训练完成", nil)
		return
	}

	provider := voiceclone.ProviderXunfei
	if clone.Provider == string(voiceclone.ProviderVolcengine) {
		provider = voiceclone.ProviderVolcengine
	}
	service, err := h.newVoiceCloneService(provider)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(provider, err), nil)
		return
	}

	audioURL, err := service.SynthesizeToStorage(c.Request.Context(), &voiceclone.SynthesizeRequest{
		AssetID:  clone.AssetID,
		Text:     req.Text,
		Language: req.Language,
	}, req.StorageKey)
	if err != nil {
		response.Fail(c, "语音合成失败", err.Error())
		return
	}

	synthesis := &models.VoiceSynthesis{
		TenantID:     tenantID,
		CreatedBy:    userID,
		VoiceCloneID: clone.ID,
		Text:         req.Text,
		Language:     req.Language,
		AudioURL:     audioURL,
		Status:       "success",
	}
	if err := h.db.Create(synthesis).Error; err != nil {
		response.Fail(c, "保存合成记录失败", err.Error())
		return
	}
	clone.IncrementUsage()
	_ = h.db.Save(&clone).Error
	response.Success(c, "语音合成成功", synthesis)
}

func (h *Handlers) getVoiceSynthesisHistory(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	provider := c.Query("provider")

	var history []models.VoiceSynthesis
	q := h.db.Model(&models.VoiceSynthesis{}).Where("tenant_id = ?", tenantID)
	if provider != "" {
		q = q.Joins("JOIN voice_clones ON voice_syntheses.voice_clone_id = voice_clones.id").
			Where("voice_clones.provider = ? AND voice_clones.deleted_at IS NULL", provider)
	}
	if err := q.Order("voice_syntheses.created_at DESC").Limit(limit).Find(&history).Error; err != nil {
		response.Fail(c, "获取合成历史失败", err.Error())
		return
	}

	cloneIDs := make([]uint, 0, len(history))
	for _, item := range history {
		cloneIDs = append(cloneIDs, item.VoiceCloneID)
	}
	providers := map[uint]string{}
	if len(cloneIDs) > 0 {
		var clones []models.VoiceClone
		if err := h.db.Select("id, provider").Where("id IN ?", cloneIDs).Find(&clones).Error; err == nil {
			for _, cl := range clones {
				providers[cl.ID] = cl.Provider
			}
		}
	}

	out := make([]vcSynthesisHistoryItem, 0, len(history))
	for _, item := range history {
		out = append(out, vcSynthesisHistoryItem{
			ID:           item.ID,
			VoiceCloneID: item.VoiceCloneID,
			Text:         item.Text,
			Language:     item.Language,
			AudioURL:     item.AudioURL,
			Status:       item.Status,
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			Provider:     providers[item.VoiceCloneID],
		})
	}
	response.Success(c, "获取合成历史成功", out)
}

func (h *Handlers) deleteVoiceSynthesisRecord(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	res := h.db.Where("tenant_id = ? AND id = ?", tenantID, req.ID).Delete(&models.VoiceSynthesis{})
	if res.Error != nil {
		response.Fail(c, "删除合成记录失败", res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		response.Fail(c, "合成记录不存在", nil)
		return
	}
	response.Success(c, "删除合成记录成功", nil)
}

func (h *Handlers) updateVoiceClone(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req vcUpdateCloneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	res := h.db.Model(&models.VoiceClone{}).
		Where("id = ? AND tenant_id = ?", req.ID, tenantID).
		Updates(map[string]any{
			"voice_name":        req.VoiceName,
			"voice_description": req.VoiceDescription,
			"updated_at":        time.Now(),
		})
	if res.Error != nil {
		response.Fail(c, "更新音色信息失败", res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		response.Fail(c, "音色不存在", nil)
		return
	}
	response.Success(c, "更新音色信息成功", nil)
}

func (h *Handlers) deleteVoiceClone(c *gin.Context) {
	tenantID, _, ok := vcTenantScope(c)
	if !ok {
		return
	}
	var req struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误", err.Error())
		return
	}
	res := h.db.Where("id = ? AND tenant_id = ?", req.ID, tenantID).Delete(&models.VoiceClone{})
	if res.Error != nil {
		response.Fail(c, "删除音色失败", res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		response.Fail(c, "音色不存在", nil)
		return
	}
	response.Success(c, "删除音色成功", nil)
}

func (h *Handlers) getVoiceTrainingTexts(c *gin.Context) {
	textID, err := strconv.ParseInt(c.DefaultQuery("textId", "5001"), 10, 64)
	if err != nil {
		response.Fail(c, "文本ID格式错误", err.Error())
		return
	}

	var text models.VoiceTrainingText
	if err := h.db.Where("text_id = ? AND is_active = ?", textID, true).First(&text).Error; err == nil {
		var segments []models.VoiceTrainingTextSegment
		h.db.Where("text_id = ?", text.ID).Find(&segments)
		text.TextSegments = segments
		response.Success(c, "获取训练文本成功", text)
		return
	}

	service, err := h.newVoiceCloneService(voiceclone.ProviderXunfei)
	if err != nil {
		response.Fail(c, voiceCloneServiceInitErr(voiceclone.ProviderXunfei, err), nil)
		return
	}
	xfText, err := service.GetTrainingTexts(c.Request.Context(), textID)
	if err != nil {
		response.Fail(c, "获取训练文本失败", err.Error())
		return
	}

	text = models.VoiceTrainingText{
		TextID:   xfText.TextID,
		TextName: xfText.TextName,
		Language: models.VCLanguageChinese,
		IsActive: true,
	}
	if err := h.db.Create(&text).Error; err != nil {
		response.Fail(c, "保存训练文本失败", err.Error())
		return
	}
	for _, seg := range xfText.Segments {
		segRow := models.VoiceTrainingTextSegment{
			TextID:  text.ID,
			SegID:   fmt.Sprintf("%v", seg.SegID),
			SegText: seg.SegText,
		}
		if err := h.db.Create(&segRow).Error; err != nil {
			response.Fail(c, "保存文本段落失败", err.Error())
			return
		}
	}
	var segments []models.VoiceTrainingTextSegment
	h.db.Where("text_id = ?", text.ID).Find(&segments)
	text.TextSegments = segments
	response.Success(c, "获取训练文本成功", text)
}

func (h *Handlers) upsertVoiceClone(ctx context.Context, tenantID, userID uint, task *models.VoiceTrainingTask, assetID, trainVID, provider string) error {
	_ = ctx
	var existing models.VoiceClone
	if err := h.db.Where("tenant_id = ? AND asset_id = ? AND provider = ?", tenantID, assetID, provider).
		First(&existing).Error; err == nil {
		existing.TrainVID = trainVID
		existing.IsActive = true
		return h.db.Save(&existing).Error
	}
	clone := &models.VoiceClone{
		TenantID:         tenantID,
		CreatedBy:        userID,
		TrainingTaskID:   task.ID,
		Provider:         provider,
		AssetID:          assetID,
		TrainVID:         trainVID,
		VoiceName:        task.TaskName,
		VoiceDescription: fmt.Sprintf("基于任务 %s 训练的音色", task.TaskName),
		IsActive:         true,
	}
	return h.db.Create(clone).Error
}
