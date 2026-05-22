// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

package apperror

import (
	"github.com/gin-gonic/gin"
)

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
