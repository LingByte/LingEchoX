// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package logger

// obsctx.go ——给 context.Context 提供"业务追踪 ID"的标准化 setter / getter。
//
// 之前的现状：
//   - 已有 RequestIDKey 装 x-reqid；
//   - tenant_id / call_id / campaign_id 各处自己 c.GetInt64 + zap.Int64 手撸；
//   - goroutine 起来后 ctx 不传，下游日志拿不到关联键；
//   - metrics label 与日志 field 不同名（"tenant" vs "tenant_id" vs "tid"），
//     排障时三套语义对不上。
//
// 现在统一约定：
//
//   - 设置：logger.WithTenantID(ctx, 42)
//   - 读取：logger.TenantIDFromContext(ctx) → int64
//   - 日志：logger.FromContext(ctx).Info(...) ——自动带上所有非空 ID
//   - metrics：metrics.LabelsFromContext(ctx) 可以走同一份提取器（后续接）
//
// 设计原则：所有 ID 在 context 里都用字符串形式存（与原 RequestIDKey 保持一致），
// 既避免 int 类型在不同包之间漂移，也让 logger output 直接是 string。

import (
	"context"
	"strconv"

	"go.uber.org/zap"
)

// WithTenantID 把 tenant id 注入 ctx。0 / 负值视为"未设置"，原样返回。
func WithTenantID(ctx context.Context, tenantID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tenantID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, TenantIDKey, strconv.FormatInt(tenantID, 10))
}

// WithCallID 把 SIP call id（或业务 call id）注入 ctx。空串视为未设置。
func WithCallID(ctx context.Context, callID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if callID == "" {
		return ctx
	}
	return context.WithValue(ctx, CallIDKey, callID)
}

// WithCampaignID 把外呼活动 id 注入 ctx。
func WithCampaignID(ctx context.Context, campaignID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if campaignID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, CampaignIDKey, strconv.FormatInt(campaignID, 10))
}

// TenantIDFromContext 读 tenant id。未设置返回 0。
// 这个返回 int64 是为了让业务侧拿来做 SQL where。
func TenantIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	v, _ := ctx.Value(TenantIDKey).(string)
	if v == "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// CallIDFromContext 读 call id（字符串原样返回）。
func CallIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(CallIDKey).(string)
	return v
}

// CampaignIDFromContext 读 campaign id。未设置返回 0。
func CampaignIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	v, _ := ctx.Value(CampaignIDKey).(string)
	if v == "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// FromContext 返回一个预绑定了所有上下文字段（x-reqid / tenant_id / call_id /
// campaign_id / user_id / trace_id）的 logger，调用方再 .Info(msg, extra...)
// 即可。Lg 未初始化时退化为 zap.NewNop()。
//
// 用例：
//
//	log := logger.FromContext(ctx)
//	log.Info("call started", zap.String("from", from))
//
// 输出会自动包含 ctx 上挂的 ID，避免每个调用点手填。
func FromContext(ctx context.Context) *zap.Logger {
	if Lg == nil {
		return zap.NewNop()
	}
	if ctx == nil {
		return Lg
	}
	fields := appendContextFields(ctx)
	if len(fields) == 0 {
		return Lg
	}
	return Lg.With(fields...)
}

// ContextFromGin 把 Gin context 上挂的 reqid / tenant_id 还原成标准 ctx
// （桥接 c.Request.Context() 与 c.GetXxx 两套设施）。供 service 层进入时
// 调用一次：ctx := logger.ContextFromGin(c)，之后整段调用链全用这个 ctx。
//
// 这里没有强依赖 gin 类型——通过接口约束反射：传入对象需要满足
// "拥有 Request.Context() 与 GetInt64 GetString" 的 contract。但实际项目里
// 直接传 *gin.Context 进来是最常用形态，所以我们提供具名 helper：见 gin.go。
