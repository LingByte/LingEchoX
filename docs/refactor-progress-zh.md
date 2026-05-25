# 对话引擎重构进度速览（中文）

> 本文档配套 `docs/refactor-architecture.md`（架构 RFC）与
> `docs/dialog-engine-migration.md`（英文 PR 历史），用中文集中说明
> 当前 SIP/对话两层重构改了什么、跑到哪一步、native 通路具备哪些能力，
> 方便对外同步与复盘。

---

## 0. 一句话总结

把原来挤在 `pkg/sip/conversation/voice.go` 里的 ASR / LLM / TTS / VAD /
打断 / 录音 / hotword 修正等逻辑，按"接口 + 流水线"的方式拆进了
`pkg/dialog/...`。**自 PR-9g 起，所有 cascaded 模式的通话默认直接走
新的 `cascaded.Engine`**，老 `attachVoiceInner` 仅在事故应急时通过
`DIALOG_NATIVE_CASCADED_DISABLE` 一键回退保留。Realtime 多模态路径
还在老实现，待后续 phase 同步搬迁。

---

## 1. 目录结构（重构后）

```
pkg/dialog/
├── engine/        # 抽象层：Engine / MediaPort / Mode / Config / Logger
├── legacy/        # 把老 AttachVoicePipeline 包成 engine.Engine 的桥
├── pipeline/      # Frame / Stage / Pipeline 责任链运行时
├── provider/      # ASR/LLM/TTS/Multimodal 三方接口（脱 SIP）
├── tenantcfg/     # 租户语音配置（VoiceEnv / Loader / 上下文缓存）
└── cascaded/      # 原生级联引擎：VAD → ASR → LLM → TTS 五个 Stage

pkg/sip/conversation/
├── dialog_engine_attach.go              # OnACK 入口 AttachVoiceViaEngine
├── dialog_engine_legacy.go              # 老路径的 per-mode 适配器
├── dialog_engine_bridge.go              # 老桥注册 + 装载 native 工厂
├── dialog_engine_streaming_port.go      # 真正的 PCM 双向流 MediaPort
├── dialog_engine_native_route.go        # PR-9d 特性开关 + native 入口
└── dialog_engine_native_providers.go    # PR-9f 真实 provider 适配器
```

---

## 2. PR 时间线

| PR | Commit | 主题 | 价值 |
|-----|--------|------|------|
| PR-1 | `fc38ab6` | 接口骨架 | `pkg/dialog` 第一版抽象 |
| PR-2 | `a460573` | Pipeline 责任链 | 后续 native 引擎的运行时基础 |
| PR-3 | `2240ef3` | Legacy 桥 | 老 attach 套上 `engine.Engine` 接口 |
| PR-4 | `c8ba03d` | SIP 接桥 | OnACK 不再硬连，走注册表 |
| PR-6 | `eebb632` | OnACK seam-flip | 通话真正流经 `engine.New().Attach` |
| PR-7 | `c1a5527` | 拆分 cascaded / realtime attacher | 让 `cfg.Mode` 真正起作用 |
| PR-8a | `7901e00` | 抽 `pkg/dialog/tenantcfg` | 解开 SIP↔tenantcfg 的循环依赖 |
| PR-8b | `70160c3` | 模式打标的 metrics | `sip_voice_attach_total{mode,result}` |
| PR-8c | `6e742ef` | `cascaded.Engine` 骨架（stub stage） | 跑通 attach/detach/取消 全链路 |
| PR-8d | `336caec` | `AttachVoicePipeline` 缩成 5 行 | 单一真理来源 |
| PR-9a | `18fcbd0` | `StreamingCallSessionPort` | 真双工 PCM Port |
| PR-9b | `237dd9e` | VAD pipeline.Stage | 打断从 SIP 移到引擎层 |
| PR-9c | `db75d68` | 真实 ASR/LLM/TTS Stage | stub 替换为 pipeline 实现 |
| PR-9d | `12cf556` | 租户级特性开关（已弃用，见 PR-9g） | 灰度铺设 |
| PR-9e | `37cc39e` | tenant env 去重加载 | 每通电话一次 DB 命中 |
| PR-9f | `7a5dc48` | 真 provider 适配器 | native 通话真出声 |
| PR-9g | `f0c5768` | 全量切到 native + hotword Stage + 录音 | 默认 native，灰度开关翻转成 kill-switch |
| PR-9h | `bb3d10a` | Turn 持久化 Stage | `RecordDialogTurn` 下沉到 pipeline.Stage |
| PR-9i | `e3d42b2` | 转人工流水接通 native | tool 注册 + 后turn 触发 + 转接期间 LLM 抑制 |
| PR-9j | `99878d9` | Native 路径接通延迟直方图（LLM/E2E） | persister 调用 `ObserveLLMFirstByte / E2EFirstByte` |
| PR-9k | `afad91a` | 补齐 `TTSFirstByte` 直方图 | persistStage 跟踪首帧 PCM 时间 |
| PR-10a | _本次_ | `pkg/dialog/realtime` 引擎骨架 | agentStage + 复用 cascaded 的 hotword/persist Stage |

---

## 3. 双模切换（cascaded ⇄ realtime）

```
                       SIP OnACK
                           │
                           ▼
              AttachVoiceViaEngine
                           │
            ResolveAttachModeWithEnv  ← VoiceEnv 落 ctx 缓存
                           │
              ┌────────────┼────────────────────────┐
              │ realtime   │ cascaded               │
              ▼            ▼                        ▼
   AttachRealtimeLegacy  AttachCascadedLegacy   useNativeCascaded? ─┐
                                                                    │ yes
                                                                    ▼
                                                        attachVoiceViaNativeCascaded
                                                                    │
                                                                    ▼
                                                       cascaded.New(cfg, opts...)
                                                                    │
                                                                    ▼
                                                     pipeline: VAD→ASR→LLM→TTS
```

- **`env.VoiceMode = "realtime"`** → 直接走老 realtime 多模态路径，不变。
- **`env.VoiceMode = "pipeline"`** + 凭据可用 → **默认走 native 引擎**
  （`attachVoiceViaNativeCascaded`）。
- 仅当 `DIALOG_NATIVE_CASCADED_DISABLE` 命中时退回 `AttachCascadedLegacy`。

老路径仅作为应急 kill-switch 保留；当 native 稳定一段时间后会在下一个
phase 直接删除 `attachVoiceInner` 主体。

---

## 4. 当前能力清单

### 4.1 抽象层（`pkg/dialog/...`）

| 能力 | 位置 |
|------|------|
| `engine.Engine` / `engine.MediaPort` / `engine.Mode` 接口 | `pkg/dialog/engine` |
| Mode 注册表 + `engine.New(cfg)` | `pkg/dialog/engine/factory.go` |
| `pipeline.Frame` / `pipeline.Stage` / `pipeline.Pipeline` | `pkg/dialog/pipeline` |
| `tenantcfg.VoiceEnv` + Loader + ctx 缓存 | `pkg/dialog/tenantcfg` |
| `legacy.Engine` 把老 attach 包成 Engine | `pkg/dialog/legacy` |

### 4.2 原生级联引擎（`pkg/dialog/cascaded`）

| 能力 | 说明 |
|------|------|
| `cascaded.Engine` | 实现 `engine.Engine`，幂等 Detach / ctx-cancel / 单次 Attach |
| `vadStage` | 监听 `KindPCM`，TTS 播放期间检测人声并下发 `KindBargeIn` |
| `asrStage` | 包 `ASRRecognizer`，PCM 透传 + 文本上抛 |
| `llmStage` | 包 `LLMService`，turn 串行化、新 turn 取消旧 turn、空回复也保证 `KindAITextDone` |
| `ttsStage` | 包 `TTSService`，按句号缓冲 → 早 flush / 末 flush，**单 worker 串行化** Speak/Finalize，barge-in 即取消并清缓存 |
| `hotwordStage` | 包 `TextRewriter`，对 `KindTextInterim/Final` 应用替换；其它帧透传，零开销 |
| 工厂注册 | `cascaded.RegisterNative()` 注册到 `ModeCascadedNative` |
| 选项注入 | `WithASRRecognizer / WithLLMService / WithTTSService / WithTextRewriter / WithVADDetector / WithBargeInHandler` |

### 4.3 SIP 桥（`pkg/sip/conversation`）

| 能力 | 说明 |
|------|------|
| `CallSessionPort` | 老桥用的 Port，简单地把 cs 透传到 legacy attacher |
| `StreamingCallSessionPort` | 真 PCM 双向流：注册 `media.PacketProcessor` 收音、`SendToOutput` 发音、自动重采样、`Close` 幂等、`OnBargeIn` 回调存储、丢最早策略防回压 |
| `useNativeCascaded(tenantID)` | **默认 true**；kill-switch `DIALOG_NATIVE_CASCADED_DISABLE=ALL` 或租户列表可临时回退老路径 |
| `attachVoiceViaNativeCascaded` | 加载 VoiceEnv → 校验凭据 → 启用立体声录音 → 构建 ASR/LLM/TTS 适配器 → 装 hotword corrector → `cascaded.New(...)` → Attach |
| `buildNativeCascadedASR` | 用 QCloud 凭据创建 `sipasr.Pipeline`（接口已对齐，零额外胶水） |
| `nativeCascadedLLM` | 包 `llm.LLMProvider.QueryStream`，叠加 ctx-cancel 守卫 |
| `nativeCascadedTTS` | 用 `atomic.Pointer[func]` 把 per-Speak `onPCM` 注入 siptts 的构造期 `SendPCMFrame`；同时把每帧透传给 `cs.WriteAIPCM`（立体声录音），无需改 TTS 引擎 |
| `enableNativeStereoRecorder` | 读 `SIP_RECORDER_CHUNK_SECS` → `cs.EnableRecorder` |
| `NewSIPHotwordCorrector` | 老的环境变量驱动 hotword 表（`SIP_HOTWORD_CORRECTIONS / _JSON`）现在直接挂在 native 引擎前 |

### 4.4 可观测性（`pkg/sip/metrics`）

| 指标 | 维度 |
|------|------|
| `sip_voice_attach_total` | `mode`(cascaded/realtime), `result`(ok/config_error) |
| `sip_voice_attach_mode_fallback_total` | `from`(pipeline), `to`(realtime) |
| `sip_voice_attach_native_total` | `result`(ok/err) — PR-9d 新增，专门盯 native 灰度 |
| `voice_metrics.ObserveLLMFirstByte / TTSFirstByte / E2EFirstByte` | 单 turn 延迟直方图（仅老路径维护，native 待补） |

### 4.5 配置与开关

| 配置 | 作用 |
|------|------|
| 租户 `voice_mode = "pipeline" \| "realtime"` | 选择 cascaded 或 realtime |
| 租户 `pipelineCredsUsable / RealtimeReady` | 决定能否走对应模式（不行就放 `config_error.wav`） |
| 环境变量 `DIALOG_NATIVE_CASCADED_DISABLE=ALL` | **应急 kill-switch**：把所有 cascaded 通话退回老路径 |
| 环境变量 `DIALOG_NATIVE_CASCADED_DISABLE=tenant-a,tenant-b` | 仅指定租户退回老路径 |
| 环境变量 `DIALOG_NATIVE_CASCADED_ALL / _TENANTS` | 历史灰度变量，已无效（保留命名兼容旧 playbook） |
| 环境变量 `SIP_HOTWORD_CORRECTIONS / _JSON` | ASR 文本修正词表，新老路径都生效 |
| 环境变量 `SIP_RECORDER_CHUNK_SECS` | 立体声录音分片周期 |
| 环境变量 `SIP_VAD_BARGE_IN` | 老路径打断开关（native 路径由 vadStage 自管） |

---

## 5. 测试矩阵

```
go test -race ./pkg/dialog/...                # 抽象层 + cascaded 全绿
go test -race ./pkg/sip/conversation/...      # 桥 + 特性开关 + adapter 全绿
go test ./...                                 # 全仓回归无 FAIL
go vet ./...                                  # 干净
```

主要新增测试：
- `pkg/dialog/cascaded/{vad,asr,llm,tts}_stage_test.go` — Stage 行为 + 集成
- `pkg/dialog/cascaded/engine_test.go` — Engine 生命周期
- `pkg/dialog/tenantcfg/{tenantcfg,context}_test.go` — VoiceEnv 解析 + ctx 缓存
- `pkg/sip/conversation/dialog_engine_streaming_port_test.go` — 真流 Port 15 项
- `pkg/sip/conversation/dialog_engine_native_route_test.go` — 特性开关 9 项
- `pkg/sip/conversation/dialog_engine_bridge_test.go` — 桥注册 + 幂等

---

## 6. 用法快速参考

### 6.1 默认行为

不需要任何配置，所有 `voice_mode=pipeline` 的通话都自动走 native
`cascaded.Engine`，享有：

- VAD-Stage 驱动的打断
- ASR-Stage 文本上抛
- Hotword-Stage 文本修正
- LLM-Stage 流式回复 + turn 序列化 + 新 turn 取消旧 turn
- TTS-Stage 句尾早 flush + 单 worker 串行化 + barge-in 即取消
- 立体声录音

### 6.2 应急回退（仅出事时）

```sh
# 全量回退老路径
DIALOG_NATIVE_CASCADED_DISABLE=ALL

# 仅某些租户回退
DIALOG_NATIVE_CASCADED_DISABLE=tenant-a,tenant-b
```

下一通通话即刻生效，不需要重启 SIP 服务。事故恢复后把变量删掉即可。

### 6.3 已搬过来 / 还没搬过来

**已经搬过来（PR-9g/h/i）**

- ASR/LLM/TTS 真 provider、立体声录音、hotword 修正、Turn 持久化
- 转人工：`transfer_to_agent` function tool 在 LLM 上注册；turn
  结束时检查 pending 标志并触发 `TriggerTransferToAgent`；
  `IsTransferInProgress` 期间 LLM 调用直接短路（避免转接中 AI
  抢话）。

**还没搬过来（仍走 legacy 路径）**

- Realtime 多模态（`env.VoiceMode=realtime`）整条链路。
- Welcome / Auto-prompt 路径上由 realtime 提供的意图识别联动。
- Native 路径的延迟直方图（`LLMFirstByte / TTSFirstByte / E2EFirstByte`
  当前只在 legacy cascaded 内采集）。

### 6.4 监控健康

```promql
# native 路由结果
sum by (result) (rate(sip_voice_attach_native_total[5m]))

# 整体 attach 健康（新老一起）
sum by (mode, result) (rate(sip_voice_attach_total[5m]))
```

---

## 7. 待办（下一阶段路线图）

1. ~~**Transfer Tool Stage**~~ — 已在 PR-9i 接通。
2. ~~**Dialog Turn Persistence Stage**~~ — 已在 PR-9h 接通。
3. ~~**Native 路径的延迟直方图**~~ — 已在 PR-9j 接通 `LLMFirstByte`
   和 `E2EFirstByte`；`TTSFirstByte` 暂留给 legacy（需要新增
   ttsStage 内部"首帧 PCM 时间"事件，下一 PR 处理）。
4. **Intent Detection Stage**：realtime 路径上的意图识别下沉成
   Stage，cascaded 与 realtime 都能复用。
5. **Native realtime 引擎**：`pkg/dialog/realtime` 仍是 stub；
   阶段三末把 realtime 多模态也搬过来，cascaded ⇄ realtime 切换变成
   纯注册表选择。
6. **删除老 `attachVoiceInner` 主体**：native 稳定一段时间后，
   `voice.go` 里 1200 行可以缩成几十行的兼容包装，最后整体删除
   `dialog_engine_legacy.go` 的 cascaded 分支。

---

## 8. 设计原则（写给后人）

- **接口先行**：所有跨边界都先写小接口（5-10 个方法以内），具体实现晚到。
- **双链路并存（带 kill-switch）**：老路径不真正删除，先放成应急回退
  钩子；新路径默认全量上线，靠环境变量一键回退。
- **Stage 内自治**：每个 Stage 自己处理串行化、取消、缓冲、终止信号；
  Engine 只负责装配 + lifecycle。
- **Port 不持有引擎状态**：MediaPort 只是一根管子，所有 PCM 复制 / 重采样
  / 丢帧策略都在 Port 内闭环；上层引擎不感知 RTP/抖动。
- **环境变量可热改**：所有 feature flag 走 `os.Getenv` 现读，避免重启。

---

_最近更新：PR-10a 之后，`pkg/dialog/realtime` 引擎骨架就位
（agentStage + 复用 cascaded 的 hotword/persist Stage）。SIP 侧
realtime 仍走 legacy 路径，PR-10b 接入 native realtime route。_
