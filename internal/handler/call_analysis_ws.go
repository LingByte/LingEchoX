package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/pkg/callanalysis"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var callAnalysisWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 256,
	WriteBufferSize: 1024 * 512,
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

type callAnalysisWSStart struct {
	Type      string `json:"type"`
	AudioURL  string `json:"audio_url"`
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
}

// callAnalysisWebSocket streams ASR sentence events then LLM result; ends with complete.export_doc (same as HTTP POST).
// Protocol: first text frame JSON — {"type":"start","audio_url":"https://..."} OR {"type":"start_file","filename":"x.mp3","size_bytes":N}
// then binary frames until N bytes (for start_file). All server messages are JSON text frames.
func (h *Handlers) callAnalysisWebSocket(c *gin.Context) {
	if h == nil || h.callAnalysisStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "call analysis store not initialized"})
		return
	}
	env := callanalysis.EnvFromProcess()
	if err := env.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conn, err := callAnalysisWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Minute))

	mt, payload, err := conn.ReadMessage()
	if err != nil {
		return
	}
	if mt != websocket.TextMessage {
		_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "first frame must be JSON text"})
		return
	}
	var start callAnalysisWSStart
	if err := json.Unmarshal(payload, &start); err != nil {
		_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "invalid json: " + err.Error()})
		return
	}
	start.Type = strings.ToLower(strings.TrimSpace(start.Type))

	var tmpPath string
	var in callanalysis.Input
	ctx := c.Request.Context()

	switch start.Type {
	case "start":
		u := strings.TrimSpace(start.AudioURL)
		if u == "" {
			_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "audio_url required"})
			return
		}
		var derr error
		tmpPath, in, derr = callanalysis.DownloadAudioURLToTemp(ctx, u)
		if derr != nil {
			_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": derr.Error()})
			return
		}
		defer func() {
			if tmpPath != "" {
				_ = os.Remove(tmpPath)
			}
		}()

	case "start_file":
		if start.SizeBytes <= 0 || start.SizeBytes > int64(callanalysis.MaxAudioBytes) {
			_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": fmt.Sprintf("size_bytes must be 1..%d", callanalysis.MaxAudioBytes)})
			return
		}
		buf := make([]byte, 0, start.SizeBytes)
		for int64(len(buf)) < start.SizeBytes {
			_ = conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
			mtb, chunk, rerr := conn.ReadMessage()
			if rerr != nil {
				_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "read file: " + rerr.Error()})
				return
			}
			if mtb != websocket.BinaryMessage {
				_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "expected binary audio chunks"})
				return
			}
			if int64(len(buf)+len(chunk)) > start.SizeBytes {
				_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "received more bytes than size_bytes"})
				return
			}
			buf = append(buf, chunk...)
		}
		ext := strings.ToLower(filepath.Ext(strings.TrimSpace(start.Filename)))
		if ext == "" {
			ext = ".audio"
		}
		f, cerr := os.CreateTemp("", "call-analysis-ws-*"+ext)
		if cerr != nil {
			_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": cerr.Error()})
			return
		}
		tmpPath = f.Name()
		if _, cerr = f.Write(buf); cerr != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": cerr.Error()})
			return
		}
		if cerr = f.Close(); cerr != nil {
			_ = os.Remove(tmpPath)
			_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": cerr.Error()})
			return
		}
		defer func() { _ = os.Remove(tmpPath) }()
		in = callanalysis.Input{
			LocalPath: tmpPath,
			Source:    "ws_upload",
			Filename:  strings.TrimSpace(start.Filename),
		}

	default:
		_ = conn.WriteJSON(map[string]interface{}{"type": "error", "message": "unknown type; use start or start_file"})
		return
	}

	asrModel := strings.TrimSpace(env.ASRModelType)
	if asrModel == "" {
		asrModel = "16k_zh"
	}
	pcmRate := callanalysis.PCMSampleRateFromASRModel(asrModel)
	pcm, derr := callanalysis.DecodeToPCMSMono(in.LocalPath, pcmRate)
	if derr != nil {
		_ = conn.WriteJSON(map[string]interface{}{"type": "error", "stage": "decode", "message": derr.Error()})
		return
	}

	var wmu sync.Mutex
	emit := func(ev map[string]interface{}) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Minute))
		return conn.WriteJSON(ev)
	}

	_ = emit(map[string]interface{}{"type": "hello", "version": 1, "pcm_sample_rate_hz": pcmRate, "asr_model": asrModel})

	doc, runErr := callanalysis.StreamFullAnalysis(ctx, env, pcm, pcmRate, in, emit)
	if runErr != nil {
		return
	}
	if doc != nil {
		h.callAnalysisStore.Put(doc)
	}
}
