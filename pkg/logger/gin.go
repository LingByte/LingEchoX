package logger

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// IncomingReqID reads client-provided request id headers (first non-empty wins).
func IncomingReqID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, h := range []string{HeaderXReqID, "X-Req-ID", "X-Request-ID", "Request-Id"} {
		if v := strings.TrimSpace(c.GetHeader(h)); v != "" {
			return v
		}
	}
	return ""
}

// ReqIDFromGin returns request id from Gin context or request context.
func ReqIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(GinCtxReqIDKey); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if c.Request != nil {
		if id := RequestIDFromContext(c.Request.Context()); id != "" {
			return id
		}
	}
	return ""
}

// FromGin returns the global logger with x-reqid field when the request carries an id.
func FromGin(c *gin.Context) *zap.Logger {
	if Lg == nil {
		return zap.NewNop()
	}
	if id := ReqIDFromGin(c); id != "" {
		return Lg.With(zap.String("x-reqid", id))
	}
	return Lg
}

// ContextFromGin 把 Gin 上挂的常用追踪字段桥接到标准 context.Context。
//
// 业务层（service / SIP / 异步 goroutine）经常需要原生 ctx，而不是 gin.Context。
// 这个 helper 把以下已知字段一次性注入到返回的 ctx 上：
//   - x-reqid          （ReqIDFromGin 的结果）
//   - auth.tenantId    （uint，由 jwt_auth / aksk_auth middleware 设置）
//   - auth.userId      （uint，同上）
//
// 之后业务侧用 logger.FromContext(ctx).Info(...) 即可自动带齐 ID，goroutine
// 透传也只需把这个 ctx 带过去。
//
// 注意：不强依赖 middleware 包以避免循环依赖；auth key 字面量与 middleware
// 包定义保持一致，由 logger 包内部 const 镜像。
const (
	ginAuthTenantIDKey = "auth.tenantId"
	ginAuthUserIDKey   = "auth.userId"
)

func ContextFromGin(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	ctx := c.Request.Context()
	if id := ReqIDFromGin(c); id != "" {
		ctx = WithRequestID(ctx, id)
	}
	if v, ok := c.Get(ginAuthTenantIDKey); ok {
		if n, ok := v.(uint); ok && n > 0 {
			ctx = WithTenantID(ctx, int64(n))
		}
	}
	if v, ok := c.Get(ginAuthUserIDKey); ok {
		if n, ok := v.(uint); ok && n > 0 {
			// UserIDKey 已存在；保持字符串编码与现有 appendContextFields 一致。
			ctx = context.WithValue(ctx, UserIDKey, formatUint(n))
		}
	}
	return ctx
}

// formatUint 是为了避免引 strconv 在 gin.go 顶部，本文件已经因 strings 引入
// 较多 stdlib，能少一个包还是少一个。
func formatUint(n uint) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

// GinZapFields returns zap fields for HTTP handlers (x-reqid, method, path).
func GinZapFields(c *gin.Context) []zap.Field {
	if c == nil {
		return nil
	}
	fields := []zap.Field{
		ZapReqIDString(ReqIDFromGin(c)),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
	}
	if q := c.Request.URL.RawQuery; q != "" {
		fields = append(fields, zap.String("query", q))
	}
	return fields
}
