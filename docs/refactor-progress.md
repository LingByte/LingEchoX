# LingEchoX 重构进度

参见 `docs/refactor-rfc.md` 总体设计。

## 已完成

### PR-A：G.711-only 政策 + webseat codec 修复
- `pkg/media/encoder/registry.go`：Opus 注册注释掉；保留 PCMA/PCMU/PCM/G722。
- `pkg/media/encoder/opus.go`：整体加 `//go:build opus_codec` build tag，默认构建排除。
- `pkg/sip/webseat/hub.go::mediaFromRemoteTrack`：删除 opus 分支，所有未识别 mime 默认 PCMA 8kHz mono。
- `pkg/media/encoder/registry_test.go`：新增。`TestRequiredCodecsRegistered` 锁住 PCMA/PCMU/PCM/G722；`TestOpusIntentionallyUnregistered` 守护"G.711 only"政策；`TestCreateEncodeDecodeForBridgeCodecs` 验证三类编解码器工厂可用。
- 根治日志里的 `bridge: encode agent: codec not supported`。

### PR-1：`pkg/sip/callid` 包 + 收口现有 Call-ID 工具
- 新增 `pkg/sip/callid/callid.go`、`dialogid.go`、`callid_test.go`。
- `pkg/sip/outbound/invite.go::newCallID` → 委托 `callid.NewLocal`。
- `pkg/sip/conversation/transfer_bridge.go::normCallID` / `bridgeCallLocalPart` → 委托 `callid` 包。
- 行为变更：**零**。
- 验收：`go build` / `go vet` / `go test ./pkg/sip/callid/...` 全绿。

### PR-2：`pkg/sip/callreg` 中心注册表（数据结构 + 测试）
- 新增 `pkg/sip/callreg/registry.go`、`registry_test.go`。
- 提供 `StartTransfer` / `BindOutbound` / `RekeyOutbound` / `SetBridge` / `SetPhase` / `Lookup` / `LookupByDialog` / `End` / `RunGCOnce` / `Close`。
- 行为变更：**零**（未被任何 caller 引用）。
- 验收：`go test -race ./pkg/sip/callreg/...` 全绿，含生产 bug 回归 `TestLookup_SBCHostRewrite` 与并发测试。

### PR-C：Codec 偏好统一为 G.722 → PCMA → PCMU
- `pkg/sip/sdp/presets.go`：`DefaultOutboundOfferCodecs` + `TransferAgentBridgeOfferCodecs` 重排序为 G.722, PCMA, PCMU, telephone-event；删除 opus；并加文档头说明项目级 codec 政策。
- `pkg/sip/session/negotiate.go::NegotiateOffer` + `pkg/sip/session/call_session.go::NewCallSession`：偏好 map 改为 `{g722:0, pcma:1, pcmu:2}`，去掉 opus。
- `pkg/sip/webseat/hub.go::newMediaEngine`：注册 G.722 (PT=9) 在 PCMA/PCMU 之前。
- `pkg/sip/webseat/hub.go::mediaFromRemoteTrack`：新增 G.722 分支（SampleRate=16000，BitDepth=16）；任何未识别 mime 仍 fallback 到 PCMA。
- 整网音质提升：常态从 G.711 8kHz 升到 G.722 16kHz（碰到老 PSTN 仍能 fallback 到 PCMA）。

### PR-B：Phase 2 转接预拨号（latency -1~2s）
- `pkg/sip/conversation/transfer.go`：
  - 新增 `PrepareTransferToAgent`（送 INVITE，不放铃音）。
  - 新增 `CompleteDeferredTransferRinging`（仅启动铃音 + UI phase=ringing；未预拨情况下回落到完整流程）。
  - 旧 `TriggerTransferToAgent` 保留（语义不变），内部走 `triggerTransferImpl(..., startRing=true)`。
  - 新增 `transferPrepared sync.Map` 跟踪预拨态。
- `pkg/sip/voicedialog/loopback.go`：LLM 意图识别瞬间调 `PrepareTransferToAgent`，TTS 同时合成"正在转接"。
- `pkg/sip/voicedialog/gateway_media.go::handleTTSSpeak`：TTS 结束后调 `CompleteDeferredTransferRinging` 而非 `TriggerTransferToAgent`。
- 预期收益：LLM 意图识别 → outbound INVITE 落网延迟从约 4s 缩到 < 100ms；坐席侧总响铃时间提早 1–2 秒。
- 顺手修：`pkg/sip/voicedialog/hub_test.go` 把废弃的 `wsTokenOK` 改为现存的 `WebSocketTokenOK`，让该包 `go test` 重新可跑。

### PR-3：shadow 模式接入 transfer_bridge ✅
- 新增 `pkg/sip/conversation/callreg_shadow.go`：`SetCallRegistry`、`parseCallregMode`、`shadowStartTransfer` / `shadowEndTransfer` / `shadowRekeyOutbound` / `shadowCompareActive`。env `CALLREG_MODE=off|shadow` 控制（默认 shadow）。
- `transfer_bridge.go` 写点旁路注册：
  - `StartTransferBridge` → `shadowStartTransfer`（含 BindOutbound + SetBridge + SetPhase(Connected)）
  - `MigrateTransferBridgeOutboundCallID` → `shadowRekeyOutbound`（顺便实现了 RFC 中 PR-4）
  - `HangupTransferBridgeIfAny` / `HangupTransferBridgeFull` / `TeardownTransferBridgeOnOutboundRemoteByeFallback` → `shadowEndTransfer`
- 读点比对：`ActiveTransferBridgeForCallID` 内 registry vs legacy 不一致时 WARN 一次/Call-ID（后续重复 DEBUG）；返回值保持 legacy。
- 装配：`internal/sipserver/sipapp.go` 启动时 `conversation.SetCallRegistry(callreg.New(callreg.Options{TTL: 10min, GCEvery: 30s}))`。
- 新增 `callreg_shadow_test.go`：覆盖 disabled-when-no-registry、mirror-writes、SBC-rewrite-rekey、disabled-by-mode、end-with-outbound-lookup 五个场景。
- **注意**：PR-3 已经覆盖了 RFC 原本的 PR-4 (`adoptOutboundDialogCallIDIfNeeded` 同步 Registry)，因为 outbound manager 通过 `OnDialogCallIDAdopted` 调 `MigrateTransferBridgeOutboundCallID`，而该函数已加 `defer shadowRekeyOutbound`。

### PR-5a：callreg primary 读模式可用 ✅
- `pkg/sip/callreg/registry.go::Entry`：新增 `InboundSession any` + `OutboundSession any` 字段；新增 `SetSessions(inbound, inS, outS any)` 方法。
- `pkg/sip/conversation/callreg_shadow.go`：
  - `parseCallregMode("primary")` 不再被 clamp 到 shadow；新增 `callregPrimary()` + `callregLookupActive()` 辅助。
  - `shadowStartTransfer` 签名增加 `inS, outS any` 参数，写入时一并 `SetSessions`。
- `pkg/sip/conversation/transfer_bridge.go::ActiveTransferBridgeForCallID`：
  - shadow 模式：仍 legacy 为真，registry vs legacy 不一致时 WARN。
  - **primary 模式**：返回 `legacyHit || regHit`，**任一命中即认为桥接存在**。这就直接修了原始 bug：SBC 改写 Call-ID host 后 BYE 来查，legacy map miss（仅靠 local-part 兜底匹配），registry 通过 local-part 索引能命中 → 转人工挂死场景消失。
  - 调用点 `StartTransferBridge` 现在传 `inbound, outboundCS` 给 shadow，registry 拿到完整 session 引用。
- 新增 `TestPrimaryMode_ReturnsRegistryHitWhenLegacyMisses` 测试覆盖 SBC drift 场景。
- 默认仍是 shadow；运维设 `CALLREG_MODE=primary` 即切。零部署回退路径：设 `CALLREG_MODE=shadow` 回去。
- 验收：`go test ./pkg/sip/conversation/... ./pkg/sip/callreg/...` 全绿。

### PR-5b：Hangup 路径在 primary 模式下从 registry 兜底 ✅
- `pkg/sip/conversation/transfer_bridge.go`：
  - 新增 `lookupRegistryBridge(callID) *transferBridgeState`：从 registry Entry 重建 `*transferBridgeState`。类型断言 `entry.Bridge` 为 `legBridge`、`entry.InboundSession/OutboundSession` 为 `*sipSession.CallSession`。`entry.OutboundLegs` 取最后一项（最近一次 RekeyOutbound 后的 Call-ID）。
  - `HangupTransferBridgeIfAny` / `HangupTransferBridgeFull`：legacy `findBridgeStateUnlocked` 未命中 + `callregPrimary()` → 回落到 `lookupRegistryBridge`。日志新增 `lookup_source=legacy|registry` 字段。
- 新增测试 `TestPrimaryMode_HangupRecoversFromSBCDrift` 直接复现原 bug 场景。
- **效果**：CALLREG_MODE=primary 下，SBC 改写 Call-ID host 导致 legacy 找不到的 BYE 现在能正常 teardown，**A leg 不再被晾着**。
- legacy `bridges` map **保留**，仍是 fallback；正式删除留给 PR-5c（等 primary 跑稳定后）。

## 下一步

### PR-5c：删除 legacy `bridges` map（可选）
**前置**：primary 模式生产跑稳定 1–2 周，`lookup_source=legacy` vs `registry` 的比例稳定。

计划：
1. 移除 `bridges sync.Map` 与 `findBridgeStateUnlocked`。
2. `transferBridgeState` 变成 lookupRegistryBridge 的局部返回值。
3. 拆 `transferStarted` / `transferRingStop` / `transferNoAgentRetry` 三个 sync.Map → 写到 registry.Entry 上。
4. 默认 `CALLREG_MODE=primary`，shadow / off 仅作 emergency rollback 留存。

### PR-6：拆 `pkg/sip/server/sip_server.go`（1353 行 → 按动词拆）
参见 RFC §2 Phase 5。可与 PR-5 并行。

### PR-7：拆 `pkg/sip/conversation` → `pkg/contactcenter/*` + `pkg/voice/*`
参见 RFC §2 Phase 5。涉及大量 import 路径改动，保留 deprecation shim 一个 release。

参见 RFC `docs/refactor-rfc.md` §2 Phase 1 与 §7。

## Phase 2+（其它阶段）

按 RFC §2：
- Phase 2 转接路径并行化 — ✅ 已通过 PR-B 完成。
- Phase 3 codec 桥接统一 — ✅ 已通过 PR-A + PR-C 完成（G.711+G.722 全链路，Opus 永久禁）。
- Phase 4 config 收口（227 处 GetEnv）。
- Phase 5 拆包。
- Phase 6 handler → service 分层。
- Phase 7 可观测性 + 前端整改。
