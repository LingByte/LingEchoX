package logger

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// HTTP header and Gin context key for request correlation (aligned with root logger package).
const (
	HeaderXReqID   = "X-Reqid"
	GinCtxReqIDKey = "x-reqid"
)

var genReqPID = uint32(time.Now().UnixNano() % 4294967291)

// GenReqID generates a URL-safe request id (same algorithm as github.com/.../logger at repo root).
func GenReqID() string {
	var b [12]byte
	binary.LittleEndian.PutUint32(b[:], genReqPID)
	binary.LittleEndian.PutUint64(b[4:], uint64(time.Now().UnixNano()))
	return base64.URLEncoding.EncodeToString(b[:])
}

// WithRequestID stores req id on context for InfoCtx / ErrorCtx and downstream RPC.
func WithRequestID(ctx context.Context, reqID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if reqID == "" {
		return ctx
	}
	return context.WithValue(ctx, RequestIDKey, reqID)
}

// RequestIDFromContext reads the request id from context.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(RequestIDKey); v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// ZapReqID returns a zap field for x-reqid when present on context.
func ZapReqID(ctx context.Context) zap.Field {
	id := RequestIDFromContext(ctx)
	if id == "" {
		return zap.Skip()
	}
	return zap.String("x-reqid", id)
}

// ZapReqIDString returns a zap field for a raw request id string.
func ZapReqIDString(reqID string) zap.Field {
	if reqID == "" {
		return zap.Skip()
	}
	return zap.String("x-reqid", reqID)
}
