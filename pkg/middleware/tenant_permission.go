package middleware

import (
	"errors"
	"net/http"
	"slices"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/constants"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func dbFromContext(c *gin.Context) (*gorm.DB, bool) {
	raw, ok := c.Get(constants.DbField)
	if !ok {
		return nil, false
	}
	db, ok := raw.(*gorm.DB)
	return db, ok
}

// AuthCredentialID is non-zero when the request was authenticated via X-Ak/X-Ts/X-Sign.
func AuthCredentialID(c *gin.Context) uint {
	if v, ok := c.Get(CtxAuthCredentialID); ok {
		if n, ok := v.(uint); ok {
			return n
		}
	}
	return 0
}

func abortForbiddenPermission(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": msg, "data": nil})
}

// RequireTenantPermissionAll requires an authenticated tenant user JWT to have every listed permission code.
// Platform-admin JWT bypasses. AK/SK requests must satisfy credential.permission_codes ( wildcards via "*").
func RequireTenantPermissionAll(codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if AuthPlatformAdminID(c) != 0 {
			c.Next()
			return
		}
		if cid := AuthCredentialID(c); cid != 0 {
			db, ok := dbFromContext(c)
			if !ok || db == nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "database unavailable", "data": nil})
				return
			}
			if AuthTenantID(c) == 0 {
				abortForbiddenPermission(c, "需要租户上下文")
				return
			}
			if len(codes) == 0 {
				c.Next()
				return
			}
			okPerm, err := models.CredentialMatchesPermissionCodes(db, cid, codes, true)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					abortForbiddenPermission(c, "访问密钥无效")
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error(), "data": nil})
				return
			}
			if !okPerm {
				abortForbiddenPermission(c, "权限不足（访问密钥）")
				return
			}
			c.Next()
			return
		}
		db, ok := dbFromContext(c)
		if !ok || db == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "database unavailable", "data": nil})
			return
		}
		uid := AuthUserID(c)
		if uid == 0 || AuthTenantID(c) == 0 {
			abortForbiddenPermission(c, "需要租户用户登录")
			return
		}
		if len(codes) == 0 {
			c.Next()
			return
		}
		userCodes, err := models.ListEffectivePermissionCodesForTenantUser(db, uid)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error(), "data": nil})
			return
		}
		for _, req := range codes {
			if !slices.Contains(userCodes, req) {
				abortForbiddenPermission(c, "权限不足："+req)
				return
			}
		}
		c.Next()
	}
}

// RequireTenantPermissionAny requires the tenant user to have at least one of the listed codes.
func RequireTenantPermissionAny(codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if AuthPlatformAdminID(c) != 0 {
			c.Next()
			return
		}
		if cid := AuthCredentialID(c); cid != 0 {
			db, ok := dbFromContext(c)
			if !ok || db == nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "database unavailable", "data": nil})
				return
			}
			if AuthTenantID(c) == 0 {
				abortForbiddenPermission(c, "需要租户上下文")
				return
			}
			if len(codes) == 0 {
				c.Next()
				return
			}
			okPerm, err := models.CredentialMatchesPermissionCodes(db, cid, codes, false)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					abortForbiddenPermission(c, "访问密钥无效")
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error(), "data": nil})
				return
			}
			if !okPerm {
				abortForbiddenPermission(c, "权限不足（访问密钥）")
				return
			}
			c.Next()
			return
		}
		db, ok := dbFromContext(c)
		if !ok || db == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "database unavailable", "data": nil})
			return
		}
		uid := AuthUserID(c)
		if uid == 0 || AuthTenantID(c) == 0 {
			abortForbiddenPermission(c, "需要租户用户登录")
			return
		}
		if len(codes) == 0 {
			c.Next()
			return
		}
		userCodes, err := models.ListEffectivePermissionCodesForTenantUser(db, uid)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error(), "data": nil})
			return
		}
		for _, req := range codes {
			if slices.Contains(userCodes, req) {
				c.Next()
				return
			}
		}
		abortForbiddenPermission(c, "权限不足")
	}
}
