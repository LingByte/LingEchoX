package middleware

import (
	"net/http"
	"strings"

	"github.com/LingByte/SoulNexus/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

const (
	CtxAuthUserID     = "auth.userId"
	CtxAuthTenantID   = "auth.tenantId"
	CtxAuthTenantSlug = "auth.tenantSlug"
	CtxAuthEmail      = "auth.email"
	CtxAuthRole       = "auth.role"
)

// RequireTenantAuth verifies Bearer JWT and injects auth claims into Gin context.
func RequireTenantAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := strings.TrimSpace(c.GetHeader("Authorization"))
		if authz == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing authorization token", "data": nil})
			return
		}
		if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid authorization header", "data": nil})
			return
		}
		token := strings.TrimSpace(authz[len("Bearer "):])
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "empty bearer token", "data": nil})
			return
		}

		km := access.JWTKeyManager()
		if km == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "jwt verifier unavailable", "data": nil})
			return
		}

		p, err := access.ParseAccessTokenWithKey(token, km)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid or expired token", "data": nil})
			return
		}
		if p.TenantID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "tenant scope required", "data": nil})
			return
		}

		c.Set(CtxAuthUserID, p.UserID)
		c.Set(CtxAuthTenantID, p.TenantID)
		c.Set(CtxAuthTenantSlug, p.TenantSlug)
		c.Set(CtxAuthEmail, p.Email)
		c.Set(CtxAuthRole, p.Role)
		c.Next()
	}
}

func AuthTenantID(c *gin.Context) uint {
	if v, ok := c.Get(CtxAuthTenantID); ok {
		if n, ok := v.(uint); ok {
			return n
		}
	}
	return 0
}

func AuthUserID(c *gin.Context) uint {
	if v, ok := c.Get(CtxAuthUserID); ok {
		if n, ok := v.(uint); ok {
			return n
		}
	}
	return 0
}

func AuthEmail(c *gin.Context) string {
	if v, ok := c.Get(CtxAuthEmail); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
