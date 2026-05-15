package middleware

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"net/http"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

const (
	CtxAuthPlatformAdminID = "auth.platformAdminId"
	AuthRolePlatformAdmin  = "platform_admin"
)

// TryAttachPlatformJWT parses Authorization Bearer for a platform-admin token (distinct issuer).
func TryAttachPlatformJWT(c *gin.Context) bool {
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return false
	}
	token := strings.TrimSpace(authz[len("Bearer "):])
	if token == "" {
		return false
	}
	km := access.JWTKeyManager()
	if km == nil {
		return false
	}
	p, err := access.ParsePlatformAccessTokenWithKey(token, km)
	if err != nil || p == nil || p.AdminID == 0 {
		return false
	}
	c.Set(CtxAuthPlatformAdminID, p.AdminID)
	c.Set(CtxAuthEmail, p.Email)
	c.Set(CtxAuthRole, AuthRolePlatformAdmin)
	// Ensure tenant context stays empty so mixed middleware cannot confuse identities.
	c.Set(CtxAuthUserID, uint(0))
	c.Set(CtxAuthTenantID, uint(0))
	c.Set(CtxAuthTenantSlug, "")
	return true
}

func AuthPlatformAdminID(c *gin.Context) uint {
	if v, ok := c.Get(CtxAuthPlatformAdminID); ok {
		if n, ok := v.(uint); ok {
			return n
		}
	}
	return 0
}

// RequirePlatformAdmin rejects requests that are not authenticated as a platform admin.
func RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if AuthPlatformAdminID(c) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "forbidden", "data": nil})
			return
		}
		c.Next()
	}
}
