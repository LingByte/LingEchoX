# Dialog Engine Migration — Progress & Roadmap

> Working document tracking the multi-PR refactor that lifts `pkg/sip/conversation`'s monolithic voice-attach plumbing into a transport-agnostic `pkg/dialog` engine abstraction. Update this file alongside each PR.

---

## TL;DR

**Goal**: Make voice-attach mode-honest, transport-agnostic, and pluggable so phase-3 native engines (real cascaded, real realtime, future multimodal) can replace the legacy `AttachVoicePipeline` path one mode at a time **without** disturbing the SIP layer.

**Current state (after PR-7)**:

- The SIP OnACK callback in `internal/sipserver/sipapp.go` no longer calls `AttachVoicePipeline` directly. It calls `conversation.AttachVoiceViaEngine`, which resolves mode → `engine.New(cfg).Attach(...)` → per-mode legacy attacher → `attachVoiceInner` / `attachRealtimeVoiceInner`.
- `engine.Mode` is **load-bearing**: cascaded and realtime are two distinct attachers registered under two distinct modes. The "pipeline-with-unusable-creds-falls-to-realtime" auto-fallback still happens, but explicitly in `ResolveAttachMode` before `engine.New`, not silently inside the dispatcher.
- Behaviour at runtime is unchanged versus pre-refactor. The seam is in place; the underlying code still does the same thing.

---

## PR History

| PR | Commit | Subject | What landed |
|----|--------|---------|-------------|
| PR-1 | `fc38ab6` | Interface scaffolding | Initial `pkg/dialog` skeleton: `engine.Engine`, `engine.Mode`, `engine.MediaPort`, `engine.Config`, `engine.Detach`. Provider interface stubs. |
| PR-1.1 | `4d594f5` | Polish | `engine.NopLogger`, `Registry.Names()` sorted output, provider registry tests. |
| PR-2 | `a460573` | Pipeline package | `pkg/dialog/pipeline`: `Frame`, `Stage`, `Pipeline` chain-of-responsibility runtime. Building block for future native engines. |
| PR-3 | `2240ef3` | Legacy bridge | `pkg/dialog/legacy`: `legacy.Engine`, `legacy.Factory`, `legacy.Attacher`, `LegacyHandle` escape hatch. Package-level `SetAttacher` / `Register` registry. `engine.ResetRegistryForTest`. |
| PR-4 | `c8ba03d` | Bridge → SIP wiring | `WireDialogEngineBridge` in `pkg/sip/conversation` installs a closure that captures `AttachVoicePipeline` and registers under both `cascaded` + `realtime`. `NewZapEngineLogger` adapter. Bootstrap from `internal/sipserver/sipapp.go`. OnACK path **still direct**. |
| PR-5 | (merged with PR-4) | — | (folded into PR-4) |
| PR-6 | `eebb632` | OnACK seam flip | `CallSessionPort` (implements `engine.MediaPort` + `legacy.LegacyHandle`). `AttachVoiceViaEngine` helper. **OnACK now flows through `engine.New(cfg).Attach`.** Behaviour-neutral. |
| PR-7 | `c1a5527` | Per-mode attachers | `AttachCascadedLegacy` + `AttachRealtimeLegacy` replace the single shared closure. `ResolveAttachMode` decides mode up-front. `cfg.Mode` becomes load-bearing. `perModeLegacyAttacher` factory in bridge. |
| PR-8d | `336caec` | Retire AttachVoicePipeline body | `voicedialog/hub.go` switched from `AttachVoicePipeline` to `AttachRealtimeLegacy`. `AttachVoicePipeline` body shrunk from ~70 LOC to a 5-line wrapper that delegates to `ResolveAttachMode` + per-mode helpers. Single source of truth for attach logic now lives in `dialog_engine_legacy.go`. |
| PR-8b | `70160c3` | Mode-tagged metrics | New `pkg/sip/metrics/voice_attach.go`: `sip_voice_attach_total{mode,result}` + `sip_voice_attach_mode_fallback_total{from,to}`. Wired into `AttachVoiceViaEngine` (per-call result) and `ResolveAttachMode` (auto-fallback). PR-7's mode-honesty is now observable. Zero allocation on hot path. |
| PR-8a | `7901e00` | Extract tenant voice config | New `pkg/dialog/tenantcfg` package owns `VoiceEnv`, `JSONLoader`, `Resolve`, `VoiceReady`/`PipelineUsable`/`RealtimeReady`, `VoiceEnvFromJSON`. `pkg/sip/conversation` becomes consumer-only via type aliases (`type VoiceEnv = tenantcfg.VoiceEnv`) — every existing call site keeps compiling. `voice_tenant_loader.go` shrinks from 526 LOC to a ~180 LOC SIP-side shim (loader-forwarding + playback helpers that depend on `*CallSession`). 96.2% coverage on the new package. Unblocks PR-8c (native engines can now read tenant config without an import cycle through `pkg/sip`). |
| PR-8c | `6e742ef` | Native cascaded engine sketch | New `pkg/dialog/cascaded` package: full `engine.Engine` implementation with ASR→LLM→TTS stage pipeline. Stages are passthrough stubs that prove the seam end-to-end. Engine lifecycle is production-shaped: single-shot Attach, idempotent Detach, ctx-cancel teardown, deterministic input/output bridges between `MediaPort` and `pipeline.Pipeline`. Does NOT auto-register — `RegisterForTesting()` is opt-in so the legacy bridge keeps owning production traffic. 88.2% coverage, race-clean. |
| PR-9e | `37cc39e` | Dedupe tenant env load | New `tenantcfg.WithVoiceEnv` / `VoiceEnvFromContext`: context-scoped carrier for the freshly-loaded `VoiceEnv`. New `ResolveAttachModeWithEnv` stashes the env on the returned context. `loadVoiceEnvOrConfigError` fast-paths on the ctx, skipping the DB load when a previous step in the same OnACK call chain already loaded. `ResolveAttachMode` becomes a thin wrapper. Result: **one DB hit per call instead of two** on the OnACK path. Legacy callers (`AttachVoicePipeline` from `voicedialog/hub.go`) keep their existing single-load behaviour — no fast-path entry there. Resolves Open Issue #1. |
| PR-9a | `18fcbd0` | Streaming MediaPort | New `StreamingCallSessionPort` in `pkg/sip/conversation/dialog_engine_streaming_port.go`. Real bi-directional PCM: inbound caller audio via a registered `media.PacketProcessor` pushing into a buffered `<-chan engine.PCMFrame`; outbound TTS via `MediaSession.SendToOutput` with automatic resample when `frame.SampleRate ≠ bridgeRate`. Lifecycle: input channel closes on `MediaSession` ctx cancel OR explicit `Close()`; `SendOutputPCM` returns `ErrPortClosed` thereafter. Frame-copy on inbound (avoid aliasing the decode-stage buffer). Overflow policy: drop-oldest non-blocking send so a stalled engine reader can't back-pressure the RTP path. Stereo recorder symmetry preserved via `cs.WriteAIPCM`. **Does NOT auto-replace `CallSessionPort`** — the OnACK seam still wires the legacy bridge port. Native engines (e.g. `pkg/dialog/cascaded`) opt in by constructing this port directly. 15 tests, race-clean. Closes Open Issue #5 from the streaming-shape side; native-engine production wiring is a follow-up. |
| PR-9b | `237dd9e` | VAD pipeline.Stage | New `pkg/dialog/cascaded/vad_stage.go`. Moves barge-in detection out of the legacy SIP attach helpers and into a `pipeline.Stage` so the native cascaded engine inherits it. Stage observes incoming `KindPCM` frames, queries an injected `BargeInDetector` interface (kept minimal so cascaded does NOT import `pkg/sip/vad` — the production wiring will adapt `*sipvad.Detector` at the seam), and on positive detection during TTS playback emits a `KindBargeIn` control frame downstream + invokes a configured handler. PCM is always passed through unchanged so ASR keeps seeing the user's audio. `cascaded.Engine` grew variadic `Option`s (`WithVADDetector`, `WithBargeInHandler`) plus an internal `ttsPlaying atomic.Bool` the engine maintains by inspecting the pipeline output stream (KindPCM out → playing=true; KindAITextDone → playing=false). The VAD stage's `isTTSPlaying` predicate reads the bit. Streaming port renamed `fireBargeIn` → `TriggerBargeIn` (now public) so production wiring can route stage callback → port drain. 13 stage-level tests + 3 engine-integration tests, race-clean. |
| PR-9c | `db75d68` | Real ASR/LLM/TTS pipeline stages | New `pkg/dialog/cascaded/{asr,llm,tts}_stage.go` replace the stub stages with real `pipeline.Stage` implementations driven by slim provider-neutral interfaces (`ASRRecognizer` / `LLMService` / `TTSService`). Engine grew `WithASRRecognizer` / `WithLLMService` / `WithTTSService` opt-in options; when an option is unset the stage falls back to the existing stub so the engine remains drop-in compatible. Highlights: ASR stage forwards PCM unchanged + emits transcripts via callback; LLM stage serialises turns on `KindTextFinal`, deduplicates terminals, and guarantees `KindAITextDone` even on empty/error replies; TTS stage buffers AI-text deltas, flushes on sentence boundaries, cancels in-flight Speak on barge-in, and **serialises** Speak/Finalize through a single worker goroutine so early-flush and end-of-turn can't race (real production bug surfaced by the early-flush test). Provider wiring (production-side adapters) is a follow-up; the native path still ships with stubs until then. |
| PR-9d | `12cf556` | Feature-flag native cascaded routing | Adds `engine.ModeCascadedNative` and registers `cascaded.factory{}` under it via `cascaded.RegisterNative()`, called from `WireDialogEngineBridge`. The legacy bridge keeps owning `ModeCascaded`. New file `pkg/sip/conversation/dialog_engine_native_route.go` houses the per-tenant routing predicate `useNativeCascaded(tenantID)`, gated by two env knobs: `DIALOG_NATIVE_CASCADED_ALL` (global override) and `DIALOG_NATIVE_CASCADED_TENANTS` (CSV/whitespace-separated allow-list). `AttachVoiceViaEngine` consults the predicate after `ResolveAttachModeWithEnv` and, for opted-in tenants in cascaded mode, calls `attachVoiceViaNativeCascaded` which constructs a `StreamingCallSessionPort`, builds `engine.New(ModeCascadedNative)`, and runs `eng.Attach` inside `cs.AttachVoiceConversation`. New metric `sip_voice_attach_native_total{result=ok\|err}` for opt-in rollout monitoring. `cascaded.Engine.Mode()` now returns `cfg.Mode` (Cascaded or CascadedNative) so logs/metrics distinguish the two registration shapes. **No production fall-back to legacy on engine error** — operators flip the flag off to recover. The native path still uses stub stages today (real provider adapters land in PR-9e); the feature flag ships in a separate PR so the routing scaffold can soak in canary clusters before any audio-affecting change. 9 predicate tests, race-clean. |
| PR-9f | _(this PR)_ | Production provider adapters for native cascaded | New `pkg/sip/conversation/dialog_engine_native_providers.go` carries the impedance-matching adapters between existing concrete providers and the slim cascaded interfaces. `sipasr.Pipeline` already satisfies `cascaded.ASRRecognizer` directly (interface was carved to match); `buildNativeCascadedASR` just constructs it from `VoiceEnv.ASR{AppID,SecretID,SecretKey,ModelType}` (QCloud) and selects 8k/16k by model name. `nativeCascadedLLM` wraps `llm.LLMProvider.QueryStream` into `cascaded.LLMService.StreamReply`, layering ctx-cancel guards on top of the provider's construction-time ctx so the stage can pre-empt cleanly. `nativeCascadedTTS` adapts `*siptts.Pipeline` by stashing the per-Speak `onPCM` in `atomic.Pointer[func]` — `siptts`'s construction-time `SendPCMFrame` reads it on every frame so we get a per-call onPCM without changing the underlying TTS engine. PaceRealtime is OFF on the native path (the engine's outbound bridge owns RTP-time pacing via MediaPort, no double-pacing). `attachVoiceViaNativeCascaded` now: loads VoiceEnv (fast-path via ctx), validates `pipelineCredsUsable`, builds all three adapters, and switches from `engine.New(...)` to `cascaded.New(cfg, WithASR/WithLLM/WithTTS)` so the per-call provider Options can flow in (the registry path stays for callers without injection). Failure of any adapter / pipelineCreds bumps the err metric and (for tenant-misconfig cases) plays `config_error.wav` instead of dropping the call silently. **Native-routed traffic now produces real audio** subject to the feature flag — soak in canary, then flip more tenants in. Race-clean; full `go test ./...` green. |

---

## Architecture (post-PR-7)

```
                  ┌─────────────────────────────────────┐
                  │  internal/sipserver/sipapp.go       │
                  │  MediaAttach callback (on ACK)      │
                  └────────────────┬────────────────────┘
                                   │ AttachVoiceViaEngine(ctx, cs, lg)
                                   ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  pkg/sip/conversation/dialog_engine_attach.go                │
   │  AttachVoiceViaEngine                                         │
   │    1. NewCallSessionPort(cs)            ← MediaPort + Handle  │
   │    2. ResolveAttachMode(ctx, cs, lg)    ← decides mode        │
   │    3. engine.New(cfg)                                         │
   │    4. engine.Attach(ctx, port, lg)                            │
   └────────────────┬──────────────────────────────────────────────┘
                    │
                    ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  pkg/dialog/engine                                            │
   │  Registry (cascaded → factory, realtime → factory)            │
   └────────────────┬──────────────────────────────────────────────┘
                    │
                    ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  pkg/dialog/legacy                                            │
   │  legacy.Engine.Attach → SetAttacher(mode) → per-mode attacher │
   └────────────────┬──────────────────────────────────────────────┘
                    │ perModeLegacyAttacher(mode, fn) — bridge closure
                    │  - extract *CallSession via LegacyHandle
                    │  - adapt engine.Logger → *zap.Logger
                    │  - warn on cfg.Mode/registered mode mismatch
                    ▼
   ┌─────────────────────────────────┐    ┌────────────────────────────────┐
   │  AttachCascadedLegacy           │    │  AttachRealtimeLegacy          │
   │   - loadVoiceEnvOrConfigError   │    │   - loadVoiceEnvOrConfigError  │
   │   - require pipelineCredsUsable │    │   - require TenantRealtimeReady│
   │   - env.VoiceMode = "pipeline"  │    │   - env.VoiceMode = "realtime" │
   │   - attachVoiceInner            │    │   - attachVoiceInner           │
   └─────────────────────────────────┘    └────────────────────────────────┘
                    │                                      │
                    └──────────────┬───────────────────────┘
                                   ▼
              attachVoiceInner / attachRealtimeVoiceInner
              (unchanged legacy code in voice.go / voice_realtime.go)
```

---

## Test Coverage (new + changed code, after PR-7)

| File | Coverage | Notes |
|------|---------:|-------|
| `dialog_engine_port.go` | 100% | `OnBargeIn` is no-op; tracked but no exec stmts |
| `dialog_engine_attach.go` | 85.7% | One defensive branch (`NewCallSessionPort` returning nil after we already nil-checked `cs`) |
| `dialog_engine_bridge.go` (per-mode) | 100% | Closure body fully covered |
| `dialog_engine_bridge.go` (Wire) | 77.8% | Registry error path uncoverable without registry-corrupting mock |
| `dialog_engine_legacy.go::ResolveAttachMode` | 100% | All 5 branches covered |
| `dialog_engine_legacy.go::loadVoiceEnvOrConfigError` | 83.3% | `ResolveTenantVoiceEnv` err path needs DB mock |
| `dialog_engine_legacy.go::ensureVoiceLogger` | 83.3% | `logger.Lg` always set in tests |
| `dialog_engine_legacy.go::AttachCascadedLegacy` | 75% | Happy path needs fully-wired MediaSession (integ test) |
| `dialog_engine_legacy.go::AttachRealtimeLegacy` | 75% | Same |
| `bridgeZapLogger` | 83.3% | `logger.Lg` always set in tests |

`pkg/dialog/{engine,legacy,pipeline,provider}` — 100% on all reachable lines.

---

## Open Issues / Tech Debt

1. ~~**Double env load per call**~~. **Resolved in PR-9e.** `ResolveAttachModeWithEnv` stashes the loaded `VoiceEnv` on the returned context; `loadVoiceEnvOrConfigError` fast-paths via `tenantcfg.VoiceEnvFromContext`. One DB hit per OnACK call instead of two.

2. ~~**`AttachVoicePipeline` still alive in `voice.go`**~~. **Resolved in PR-8d.** `voicedialog/hub.go` migrated to `AttachRealtimeLegacy`; `AttachVoicePipeline` is now a 5-line compatibility wrapper that delegates to the per-mode helpers.

3. ~~**No mode-tagged metrics**~~. **Resolved in PR-8b.** `sip_voice_attach_total{mode,result}` + `sip_voice_attach_mode_fallback_total{from,to}` are emitted from `AttachVoiceViaEngine` and `ResolveAttachMode` respectively.

4. ~~**No native engine yet**~~. **Sketched in PR-8c.** `pkg/dialog/cascaded.Engine` implements `engine.Engine` natively over `pipeline.Pipeline`. Stages are passthrough stubs today; real ASR/LLM/TTS providers swap in via the `Stage` interface in follow-up PRs. Not registered in production — legacy bridge still owns `ModeCascaded` until each stage is hardened.

5. ~~**MediaPort streaming methods are stubs on `CallSessionPort`**~~. **Resolved in PR-9a** (streaming side). `StreamingCallSessionPort` is the production-shape MediaPort: real `<-chan PCMFrame` ingress fed by a `media.PacketProcessor`, real `SendToOutput` egress with auto-resample, ctx-cancel teardown, recorder symmetry. `CallSessionPort` (the legacy-bridge port) stays unchanged for the existing OnACK seam; native engines opt in to the streaming port. Next step: feature-flag the cascaded native engine through `engine.Register` and have one tenant route through it.

---

## PR-8+ Candidates

Choose by priority; they're independent:

### ~~PR-8a~~ — **Done.** `VoiceEnv` + parser + predicates + loader live in `pkg/dialog/tenantcfg`; `pkg/sip/conversation` consumes via type aliases.

### ~~PR-8b~~ — **Done.** Mode-tagged metrics wired via `pkg/sip/metrics/voice_attach.go`.

### ~~PR-8c~~ — **Done.** `pkg/dialog/cascaded` ships a full `engine.Engine` skeleton with passthrough stages, `RegisterForTesting()` opt-in, and an in-process `fakeMediaPort` test rig. Real provider stages are the next follow-up.

### ~~PR-8d~~ — **Done.** Migrated `voicedialog/hub.go` to `AttachRealtimeLegacy`; `AttachVoicePipeline` thinned to a 5-line wrapper.

---

## File Map (post-PR-7)

```
pkg/dialog/
├── engine/
│   ├── engine.go              # Engine, Config, Detach, Mode, MediaPort, Logger, PCMFrame, CodecSpec
│   ├── registry.go            # Mode registry, New(), RegisteredModes()
│   ├── nop_logger.go          # NopLogger
│   └── *_test.go              # 100% covered
├── legacy/
│   ├── engine.go              # legacy.Engine, Factory
│   ├── attacher.go            # Attacher type, SetAttacher, ErrNoLegacySession, LegacyHandle
│   ├── register.go            # legacy.Register(mode)
│   └── *_test.go
├── pipeline/
│   ├── frame.go               # Frame
│   ├── stage.go               # Stage
│   ├── pipeline.go            # Pipeline
│   └── *_test.go
└── provider/
    ├── provider.go            # ASR, LLM, TTS, Multimodal provider interfaces
    └── *_test.go

pkg/sip/conversation/
├── dialog_engine_port.go      # CallSessionPort (MediaPort + LegacyHandle)
├── dialog_engine_attach.go    # AttachVoiceViaEngine + EngineAttachFallbackMode
├── dialog_engine_bridge.go    # WireDialogEngineBridge + perModeLegacyAttacher + zap adapter
├── dialog_engine_legacy.go    # AttachCascadedLegacy + AttachRealtimeLegacy + ResolveAttachMode + helpers
├── voice.go                   # (unchanged) attachVoiceInner, AttachVoicePipeline
├── voice_realtime.go          # (unchanged) attachRealtimeVoiceInner
├── voice_tenant_loader.go     # (will move in PR-8a) VoiceEnv, ResolveTenantVoiceEnv, TenantVoiceReady, pipelineCredsUsable, TenantRealtimeReady, attachTenantConfigErrorPlayback
└── dialog_engine_*_test.go    # New tests for everything above

internal/sipserver/
└── sipapp.go                  # MediaAttach → AttachVoiceViaEngine + WireDialogEngineBridge bootstrap
```

---

## How to Resume This Work

1. Read this file end-to-end.
2. Read the most recent PR commit message (`git log --format=fuller -1`).
3. Pick a candidate from "PR-8+ Candidates" or propose a new one.
4. Branch from `main`, draft the PR with the same level of detail (interface contracts, test coverage targets ≥75% on new code, behavioural notes in the commit message).
5. Update this file in the same PR.

---

_Last updated: PR-7 (commit `c1a5527`)_
