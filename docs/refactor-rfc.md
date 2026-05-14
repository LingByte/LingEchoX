# LingEchoX 全量重构与延迟优化 RFC

> 状态：草案（待 review）
> 范围：Go 后端（`pkg/`、`internal/`、`cmd/`）+ 少量前端（`web/`）
> 目标：（1）根治转接 / 桥接 Call-ID 错位与 codec 失败 bug；（2）端到端转人工 / 首字 TTS 延迟降低 1–2s；（3）将 67k 行单体收敛到可演进的分层结构。
>
> 设计原则：
> - 每一阶段都能独立合并、可灰度、可回滚；不存在"半重构就崩"。
> - 旧 API 用 deprecation shim 留 1 个 release，避免大爆炸式改动。
> - 任何热路径改动必须先有 benchmark / call trace，再合并。

---

## 0. 现状结论速览

| 类别 | 现状 | 主要风险 |
| --- | --- | --- |
| 包结构 | `pkg/sip/` 单包 165 项，`conversation` 混入 voicedialog/webseat/playback | 模块边界模糊，bug 互相牵动 |
| 状态管理 | 17 处 package-level `sync.Map` + 13 处 `Set*` 回调注入 | 测试无法并发；Call-ID 错位是直接后果 |
| 配置 | 227 处 `os.Getenv` / `utils.GetEnv`，76 个 env key | 没有 typed config，无法 lint 未声明项 |
| 媒体 | `pkg/media/session.go` 954 行单文件 | 编解码协商、桥接逻辑高度耦合 |
| Provider | ASR 13 家 / TTS 15 家各自约 400–540 行 | 复制粘贴，新增成本高 |
| Handlers | `tenant_users.go` 784 行、多 handler 8k–15k 字节 | 业务直写 handler，无 service 层 |
| 可观测性 | 仅 zap INFO 日志 | 无 metrics / tracing，per-call 时间线靠肉眼 |
| 测试 | 业务文件 273 / 测试 96 | 转接、webseat、voicedialog 几乎裸奔 |

---

## 1. 目标架构（target tree）

```
cmd/
  server/        # HTTP + 内嵌 SIP 启动器
  bootstrap/     # 迁移、种子、JWKS

internal/
  app/           # 组装层（DI 容器、依赖装配）
  http/          # gin 路由 + 中间件
  http/handler/  # 仅做参数解析 / DTO，转发到 service
  service/       # 业务层（tenant / sip_campaign / acd / webseat / call）
  repo/          # GORM 仓储（替代 handler 直连 DB）
  worker/        # 长跑后台任务（migrate 自 internal/tasks、sipserver/worker）

pkg/
  config/        # typed Config，唯一 env 入口
  logger/
  observability/ # NEW：metrics、tracing、call timeline

  sip/
    callid/      # NEW：CallID 类型 + 规范化 + DialogID
    callreg/     # NEW：中心化 per-call 状态注册表（替代 17 处 sync.Map）
    stack/       # 协议栈（消息、头部解析）
    transaction/ # 事务层
    dialog/      # 对话层
    transport/   # UDP/TCP（自 server 拆出）
    uas/         # 服务端核心（INVITE/BYE/ACK/INFO/REFER 分文件）
    uac/         # 替代 outbound/，对外 Dial 接口
    registrar/   # SIP 注册（自 server 拆出）

  media/
    session/     # 会话状态机（拆自 session.go 954 行）
    transport/   # RTP / SRTP / 资源
    codec/       # 编解码 + 注册表（替代全局 SetDefaultResampler）
    pipeline/    # NEW：统一 transcode pipeline（PCMA↔PCM16↔Opus）
    recorder/    # NEW：async WAV writer

  voice/
    dialog/      # 自 sip/voicedialog
    asr/         # 自 recognizer/
    tts/         # 自 synthesizer/
    llm/         # 自 llm/

  contactcenter/
    bridge/      # 转接 PSTN ↔ 坐席（自 sip/conversation/transfer_*.go）
    webseat/     # 浏览器坐席（自 sip/webseat）
    routing/     # ACD（自 sip_acd_targets handler 抽出）

  stores/        # 对象存储（不变）
  middleware/    # 不变
```

向旧导入路径保留 type alias 一个 release，确保渐进迁移。

---

## 2. 阶段路线

每个阶段独立 PR；标 `[fast-path]` 的 PR 直接影响通话延迟。

### Phase 1 — Call 中心化与 Call-ID 规范化  `[fast-path]`

**目的**：根治当前 Call-ID `@1.14.99.158` vs `@10.0.4.12` 错位导致 BYE 找不到 bridge → A 被晾着。

**改动**：

1. 新建 `pkg/sip/callid`：
   ```go
   type CallID string  // 规范化后存储的不可变 ID

   // 规范化：trim、转小写 host、保留 local-part 原样
   func Normalize(raw string) CallID

   // DialogID = Call-ID local-part + sorted(from-tag, to-tag)
   // SBC 改写 host 时仍能匹配
   type DialogID struct { ... }
   func DialogIDFromMessage(msg *stack.Message) DialogID
   ```

2. 新建 `pkg/sip/callreg`：
   ```go
   type Registry struct { ... }

   type Entry struct {
       Inbound      CallID
       OutboundLegs []CallID         // 一次转接可能尝试多个 agent
       DialogIDs    []DialogID       // 用于 SBC Call-ID rewrite 跟踪
       Phase        TransferPhase
       Bridge       BridgeRef
       CreatedAt    time.Time
   }

   func (r *Registry) Bind(inbound CallID, outbound CallID, did DialogID)
   func (r *Registry) RekeyOutbound(inbound, oldOut, newOut CallID)
   func (r *Registry) Lookup(anyID CallID) (*Entry, bool)
   func (r *Registry) LookupByDialog(did DialogID) (*Entry, bool)
   func (r *Registry) Release(inbound CallID)

   // 自动 GC：超过 N 分钟未活跃 entry 被清理 + 告警（防止 goroutine 泄露驻留）
   ```

3. 替换以下 16 处包级状态（按 PR 拆分）：
   - `conversation`: `transferStarted`, `transferRingStop`, `transferNoAgentRetry`, `transferLastACDRowByInbound`, `bridges`, `transferExclude*`
   - `outbound`: `outLeg` 内部 map（保留实例字段，但 key 用 `callid.CallID`）
   - `webseat/hub.go`: `awaiting`, `active`, `acdBinding`

4. `outbound/manager.go::handleResponse` 在收到 200 OK 时：
   - 若 200 OK 的 Call-ID 与 INVITE 不一致（SBC 改写），调用 `registry.RekeyOutbound(inbound, sentCID, recvCID)`。
   - 现在的 `MigrateTransferBridgeOutboundCallID` 走 registry，**不再依赖外部 conversation 包**。

5. `pkg/sip/uas/bye.go`（从 server 拆出）：BYE 处理直接 `registry.LookupByDialog(DialogIDFromMessage(msg))`，host 不一致也命中。

**验收**：
- 单测：构造 SBC host 改写场景，BYE 能正确命中 bridge → 触发对端 BYE。
- 集成：手工复现今天日志的转接挂死场景，A 端在 2s 内被挂断。

**回滚**：保留旧的 `bridges` map 与 `MigrateTransferBridgeOutboundCallID` 一个 release，只在新 registry miss 时回退查旧表。

---

### Phase 2 — 转接路径并行化  `[fast-path]`

**目的**：当前流程是 `LLM 决策 → TTS 完整播完 →（200ms 后）发 outbound INVITE → ring`。日志显示这段约 4 秒。

**改动**：

1. `voicedialog/loopback.go::markTransferAfterNextTTS` 改为**预决策**：
   - LLM 输出包含 transfer intent 的瞬间，并行启动 `TriggerTransferToAgent`；TTS 继续合成"正在转接"。
   - 引入 `TransferIntentDetected(callID)` 事件，由 `conversation/transfer.go` 订阅。
   - 加超时保险：若 TTS 在 N ms 内未结束而 outbound 已 200 OK，立即停 TTS 推送 ring。

2. `conversation/transfer.go::TriggerTransferToAgent`：
   - `notifyTransferPhase` → `loading` 与 `dial goroutine` 之间不再有顺序锁，直接 fire-and-forget。
   - ring 播放与 dial 完全并行（已经是 goroutine，但需确认 wav 加载不阻塞主路径；改为预加载缓存）。

3. WAV 预加载：启动时把 `ringing.wav` / `welcome.wav` 加载到 `pkg/media/cache` 单例，避免每次 transfer 都做 `LoadWAVAsPCM16Mono`。

**预期收益**：
- "意图识别 → INVITE 发出" 从约 3.5s 缩到约 0.5s。
- 用户感知"转接中"语音与拨号并发。

**验收**：
- benchmark：record 一段固定意图音频，统计 LLM final → outbound INVITE Tx 时间。目标 < 800ms（含 LLM 自身延迟）。

---

### Phase 3 — 媒体桥接 codec 统一  `[fast-path]`

**目的**：根治 `bridge: encode agent: codec not supported`。

**改动**：

1. 新建 `pkg/media/codec/registry.go`：
   ```go
   type Codec interface {
       Name() string
       SampleRate() int
       Channels() int
       Encode(pcm []int16) ([]byte, error)
       Decode(payload []byte) ([]int16, error)
   }
   func Register(c Codec)
   func Lookup(name string) (Codec, bool)
   ```
   默认注册：`pcma`, `pcmu`, `opus`, `g722`, `pcm16`。

2. 新建 `pkg/media/pipeline/transcoder.go`：
   ```go
   type Transcoder struct {
       src, dst Codec
       resamp   Resampler
   }
   func New(src, dst string) (*Transcoder, error)  // 自动选 resampler
   func (t *Transcoder) Process(in []byte) ([]byte, error)
   ```

3. `pkg/sip/bridge`：
   - `NewTwoLegPCMBridge` 内部用 `pipeline.Transcoder` 而不是手写 `siprtp.NewSIPRTPTransport` 选择编码器；任何 codec 组合（含 PCMA↔Opus）都能直接成桥。
   - webseat 与 conversation transfer 共用 **同一个** `bridge.Build(legA, legB, opts)` 入口。

4. webseat 在 SDP 协商时不再硬要 Opus：
   - 优先 Opus（如果浏览器 offer 含），其次 PCMA/PCMU（不要再失败）。
   - 失败回退路径不再 `encode agent: codec not supported`，而是协商失败 → 主动挂断并告警。

**验收**：
- 单测：transcoder PCMA→Opus 与 Opus→PCMA 互通，buffer 对齐。
- 集成：复现今天日志的 PSTN PCMA + 浏览器 Opus 场景，bridge 成功 + 双向音频可达。

---

### Phase 4 — 配置收口

**目的**：消灭 227 处 `os.Getenv` 散落。

**改动**：

1. 新建 `pkg/config/config.go`：
   ```go
   type Config struct {
       SIP      SIPConfig
       Media    MediaConfig
       WebSeat  WebSeatConfig
       LLM      LLMConfig
       ASR      ASRConfig
       TTS      TTSConfig
       Storage  StorageConfig
       DB       DBConfig
       Auth     AuthConfig
   }

   func Load() (*Config, error)  // godotenv + envconfig + 校验
   ```
   字段 tag 走 `env:"SIP_RTP_PORT_START" envDefault:"30000"`。

2. 启动时打印：
   - 已识别 N 项配置；
   - 未识别的 `SIP_*` / `LING_*` env（拼写错误检测）。

3. 渐进迁移：先把 `cmd/server/main.go` 启动期 env 收口；运行期热路径（如 transfer.go 里读 `SIP_TRANSFER_RINGING_WAV_PATH`）改为构造时注入。

4. CI lint：`scripts/check-env.sh` 对比 `env.example` 与代码中实际引用，报告漂移。

---

### Phase 5 — `pkg/sip/conversation` 与 `sip_server.go` 拆分

**目的**：文件级别拆分，减少 merge 冲突，便于补测试。

**改动**：

1. `pkg/sip/server/sip_server.go` (1353 行) → 按动词拆：
   ```
   pkg/sip/uas/
     server.go       # UAS 初始化、accept loop
     invite.go       # INVITE 路由
     bye.go          # BYE 处理（含 transfer bridge 联动）
     ack.go
     info.go         # DTMF / SIP INFO
     refer.go
     options.go
     dispatch.go     # 响应分发到 transaction
     digest_auth.go
     presence.go
   ```

2. `pkg/sip/conversation/` 拆为：
   ```
   pkg/contactcenter/bridge/
     transfer.go         # TriggerTransferToAgent
     bridge.go           # StartTransferBridge / Hangup*
     ring.go             # startTransferRinging
     retry.go            # startNoAgentRetryLoop
     persist.go
     agent_events.go
   pkg/contactcenter/webseat/  # 自 sip/webseat 整体迁移
   pkg/voice/dialog/
     hub.go              # 自 sip/voicedialog/hub.go
     gateway_media.go
     loopback.go
     protocol.go
     playback.go
   pkg/voice/script/
     mode_flags.go
     listen.go
   ```

3. `pkg/sip/conversation/voice.go` (1140 行) → 按数据流拆：
   ```
   pkg/voice/asr/state.go
   pkg/voice/llm/turn.go
   pkg/voice/tools/registry.go
   ```

4. 旧 import 路径保留 alias：
   ```go
   // pkg/sip/conversation/transfer.go (deprecation shim)
   //go:deprecated 使用 pkg/contactcenter/bridge
   package conversation
   import "github.com/.../pkg/contactcenter/bridge"
   var TriggerTransferToAgent = bridge.TriggerTransferToAgent
   ```

---

### Phase 6 — Handler → Service 分层

**目的**：把 `tenant_users.go` 784 行级别的 handler 砍到 < 200 行。

**改动**：

1. 新建 `internal/service/` 与 `internal/repo/`：
   ```go
   // internal/repo/tenant_user.go
   type TenantUserRepo interface {
       FindByID(ctx, id) (*model.TenantUser, error)
       ListByTenant(ctx, tenantID, page) ([]*model.TenantUser, int64, error)
       Create(ctx, u *model.TenantUser) error
       Update(ctx, u *model.TenantUser) error
       SoftDelete(ctx, id) error
   }

   // internal/service/tenant_user.go
   type TenantUserService struct { repo TenantUserRepo; ... }
   func (s *TenantUserService) Invite(ctx, req) (*dto.TenantUser, error)
   ```

2. 优先迁移的 3 个最大 handler：
   - `tenant_users.go` (784 行)
   - `tenant_organization.go` (12.9k 字节)
   - `sip_campaigns.go` (15.3k 字节)

3. handler 仅做：参数绑定 + 调 service + DTO 响应。

4. DI：用 `internal/app/wire.go`（手写，不引入 wire 工具）统一组装。

---

### Phase 7（可选）— 可观测性 + 前端微调

**后端**：
- 新增 `pkg/observability`：Prometheus metrics（per-call 时长、ASR/LLM/TTS 延迟分桶、bridge mode 比例、转接成功率）。
- Per-call timeline span（不引入 OTel SDK 也行，先内部 JSON timeline，按 call_id 落盘）。
- pprof 路由 + goroutine 泄露监控（每 60s 打印 goroutine 数差异）。

**前端**：
- 引入 `@tanstack/react-query` 取代裸 axios + 各页面手动 loading state。
- 评估去掉 `@visactor/react-vchart`（10MB+），Overview 改用 `recharts`（~500KB）。
- 统一 UI 体系：要么全 Arco、要么全 shadcn/ui，避免 Arco + Tailwind utility 双轨。
- `web/package.json::name` 改回 `lingechox-web`。

---

## 3. 延迟优化专项（独立于阶段，可任意时间合并）

| 优化项 | 位置 | 预计省 | 风险 |
| --- | --- | --- | --- |
| 转接意图并行 dial（见 Phase 2） | `voicedialog/loopback.go`、`conversation/transfer.go` | 1–2s | 低 |
| WAV 预加载 | `media/cache.go` | 30–100ms | 极低 |
| ASR partial 阈值调优 | `conversation/asr_state.go` | 200–500ms | 中（误打断风险） |
| async 录音 writer | `utils/sip_recording_wav.go` | 高并发下 RTP jitter ↓ | 中（落盘失败需告警） |
| LLM 流式 → TTS 流式 chunk pipeline | `voice/dialog/gateway_media.go` | 500ms+ 首字 | 高（需 TTS provider 支持） |
| RTP 收发 goroutine pool | `pkg/sip/rtp` | 大量并发下 CPU ↓ | 中 |
| outbound INVITE 预解析目标 SRV | `outbound/dial.go` | 50–200ms | 低 |

---

## 4. 测试与回归策略

1. **新增 `pkg/sip/callid` / `callreg` / `media/pipeline` 三个包必须 90%+ 覆盖率**。
2. 每个 Phase PR 自带：
   - 单测；
   - 一个端到端回放 fixture（PCAP / 文本时序），跑在 CI 上。
3. 引入 `make test-race`、`make test-leak`（`go.uber.org/goleak`）作为 CI 必跑项。
4. 现有依赖 `init()`/全局变量的测试逐步标记 `t.Parallel()`，最后一阶段全部 parallel。

---

## 5. 时间线建议

| 周 | 内容 |
| --- | --- |
| W1 | Phase 1（CallRegistry）+ Phase 2（并行转接）+ 修当前 bug |
| W2 | Phase 3（codec 桥接）+ Phase 4（config 收口）|
| W3 | Phase 5（拆包，多 PR）|
| W4 | Phase 6（handler 分层，前 3 大 handler）|
| W5 | Phase 6 收尾 + Phase 7 可观测性 |
| W6 | 前端整改 + 文档更新 + 全量回归 |

---

## 6. 风险与回滚

- **风险 1：拆包导致大量 import 改动**。对策：每次拆 1 个子包，旧路径保留 deprecation shim 一个 release。
- **风险 2：CallRegistry 并发 bug**。对策：先与旧 map 双写、读旁路对比、灰度 1 周后切主。
- **风险 3：codec pipeline 引入新音质问题**。对策：transcoder 路径添加可选的 dump，逐通话 A/B 对比 RMS / clipping。
- **风险 4：转接并行化导致 TTS 与 ring 重叠播放**。对策：`MediaSession` 引入显式优先级队列（ring < tts < bridge），当 bridge 起来时强制停所有播放。

---

## 7. Phase 1 已经识别的具体接口（开工前 freeze）

```go
// pkg/sip/callid/callid.go
package callid

type CallID string

func Normalize(raw string) CallID { ... }
func (c CallID) LocalPart() string { ... }
func (c CallID) Host() string { ... }
func (c CallID) String() string { return string(c) }

type DialogID struct {
    LocalPart string
    Tag1, Tag2 string // sorted
}

func DialogIDFromRequest(msg *stack.Message) (DialogID, bool)
func DialogIDFromResponse(msg *stack.Message) (DialogID, bool)
```

```go
// pkg/sip/callreg/registry.go
package callreg

type Phase int
const (
    PhaseIdle Phase = iota
    PhaseTransferRequested
    PhaseTransferRinging
    PhaseTransferConnected
    PhaseEnded
)

type Entry struct {
    Inbound      callid.CallID
    OutboundLegs []callid.CallID
    Dialogs      []callid.DialogID
    Phase        Phase
    BridgeRef    any // 指向 bridge.Bridge，import cycle 时用 any
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type Registry struct { ... }

func New(gcEvery, ttl time.Duration) *Registry

func (r *Registry) StartTransfer(inbound callid.CallID) (*Entry, bool /*created*/)
func (r *Registry) BindOutbound(inbound, outbound callid.CallID, did callid.DialogID) error
func (r *Registry) RekeyOutbound(inbound, oldOut, newOut callid.CallID) error
func (r *Registry) SetBridge(inbound callid.CallID, br any) error
func (r *Registry) SetPhase(inbound callid.CallID, p Phase) error

func (r *Registry) Lookup(anyID callid.CallID) (*Entry, bool)
func (r *Registry) LookupByDialog(did callid.DialogID) (*Entry, bool)

func (r *Registry) End(inbound callid.CallID) (*Entry, bool)
```

Phase 1 提交顺序：
1. PR-1：`callid` 包 + 替换 `outbound/invite.go::newCallID` 与 `conversation/transfer_bridge.go::normCallID`。
2. PR-2：`callreg` 包，仅作为旁路（双写读旧），开关 `CALLREG_ENABLED=shadow`。
3. PR-3：`conversation/transfer.go` 与 `transfer_bridge.go` 切到 registry 主读，旧 map 仅 fallback。
4. PR-4：`outbound/manager.go::handleResponse` 在 200 OK Call-ID 改写时调 `RekeyOutbound`。
5. PR-5：`uas/bye.go`（从 server 拆出的最小子集）走 `LookupByDialog`。
6. PR-6：拆 `transferStarted` / `transferRingStop` / `transferNoAgentRetry`，全部走 registry phase。
7. PR-7：删除旧 map，CALLREG_ENABLED 默认 on。

每个 PR 独立可回滚。
