package conversation

// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0
//
// PR-8a — tenant voice config shim.
//
// All the parsing, predicate, and lookup logic that used to live in
// this file has moved to pkg/dialog/tenantcfg so phase-3 native engines
// can consume it without an import cycle. This file is now a thin
// shim that:
//
//   1. Re-exports the public names via type aliases (TenantVoiceJSONLoader
//      = tenantcfg.JSONLoader) so existing call sites keep compiling.
//   2. Forwards SetTenantVoiceJSONLoader / ResolveTenantVoiceEnv /
//      VoiceEnvFromTenantJSON / TenantVoiceReady / TenantRealtimeReady /
//      pipelineCredsUsable to their tenantcfg equivalents.
//   3. Keeps the SIP-dependent attach/playback helpers
//      (AttachInboundNotBoundPlayback, attachTenantConfigErrorPlayback,
//      playLocalAnnounceThenHangup) here because they depend on
//      *sipSession.CallSession and the media subsystem.
//   4. Keeps small JSON helpers (intFromMap) that are still consumed
//      by other files in the conversation package
//      (transfer_confirm.go) and not part of the tenantcfg public API.
//   5. Keeps voiceEnvLLMReady (consumed by voicedialog_loopback.go) as
//      a local helper delegating to the package-internal logic.

import (
	"context"
	"fmt"
	"strings"

	"github.com/LinByte/VoiceServer/pkg/dialog/tenantcfg"
	sipSession "github.com/LinByte/VoiceServer/pkg/sip/session"
	"go.uber.org/zap"
)

// TenantVoiceJSONLoader is an alias for tenantcfg.JSONLoader.
type TenantVoiceJSONLoader = tenantcfg.JSONLoader

// SetTenantVoiceJSONLoader installs DB-backed tenant AI JSON loading
// (required for tenant-scoped voice). Delegates to tenantcfg.SetLoader.
func SetTenantVoiceJSONLoader(fn TenantVoiceJSONLoader) {
	tenantcfg.SetLoader(fn)
}

// VoiceEnvFromTenantJSON builds VoiceEnv from tenant JSON blobs.
// Delegates to tenantcfg.VoiceEnvFromJSON.
func VoiceEnvFromTenantJSON(asrRaw, ttsRaw, llmRaw, realtimeRaw []byte, voiceMode string) (VoiceEnv, error) {
	return tenantcfg.VoiceEnvFromJSON(asrRaw, ttsRaw, llmRaw, realtimeRaw, voiceMode)
}

// ResolveTenantVoiceEnv loads tenant JSON and merges into VoiceEnv.
// Delegates to tenantcfg.Resolve after extracting tenant ID.
func ResolveTenantVoiceEnv(ctx context.Context, cs *sipSession.CallSession) (VoiceEnv, bool, error) {
	if cs == nil {
		return VoiceEnv{}, false, nil
	}
	return tenantcfg.Resolve(ctx, cs.TenantID())
}

// TenantVoiceReady reports whether env has the minimum fields for the
// configured voice attach path. Delegates to tenantcfg.VoiceReady.
func TenantVoiceReady(env VoiceEnv) bool { return tenantcfg.VoiceReady(env) }

// TenantRealtimeReady is the realtime-mode credential gate.
// Delegates to tenantcfg.RealtimeReady.
func TenantRealtimeReady(env VoiceEnv) bool { return tenantcfg.RealtimeReady(env) }

// pipelineCredsUsable is the cascaded-mode credential gate.
// Delegates to tenantcfg.PipelineUsable.
func pipelineCredsUsable(env VoiceEnv) bool { return tenantcfg.PipelineUsable(env) }

// voiceEnvLLMReady is the local LLM-readiness check consumed by
// voicedialog_loopback.go. Kept here (rather than re-exported from
// tenantcfg) because its primary consumer is the loopback path and
// the function is too small to justify re-importing tenantcfg from
// every voicedialog helper.
func voiceEnvLLMReady(e VoiceEnv) bool {
	if strings.EqualFold(e.LLMProvider, "alibaba") {
		return e.LLMAPIKey != "" && e.LLMAppID != ""
	}
	if strings.EqualFold(e.LLMProvider, "coze") {
		return e.LLMAPIKey != "" && e.LLMBaseURL != ""
	}
	if strings.EqualFold(e.LLMProvider, "ollama") {
		return strings.TrimSpace(e.LLMBaseURL) != ""
	}
	return e.LLMAPIKey != ""
}

// intFromMap is the local int-from-map helper consumed by
// transfer_confirm.go. Same body as tenantcfg's private version;
// kept here to avoid a re-export of the tenantcfg helper just for
// one in-package caller.
func intFromMap(m map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s == "" {
				continue
			}
			var n int
			_, _ = fmt.Sscanf(s, "%d", &n)
			if n > 0 {
				return n
			}
		case float64:
			return int(t)
		}
	}
	return 0
}

// ----- SIP-dependent attach/playback helpers (kept in conversation) -

// AttachInboundNotBoundPlayback plays scripts/not_bind.wav then server BYE.
func AttachInboundNotBoundPlayback(ctx context.Context, cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	return cs.AttachVoiceConversation(func() error {
		return playLocalAnnounceThenHangup(cs, "scripts/not_bind.wav", lg)
	})
}

// AttachInboundTenantAIConfigErrorPlayback plays scripts/config_error.wav.
func AttachInboundTenantAIConfigErrorPlayback(cs *sipSession.CallSession, lg *zap.Logger) error {
	return attachTenantConfigErrorPlayback(cs, lg)
}

// attachTenantConfigErrorPlayback registers playback of scripts/config_error.wav then hangup.
func attachTenantConfigErrorPlayback(cs *sipSession.CallSession, lg *zap.Logger) error {
	if cs == nil {
		return nil
	}
	if lg == nil {
		lg = zap.NewNop()
	}
	return cs.AttachVoiceConversation(func() error {
		return playLocalAnnounceThenHangup(cs, "scripts/config_error.wav", lg)
	})
}

func playLocalAnnounceThenHangup(cs *sipSession.CallSession, wavPath string, lg *zap.Logger) error {
	cs.StartOnACK()
	pcmSR := cs.PCMSampleRate()
	if pcmSR <= 0 {
		pcmSR = 8000
	}
	pcm, err := LoadWAVAsPCM16Mono(wavPath, pcmSR)
	if err != nil {
		lg.Warn("sip announce wav load failed", zap.String("call_id", cs.CallID), zap.String("path", wavPath), zap.Error(err))
		RequestSIPHangup(cs.CallID)
		return err
	}
	ms := cs.MediaSession()
	if ms == nil {
		RequestSIPHangup(cs.CallID)
		return fmt.Errorf("nil media session")
	}
	playCtx := ms.GetContext()
	if playCtx == nil {
		playCtx = context.Background()
	}
	_ = playWelcomePCM(playCtx, pcm, ms, lg, pcmSR, cs.WriteAIPCM)
	RequestSIPHangup(cs.CallID)
	return nil
}
