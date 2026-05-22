// Package ginutil provides shared Gin handler helpers (param parsing, binding, errors).
package ginutil

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/stores"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ParamID parses c.Param(name) as uint. On failure writes {"msg":"invalid id"} and returns ok=false.
func ParamID(c *gin.Context, name string) (id uint, ok bool) {
	v, err := utils.ParseID(c.Param(name))
	if err != nil {
		response.Fail(c, "invalid id", nil)
		return 0, false
	}
	return v, true
}

// QueryPage reads page/size query params and clamps them (default max size 100).
func QueryPage(c *gin.Context, maxSize int) (page, size int) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	s, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	return utils.NormalizePage(p, s, maxSize)
}

// BindJSON binds JSON body into dest. On failure writes invalid body response and returns false.
func BindJSON(c *gin.Context, dest any) bool {
	if err := c.ShouldBindJSON(dest); err != nil {
		response.Fail(c, "invalid body", err.Error())
		return false
	}
	return true
}

// RequireAuthTenant returns JWT tenant id or writes unauthorized and returns ok=false.
func RequireAuthTenant(c *gin.Context) (tenantID uint, ok bool) {
	tid := middleware.AuthTenantID(c)
	if tid == 0 {
		response.Fail(c, "unauthorized", nil)
		return 0, false
	}
	return tid, true
}

// WriteGORMError maps err to client response. Returns true when err != nil (caller should return).
func WriteGORMError(c *gin.Context, err error, notFoundMsg string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if notFoundMsg == "" {
			notFoundMsg = "not found"
		}
		response.Fail(c, notFoundMsg, nil)
		return true
	}
	response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
	return true
}

// WriteInternalError aborts with HTTP 500 and returns true (caller should return).
func WriteInternalError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	response.AbortWithStatusJSON(c, http.StatusInternalServerError, err)
	return true
}

// PageSuccess writes a standard paginated list payload.
func PageSuccess(c *gin.Context, list any, total int64, page, size int) {
	response.Success(c, "success", gin.H{
		"list":  list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// UploadURL resolves a public URL for an object key (absolute when possible, else /uploads/<key>).
func UploadURL(c *gin.Context, key string) string {
	st := stores.Default()
	u := strings.TrimSpace(stores.PublicObjectURL(st, key))
	if lower := strings.ToLower(u); strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return u
	}
	// PublicObjectURL only returns a non-http value for local-disk stores
	// without STORAGE_PUBLIC_BASE_URL, in which case it's empty — we fall
	// through to host synthesis below. No dead assignment to u; the value
	// would never be observed past this point.
	proto := strings.TrimSpace(c.Request.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		proto = "http"
		if c.Request.TLS != nil {
			proto = "https"
		}
	}
	if host := strings.TrimSpace(c.Request.Host); host != "" {
		return proto + "://" + host + "/uploads/" + strings.TrimPrefix(key, "/")
	}
	return "/uploads/" + strings.TrimPrefix(key, "/")
}
