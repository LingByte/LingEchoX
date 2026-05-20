package logger

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// reqIDPrefixCore makes the request id a first-class part of every log
// line so operators can grep one token to retrieve the entire request
// chain.
//
// Why a Core wrapper instead of an encoder hack:
//   - We want the prefix on BOTH the colourised console line AND the
//     persistent JSON file — those go through two different encoders.
//     A Core wrapper sits above the encoder split, so we touch the
//     Entry / fields once and both downstream encoders see the result.
//   - With(...) carries fields per logger instance; a naive encoder
//     wrapper would only see per-call fields. The Core wrapper hooks
//     With as well, remembering the last x-reqid seen up the chain.
//
// Output shape after wrapping (console, dev mode):
//
//   2026-05-20 22:39:48.915 [INFO] middleware/logger.go:75 [reqid:87fe…] http request {status: 200, ...}
//
// And for the persisted JSON file the field is folded into the message:
//
//   {"level":"INFO","time":"...","msg":"[reqid:87fe…] http request","status":200,...}
//
// Online grep: `grep 'reqid:87fe…' app.log` returns every log line of
// that request — middleware access log, downstream service calls,
// errors, anything emitted via FromGin/FromCtx during the request.

import (
	"go.uber.org/zap/zapcore"
)

// reqIDFieldKeys are the field keys that carry the per-request id.
// Kept in sync with HeaderXReqID / GinCtxReqIDKey defined in reqid.go.
// "reqid" is accepted as a short alias if callers ever start emitting it
// — we strip both to avoid double printing.
var reqIDFieldKeys = map[string]struct{}{
	"x-reqid": {},
	"reqid":   {},
}

type reqIDPrefixCore struct {
	zapcore.Core
	reqid string // accumulated via With(...)
}

// WrapCoreWithReqIDPrefix returns a Core that strips request-id fields
// and prepends "[reqid:<value>] " to each entry's message.
// inner must be non-nil; nil short-circuits to inner to avoid hiding
// configuration mistakes.
func WrapCoreWithReqIDPrefix(inner zapcore.Core) zapcore.Core {
	if inner == nil {
		return inner
	}
	return &reqIDPrefixCore{Core: inner}
}

// With extracts the latest x-reqid from `fields` (if any), keeps it on
// the returned core, and forwards the remaining fields to the inner
// core. The previously accumulated reqid is preserved when the With
// batch carries none, so nested With(...) calls don't drop it.
func (c *reqIDPrefixCore) With(fields []zapcore.Field) zapcore.Core {
	reqid, filtered := extractReqID(fields, c.reqid)
	return &reqIDPrefixCore{
		Core:  c.Core.With(filtered),
		reqid: reqid,
	}
}

// Check must add this wrapping core (not the inner one) so Write
// observes per-call fields. Without overriding Check, zap would route
// writes straight to the inner core and our message rewriting would be
// skipped.
func (c *reqIDPrefixCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write strips x-reqid from per-call fields, falls back to the
// accumulated value from With(...), and rewrites the message prefix.
func (c *reqIDPrefixCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	reqid, filtered := extractReqID(fields, c.reqid)
	if reqid != "" {
		ent.Message = "[reqid:" + reqid + "] " + ent.Message
	}
	return c.Core.Write(ent, filtered)
}

func (c *reqIDPrefixCore) Sync() error { return c.Core.Sync() }

// extractReqID scans fields for any request-id key and returns
//   - the latest value (or fallback when none found),
//   - the field slice with reqid entries removed.
//
// Returns fields unchanged (same underlying array) when nothing matches
// to avoid an allocation on the hot path.
func extractReqID(fields []zapcore.Field, fallback string) (string, []zapcore.Field) {
	hit := false
	for _, f := range fields {
		if _, ok := reqIDFieldKeys[f.Key]; ok {
			hit = true
			break
		}
	}
	if !hit {
		return fallback, fields
	}
	reqid := fallback
	filtered := make([]zapcore.Field, 0, len(fields))
	for _, f := range fields {
		if _, ok := reqIDFieldKeys[f.Key]; ok {
			if f.Type == zapcore.StringType && f.String != "" {
				reqid = f.String
			}
			continue
		}
		filtered = append(filtered, f)
	}
	return reqid, filtered
}
