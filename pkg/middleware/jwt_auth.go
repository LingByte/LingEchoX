package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/i18n"
	"github.com/LinByte/VoiceServer/pkg/utils/access"
	"github.com/gin-gonic/gin"
)

const (
	CtxAuthUserID     = "auth.userId"
	CtxAuthTenantID   = "auth.tenantId"
	CtxAuthTenantSlug = "auth.tenantSlug"
	CtxAuthEmail      = "auth.email"
	CtxAuthRole       = "auth.role"
)

// TryAttachTenantJWT parses Authorization: Bearer JWT and sets tenant auth context. Returns false if absent or invalid (no response written).
func TryAttachTenantJWT(c *gin.Context) bool {
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
	p, err := access.ParseAccessTokenWithKey(token, km)
	if err != nil || p.TenantID == 0 {
		return false
	}
	c.Set(CtxAuthUserID, p.UserID)
	c.Set(CtxAuthTenantID, p.TenantID)
	c.Set(CtxAuthTenantSlug, p.TenantSlug)
	c.Set(CtxAuthEmail, p.Email)
	c.Set(CtxAuthRole, p.Role)
	return true
}

// RequireTenantAuth verifies Bearer JWT only (no AK/SK).
func RequireTenantAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := strings.TrimSpace(c.GetHeader("Authorization"))
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": i18n.TGin(c, i18n.KeyAuthMissingToken), "data": nil})
			return
		}
		if !TryAttachTenantJWT(c) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": i18n.TGin(c, i18n.KeyAuthInvalidToken), "data": nil})
			return
		}
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

// CurrentTenantID is an alias for AuthTenantID for use in handlers
// where middleware has already guaranteed a non-zero tenant context.
func CurrentTenantID(c *gin.Context) uint { return AuthTenantID(c) }

// RequireHumanJWTUser rejects requests that lack a human user ID (i.e. AKSK-only callers).
// Use on routes where only logged-in humans should operate (e.g. credential management).
func RequireHumanJWTUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if AuthUserID(c) == 0 || AuthTenantID(c) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": i18n.TGin(c, i18n.KeyForbidden), "data": nil})
			return
		}
		c.Next()
	}
}

func AuthEmail(c *gin.Context) string {
	if v, ok := c.Get(CtxAuthEmail); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// AuditOperator returns email, user id string, or "system" for create_by / update_by fields.
func AuditOperator(c *gin.Context) string {
	if s := strings.TrimSpace(AuthEmail(c)); s != "" {
		return s
	}
	if uid := AuthUserID(c); uid > 0 {
		return strconv.FormatUint(uint64(uid), 10)
	}
	return "system"
}
