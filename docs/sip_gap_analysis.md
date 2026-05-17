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
| 1C. RFC 4028 Session-Timer | ✅ done (2026-05-17) | `@/Users/cetide/Desktop/LingEchoX/pkg/sip/session_timer/`（新包，Parse / Format / Negotiate 纯逻辑）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/session_timer.go`（UAS 接入）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/session/call_session.go:180-237`（看门狗 Arm/Touch/Stop）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/sip_server.go:665-673`（422出口）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/reinvite.go:23-27`（re-INVITE 触摸）+ `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/extra_handlers.go:33-40`（UPDATE 触摸）| 单测 27 个通过（session_timer 17 + watchdog 7 + outbound buildINVITE 3）。UAS 侧完整（协商、422 too-small、200 OK 回写、看门狗 BYE、re-INVITE/UPDATE 重置）；UAC 侧只发 `Supported: timer` 不提 `Session-Expires`（当前依赖对端主动刷新）。**Follow-up（1C-2）**：UAC-side refresher（主动发 in-dialog UPDATE 刷新）需要先建设 in-dialog UAC 请求机制，待后续 |

> ~~IPv6 dual-stack~~：**不做**（产品决策 2026-05-16）。运营商 SBC + 企业内网 未来 2-3 年依然以 IPv4 为主，额外补 IPv6 收益低。

### 批次 2：传输层升级

> **2026-05-17 重大修正**：入局 TCP/TLS 听器实际上**已经在 `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/tcp_sig.go` 中实现**（`SIP_TCP_PORT` / `SIP_TLS_LISTEN` env 开启），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/stack/message.go:304-340` 也用 Content-Length 实现了 RFC 3261 §18 帧拆包。剩余缺口集中在出局侧。

| Item | 状态 | 落地点 | 验收 |
|---|---|---|---|
| 2A-in. 入局 TCP/TLS 监听 | ✅ 已实现（早期代码，2026-05-17 被重新发现）| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/tcp_sig.go`（listenTCP / listenTLS / runOneTCPConn / dispatchSignalingRequestTCP）、`@/Users/cetide/Desktop/LingEchoX/pkg/sip/stack/message.go:ReadMessage`（RFC 3261 §18 帧拆包）、`@/Users/cetide/Desktop/LingEchoX/pkg/sip/stack/endpoint.go:DispatchRequest`（同一 handler 路径）| `SIP_TCP_PORT=5060 ./voiceserver` 启动 TCP；`SIP_TLS_LISTEN=:5061 SIP_TLS_CERT_FILE=cert.pem SIP_TLS_KEY_FILE=key.pem` 启动 TLS。与 UDP 共享同一 INVITE/ACK/BYE handler |
| 2A-out. 出局 TCP/TLS 拨号 | ✅ done (2026-05-17) | **Slice 1**：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/transport.go`（Transport enum + URI param 解析 + ResolveTransport 优先级 + Via token），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/types.go:40-49`（DialTarget.Transport 字段）。**Slice 2**：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/peer.go`（signalingPeer 接口 + udpPeer / connPeer for TCP+TLS + 5s 写超时 + 90s 读超时 + 15s TCP keep-alive），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/pool.go`（per-target 连接池 RFC 5923 + 5min 空闲清扫 + EOF 自动驱逐 + 并发 dial 去重 + TLS ServerName 自动填充），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/manager.go`（Dial → ResolveTransport → pool.Get → peer.Send；outLeg.peer + sendOnPeer 接管 ACK/BYE），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/invite.go:formatVia`（Via 头按 transport 渲染，buildINVITE/ACK/BYE 共用），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/manager.go:166-173`（ManagerConfig.TLSConfig，nil 默认严格验证）| 单测 14 个通过（transport 选择 6 + peer TCP send+回应路由 / TLS 自签握手 / send-after-close / UDP per-call 不池化 / TCP 同目标连接复用 / EOF 自动驱逐 / Via transport token 4 case）。决策落地：URI `;transport=` > trunk 配置 > UDP；TCP/TLS per-target 池化；TLS 默认严格 verify (`tls.Config{ServerName: dialHost}`)，调试侧通过 `ManagerConfig.TLSConfig` 注入 `InsecureSkipVerify=true`（不开 env 后门避免生产事故） |
| 2B. TLS 证书错互认证 / SRV 发现 | ⏳ pending | 证书互认证；`_sip._tls.example.com` SRV 记录查询 | 入局看孢证书反向验证；出局能从 SRV 选 transport |

### 批次 3：互通与合规（预估 10-17 工作日）

| Item | 状态 | 落地点 | 阻塞条件 |
|---|---|---|---|
| 3A. DTLS-SRTP | 🟡 in-progress（slice A-1 + A-2 done 2026-05-17）| **A-1 SDP 协商**：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/dtls.go`（DTLSRole + AnswerRole RFC 5763 §5 / Fingerprint format+parse RFC 8122 / IsDTLSTransport 识别 UDP/TLS/RTP/SAVP/SAVPF），`@/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/sdp.go:Info` 增加 `Fingerprints` + `DTLSRole` 字段并接入 Parse。**A-2 DTLS 握手 + 密钥派生**：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/dtls.go`（`IsDTLSPacket`/`IsRTPPacket` first-byte 多路复用判断 RFC 5764 §5.1.2；`SelfSignedDTLSCert` per-call 30 天自签 P-256；`FingerprintSHA256` 渲染 SDP 用 colon-hex；`DTLSEndpoint` 跑 pion/dtls/v2 握手；`SRTPKeys.AsLocalRemote` 按 client/server 角色映射；`Session.EnableDTLSSRTP` 与现有 SDES 路径同质装载 srtp.Context）。**待办（A-3）**：把 DTLSEndpoint 接入 `Session` 读循环做单 socket 多路复用（RFC 5764 §5.1）+ outbound/inbound INVITE SDP offer/answer 渲染 fingerprint+setup | 单测 19 个通过：SDP 10 个（DTLSRole 状态机 / fingerprint round-trip + 弱哈希拒绝 + 大小写规范化 / `Parse` 提取 fingerprint+setup / SDES 不误识别为 DTLS）+ RTP 9 个（多路复用 first-byte / 自签 cert + 95-char SHA-256 fingerprint / 角色映射 client⇄server / **真实端到端 DTLS 握手 + 双方派生 client/server SRTP key+salt 字节级一致性 + 角色互补**）|
| 3B. RFC 8224 STIR/SHAKEN | ✅ done (2026-05-17) | **核心库**：新包 `@/Users/cetide/Desktop/LingEchoX/pkg/sip/stir/`（`passport.go` RFC 8225 PASSporT JWS sign/verify + ES256 严格校验 + ASN.1/JWS 双签名兼容；`header.go` RFC 8224 Identity 头 format+parse + 兼容 RFC 4474 老 quoted 形式；`keys.go` PEM/PKCS8/SEC1 ES256 密钥加载；`fetcher.go` x5u HTTPS fetch + RFC 8226 链验 + 1h TTL 缓存 + 1m 负缓存 + 64KB 防 DoS；`verifier.go` 端到端编排 parse → fetch → 验签 → info=x5u 一致性 → iat 60s 重放窗口 → From TN/URI 比对 → attest 过滤 → 5 种 Verdict 映射 SIP 437/438/403）。**outbound 接入**：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/stir.go`（`STIRSigner` 配置 + `Sign` 输入校验 + 软失败语义；`extractTNFromRequestURI`；`signOutboundIdentity` 走 buildINVITE 写 `Identity:` 头），`ManagerConfig.STIRSigner` opt-in（默认 nil = 不签）。**inbound 接入**：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/stir.go`（`STIRConfig` 失败策略：默认软警告 + OnVerdict 钩子 + 可选硬拒绝 437/438/403；`verifyInboundIdentity` 在 Session-Timer 协商之后、SDP 解析之前调用）；`SIPServer.SetSTIRConfig` 安装。设计决策：失败默认软告警（不拦截呼叫中心流量），STI-CA 信任根通过 `Verifier.Fetcher.RootCAs` 注入，TLS 配置不开 env 后门避免事故 | 单测 58 个通过：核心库 38 个 + outbound 9 个（签名 round-trip / 非 E.164 拒绝 / "+1 (555) 123-4567" 规范化 / metrics hook 不计输入校验错 / `extractTNFromRequestURI` 6 case / `buildINVITE` Identity 头注入与省略）+ inbound 11 个（默认禁用 / 有效头通过 + OnVerdict 触发 / 缺失头软接受 + 硬拒绝 / 篡改签名软接受 + 硬拒绝 / TN 不符硬拒绝 / OnVerdict 钩子 panic 安全 / `extractFromTN` + `extractURIFromFromHeader` 处理 7 种 From 头形态） |

> 每批落地后回到本节打 ✅ + 加 commit 链接 / 测试结果。

---

## 二、SIP 协议层缺口（按 RFC 检查）

> 标注 P0 = 影响互操作性 / 上线门槛；P1 = 增强 / 可选；P2 = 长尾

### 2.1 P0 缺口

| RFC / 特性 | 现状 | 影响 |
|---|---|---|
| **RFC 4028 Session Timer**（`Session-Expires` / `Min-SE`） | ✅ 已实现（2026-05-17）| **UAS-refreshee**：协商 / 422 too-small / 200 OK 回写 / re-INVITE+UPDATE 重置看门狗 / 过期自动 BYE（Reason: SIP;cause=408）。**UAC-refresher**（2026-05-17 新增）：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/refresher.go` 在 2xx OK 含 `refresher=uac` 时启动 SE/2 间隔的 in-dialog UPDATE（RFC 3311，无 SDP），处理 200/422/481/transient 错误；422 自动按 peer Min-SE 提升 SE 并重试一次；481 / 角色被 swap 为 uas 时停泵；`cleanupLeg` 同步停止避免 goroutine 泄漏。13 个单测覆盖。详见批次 1C |
| **RFC 3325 P-Asserted-Identity / Privacy** | ✅ 已实现（2026-05-16）| 入/出局 INVITE 都处理 PAI/Privacy；trust-domain 过滤。详见上方批次 1A |
| **TLS / TCP signaling transport** | ✅ 已实现（2026-05-17）| 入局：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/tcp_sig.go` 通过 `SIP_TCP_PORT` / `SIP_TLS_LISTEN` 听器；出局：`@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/{transport,peer,pool}.go` 提供 signalingPeer + RFC 5923 连接池，按 `;transport=` URI param 或 trunk 配置路由。详见批次 2 |
| **RFC 4474 / 8224 Identity（SHAKEN/STIR）** | ✅ 已实现（2026-05-17）| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/stir/` 提供 PASSporT/Identity 头/x5u 证书链 + Verifier 编排；outbound 通过 `ManagerConfig.STIRSigner` opt-in 注入 `Identity:` 头；inbound `SIPServer.SetSTIRConfig` 钩子默认软告警 + 可选硬拒绝 437/438/403。58 个单测通过。详见批次 3B |
| **DTLS-SRTP 密钥协商** | ✅ 已实现（2026-05-17）| **SDP 协商** `@/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/dtls.go`（DTLSRole + AnswerRole / Fingerprint 解析渲染 / FormatFingerprintLine + FormatSetupLine / VerifyDTLSCertFingerprint RFC 5763 §3 共享验证器）+ `Info.Fingerprints/DTLSRole` 接入 Parse。**握手 + 密钥派生** `@/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/dtls.go`（pion/dtls/v2 + SelfSignedDTLSCert + RFC 5764 §4.2 EXTRACTOR-dtls_srtp 派生 + PeerCertificates 暴露）。**单 socket 多路复用** `@/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/dtls_session.go`（dtlsConn + Session.StartDTLS + RFC 5764 §5.1.2 first-byte demux）。**inbound 接入** `@/Users/cetide/Desktop/LingEchoX/pkg/sip/server/dtls.go`（`SIPServer.SetInboundDTLSAccept` opt-in + `prepareDTLSAnswer` 在 handleInvite SDP 后接入 `a=fingerprint`+`a=setup:passive` 渲染 + `runInboundDTLSHandshake` 在 handleAck 后 goroutine 跑握手 + fingerprint 验证失败 BYE）。**outbound 接入** `@/Users/cetide/Desktop/LingEchoX/pkg/sip/outbound/dtls.go`（`DialRequest.OfferDTLSSRTP` opt-in + `prepareOutboundDTLSOffer` 在 Manager.Dial 选 UDP/TLS/RTP/SAVP + `a=setup:actpass` + `startOutboundDTLSHandshake` 在 2xx 后跑握手 + 安装 SRTP）。51 个单测通过（含真实端到端两 Session 握手 + 加密 RTP 双向 + SDP 渲染往返 + 仿真 MITM 拒绝）|

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
| **RFC 3711 SRTP 多 crypto suite** | ✅ 已实现（2026-05-17）| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/sdp/crypto.go` 增加 `SuiteAESCM128HMACSHA132` + `PionProfileForSuite` + `PickSupportedSDESOffer`（首选 _80，回落 _32）；`@/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/srtp_session.go` 提供 `EnableSDESSRTPWithProfile` 显式选 profile；inbound INVITE / re-INVITE / outbound 200 OK answer 全部走多套件路径；Cisco CUCM / Avaya 在 answer 把 _80 降级成 _32 不再误拒。新增 12 个单测覆盖偏好顺序、向后兼容、降级回归 |
| **RFC 3312 / 4032 Preconditions（QoS / 安全 / 连通性）** | ❌ | 与运营商网络协商 QoS／资源预留，3GPP IMS 必备。原表此处误标为 RFC 6079（HIP BONE，overlay 网络实验 RFC，与 QoS 无关）—— 2026-05-17 已修正 |

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
| **RTCP SR/RR 上报** | ✅ 已实现（2026-05-17）| `@/Users/cetide/Desktop/LingEchoX/pkg/sip/rtp/rtcp.go`：自动在 RTP-port+1 绑配套 socket（best-effort，bind 失败不影响通话）；RFC 3550 §6.2 / §6.3.1 间隔（5s ± 0.5x 随机化）发送复合 [SR\|RR]+SDES(CNAME) 包；§A.1 序号回卷追踪 + §A.3 fraction-lost 增量计算 + §A.8 jitter EWMA；接收 RR 后用 LSR/DLSR 算 RTT 并暴露 `Session.RTCPSnapshot()`（`PeerSeenRR/RTTMs/PeerJitter/PeerLossFraction/LocalJitter`）。SSRC mid-call swap（re-INVITE/transfer）自动重置 jitter 累加避免毛刺。16 个单测覆盖。RTCP-MUX(RFC 5761) 后续做 |
| **RFC 4585 RTCP-FB / NACK** | ❌ | Opus over UDP 时丢包重传 |
| **DTMF 兼容性** | RFC 2833（telephone-event）✅；INFO body ⚠️；in-band tone detect ❌ | 老 PSTN 网关常 SIP INFO 推 DTMF；有些客户的 IVR 还发 in-band tones |
| **音频抗混叠 LPF / 抗镜像 LPF / DC-block HPF** | ✅ 已实现（2026-05-17）| `@/Users/cetide/Desktop/LingEchoX/pkg/media/lowpass.go`：① **下采样抗混叠** `newDownsamplingLowPass`，31-tap Hamming 窗 sinc，cutoff = 0.45×target/source，pre-filter（解决之前 16k→8k 电流音事件）；② **上采样抗镜像** `newUpsamplingAntiImagingLowPass`，对偶设计 cutoff = 0.45×source/target，post-filter（覆盖未来 16k→48k Opus 路径）；③ **DC-block HPF** `DCBlockHPF`，一阶 IIR，corner ≈ 30 Hz，与重采样正交、清除 PSTN 链路常见的 DC 偏置和工频纹波。所有滤波器跨 chunk 保持状态、unity DC gain、int16 饱和截断不卷绕。15 个单测覆盖（passband / stopband / 跨块状态 / 饱和 / nil-safe）。集成在 `InterpolatingConverter` 两个 factory 中 |
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
