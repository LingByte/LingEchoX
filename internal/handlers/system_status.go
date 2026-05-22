package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// 平台管理员可见的系统监控只读端点。数据源：
//   - pkg/utils/system.GetSystemStatus()   → CPU / Memory / Disk
//   - pkg/utils/system.GetDiskCacheStats() → 磁盘缓存命中 / 占用
//   - pkg/utils/system.GetPerformanceMonitorConfig() → 运行时阈值
//
// 这些值由 main.go 启动期挂起的后台 goroutine 持续刷新（system.StartSystemMonitor）。
// 端点本身只做内存级读取，无 DB / 网络 IO，可在 /metrics ACL 不便覆盖时作为
// 紧急排障入口。访问受 RequirePlatformAdmin 控制，不能被租户用户读取。

import (
	"runtime"
	"time"

	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/utils/system"
	"github.com/gin-gonic/gin"
)

// systemStartedAt 由 init 记录，可用于在响应里给运维一个 uptime 数字，
// 比让前端去算服务启动时间方便。
var systemStartedAt = time.Now()

// getSystemStatus 返回当前进程的实时资源使用快照。
// GET /system/status (RequirePlatformAdmin)
func (h *Handlers) getSystemStatus(c *gin.Context) {
	st := system.GetSystemStatus()
	cfg := system.GetPerformanceMonitorConfig()
	disk := system.GetDiskSpaceInfo()
	cacheStats := system.GetDiskCacheStats()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	response.Success(c, "success", gin.H{
		"uptimeSeconds": int64(time.Since(systemStartedAt).Seconds()),
		"goroutines":    runtime.NumGoroutine(),
		"go":            runtime.Version(),
		"resource": gin.H{
			"cpuUsage":    st.CPUUsage,
			"memoryUsage": st.MemoryUsage,
			"diskUsage":   st.DiskUsage,
		},
		"disk": gin.H{
			"total":       disk.Total,
			"free":        disk.Free,
			"used":        disk.Used,
			"usedPercent": disk.UsedPercent,
		},
		"runtimeMemory": gin.H{
			// Sys 是 Go 运行时向 OS 申请的总字节数（包括内部堆/栈/GC 元数据），
			// HeapAlloc 是当前活跃堆对象的字节数。两个一起看可以判断
			// GC 压力（HeapAlloc 高 + Sys 高 = 真有内存使用；HeapAlloc
			// 低 + Sys 高 = 历史峰值占着没归还）。
			"sys":         mem.Sys,
			"heapAlloc":   mem.HeapAlloc,
			"heapInUse":   mem.HeapInuse,
			"heapObjects": mem.HeapObjects,
			"gcCount":     mem.NumGC,
		},
		"performanceMonitor": gin.H{
			"enabled":         cfg.Enabled,
			"cpuThreshold":    cfg.CPUThreshold,
			"memoryThreshold": cfg.MemoryThreshold,
			"diskThreshold":   cfg.DiskThreshold,
		},
		"diskCache": gin.H{
			"activeFiles":     cacheStats.ActiveDiskFiles,
			"diskUsageBytes":  cacheStats.CurrentDiskUsageBytes,
			"diskHits":        cacheStats.DiskCacheHits,
			"memoryHits":      cacheStats.MemoryCacheHits,
			"diskMaxBytes":    cacheStats.DiskCacheMaxBytes,
			"diskThreshold":   cacheStats.DiskCacheThresholdBytes,
			"memoryBuffers":   cacheStats.ActiveMemoryBuffers,
			"memoryUsageByte": cacheStats.CurrentMemoryUsageBytes,
		},
	})
}
