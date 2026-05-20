package middleware

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/gin-gonic/gin"
)

// RequestIDMiddleware assigns X-Reqid on every HTTP request (reuse inbound header or generate).
// Must run before LoggerMiddleware and handlers so pkg/logger.FromGin / *Ctx helpers see the id.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := logger.IncomingReqID(c)
		if reqID == "" {
			reqID = logger.GenReqID()
		}
		c.Header(logger.HeaderXReqID, reqID)
		c.Set(logger.GinCtxReqIDKey, reqID)
		if c.Request != nil {
			c.Request = c.Request.WithContext(logger.WithRequestID(c.Request.Context(), reqID))
		}
		c.Next()
	}
}

// ReqIDFromGin is a convenience alias for logger.ReqIDFromGin.
func ReqIDFromGin(c *gin.Context) string {
	return logger.ReqIDFromGin(c)
}
