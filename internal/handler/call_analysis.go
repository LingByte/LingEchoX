package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/callanalysis"
	"github.com/LingByte/SoulNexus/pkg/response"
	"github.com/gin-gonic/gin"
)

const callAnalysisMultipartMax = 96 << 20

type callAnalysisJSONBody struct {
	AudioURL string `json:"audio_url"`
}

func (h *Handlers) createCallAnalysis(c *gin.Context) {
	if h == nil || h.callAnalysisStore == nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, fmt.Errorf("call analysis store not initialized"))
		return
	}
	env := callanalysis.EnvFromProcess()
	if err := env.Validate(); err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}

	tmpPath, in, err := h.resolveCallAnalysisInput(c)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	if tmpPath != "" {
		defer func() { _ = os.Remove(tmpPath) }()
	}

	ctx := c.Request.Context()
	doc, err := callanalysis.Run(ctx, env, in)
	if err != nil {
		response.Fail(c, err.Error(), nil)
		return
	}
	h.callAnalysisStore.Put(doc)

	response.Success(c, "success", gin.H{
		"id":         doc.ID,
		"meta":       doc.Meta,
		"asr":        doc.ASR,
		"llm":        json.RawMessage(doc.LLM),
		"llm_raw":    doc.LLMRaw,
		"export_doc": doc,
	})
}

func (h *Handlers) getCallAnalysis(c *gin.Context) {
	if h == nil || h.callAnalysisStore == nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, fmt.Errorf("call analysis store not initialized"))
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	doc, ok := h.callAnalysisStore.Get(id)
	if !ok {
		response.Fail(c, "not found or expired", nil)
		return
	}
	response.Success(c, "success", doc)
}

func (h *Handlers) exportCallAnalysisJSON(c *gin.Context) {
	if h == nil || h.callAnalysisStore == nil {
		response.AbortWithStatusJSON(c, http.StatusInternalServerError, fmt.Errorf("call analysis store not initialized"))
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	doc, ok := h.callAnalysisStore.Get(id)
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="call-analysis-%s.json"`, id))
	c.JSON(http.StatusOK, doc)
}

func (h *Handlers) resolveCallAnalysisInput(c *gin.Context) (tmpPath string, in callanalysis.Input, err error) {
	ct := strings.ToLower(c.ContentType())
	if strings.Contains(ct, "application/json") {
		var body callAnalysisJSONBody
		if err := c.ShouldBindJSON(&body); err != nil {
			return "", in, fmt.Errorf("invalid json: %w", err)
		}
		u := strings.TrimSpace(body.AudioURL)
		if u == "" {
			return "", in, fmt.Errorf("audio_url is required in json body")
		}
		return h.downloadAudioToTemp(c, u)
	}

	if err := c.Request.ParseMultipartForm(callAnalysisMultipartMax); err != nil {
		return "", in, fmt.Errorf("parse multipart: %w", err)
	}
	if u := strings.TrimSpace(c.PostForm("audio_url")); u != "" {
		return h.downloadAudioToTemp(c, u)
	}
	fh, err := c.FormFile("audio")
	if err != nil {
		return "", in, fmt.Errorf("missing audio file (form field \"audio\") or audio_url")
	}
	if fh.Size > callanalysis.MaxAudioBytes {
		return "", in, fmt.Errorf("upload exceeds %d bytes", callanalysis.MaxAudioBytes)
	}
	src, err := fh.Open()
	if err != nil {
		return "", in, err
	}
	defer func() { _ = src.Close() }()

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext == "" {
		ext = ".audio"
	}
	f, err := os.CreateTemp("", "call-analysis-upload-*"+ext)
	if err != nil {
		return "", in, err
	}
	tmpPath = f.Name()
	if _, err := io.Copy(f, io.LimitReader(src, callanalysis.MaxAudioBytes+1)); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", in, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", in, err
	}
	st, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", in, err
	}
	if st.Size() > int64(callanalysis.MaxAudioBytes) {
		_ = os.Remove(tmpPath)
		return "", in, fmt.Errorf("upload exceeds %d bytes", callanalysis.MaxAudioBytes)
	}
	in = callanalysis.Input{
		LocalPath: tmpPath,
		Source:    "upload",
		Filename:  fh.Filename,
	}
	return tmpPath, in, nil
}

func (h *Handlers) downloadAudioToTemp(c *gin.Context, rawURL string) (tmpPath string, in callanalysis.Input, err error) {
	return callanalysis.DownloadAudioURLToTemp(c.Request.Context(), rawURL)
}
