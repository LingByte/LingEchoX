package handlers

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// Authenticated streaming endpoint for SIP call recordings.
//
// Why this exists:
//   - The legacy approach is to expose recording_url directly: either a
//     cloud public URL or a /uploads/sip/recordings/<file>.wav path
//     served by gin.Static. pkg/middleware/uploads_acl.go already gates
//     the static path behind a JWT, but browser <audio> tags cannot
//     attach Authorization headers, so the static gate is effectively
//     all-or-nothing (UPLOADS_RECORDINGS_PUBLIC=true to make playback
//     work, leaking everything).
//   - This endpoint reuses the regular tenant RBAC stack
//     (api.sip.calls.read), scopes the lookup by tenant_id, and streams
//     bytes from the storage backend. The frontend can include the
//     existing tenant JWT in fetch() / XHR / MediaSource requests
//     instead of relying on cookies.
//
// Behaviour:
//   - GET  /sip-center/calls/:id/recording        → stream the WAV
//   - HEAD /sip-center/calls/:id/recording        → metadata only
//
// Storage backend compatibility (pkg/stores):
//   - local       — Read returns *os.File (io.ReadSeeker), so
//                   http.ServeContent handles Range / 206 / If-Modified-Since.
//   - minio       — Read returns *minio.Object which implements
//                   io.ReadSeeker; also goes through ServeContent and
//                   gets server-side Range fan-out.
//   - s3 / cos / oss / qiniu — Read returns a non-seekable
//                   io.ReadCloser; we fall through to a single full-body
//                   stream with Accept-Ranges: none. The current frontend
//                   buffers the whole WAV into a Blob anyway, so seek
//                   inside <audio> still works post-load. A future
//                   improvement is presigned-URL redirect for these
//                   backends so we don't bounce bytes through the
//                   server; that requires extending the Store interface.
//
// Authorization:
//   - Caller must already have passed RequireTenantPermissionAll
//     ("api.sip.calls.read") via the parent route group.
//   - Platform admins see every tenant's call. Tenant users are
//     restricted to their own tenant_id, identical to getSIPCall.

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/LinByte/VoiceServer/pkg/logger"
	"github.com/LinByte/VoiceServer/pkg/middleware"
	"github.com/LinByte/VoiceServer/pkg/response"
	"github.com/LinByte/VoiceServer/pkg/sip/persist"
	"github.com/LinByte/VoiceServer/pkg/stores"
	"github.com/LinByte/VoiceServer/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// streamSIPCallRecording streams the WAV (or raw SN*) recording for a
// single call. It is intentionally kept under the same RBAC scope as
// the call detail endpoint so granting api.sip.calls.read is sufficient.
func (h *Handlers) streamSIPCallRecording(c *gin.Context) {
	id, idErr := utils.ParseID(c.Param("id"))
	if idErr != nil {
		response.Fail(c, "invalid id", nil)
		return
	}

	var (
		row persist.SIPCall
		err error
	)
	if middleware.AuthPlatformAdminID(c) > 0 {
		row, err = persist.GetActiveSIPCallByID(h.db, id)
	} else {
		tid := middleware.CurrentTenantID(c)
		row, err = persist.GetActiveSIPCallForTenant(h.db, id, tid)
	}
	if err != nil {
		response.Fail(c, "not found", nil)
		return
	}

	recURL := strings.TrimSpace(row.RecordingURL)
	if recURL == "" {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": 404, "msg": "recording not available", "data": nil})
		return
	}

	key := recordingStorageKeyFromURL(recURL)
	if key == "" {
		// We could not derive a storage key (e.g. recording_url points
		// to an unrelated origin). Refuse rather than blindly proxying.
		logger.Lg.Warn("sip call recording: cannot derive storage key",
			zap.Uint("call_id_pk", row.ID),
			zap.String("recording_url", recURL),
		)
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": 404, "msg": "recording key not resolvable", "data": nil})
		return
	}

	store := stores.Default()
	if store == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "storage backend unavailable", "data": nil})
		return
	}

	rc, size, err := store.Read(key)
	if err != nil {
		// Backends report "not found" with backend-specific error types
		// (os.ErrNotExist for local, MinIO/S3 NoSuchKey, etc.). We fold
		// the obvious local case into a clean 404 and leave the rest as
		// 500 with a logged backend kind so ops can distinguish bucket
		// misconfiguration from genuinely missing recordings.
		if errors.Is(err, os.ErrNotExist) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": 404, "msg": "recording file missing", "data": nil})
			return
		}
		logger.Lg.Warn("sip call recording: storage read failed",
			zap.Uint("call_id_pk", row.ID),
			zap.String("key", key),
			zap.String("storage_kind", stores.DefaultStoreKind),
			zap.Error(err),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "recording read failed", "data": nil})
		return
	}
	defer rc.Close()

	contentType := recordingContentTypeForKey(key)
	c.Header("Content-Type", contentType)
	// Avoid caching across users; recording bytes are tenant-private.
	c.Header("Cache-Control", "private, max-age=0, must-revalidate")
	// inline so <audio> can play directly; download UI can override
	// via ?download=1 if desired.
	disp := "inline"
	if strings.EqualFold(strings.TrimSpace(c.Query("download")), "1") {
		disp = "attachment"
	}
	c.Header("Content-Disposition", disp+"; filename=\""+path.Base(key)+"\"")

	// Fast path: seekable readers (local *os.File, MinIO *minio.Object)
	// get proper Range / If-Modified-Since handling via http.ServeContent.
	// ServeContent itself emits Accept-Ranges: bytes when it serves Ranges.
	if seeker, ok := rc.(io.ReadSeeker); ok {
		modTime := time.Time{}
		if f, ok := rc.(*os.File); ok {
			if st, statErr := f.Stat(); statErr == nil {
				modTime = st.ModTime()
			}
		}
		http.ServeContent(c.Writer, c.Request, path.Base(key), modTime, seeker)
		return
	}

	// Non-seekable backend (S3 / COS / OSS / Qiniu): full-body stream,
	// no Range support. We only set Content-Length when the backend
	// reported a positive size; otherwise let the http stack pick
	// chunked transfer encoding.
	if size > 0 {
		c.Header("Content-Length", strconv.FormatInt(size, 10))
	}
	c.Header("Accept-Ranges", "none")
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return
	}
	if _, copyErr := io.Copy(c.Writer, rc); copyErr != nil {
		logger.Lg.Warn("sip call recording: stream copy failed",
			zap.Uint("call_id_pk", row.ID),
			zap.String("key", key),
			zap.String("storage_kind", stores.DefaultStoreKind),
			zap.Error(copyErr),
		)
	}
}

// recordingStorageKeyFromURL converts a stored recording_url back into
// a storage-backend key (e.g. "sip/recordings/<callID>_<ts>.wav").
//
// Inputs we accept:
//   - "/uploads/sip/recordings/foo.wav"                (local store)
//   - "https://cdn.example.com/sip/recordings/foo.wav" (cloud public)
//   - "https://api.example.com/media/sip/recordings/…" (STORAGE_PUBLIC_BASE_URL)
//   - "sip/recordings/foo.wav"                         (already a key)
//
// We do NOT trust arbitrary path segments — the result MUST start with
// "sip/recordings/" or "sip/" to keep tenants from probing other
// namespaces (e.g. avatars/, trunk-audio/).
func recordingStorageKeyFromURL(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return ""
	}
	// Absolute URL — keep the path component only.
	if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		s = u.Path
	}
	s = strings.TrimPrefix(s, "/")

	// Strip a leading mount prefix so we land on the storage key.
	const recPrefix = "sip/recordings/"
	if i := strings.Index(s, recPrefix); i >= 0 {
		s = s[i:]
	} else {
		return ""
	}
	// Defence-in-depth: forbid traversal even though storage backends
	// already reject it.
	if strings.Contains(s, "..") {
		return ""
	}
	return s
}

func recordingContentTypeForKey(key string) string {
	lower := strings.ToLower(key)
	switch {
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".ogg"), strings.HasSuffix(lower, ".opus"):
		return "audio/ogg"
	case strings.HasSuffix(lower, ".sn1"), strings.HasSuffix(lower, ".sn2"), strings.HasSuffix(lower, ".sn3"):
		// Tagged raw recordings — opaque binary.
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}
