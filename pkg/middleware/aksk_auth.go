package middleware

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"bytes"
	"crypto/hmac"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/internal/constants"
	"github.com/LinByte/VoiceServer/internal/models"
	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	HeaderXAk   = "X-Ak"
	HeaderXTs   = "X-Ts"
	HeaderXSign = "X-Sign"

	// CtxAuthCredentialID is set when the request is authenticated via X-Ak/X-Ts/X-Sign.
	CtxAuthCredentialID = "auth.credentialId"
	AuthRoleCredential  = "credential_aksk"
)

var akskSkew = int64(300) // 5 minutes

// RequireTenantJWTOrAKSK accepts (in order): tenant JWT, platform-admin JWT, or X-Ak/X-Ts/X-Sign (external integration).
func RequireTenantJWTOrAKSK() gin.HandlerFunc {
	return func(c *gin.Context) {
		if TryAttachTenantJWT(c) {
			c.Next()
			return
		}
		if TryAttachPlatformJWT(c) {
			c.Next()
			return
		}
		dbIface, exists := c.Get(constants.DbField)
		db, ok := dbIface.(*gorm.DB)
		if !exists || !ok || db == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "database unavailable", "data": nil})
			return
		}
		attempted, authed := tryAttachCredentialAKSK(c, db)
		if attempted && !authed {
			return
		}
		if authed {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing or invalid authorization", "data": nil})
	}
}

func tryAttachCredentialAKSK(c *gin.Context, db *gorm.DB) (attempted, ok bool) {
	ak := strings.TrimSpace(strings.TrimPrefix(c.GetHeader(HeaderXAk), "\ufeff"))
	ts := strings.TrimSpace(c.GetHeader(HeaderXTs))
	sig := strings.TrimSpace(c.GetHeader(HeaderXSign))
	if ak == "" && ts == "" && sig == "" {
		return false, false
	}
	attempted = true

	if ak == "" || ts == "" || sig == "" {
		abortAuthJSON(c, http.StatusUnauthorized, 401, "missing X-Ak/X-Ts/X-Sign headers")
		return true, false
	}

	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		abortAuthJSON(c, http.StatusUnauthorized, 401, "invalid X-Ts")
		return true, false
	}
	now := time.Now().Unix()
	if abs(now-tsUnix) > akskSkew {
		abortAuthJSON(c, http.StatusUnauthorized, 401, "request expired or excessive clock skew")
		return true, false
	}

	cred, err := models.GetActiveCredentialByAccessKey(db, ak)
	if err != nil {
		logger.Errorf("get active credential error %s", err)
		abortAuthJSON(c, http.StatusUnauthorized, 401, "unknown or revoked access key")
		return true, false
	}

	bodyBytes, rerr := readBodyForSigning(c)
	if rerr != nil {
		abortAuthJSON(c, http.StatusUnauthorized, 401, "unable to read request body")
		return true, false
	}

	method := strings.ToUpper(c.Request.Method)
	pathWithQuery := models.CredentialSignPathWithSortedQuery(c.Request.URL.Path, c.Request.URL.RawQuery)
	stringToSign := models.CredentialBuildStringToSign(method, pathWithQuery, ts, bodyBytes)
	expected := models.CredentialSignHex(cred.SecretKey, stringToSign)
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected))) {
		abortAuthJSON(c, http.StatusForbidden, 403, "invalid signature")
		return true, false
	}

	clientIP := strings.TrimSpace(c.ClientIP())
	if !models.CredentialClientIPAllowed(cred.AllowIP, clientIP) {
		abortAuthJSON(c, http.StatusForbidden, 403, "ip not allowed")
		return true, false
	}

	c.Set(CtxAuthUserID, uint(0))
	c.Set(CtxAuthTenantID, cred.TenantID)
	c.Set(CtxAuthTenantSlug, "")
	c.Set(CtxAuthEmail, "credential:"+cred.AccessKey)
	c.Set(CtxAuthRole, AuthRoleCredential)
	c.Set(CtxAuthCredentialID, cred.ID)
	return true, true
}

func readBodyForSigning(c *gin.Context) ([]byte, error) {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return nil, nil
	default:
		bodyBytes, err := c.GetRawData()
		if err != nil {
			return nil, err
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return bodyBytes, nil
	}
}

func abortAuthJSON(c *gin.Context, httpStatus, code int, msg string) {
	c.AbortWithStatusJSON(httpStatus, gin.H{"code": code, "msg": msg, "data": nil})
}
