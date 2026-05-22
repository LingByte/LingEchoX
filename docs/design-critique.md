# LingEchoX 设计批评与重构规划

> 编制者：代码审查（2026-05）
> 目标读者：架构 / 后端核心维护者
> 目的：把"业余感"的来源量化为一份可执行清单——指出**哪里不优雅 / 不可拓展 / 不可测**，并给出**下一阶段的具体重构动作**。本文只谈*设计*问题，不重复已修过的安全 / 稳定性 bug。

---

## 0. TL;DR

| 维度 | 现状评分 | 主要扣分项 |
|---|---|---|
| **模块边界** | 6 / 10 | `handlers` 退化成胖 service 层；SIP `manager.go` 1200+ 行扛了 dial/refer/cdr/qos 多职责 |
| **接口抽象** | 4 / 10 | 关键扩展点（codec / TTS provider / 拨号策略 / 鉴权）写死在 switch；外部要插件就只能改源 |
| **可观测** | 5 / 10 | metrics 已经有，但**没有统一上下文**：日志、metrics、trace 三套 ID 互不打通 |
| **测试覆盖** | 4 / 10 | SIP 协议层有少量单测，handlers / outbound / campaign 几乎是裸奔；e2e 全靠手测 |
| **配置 / 启动** | 5 / 10 | `cmd/bootstrap` 拼装顺序硬编码；新增子系统都要去翻 `main.go` 手插 |
| **错误传播** | 5 / 10 | `errors.New + fmt.Sprintf`、字符串错误码、`response.Fail("err",  nil)` 占主流，前端拿不到稳定错误码 |

定性结论：**这套代码"能跑、能扩、但扩起来全靠改主干"** —— 还没有一个稳定的*可拓展骨架*。下一阶段重构的关键词不是"再分几个文件"，而是 **"先定义可发布的接口，再切实现"**。

---

## 1. 业余感最强的两个点 —— 用户点名

### 1.1 `pkg/sip/session_timer`

**它干了什么**：纯函数实现 RFC 4028 Session-Timer 头部的 parse / format / negotiate（UAS）。代码本身写得很认真，包文档良好。

**为什么显得"业余"？**

| 问题 | 现象 | 影响 |
|---|---|---|
| **A. 包内只覆盖 UAS 协商一半流程** | `NegotiateUAS` 存在；`NegotiateUAC`、422 重协商、refresher 切换都散在 `pkg/sip/outbound/refresher.go` 和 `pkg/sip/server` 各处 | 知识被三处分别"重新发明"一次：协议常量在 session_timer，UAC 状态机在 outbound，UAS watchdog 在 server。改一处规则要同时同步三处。 |
| **B. 没有 `Timer` 这个 Subject 抽象** | watchdog goroutine 直接被 `cs.armSessionTimer(...)` 起在 call_session 里；refresher loop 起在 outLeg；二者互相不知道对方存在 | 后续要加：(1) 计时器停 SIPS-only 场景、(2) re-INVITE 内嵌 session-timer 重算、(3) 把 UAS / UAC 改造成 refreshee+refresher 双向能力——都得"全局搜索"才能改全 |
| **C. 协议层耦合到日志 / metrics** | `Decision.String()` 兜底，但调用方还是用 `zap.Int("session_expires", ...)` 手填字段 | 想加 prometheus label "session_timer_state" 要全局加补丁 |
| **D. 测试不能驱动真实状态机** | `session_timer_test.go` 全是纯 parse + Decision 单测；422 重试、refresher 切换、超时 BYE 这些**真状态机**没覆盖 | 任何改动只能靠 staging 真机验证 |

**结论**：协议**头处理是合格的，状态机扩散是不合格的**。session_timer 应该是 *one type owns the lifecycle*，目前是 *协议头解析独自成包，状态机寄养在 outbound/server*。

### 1.2 `pkg/welcomeaudio`

**它干了什么**：欢迎语 WAV 的 URL 校验、远程拉取 + 解码 + 进程内缓存。

**为什么显得"业余"？**

| 问题 | 现象 | 影响 |
|---|---|---|
| **A. 缓存粒度耦合在调用方** | `cache key = url \| sampleRate`，调用方必须知道"按租户做隔离"要自己拼 key 前缀 | 已经有同一 URL 被两个租户配的场景；缓存命中**跨租户**，不踩雷只是因为目前公网 URL 没有 ACL |
| **B. 缓存大小无上限** | `sync.Map` + 时间 TTL，没有最大字节 / 最大条目数，进程吃内存吃到 OOM 都不会反压 | 单实例如果配 200 个 DID × 10MB WAV → 2GB 内存淤泥 |
| **C. 解码函数靠回调注入** | `FetchPCM(... decodeWAV func(...))` 故意不在包内 import `conversation` 避免循环依赖 | 接口**正确**，但暴露给业务后大家都各传各的 decode 实现，没有统一 codec 抽象（PCM16 默认 mono 是隐式约定，没强类型 enforce） |
| **D. 没有"音频源"抽象** | 整个包**只支持 HTTP 拉 WAV**。要支持 OSS 直读 / 本地预置 / TTS 实时合成？得另开一个包 | 这是真正的扩展性问题：欢迎语在产品角度有 3 种来源，代码里钉死了 1 种 |
| **E. ValidateURL 干了"HEAD + Range GET"，但 fetch 时不再校验** | 校验是写时一次性的；3 个月后 URL 改成了 .mp3，运行时才会 decode 失败 | 应该有"周期性 revalidate + 标记 broken"机制 |

**结论**：包名应该是 `media.Resource`/`media.Welcome`，包里应该有 `AudioSource` 接口（HTTP / OSS / Local / TTS 都是它的 impl），现在是把 HTTP fetch 当成了"业务包"。

---

## 2. 项目级"系统性"槽点

排在 session_timer / welcomeaudio 之后，按影响力从大到小：

### 2.1 `internal/handlers` 是个胖 service 层

**症状**：
- `handlers/sip_users.go` (840+ 行) 既做参数 binding 也做业务规则；
- `handlers/credentials.go` 直接生成 AK/SK 并写 DB，没经过 service 层；
- `handlers/tenant_users.go` 在第 5 轮重构里被拆出去一部分，但还是 355 行；
- 几乎所有 handler 都直接 `h.db.Where(...)`，DB 查询和业务规则缝在一起；
- "service" 这一层根本不存在 —— `internal/services` 是空的。

**后果**：
- 业务规则**不能被复用**（campaign worker 也想检查 tenant quota？得 copy 一遍 SQL）；
- 业务**不能 mock**（要测 createTenantUser 唯一现实办法是开 sqlite）；
- 不同入口（HTTP / AK-SK 签名 / 内部 RPC）想共享同一段业务 → 各自重写。

**重构方向**：引入 `internal/services` 层，handler 只做参数 binding + 调 service + format response。Service 持有 `db *gorm.DB` 和 *接口形态* 的依赖（钱包、限额、审计），方便单测。

### 2.2 `pkg/sip/outbound/manager.go` (1225 行) 是个 god struct

**承担的职责**：
1. 信令池（TCP/TLS 复用）
2. RTP 端口分配
3. Dial 状态机（INVITE → 100/180/200/4xx → ACK）
4. SDP offer/answer 处理
5. SRTP / DTLS-SRTP 协商
6. Refer / Transfer
7. CDR 状态机
8. Session-Timer UAC（实际拆到 refresher.go 还行）
9. 早期媒体管理
10. RTP / RTCP 报文转发回业务层

**问题**：
- 不存在"`Dialer` 接口"——想插入第三方 SBC 适配器就只能 fork manager；
- `outLeg` struct 字段超过 30 个，跨文件被 12 个方法读写，无人能完整描述它的状态机；
- `cleanupLeg` 收尾了至少 4 个 goroutine + 3 个 chan + 2 张全局 map，任何路径漏掉一处都会泄漏。

**重构方向**：
- 抽 `OutboundDialer` 接口：`Dial(ctx, DialRequest) (LegHandle, error)` / `Refer(...)` / `Cancel(...)`；
- `manager.go` 拆 `dial.go` / `refer.go` / `signaling_pool.go` / `cdr.go` 四个文件；
- `outLeg` 状态机用 stringly-typed enum + transition log，便于排查"卡在某 state"的现场。

### 2.3 关键扩展点全部是 switch / if

**统计**（粗略 grep）：

| 扩展维度 | 现状 | 想加新实现时 |
|---|---|---|
| **TTS provider** | `pkg/synthesizer/synthesis.go` 一个大 switch（aliyun / baidu / qcloud / qiniu / openai / aws / google） | 改 switch + 加文件 |
| **ASR provider** | `pkg/recognizer/*.go` 各自 New 但调度也是 switch | 同上 |
| **LLM provider** | `pkg/llm/*.go` 类似 | 同上 |
| **storage backend** | `pkg/stores/*.go` 比较好，有 `Store` 接口；但 `Default()` 是全局单例 | 单实例多租户不好做 |
| **codec** | `pkg/sip/session/call_session.go` switch by name | 加 codec 必须改 switch + 全局枚举 |
| **dispatch mode** | ACD `dispatch_mode` 字段串 + 大段 if-else 在 outbound worker | 加新策略要改 worker + DB enum |

**结论**：项目缺一套"**Provider Registry**"模式。每个 provider 启动期注册自己，业务层只拿 interface 用。

### 2.4 配置系统是"全局变量 + os.Getenv 直读"混着用

**症状**：
- `pkg/config` 有 `GlobalConfig` 单例，理论上是配置中心；
- 实际上**一半包**绕过它直接 `utils.GetEnv("SIP_RTP_PORT")` 读环境变量；
- 测试时要换某个值 → `os.Setenv` 全局污染；
- 想加"按租户覆盖配置"→ 不存在抽象。

**重构方向**：
- 所有 env-bound 配置必须经过 `config.Provider` 接口；
- 默认实现读 env + 文件；
- 测试用 `config.NewMemProvider(map)` 替换；
- 引入 `config.PerTenant(tenantID, key)` 作为后续的功能开关基础。

### 2.5 启动期 `cmd/server/main.go` 是过程式拼装

**症状**：250+ 行的 `main()`，子系统启动顺序硬编码（DB → KeyManager → SIP → HTTP）。新加一个子系统（比如本轮的 system monitor）要去这个函数里抠位置。

**重构方向**：定义 `Component` 接口（`Name()` / `Start(ctx) error` / `Stop(ctx) error`），bootstrap 里维护一个 ordered 依赖图，main 只是 `app.Run()`。Graceful shutdown 自动按反序停。

### 2.6 错误处理 / 错误码碎片化

**症状**：
- 业务层用 `errors.New("xxx already exists")`，HTTP 层用 `response.Fail(c, "xxx already exists", nil)` —— **错误码靠字符串字面量对齐**；
- 没有 `apperrors.Code` enum；
- 前端拿到的 `code: 200/400/500` 是 HTTP-style，业务错误一律 400 message=xxx；
- i18n 完全不可能。

**重构方向**：
- 定义 `apperror.AppError { Code int; Msg string; Cause error; HTTPStatus int }`；
- service 返回 `*AppError`，handler 统一 `response.AppError(c, err)` 渲染；
- 前端按 `code` 做国际化文案 + 行为分支。

### 2.7 日志、metrics、trace 三套上下文不打通

**症状**：
- 日志靠 `zap.String("call_id", ...)` 手填；
- metrics 标签 `tenant_id` / `call_id` 各自拼；
- 没有 OpenTelemetry trace；
- request-id 中间件已经塞 `X-Reqid`，但下游业务层取不到（`c.GetString("reqid")` 工作，goroutine 起来就丢了）。

**重构方向**：
- 把 request id / call id / tenant id 装进 `context.Context`，全程透传；
- `logger.WithContext(ctx)` 自动注入；
- metrics label 走同一套上下文提取器；
- 后期可平滑接入 OTLP exporter。

### 2.8 测试 / e2e 黑洞

- SIP 协议层有 `pkg/sip/stack`、`session_timer`、`sdp` 的单测；
- 业务层（handlers、outbound、campaign、tenant）几乎没单测；
- 没有 e2e / acceptance test framework；
- 改 SQL / 改 handler → 只能跑全量 `go test` + 上 staging 手测。

**重构方向**：
- 引入 `testcontainers-go` 起 mysql/redis 跑 e2e；
- 给 handlers 写一层 `httptest` 级单测（基于 service 接口 mock）；
- CI 增加 e2e job，PR 必须 green。

---

## 3. 重构路线（按优先级）

### Phase X1 — "接口先行"（2 周）
> 阻塞的是后续所有扩展性，先把它做了。

- [ ] `internal/services` 骨架：把 tenant_users / sip_users / credentials 三组业务从 handler 抽出来；
- [ ] `pkg/llm`、`pkg/recognizer`、`pkg/synthesizer` 引入 Provider Registry；
- [ ] `pkg/welcomeaudio` 改造为 `pkg/media`，定义 `AudioSource` 接口；HTTP 改 `pkg/media/httpsource`；
- [ ] `apperror` 包 + handler 统一错误渲染中间件；

**验证**：handlers 的 import 列表里不再出现 `gorm.DB` 直接调用。

### Phase X2 — "Session Timer 整顿"（1 周）
> 用户点名的业余感来源。

- [ ] `pkg/sip/session_timer` 升级为 `Lifecycle` 包：
  - 暴露 `type Timer struct { ... }`，封装 watchdog goroutine、refresher 调度、422 重试；
  - UAS 和 UAC 都用同一 Timer，refresher / refreshee 由参数决定；
  - 测试驱动：用假时钟 + 模拟 SIP transport 跑完整 happy path / 边界 case；
- [ ] `pkg/sip/server` 与 `pkg/sip/outbound` 改成"注入 Timer"模式，不再各起 goroutine；
- [ ] 协议头 parse/format 保持纯函数（已合格，不动）。

**验证**：grep 全代码库不再有"自起 session-timer watchdog"的地方；测试覆盖 ≥80%。

### Phase X3 — "Outbound Manager 拆分"（2 周）
- [ ] 抽 `OutboundDialer` interface；
- [ ] `manager.go` → `manager.go` (注入 + 生命周期) + `dial.go` + `refer.go` + `signaling_pool.go` + `cdr.go`；
- [ ] `outLeg` 状态机显式化：state machine + transition log；
- [ ] 单测：dial happy / 408 / 4xx / 503 / refer / cancel；

**验证**：`outLeg` 字段数减半；`go test ./pkg/sip/outbound/...` 跑过完整状态机用例。

### Phase X4 — "可观测性统一"（1 周）
- [ ] context-aware logger（`logger.FromContext(ctx)`）；
- [ ] request id / call id / tenant id 从 middleware 注入 ctx，业务层从 ctx 取；
- [ ] metrics label extractor 走同一套 ctx；
- [ ] 留接口给后续 OpenTelemetry。

### Phase X5 — "配置 + 启动重构"（1 周）
- [ ] `config.Provider` 接口；
- [ ] `Component` 接口 + bootstrap 依赖图；
- [ ] `main.go` 瘦身到 50 行以内。

### Phase X6 — "测试基础设施"（持续）
- [ ] testcontainers-go + mysql 镜像；
- [ ] handlers 层 e2e fixture；
- [ ] CI 加 e2e job。

---

## 4. 不重构的部分（明确"OK"）

避免误伤——以下模块**没有大问题**，本轮不动：

| 模块 | 为什么 OK |
|---|---|
| `pkg/sip/stack` | 协议解析层抽象得当；transport 已经 interface 化 |
| `pkg/sip/sdp` | 纯函数，单测充分 |
| `pkg/middleware` | 职责清晰，命名规范；近期已加 CORS / rate limit / panic recovery |
| `pkg/logger` | 已有 SafeGo / WithFields / context-aware overload，封装到位 |
| `pkg/utils/system` | 本轮刚整顿过，pprof Monitor 已加 panic-safe 和阈值化 |
| `pkg/stores` | 有 `Store` 接口，COS / S3 / Qiniu / Local 都正确实现 |

---

## 5. 量化收益预估

| Phase | 预期人日 | 主要收益 |
|---|---|---|
| X1 | 10 d | 业务可单测，扩展点开放，错误码统一 |
| X2 | 5 d | session-timer 改动只在一处；可靠性显著上升 |
| X3 | 10 d | outbound 二次开发门槛大幅下降；SBC 兼容更容易 |
| X4 | 5 d | 日志/metrics/trace 三合一，排障效率 ↑↑ |
| X5 | 5 d | 新增子系统 1 行代码；启动顺序声明式 |
| X6 | 持续 | 回归 bug ↓↓，CI 把关 |

合计 **35~40 人日**，相当于 1.5~2 个月单人专注。建议双开（一人 X1+X3，一人 X2+X4），3~4 周可完成主干。

---

## 6. 给维护者的具体行动建议

**这周（不需要发版即可做的）**：
1. 给 `internal/services` 创建空骨架包，让人看到该层应当存在；
2. 给 `apperror` 包写出 `AppError` 类型，并在一个 handler（建议 createTenantUser）实践一次；
3. 在 `pkg/sip/session_timer` 顶部加一份 README，把"为什么状态机在外面"的历史决策写下，引导后续 contributor；
4. 在 CI 增加 `staticcheck` 步骤（gate 在新增 issue 数 = 0 即可，不要求零基础线）。

**这月**：开 Phase X1 + X2，先把"接口形状"定下来，之后所有重构都是基于这个形状的填实。

---

## 7. 元批评：为什么会变成这样

回顾整个代码库的 git history（推断自结构）：

1. **第一版**：演示 demo 心态，handler 直连 db，全 switch 路由 provider。能 demo 起来就 ship。
2. **第二版**：上租户隔离，handler 里塞 `tenant_id` 校验。这一波非常重要但**没顺手抽 service 层**，于是 handler 变成胖子。
3. **第三版**：上 SIP outbound，campaign，dispatch。这一波 `pkg/sip/outbound/manager.go` 一文搞定后再没拆。
4. **第四版**：上 metrics、loadbalancer 之类的真生产能力。这一波添加都很扎实但**继续沿用既有的 god-struct 模式**，没人有动机回去拆旧代码。

**核心病因**：项目从来没经过一次"我们要做平台型而不是 demo"的接口设计 review。每次新功能上来，最快路径都是"加 switch 或在 handler 里加分支"。这套增量法在前 5 万行还能撑，过了之后会爆炸。

**修正方向**：把 Phase X1 当成"门"，之后任何新 PR 要求**先过 interface review 再过实现 review**。一旦这个习惯固化，5 个迭代后代码会自然回归整洁。

---

*文档结束。Author 留口子：本文不针对任何具体提交者，只针对**项目演化路径**。改的不是人，是工程惯性。*
