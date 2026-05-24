// Copyright (c) 2026 LinByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

// Package cascaded is the phase-3 sketch of a native engine.Engine
// implementation for the cascaded (ASR + LLM + TTS) dialog mode.
//
// Phase status:
//
//   - PR-8c (this PR): SKETCH ONLY. Stages are passthrough stubs that
//     log on entry/exit. The engine wires MediaPort PCM input through
//     a pipeline.Pipeline, emits one synthetic AI text turn back, and
//     returns clean. No real ASR / LLM / TTS providers are called.
//
//     The skeleton proves three things:
//       1. engine.Engine can be implemented natively (no legacy bridge).
//       2. pipeline.Pipeline composes Stage goroutines correctly under
//          a real MediaPort.
//       3. Detach is idempotent and races cleanly with engine-side
//          context cancellation.
//
//   - Future PRs replace the stub stages with real providers
//     (recognizer.NewASRClient, llm.NewLLMProvider,
//     synthesizer.NewStreamingFromCredential) one at a time, each
//     hidden behind a feature flag and exercised in staging before
//     promotion.
//
// IMPORTANT: This package does NOT auto-register itself with
// engine.Register from init(). Production traffic continues to flow
// through the legacy bridge (pkg/sip/conversation/dialog_engine_bridge.go).
// To exercise the native engine, call cascaded.RegisterForTesting in
// test setup or behind a feature flag — explicit opt-in keeps phase-1
// stability while phase-3 matures.
//
// Boundary discipline:
//
//   - Imports only pkg/dialog/{engine,pipeline,tenantcfg}. Never
//     imports pkg/sip/* (would re-introduce the cycle this whole
//     refactor was designed to break).
//   - Logging is via engine.Logger. No zap dependency in this
//     package.
package cascaded
