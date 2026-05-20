package middleware

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// UploadsRecordingsACL hardens the legacy `engine.Static("/uploads", …)`
// mount, which served EVERY file under UPLOAD_DIR with no auth — that
// includes call recordings written by pkg/sip/persist (key path
// `sip/recordings/<callID>_<ts>.wav`). Anyone who could enumerate or
// guess a Call-ID + timestamp could download a customer conversation.
//
// Behaviour:
//   - Requests under `/uploads/sip/recordings/` require Authorization:
//     Bearer JWT (tenant or platform admin) by default. AK/SK is NOT
//     accepted because external integrations should fetch via the
//     authenticated SIP-call API, which yields a signed URL when
//     STORAGE_PUBLIC_BASE_URL is configured.
//   - Override: UPLOADS_RECORDINGS_PUBLIC=true keeps the old behaviour
//     for legacy local-storage deployments where browser <audio> tags
//     hit /uploads directly. NOT recommended outside dev/private nets.
//   - All other /uploads/* paths (avatars, trunk-audio) keep public
//     read because they don't expose customer voice content. We don't
//     touch directory listing — gin.Static doesn't list dirs anyway.
//
// Note on browser playback: <audio src="/uploads/sip/recordings/x.wav">
// will NOT send Authorization headers, so frontend must either
//   (a) deploy STORAGE_PUBLIC_BASE_URL pointing at an authed proxy
//       (nginx auth_request, signed URLs, COS/OSS), or
//   (b) set UPLOADS_RECORDINGS_PUBLIC=true and accept the leak.
// A future improvement is a /api/sip-center/calls/:id/recording
// streaming endpoint that handles auth + Range requests server-side.

import (
	"net/http"
	"strings"
	"sync"

	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
)

const (
	envUploadsRecordingsPublic = "UPLOADS_RECORDINGS_PUBLIC"

	// uploadsRecordingsPrefix is the URL prefix relative to the static
	// mount; matches the key shape used by pkg/sip/persist/call_store.go.
	uploadsRecordingsPrefix = "/uploads/sip/recordings/"
)

var (
	uploadsRecordingsPublicOnce sync.Once
	uploadsRecordingsPublic     bool
)

func uploadsRecordingsPublicAllowed() bool {
	uploadsRecordingsPublicOnce.Do(func() {
		uploadsRecordingsPublic = strings.EqualFold(strings.TrimSpace(utils.GetEnv(envUploadsRecordingsPublic)), "true")
	})
	return uploadsRecordingsPublic
}

// UploadsACL is a global middleware that intercepts requests to the
// static /uploads mount and enforces auth on the recordings subtree.
// Mount it on the engine BEFORE engine.Static("/uploads", …).
func UploadsACL() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if !strings.HasPrefix(p, uploadsRecordingsPrefix) {
			c.Next()
			return
		}
		if uploadsRecordingsPublicAllowed() {
			c.Next()
			return
		}
		// Browser <audio> can't add Authorization. Accept either:
		//   - Authorization: Bearer <jwt>  (curl / ops / signed proxy)
		//   - ?token=<short-lived-sig>     (future signed URL — not yet
		//                                    implemented; placeholder)
		// We deliberately do NOT accept AK/SK: recordings are a
		// customer-data plane, not a programmatic integration target.
		if !TryAttachTenantJWT(c) && !TryAttachPlatformJWT(c) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "recording access requires authenticated bearer token; set UPLOADS_RECORDINGS_PUBLIC=true to disable (NOT for production)",
			})
			return
		}
		c.Next()
	}
}
