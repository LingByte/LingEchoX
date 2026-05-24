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

1. **Double env load per call**. `ResolveAttachMode` loads tenant env to pick mode, then `AttachCascadedLegacy` / `AttachRealtimeLegacy` reload it. Each load is one DB hit (~10ms behind the loader cache, if any). Will be deduped in PR-8a when `VoiceEnv` moves into `engine.Config`.

2. **`AttachVoicePipeline` still alive in `voice.go`**. It's the entry point for non-OnACK callers (`pkg/sip/voicedialog/hub.go`). Will be retired (or thinned to a wrapper around the per-mode helpers) once that caller is migrated.

3. **No mode-tagged metrics**. The PR-7 mode-honesty work isn't yet observable in dashboards. PR-8b would close this loop.

4. **No native engine yet**. Every engine.Mode still ultimately calls `attachVoiceInner` via the legacy bridge. Phase 3 (PR-8c onward) starts replacing one mode with a native `engine.Engine` implementation.

5. **MediaPort streaming methods are stubs on `CallSessionPort`**. `InputPCM` returns a closed channel; `SendOutputPCM` returns `ErrLegacyBridgeOnly`. Real streaming bridges the underlying `media.MediaSession` ↔ `engine.PCMFrame` channels — phase 3 work.

---

## PR-8+ Candidates

Choose by priority; they're independent:

### PR-8a — Extract VoiceEnv to `pkg/dialog/tenantcfg` (~600 LOC)
- Move `VoiceEnv`, `pipelineCredsUsable`, `TenantRealtimeReady`, `TenantVoiceJSONLoader`, `ResolveTenantVoiceEnv` to a new top-level package.
- Keep `conversation` package focused on attach logic.
- **Unblocks**: phase 3 native engines (they need to read env without depending on `conversation`).
- **Cost**: ~10 import sites updated. One DB-mock helper in `pkg/dialog/tenantcfg` for tests.

### PR-8b — Mode-tagged metrics (~150 LOC)
- Add prometheus counters:
  - `sip_voice_attach_total{mode}` — incremented in `AttachVoiceViaEngine` after `engine.New` succeeds.
  - `sip_voice_attach_failed_total{mode, reason}` — incremented when the per-mode attacher returns config_error.
  - `sip_voice_attach_mode_fallback_total{from, to}` — incremented when `ResolveAttachMode` applies the pipeline→realtime auto-fallback.
- Wire into existing `pkg/metrics` registry.
- **Unblocks**: dashboards / alerts on the PR-7 mode-honesty work.
- **Cost**: small, no behaviour change.

### PR-8c — Native cascaded engine sketch (~400 LOC)
- Create `pkg/dialog/cascaded` package with a minimal `engine.Engine` implementation that uses `pipeline.Pipeline` from PR-2.
- Initially only wires ASR (other stages no-op), demonstrating the seam without touching production traffic. Gated behind a build tag or registry override.
- **Unblocks**: full phase 3. After this lands, follow-up PRs port LLM + TTS stages and the legacy bridge can be retired for cascaded.
- **Cost**: medium. Requires a real `MediaPort` streaming implementation on `CallSessionPort` (or a sibling adapter).

### PR-8d — Migrate `voicedialog/hub.go` to `AttachVoiceViaEngine`
- The only remaining direct caller of `AttachVoicePipeline`.
- After this PR, `AttachVoicePipeline` becomes private (`attachVoicePipelineLegacy`) or is deleted entirely.
- **Cost**: tiny (~30 LOC). Worth doing before PR-8a so the env extraction has fewer call sites.

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
