# SIP 模块差距分析与增强规划

> **生成时间**：2026-05-16
> **最近更新**：2026-05-16（追加 P0 三批次实施路线 + 在建项）
> **基线对比**：`@/Users/cetide/Desktop/VoiceServer/pkg/sip/` vs `@/Users/cetide/Desktop/LingEchoX/pkg/sip/`
> **结论**：LingEchoX 已**完全覆盖** VoiceServer 上游 SIP 代码，并在多个方向作出实质增强；下方列出的是**协议层 / 工程化层** 仍可补强的点，按优先级排序。

---

## ⭐ P0 三批次实施路线（in-progress）

> 6 个 RFC 缺口整理为 3 批，每批独立 PR、独立验收。每完成一个批次会在此打勾并附 commit / 验收笔记。

### 📠 转接架构说明（与下面 1B 相关）

**现状**：SIP REFER 入局 ✅ 支持（`@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/refer_handler.go:14` 响应 202 + NOTIFY 回报）；出局 ❌ 不发。两条路径最终都收敛到同一个 B2BUA 模型：

| 触发源 | 走到哪里 | 外部表现 |
|---|---|---|
| 对端 REFER | `pkg/sip/server/refer_handler.go` → `conversation.TriggerTransferFromReferTo` → `outbound.Manager.Dial` | 接受 REFER，解析 `Refer-To` 后**平台自己发起新的 INVITE**，原对端跳过，同时 NOTIFY 回报 100/200/603 |
| AI/工具/免责文案 | `conversation.TriggerTransferToAgent` → `outbound.Manager.Dial` | 直接 ACD pool / env 目标 |

两条路径都走 `MediaProfileTransferBridge`，最终都是**两条 leg 中间做 RTP 桥接**。选这个 B2BUA 设计的原因：
- 全程控制媒体 → 录音/转写/质检不会丢
- 老 PBX / 老话机多数不支持发出 REFER，能发也无法靠 PSTN 正确处理
- 跨运营商转接时，对端需要看到「谁发起」与「原始被叫号」，这里就需要补 RFC 7044 History-Info + RFC 5806 Diversion（下表 1B）重建溯源链

### 批次 1：低风险 + 跨网部署必需（预估 2-3 工作日）

| Item | 状态 | 落地点 | 验收 |
|---|---|---|---|
| 1A. RFC 3325 P-Asserted-Identity / Privacy | ✅ done (2026-05-16) | `@/Users/cetide/Desktop/LingEchoX/pkg/sip/identity/identity.go`（新包）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/invite.go:164-183`（注入头）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/incoming.go:62-87`（提取头）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/types.go:60-78`（DialRequest 字段）| 单测 19 个通过；trust-domain 通过 `SIP_PAI_TRUST_DOMAINS` env 配置（空=信任所有；生产环境填运营商 SBC IP，逗号分隔）。**业务侧消费**：`IncomingCall.AssertedIdentities`（trust-domain 内有效）+ `.PrivacyTokens`；下游 router / CDR 持久化层可继续替换 FromURI 为 PAI |
| 1B. RFC 7044 History-Info + RFC 5806 Diversion | ✅ done (2026-05-16) | `@/Users/cetide/Desktop/LingEchoX/pkg/sip/historyinfo/`（新包，parse / format / chain extension）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/invite.go:204-214`（注入两头）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/incoming.go:78-99`（入局解析）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/session/call_session.go:124-164`（CallSession 缓存原始头供转接路径读取）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/conversation/transfer_retarget.go`（applyRetargetHeaders 串联链）| 单测 16 个通过（historyinfo 9 + transfer_retarget 5 + buildINVITE 注入 2）。AI/ACD 路径使用 `cause=302 / unconditional`，入局 REFER 路径使用 `cause=302 / deflection`。两头**始终同时发出**因为 PBX 群体读 7044 / 老话机读 5806；无入局头时不伪造（避免误导下游"以为发生过 redirect"）|
| 1C. RFC 4028 Session-Timer | ⏳ pending | 新增 `@/Users/cetide/Desktop/LingEchoX/pkg/sip/session_timer/` + 与 INVITE/200/UPDATE 路径整合 | `Session-Expires`/`Min-SE` 协商；UAC/UAS refresh re-INVITE/UPDATE；过期自动 BYE |

> ~~IPv6 dual-stack~~：**不做**（产品决策 2026-05-16）。运营商 SBC + 企业内网 未来 2-3 年依然以 IPv4 为主，额外补 IPv6 收益低。

### 批次 2：传输层升级（预估 3-5 工作日）

| Item | 状态 | 落地点 | 阻塞条件 |
|---|---|---|---|
| 2A. TCP signaling transport | ⏳ pending | 重写 `@/Users/cetide/Desktop/LingEchoX/pkg/sip/stack/transport.go` 抽象，新增 TCP framing (RFC 3261 §18) | 无 |
| 2B. TLS signaling (SIPS) | ⏳ pending | 在 2A 基础上加 TLS wrapper；Contact/Via 用 `sips:` scheme | 需要 TLS 证书（自签 OK） |

### 批次 3：互通与合规（预估 10-17 工作日）

| Item | 状态 | 落地点 | 阻塞条件 |
|---|---|---|---|
| 3A. DTLS-SRTP | ⏳ pending | `@/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/transport.go` + `@/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/sdp.go` `a=fingerprint` 协商 | 自签 cert 即可，WebRTC 互通必备 |
| 3B. RFC 8224 STIR/SHAKEN | ⏳ pending | 新增 `@/Users/cetide/Desktop/LingEchoX/pkg/sip/identity/` (JWS sign / verify) + INVITE `Identity:` 头 | 需要 STI-CA 签发 SP 证书（北美运营商必需），可先做 stub + 自签验证链 |

> 每批落地后回到本节打 ✅ + 加 commit 链接 / 测试结果。

---

## 一、与 VoiceServer 同步状态

### 1.1 VoiceServer 独有但 LingEchoX **没有**的真正源码（非测试）

| VS 文件 | 功能 | LingEchoX 现状 |
|---|---|---|
| `session/decoder.go` (`passthroughDTMFDecode`) | DTMF 透传解码 wrapper | ✅ 已 inline 在 `@/Users/cetide/Desktop/LingEchoX/pkg/sip/session/call_session.go:260` |

**结论**：源码层零真实缺失。剩余差异全是 VS 为提升 coverage 数字写的 `coverage_boost*_test.go` 类测试（`bridge_test.go`, `dtmf/coverage_boost_test.go`, `server/handlers_coverage_test.go` 等共 ~12 个），不影响功能。

### 1.2 LingEchoX 比 VoiceServer 多出来的能力（**已实现**）

| 路径 | 增强 |
|---|---|
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/voicedialog/` | 新一代 dialog 网关：tts_segmenter / chanReplayService / 流式预取 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/hybrid_script_*.go` | 脚本+LLM 混合外呼路由 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/transfer.go` + `refer_parse*` | REFER 转接全套 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/acd_trunk.go` | ACD 中继路由 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/persist/` | 会话/录音持久化 + 注册凭据库（VS 完全无此能力） |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/vad/` | RMS VAD + barge-in |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/webseat/` | Web 端坐席桥接 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/conversation/` | LLM/ASR/TTS 整体编排 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/voice/recorder/` | Wall-clock + jitter snap 立体声录音器 |
| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/session/call_session.go` | CallSession 拓展 + 新录音器集成 |

---

## 二、SIP 协议层缺口（按 RFC 检查）

> 标注 P0 = 影响互操作性 / 上线门槛；P1 = 增强 / 可选；P2 = 长尾

### 2.1 P0 缺口

| RFC / 特性 | 现状 | 影响 |
|---|---|---|
| **RFC 4028 Session Timer**（`Session-Expires` / `Min-SE`） | ❌ 未实现 | 长时通话被运营商/SBC 中途单方面 BYE；现在我们既不发送也不响应 refresh re-INVITE/UPDATE，跨网部署一定踩坑 |
| **RFC 3325 P-Asserted-Identity / Privacy** | ✅ 已实现（2026-05-16）| 入/出局 INVITE 都处理 PAI/Privacy；trust-domain 过滤。详见上方批次 1A |
| **TLS / TCP signaling transport** | ⚠️ 仅 UDP | 公网部署/合规审计场景必须 TLS；当前 outbound 全部 `net.ResolveUDPAddr("udp", …)` |
| **RFC 4474 / 8224 Identity（SHAKEN/STIR）** | ❌ 未实现 | 北美运营商已强制；不做就被标骚扰拦截 |
| **DTLS-SRTP 密钥协商** | ❌ 未实现 | 仅有 SDES-SRTP；WebRTC 互通必备 |

### 2.2 P1 缺口

| 特性 | 现状 | 用途 |
|---|---|---|
| **RFC 7044 History-Info** | ✅ 已实现（2026-05-16）| 转接溯源、呼叫追责。详见批次 1B |
| **RFC 5806 Diversion** | ✅ 已实现（2026-05-16）| 老式 PBX 转接信息。详见批次 1B |
| **RFC 3327 Path** / **RFC 3608 Service-Route** | ❌ | 多级 registrar/proxy 部署 |
| **RFC 3262 PRACK** | ✅ 已有 `invite_rfc3262.go` | — |
| **RFC 3891 Replaces** | ✅ 已有 | — |
| **RFC 6442 Geolocation** | ❌ | 紧急呼叫合规 |
| **T.30/T.38 fax pass-through** | ❌ | 企业传真 |
| **RFC 3711 SRTP MKI / 多 crypto suite** | ⚠️ 仅 AES_CM_128_HMAC_SHA1_80 | 与 Cisco/Avaya 互通时常需要 32 |
| **RFC 6079 Connectivity Preconditions** | ❌ | 与运营商网络协商 QoS |

### 2.3 编解码 / 媒体

| Codec | 现状 |
|---|---|
| PCMA / PCMU | ✅ |
| G.722 | ✅ |
| Opus（含 inband-FEC） | ✅（FEC 待复核） |

> **产品决策 2026-05-16**：G.729 / iLBC / AMR-WB / L16 / L24 **不接入**。现有 PCMA/PCMU/G.722/Opus 足以覆盖目标场景（互联网业务 + 主流运营商中继），额外 codec 不同价付出 transcoding CPU 成本。（注：`pkg/sip/bridge/two_leg_relay.go` 以 L16 作为 raw RTP relay 识别器，不是外向协商提议，保留不动）。

---

## 三、媒体 / RTP / 录音 工程改进项

| 项目 | 现状 | 建议 |
|---|---|---|
| **RTCP SR/RR 上报** | 仅 RTP，不发 RTCP | 实现 SR/RR + 公布 jitter/loss/RTT；运营商/SBC 会以此判定我们是不是合规 UA |
| **RFC 4585 RTCP-FB / NACK** | ❌ | Opus over UDP 时丢包重传 |
| **DTMF 兼容性** | RFC 2833（telephone-event）✅；INFO body ⚠️；in-band tone detect ❌ | 老 PSTN 网关常 SIP INFO 推 DTMF；有些客户的 IVR 还发 in-band tones |
| **音频抗混叠 LPF** | ❌ — 我们没有真正的低通滤波器 | 上次电流音事件就是因为 16k→8k 缺 LPF；当前用"云端直出 8k"绕过，但 FreeSWITCH/Asterisk 互通若返回非桥接率 PCM 会重现。建议补一个 31-tap halfband FIR |
| **录音 SN3 → S3 流式上传** | 进程结束时一次性 upload | 大于 1 小时的通话内存压力大；建议 S3 multipart upload + 落盘临时文件兜底 |
| **录音格式可选 Opus/AAC** | ❌ — 只有 16-bit PCM WAV | WAV 1 小时通话 ~115 MB，Opus 32kbps 仅 ~14 MB |
| **抖动缓冲自适应** | `JitterPlaybackDelay = DefaultJitterPlaybackDelay` 静态 | 加自适应：根据 RTP 包到达间隔标准差动态调整深度 |
| **VAD 级联** | RMS 阈值 | RMS + 自适应噪声底；或集成 webrtcvad/silero |

---

## 四、可观测 / 运维

| 项目 | 现状 | 建议 |
|---|---|---|
| **OpenTelemetry trace 贯穿 SIP→ASR→LLM→TTS** | 部分日志 | 一次通话生成一条 trace_id，覆盖 INVITE→200OK→ASR partial→LLM token→TTS first byte→BYE |
| **Prometheus metrics（call_setup_ms / asr_first_partial_ms / tts_ttfb_ms / barge_in_count / packet_loss_pct）** | 无统一 metrics | 必备运营指标 |
| **Call Detail Records（CDR）** | sip_calls 表 | 加 `setup_time_ms`, `pdd_ms`（Post-Dial Delay）、`mos_estimate`（基于 jitter+loss 推算 MOS）字段 |
| **健康检查端点** | server.go 有 health？ | 区分 liveness / readiness；readiness 应反映 SIP 监听 socket 可达性、Redis、DB |
| **配额 / 限流** | 无 | 单租户 RPS / 并发通话 hard cap；防 LLM 成本失控 |
| **审计日志** | 无 | 谁触发了外呼、谁查看了录音 |
| **优雅降级** | LLM 挂了直接错 | LLM 不可用时回退到固定 TTS 应答；TTS 不可用时 fallback wav |

---

## 五、安全 / 合规

| 项目 | 现状 | 建议 |
|---|---|---|
| **Digest auth nonce 防重放窗口** | 已有但 nonce 时效未深度审计 | nonce 30s 滑动窗口 + nc 计数器校验 |
| **SDP 注入校验** | 部分校验 | INVITE Body 走 strict SDP parse，拒绝多 m= 段、奇怪 c= |
| **SIP fuzzing 防御** | 基础 | 长 header / 畸形 URI 单测覆盖 |
| **录音加密** | S3 服务端加密 | 增加客户侧加密 key（KMS envelope）防 S3 桶访问泄漏 |
| **PII 脱敏** | ASR 文本入库明文 | 手机号 / 身份证可配置正则替换为 `***` 后入库 |
| **TLS-SRTP 端到端** | 仅 SDES | 公网通话必须 DTLS-SRTP |
| **抗 SPIT / robocall** | 无 | 主叫频次/黑名单/呼叫指纹检测 |

---

## 六、用户体验 / 对话质量

| 项目 | 现状 | 建议 |
|---|---|---|
| **Barge-in 灵敏度** | RMS 阈值固定 | 自适应 SNR + 持续 N 帧才 barge-in，避免咳嗽误触发 |
| **TTS 段间衔接** | 已用 Pipeline 整体节拍 | 段间增加 50-100ms 自然停顿用于呼吸感 |
| **ASR 热词动态注入** | `SIPHotwordCorrector` 静态 | 通话上下文动态拉取（如 LLM 生成预期下一轮词汇喂给 ASR） |
| **LLM 流式 → TTS 流式** | partial 已串联 | Token 级流式 + sentence boundary detector，让首字延迟更短 |
| **Echo cancellation** | 无 | SIP 桥接到 Web 坐席时回声很常见；AEC 模块 |
| **多语言自动切换** | 单 ASR 模型 | 检测到非中文片段动态切换 ASR 引擎 |

---

## 七、推荐落地顺序

> 按 **影响 × 可控成本** 排序。

### Sprint 1（互操作性硬门槛）
1. **RFC 4028 Session Timer**（P0，长时通话稳定性）
2. **TLS signaling transport**（P0，公网/合规）
3. **RTCP SR/RR**（P0，运营商互通）
4. **G.729 codec**（国内 PBX 互通）

### Sprint 2（生产可观测）
5. **Prometheus metrics 全套**
6. **OpenTelemetry trace 全链路**
7. **CDR 增强（PDD / MOS）**

### Sprint 3（安全合规）
8. **DTLS-SRTP**
9. **P-Asserted-Identity / Privacy**
10. **录音 KMS envelope 加密**

### Sprint 4（体验）
11. **音频抗混叠 FIR + 自适应抖动缓冲**
12. **AEC**
13. **自适应 barge-in**
14. **LLM 流式 → TTS sentence-boundary 切片**

---

## 八、当前已修复但值得 cross-check 的点

- **2026-05-16 入库录音电流音**：根因是 `@/Users/cetide/Desktop/LingEchoX/pkg/utils/sip_recording_wav.go:211 placeWallPCMTrack` 缺 jitter snap。已套上和 `pkg/voice/recorder` 同款 80ms snap。**注意**：还有几个相邻文件用 wall-clock 网格贴帧，建议复核：
  - `@/Users/cetide/Desktop/LingEchoX/pkg/utils/sip_recording_wav.go:428 decodeG722LegWallTimeline` → 也调 `placeWallPCMTrack`（已修）
  - Opus / G.722 leg 同样路径已自动获益
- **TTS 16k→8k 降采**：目前**靠云端直出 8k 绕过**。若将来需要从云端 16k 输出降采到 8k 桥接率（如换 voice 引擎），必须先实现 31-tap halfband FIR，否则会回归 2026-05-16 那次的 4-8 kHz 混叠问题。

---

## 九、文件级 TODO 索引

> 当真正开工时，先打开这些文件作为切入点。

```
RFC 4028:        @/Users/cetide/Desktop/LingEchoX/pkg/sip/server/invite_rfc3262.go (旁路实现)
TLS transport:   @/Users/cetide/Desktop/LingEchoX/pkg/sip/server/sip_server.go
                 @/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/manager.go
RTCP:            @/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/transport.go
G.729:           @/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/presets.go (codec 注册)
Anti-alias FIR:  @/Users/cetide/Desktop/LingEchoX/pkg/media/                (resample.go)
Metrics:         @/Users/cetide/Desktop/LingEchoX/pkg/sip/server/sip_server.go
CDR:             @/Users/cetide/Desktop/LingEchoX/pkg/sip/persist/call_store.go
DTLS-SRTP:       @/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/sdp.go
                 @/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/transport.go
```
