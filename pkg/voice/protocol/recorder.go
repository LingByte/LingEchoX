package protocol

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AudioRecorder 音频录音器（内存存储）
type AudioRecorder struct {
	mu            sync.RWMutex
	data          []byte
	isRecording   bool
	startTime     time.Time
	totalBytes    int64
	sampleRate    int
	channels      int
	bitsPerSample int
	logger        *zap.Logger
}

// NewAudioRecorder 创建音频录音器（内存缓冲）
func NewAudioRecorder(sampleRate, channels, bitsPerSample int, logger *zap.Logger) (*AudioRecorder, error) {
	if logger == nil {
		logger = zap.L()
	}

	recorder := &AudioRecorder{
		data:          make([]byte, 0, 64*1024),
		isRecording:   true,
		startTime:     time.Now(),
		sampleRate:    sampleRate,
		channels:      channels,
		bitsPerSample: bitsPerSample,
		logger:        logger,
	}

	logger.Info("[Recorder] 音频录音器已创建",
		zap.Int("sampleRate", sampleRate),
		zap.Int("channels", channels))

	return recorder, nil
}

// WriteAudio 写入音频数据
func (r *AudioRecorder) WriteAudio(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording {
		return fmt.Errorf("录音器未激活")
	}
	r.data = append(r.data, data...)
	r.totalBytes += int64(len(data))
	return nil
}

// Stop 停止录音并关闭文件
func (r *AudioRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording {
		return nil
	}
	r.isRecording = false

	duration := time.Since(r.startTime)
	r.logger.Info("[Recorder] 音频录音已停止",
		zap.Int64("totalBytes", r.totalBytes),
		zap.Duration("duration", duration))

	return nil
}

// GetWAVData 获取完整 WAV 数据
func (r *AudioRecorder) GetWAVData() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return buildWAV(r.data, r.sampleRate, r.channels, r.bitsPerSample)
}

// GetTotalBytes 获取录音总字节数
func (r *AudioRecorder) GetTotalBytes() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.totalBytes
}

// GetDuration 获取录音时长（秒）
func (r *AudioRecorder) GetDuration() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.sampleRate == 0 {
		return 0
	}

	// PCM数据字节数 / (采样率 * 声道数 * 字节数/样本)
	bytesPerSample := r.bitsPerSample / 8
	totalSamples := r.totalBytes / int64(r.channels*bytesPerSample)
	return int(totalSamples / int64(r.sampleRate))
}

// buildWAV 构建 WAV 字节流
func buildWAV(pcm []byte, sampleRate, channels, bitDepth int) []byte {
	header := make([]byte, 44)
	dataSize := len(pcm)
	copy(header[0:4], "RIFF")
	fileSize := uint32(36 + dataSize)
	header[4] = byte(fileSize)
	header[5] = byte(fileSize >> 8)
	header[6] = byte(fileSize >> 16)
	header[7] = byte(fileSize >> 24)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	header[16] = 16
	header[20] = 1
	header[22] = byte(channels)
	sr := uint32(sampleRate)
	header[24] = byte(sr)
	header[25] = byte(sr >> 8)
	header[26] = byte(sr >> 16)
	header[27] = byte(sr >> 24)
	byteRate := uint32(sampleRate * channels * bitDepth / 8)
	header[28] = byte(byteRate)
	header[29] = byte(byteRate >> 8)
	header[30] = byte(byteRate >> 16)
	header[31] = byte(byteRate >> 24)
	blockAlign := uint16(channels * bitDepth / 8)
	header[32] = byte(blockAlign)
	header[33] = byte(blockAlign >> 8)
	header[34] = byte(bitDepth)
	copy(header[36:40], "data")
	ds := uint32(dataSize)
	header[40] = byte(ds)
	header[41] = byte(ds >> 8)
	header[42] = byte(ds >> 16)
	header[43] = byte(ds >> 24)
	return append(header, pcm...)
}
