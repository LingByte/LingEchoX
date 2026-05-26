// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Package apperror defines the canonical application error envelope.
//
// Background
// ----------
// Pre-refactor状态：handler / service / utils 各自用 `errors.New("xxx exists")` +
// `response.Fail(c, "xxx exists", nil)` 来表达业务错误。前端拿到的 code 永远是 500/200
// HTTP-style，业务错误码用中文消息 字面量去做分支。i18n 完全没法做。
//
// 这个包给出唯一答案：
//
//   - 业务层返回 *AppError；
//   - HTTP 层用 Render(c, err) 一行渲染；
//   - 前端按 Code 字段做行为分支，按 Message 字段做兜底文案；
//   - 后端日志结构化记录 (Code, Cause, HTTPStatus)，不再 grep 错误字符串。
//
// Design notes
// ------------
// 1) Code 是稳定的 *字符串* 枚举（不是 int）。字符串便于跨语言对齐 + 增删时不撕已有 client。
// 2) AppError 实现 error 接口，可以 errors.Is/As 套娃 wrap 底层错误。
// 3) HTTPStatus 内置在错误里，避免 handler 还要写 switch err.code -> http status 一遍。
// 4) 不引入 i18n bundle —— 这个包只负责"错误结构"，文案如何本地化交给上层。
package apperror

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Code 是稳定的业务错误码。前端按它做行为分支与文案选择。
// 添加新码：直接 const 一个；不要复用旧码语义。
type Code string

const (
	// 通用类（与 HTTP 1:1，不带业务语义）
	CodeBadRequest     Code = "BAD_REQUEST"
	CodeUnauthorized   Code = "UNAUTHORIZED"
	CodeForbidden      Code = "FORBIDDEN"
	CodeNotFound       Code = "NOT_FOUND"
	CodeConflict       Code = "CONFLICT"
	CodeRateLimited    Code = "RATE_LIMITED"
	CodeInternal       Code = "INTERNAL"
	CodeServiceUnavail Code = "SERVICE_UNAVAILABLE"

	// 业务类（按需扩展；命名风格 <DOMAIN>_<REASON>）
	CodeValidation        Code = "VALIDATION_FAILED"
	CodeTenantMismatch    Code = "TENANT_MISMATCH"
	CodeAuthFailed        Code = "AUTH_FAILED"
	CodeCredentialInvalid Code = "CREDENTIAL_INVALID"
	CodeQuotaExceeded     Code = "QUOTA_EXCEEDED"
	CodeProviderError     Code = "PROVIDER_ERROR"
	CodeUpstreamTimeout   Code = "UPSTREAM_TIMEOUT"
	CodeDuplicate         Code = "DUPLICATE"
)

// AppError 是业务错误的统一形态。
//
// 字段语义：
//   - Code        : 稳定字符串枚举，前端按它分支；不要在日志/消息里替代它出现。
//   - Message     : 给最终用户看的友好文案，可中文；不在日志里反复打印（日志靠 Code）。
//   - HTTPStatus  : 渲染到 HTTP 响应时使用的 status code；不写则自动按 Code 推断。
//   - Cause       : 底层错误，errors.Is/As 链上；Render 不外泄给客户端。
//   - Details     : 渲染到响应 data 字段的结构化补充信息（字段错误、租户 id 等）。
//
// 用例：
//
//	if user.TenantID != tenantID {
//	    return apperror.New(apperror.CodeTenantMismatch, "用户不属于当前租户")
//	}
type AppError struct {
	Code       Code
	Message    string
	HTTPStatus int
	Cause      error
	Details    map[string]any
}

// Error 实现 error 接口。日志友好（出现 code、message、cause），不要直接渲染到前端。
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 让 errors.Is / errors.As 走到底层 Cause。
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is 让 errors.Is 直接按 Code 比较（apperror.Is(err, ErrSentinel) 这种用例）。
// 仅当 target 也是 *AppError 时比较 Code，否则走 Cause 链。
func (e *AppError) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	var t *AppError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// New 构造一个新错误。HTTPStatus 默认按 Code 推断。
func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: HTTPStatusFor(code)}
}

// Newf 是 New 的 printf 版本，便于格式化 message。
func Newf(code Code, format string, args ...any) *AppError {
	return New(code, fmt.Sprintf(format, args...))
}

// Wrap 把底层 error 包成 AppError；底层错误透传给 errors.Is/As 链。
// 在 service 边界使用：把 gorm.ErrRecordNotFound / context.DeadlineExceeded 之类
// 翻译成业务码，原始错误保留在 Cause 里给日志看。
func Wrap(code Code, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: HTTPStatusFor(code),
		Cause:      cause,
	}
}

// WithStatus 显式覆盖 HTTPStatus；用于 Code 默认推断不对的场景。
func (e *AppError) WithStatus(status int) *AppError {
	if e == nil {
		return nil
	}
	e.HTTPStatus = status
	return e
}

// WithDetails 设置渲染到响应 data.details 的结构化补充。
// 注意：这里放进去的字段会送给客户端，不要把内部诊断信息塞进来。
func (e *AppError) WithDetails(d map[string]any) *AppError {
	if e == nil {
		return nil
	}
	e.Details = d
	return e
}

// WithCause 追加 / 替换底层错误。
func (e *AppError) WithCause(cause error) *AppError {
	if e == nil {
		return nil
	}
	e.Cause = cause
	return e
}

// From 尝试把任意 error 转成 *AppError。
//   - 已是 *AppError 时原样返回；
//   - nil 返回 nil；
//   - 其他错误被包成 CodeInternal（避免上层在 nil-check 之外还要做类型断言）。
func From(err error) *AppError {
	if err == nil {
		return nil
	}
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return Wrap(CodeInternal, "internal error", err)
}

// HTTPStatusFor 把 Code 映射到默认 HTTP status code。
// 业务可以通过 WithStatus 覆盖。
func HTTPStatusFor(code Code) int {
	switch code {
	case CodeBadRequest, CodeValidation:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeAuthFailed, CodeCredentialInvalid:
		return http.StatusUnauthorized
	case CodeForbidden, CodeTenantMismatch:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeDuplicate:
		return http.StatusConflict
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeQuotaExceeded:
		return http.StatusPaymentRequired
	case CodeUpstreamTimeout:
		return http.StatusGatewayTimeout
	case CodeServiceUnavail, CodeProviderError:
		return http.StatusServiceUnavailable
	case CodeInternal:
		fallthrough
	default:
		return http.StatusInternalServerError
	}
}

// Render 把 AppError 写到 HTTP 响应。
//
// 响应体格式（与既有 pkg/response 风格保持一致，便于前端兼容）：
//
//	{
//	  "code": 400,                 // HTTP-style 数字码，前端历史代码靠这个
//	  "msg":  "用户不属于当前租户",
//	  "error": "TENANT_MISMATCH",  // 稳定字符串码，新代码靠这个
//	  "data": { ... }              // Details 透传；nil 时返回 null
//	}
//
// 为什么同时返回 code 和 error：
//
//   - 兼容已经写好的前端：前端旧逻辑读 code === 200 判断成功，迁移期需要兼容。
//   - 新逻辑用 error 这个字符串做行为分支，更稳定。
//
// 如果 err 为 nil，直接 no-op（让 handler 主流程能写：return Render(c, err) 收尾）。
func Render(c *gin.Context, err error) {
	if err == nil {
		return
	}
	ae := From(err)
	if ae == nil {
		return
	}
	status := ae.HTTPStatus
	if status == 0 {
		status = HTTPStatusFor(ae.Code)
	}
	body := gin.H{
		"code":  status,
		"msg":   ae.Message,
		"error": string(ae.Code),
		"data":  ae.Details,
	}
	c.AbortWithStatusJSON(status, body)
}
