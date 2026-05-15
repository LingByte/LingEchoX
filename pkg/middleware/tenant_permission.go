package middleware

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/constants"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// permCache caches user permission codes for 30s to avoid DB hit per request.
var permCache = utils.NewExpiredLRUCache[uint, []string](2048, 30*time.Second)

// InvalidatePermissionCache removes a user's cached permission codes.
// Call this after role/permission assignment changes.
func InvalidatePermissionCache(userID uint) {
	permCache.Remove(userID)
}

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
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "internal error", "data": nil})
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
		userCodes := cachedPermissionCodes(db, uid)
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
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "internal error", "data": nil})
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
		userCodes := cachedPermissionCodes(db, uid)
		for _, req := range codes {
			if slices.Contains(userCodes, req) {
				c.Next()
				return
			}
		}
		abortForbiddenPermission(c, "权限不足")
	}
}

// cachedPermissionCodes returns permission codes from cache or DB.
func cachedPermissionCodes(db *gorm.DB, uid uint) []string {
	if codes, ok := permCache.Get(uid); ok {
		return codes
	}
	codes, _ := models.ListEffectivePermissionCodesForTenantUser(db, uid)
	permCache.Add(uid, codes)
	return codes
}
