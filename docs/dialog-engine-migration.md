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
| PR-8c | _(this PR)_ | Native cascaded engine sketch | New `pkg/dialog/cascaded` package: full `engine.Engine` implementation with ASR→LLM→TTS stage pipeline. Stages are passthrough stubs that prove the seam end-to-end. Engine lifecycle is production-shaped: single-shot Attach, idempotent Detach, ctx-cancel teardown, deterministic input/output bridges between `MediaPort` and `pipeline.Pipeline`. Does NOT auto-register — `RegisterForTesting()` is opt-in so the legacy bridge keeps owning production traffic. 88.2% coverage, race-clean. |

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

1. **Double env load per call**. `ResolveAttachMode` loads tenant env to pick mode, then `AttachCascadedLegacy` / `AttachRealtimeLegacy` reload it. Each load is one DB hit (~10ms behind the loader cache, if any). Now feasible to dedupe via `engine.Config.VoiceEnv` in a follow-up since PR-8a moved `VoiceEnv` to `pkg/dialog/tenantcfg` (no import cycle).

2. ~~**`AttachVoicePipeline` still alive in `voice.go`**~~. **Resolved in PR-8d.** `voicedialog/hub.go` migrated to `AttachRealtimeLegacy`; `AttachVoicePipeline` is now a 5-line compatibility wrapper that delegates to the per-mode helpers.

3. ~~**No mode-tagged metrics**~~. **Resolved in PR-8b.** `sip_voice_attach_total{mode,result}` + `sip_voice_attach_mode_fallback_total{from,to}` are emitted from `AttachVoiceViaEngine` and `ResolveAttachMode` respectively.

4. ~~**No native engine yet**~~. **Sketched in PR-8c.** `pkg/dialog/cascaded.Engine` implements `engine.Engine` natively over `pipeline.Pipeline`. Stages are passthrough stubs today; real ASR/LLM/TTS providers swap in via the `Stage` interface in follow-up PRs. Not registered in production — legacy bridge still owns `ModeCascaded` until each stage is hardened.

5. **MediaPort streaming methods are stubs on `CallSessionPort`**. `InputPCM` returns a closed channel; `SendOutputPCM` returns `ErrLegacyBridgeOnly`. The cascaded engine sketch tests against a `fakeMediaPort`; real streaming bridging `media.MediaSession` ↔ `engine.PCMFrame` is the next blocker before the native engine can serve real calls.

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
