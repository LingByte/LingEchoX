package response

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Response struct {
	Code    int         `json:"code"` // 状态码，通常为 200 表示成功，非 200 为错误码
	Message string      `json:"msg"`  // 响应的消息描述
	Data    interface{} `json:"data"` // 返回的数据，可以是任意类型
}

func Success(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  msg,
		"data": data,
	})
}

func Fail(c *gin.Context, msg string, data interface{}) {
	// Standardize error response format
	errorResponse := gin.H{
		"code": 500,
		"msg":  msg,
		"data": data,
	}

	// If data contains error information, extract it for consistent format
	if dataMap, ok := data.(gin.H); ok {
		if errorCode, exists := dataMap["error"]; exists {
			errorResponse["error"] = errorCode
		}
		if message, exists := dataMap["message"]; exists && msg == "" {
			errorResponse["msg"] = message
		}
	}

	c.JSON(http.StatusOK, errorResponse)
}

func Result(context *gin.Context, httpStatus int, code int, msg string, data gin.H) {
	context.JSON(httpStatus, gin.H{
		"code": code,
		"msg":  msg,
		"data": data,
	})
}

func AbortWithStatus(c *gin.Context, httpStatus int) {
	c.AbortWithStatus(httpStatus)
}

// knownError maps well-known error substrings to user-friendly messages and error codes.
type knownError struct {
	substr  string
	msg     string
	errCode string
}

var knownErrors = []knownError{
	{"username must be at least 2 characters long", "用户名至少需要2个字符", "INVALID_USERNAME_LENGTH"},
	{"username can only contain", "用户名只能包含字母（包括中文）、数字、下划线和连字符", "INVALID_USERNAME_FORMAT"},
	{"email has exists", "该邮箱已被注册", "EMAIL_EXISTS"},
	{"password must be at least 8 characters long", "密码至少需要8个字符", "INVALID_PASSWORD_LENGTH"},
	{"captcha is required", "请输入验证码", "CAPTCHA_REQUIRED"},
	{"invalid captcha code", "验证码错误", "INVALID_CAPTCHA"},
}

func AbortWithStatusJSON(c *gin.Context, httpStatus int, err error) {
	errorMsg := err.Error()

	// Check for known client errors — return friendly message.
	for _, ke := range knownErrors {
		if strings.Contains(errorMsg, ke.substr) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":  400,
				"msg":   ke.msg,
				"error": ke.errCode,
				"data":  nil,
			})
			return
		}
	}

	// Unmatched error path. We split by status class:
	//
	//  - 5xx → "unknown server error" path: the original message is
	//    surfaced to the client (verbatim) tagged UNKNOWN_ERROR. This
	//    is intentional: a 5xx that didn't match a known-error
	//    pattern indicates a programmer/runtime issue, and hiding
	//    the cause behind a generic string forces ops to grep
	//    server logs for every report. The downside is leaking
	//    internal phrases — keep this in mind when writing error
	//    messages on the server side.
	//
	//  - Anything else (4xx) → "internal" sanitised path: callers
	//    that pass an unmapped error with a 4xx status are presumed
	//    misusing the helper; we hide the message and stamp
	//    INTERNAL_ERROR so the client gets a stable signal.
	logger.Error("internal server error",
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
		zap.Error(err),
	)
	if httpStatus >= 500 {
		c.AbortWithStatusJSON(httpStatus, gin.H{
			"code":  httpStatus,
			"msg":   errorMsg,
			"error": "UNKNOWN_ERROR",
			"data":  nil,
		})
		return
	}
	c.AbortWithStatusJSON(httpStatus, gin.H{
		"code":  httpStatus,
		"msg":   "internal error",
		"error": "INTERNAL_ERROR",
		"data":  nil,
	})
}
