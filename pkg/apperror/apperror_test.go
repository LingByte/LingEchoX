// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package apperror_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LinByte/VoiceServer/pkg/apperror"
	"github.com/gin-gonic/gin"
)

func TestNew_DefaultsHTTPStatus(t *testing.T) {
	e := apperror.New(apperror.CodeNotFound, "缺资源")
	if e.HTTPStatus != http.StatusNotFound {
		t.Fatalf("want 404, got %d", e.HTTPStatus)
	}
	if e.Error() == "" {
		t.Fatalf("Error() should be non-empty")
	}
}

func TestWrap_PreservesCause(t *testing.T) {
	root := errors.New("db gone")
	e := apperror.Wrap(apperror.CodeInternal, "查询失败", root)
	if !errors.Is(e, root) {
		t.Fatalf("errors.Is should chain through Cause")
	}
}

func TestIs_ByCode(t *testing.T) {
	a := apperror.New(apperror.CodeTenantMismatch, "X")
	b := apperror.New(apperror.CodeTenantMismatch, "Y")
	if !errors.Is(a, b) {
		t.Fatalf("two errors with same Code should be Is-equal")
	}
	c := apperror.New(apperror.CodeNotFound, "Z")
	if errors.Is(a, c) {
		t.Fatalf("different codes should not be Is-equal")
	}
}

func TestFrom_PassthroughAndWrap(t *testing.T) {
	pass := apperror.New(apperror.CodeForbidden, "no")
	if got := apperror.From(pass); got != pass {
		t.Fatalf("From should passthrough existing AppError")
	}
	wrapped := apperror.From(errors.New("raw"))
	if wrapped.Code != apperror.CodeInternal {
		t.Fatalf("raw error should be wrapped to CodeInternal, got %s", wrapped.Code)
	}
	if apperror.From(nil) != nil {
		t.Fatalf("From(nil) should be nil")
	}
}

func TestRender_WritesStableEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperror.New(apperror.CodeValidation, "字段不合法").
		WithDetails(map[string]any{"field": "email"})

	apperror.Render(c, err)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("HTTP status want 400, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body json: %v", err)
	}
	if body["error"] != "VALIDATION_FAILED" {
		t.Fatalf("error code mismatch: %v", body["error"])
	}
	if body["msg"] != "字段不合法" {
		t.Fatalf("msg mismatch: %v", body["msg"])
	}
	if d, ok := body["data"].(map[string]any); !ok || d["field"] != "email" {
		t.Fatalf("data details missing: %v", body["data"])
	}
}

func TestRender_NilNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	apperror.Render(c, nil)
	if w.Code != http.StatusOK { // httptest default
		t.Fatalf("nil Render should not write status, got %d", w.Code)
	}
}
