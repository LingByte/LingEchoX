// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package logger

import (
	"context"
	"testing"
)

func TestTenantIDRoundTrip(t *testing.T) {
	ctx := WithTenantID(context.Background(), 42)
	if got := TenantIDFromContext(ctx); got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
	// 0 应被视为未设置，不修改 ctx
	ctx0 := WithTenantID(context.Background(), 0)
	if got := TenantIDFromContext(ctx0); got != 0 {
		t.Fatalf("0 should remain unset, got %d", got)
	}
}

func TestCallIDRoundTrip(t *testing.T) {
	ctx := WithCallID(context.Background(), "abc@x")
	if got := CallIDFromContext(ctx); got != "abc@x" {
		t.Fatalf("call id mismatch: %s", got)
	}
	if CallIDFromContext(nil) != "" {
		t.Fatalf("nil ctx should yield empty call id")
	}
}

func TestCampaignIDRoundTrip(t *testing.T) {
	ctx := WithCampaignID(context.Background(), 99)
	if got := CampaignIDFromContext(ctx); got != 99 {
		t.Fatalf("campaign id mismatch: %d", got)
	}
}

func TestFromContext_NilLoggerSafe(t *testing.T) {
	// 不调用 InitLogger 时 Lg 可能为 nil，FromContext 应回退到 nop logger 而非 panic。
	saved := Lg
	Lg = nil
	defer func() { Lg = saved }()

	log := FromContext(context.Background())
	if log == nil {
		t.Fatal("FromContext must never return nil")
	}
	// nop logger.Info shouldn't crash.
	log.Info("smoke")
}
