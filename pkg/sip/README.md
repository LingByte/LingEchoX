# pkg/sip - SIP 电话协议栈

LingEchoX 的嵌入式 SIP 协议栈，提供从信令到媒体处理、再到 AI 语音对话的完整链路。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              应用层 (internal/sipserver)                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                              业务编排层                                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐   │
│  │ conversation│  │   outbound  │  │  voicedialog │  │      webseat       │   │
│  │  (AI 语音)   │  │  (外呼/转接) │  │ (HTTP/WS桥)  │  │   (WebRTC座席)    │   │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────────┬───────────┘   │
├─────────┴────────────────┴────────────────┴──────────────────┴───────────────┤
│                              会话管理层                                        │
│       ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │
│       │   session   │  │   dialog    │  │ transaction │                    │
│       │ (呼叫/媒体)  │  │  (对话管理)  │  │  (事务状态)  │                    │
│       └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                    │
├──────────────┴────────────────┴────────────────┴───────────────────────────┤
│                              信令与媒体层                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   server    │  │    stack    │  │   protocol  │  │    uas/bridge       │  │
│  │  (SIP服务器) │  │ (SIP协议栈)  │  │ (底层协议)  │  │ (UAS/两路桥接)      │  │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────────┬───────────┘  │
├─────────┴────────────────┴────────────────┴──────────────────┴──────────────┤
│                              媒体处理层                                        │
│       ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│       │    rtp      │  │    sdp      │  │    dtmf     │  │    vad      │    │
│       │  (RTP传输)   │  │  (SDP协商)   │  │ (DTMF处理)   │  │ (语音活动检测)│    │
│       └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘    │
├─────────────────────────────────────────────────────────────────────────────┤
│                              持久化层                                          │
│                    ┌─────────────┐  ┌─────────────┐                          │
│                    │   persist   │  │   siputil   │                          │
│                    │ (用户/通话)  │  │  (工具函数)  │                          │
│                    └─────────────┘  └─────────────┘                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 模块详解

### 1. 信令层

#### `protocol/` - 底层 SIP 协议
- **职责**: UDP 数据包收发、SIP 消息解析/序列化
- **关键文件**: `server.go` (UDP服务器), `events.go` (事件定义)
- **特点**: 最底层，无状态，仅处理字节流与消息结构转换

#### `stack/` - SIP 协议栈
- **职责**: RFC 3261 风格的消息解析、端点抽象、请求分发
- **关键文件**: 
  - `endpoint.go`: 核心端点，管理监听与请求路由
  - `message.go`: SIP 消息结构（请求/响应）
  - `transport.go`: 传输层抽象
  - `cseq.go`: CSeq 生成器
- **特点**: 无全局可变状态，通过结构体配置；支持 `OnResponseSent` 回调

#### `transaction/` - 事务层
- **职责**: SIP 事务状态管理（UAC/UAS）
- **关键文件**:
  - `invite_client.go`: INVITE 客户端事务
  - `invite_server.go`: INVITE 服务端事务
  - `noninvite_client.go/noninvite_server.go`: 非 INVITE 事务
  - `cancel.go`: CANCEL 处理
  - `ack.go`: ACK 处理
- **核心概念**: 
  - `InviteTransactionKey`: 通过 branch + Call-ID 标识事务
  - `BeginInviteServer/ClearPendingInviteServer`: 服务端事务生命周期

### 2. 服务器层

#### `server/` - SIP 服务器
- **职责**: UAS（User Agent Server）逻辑，处理入站呼叫
- **关键文件**:
  - `sip_server.go`: 主服务器，处理 INVITE/ACK/BYE/OPTIONS/REGISTER/CANCEL/INFO/Publish/Prack
  - `registrar.go`: REGISTER 处理与 SIP 用户注册
  - `dialog.go`: 入站对话状态管理
  - `invite_rfc3262.go`: RFC 3262 可靠临时响应 (100rel/PRACK)
  - `digest_auth.go`: HTTP Digest 认证
  - `invite_policy.go`: INVITE 策略（速率限制、IP 白名单）
- **关键结构**: `SIPServer` - 包含 callStore（Call-ID → CallSession 映射）、regStore（注册存储）

#### `uas/` - UAS 事务绑定
- **职责**: 将事务层与 UAS 响应关联
- **关键文件**:
  - `server_tx.go`: 服务端事务管理
  - `attach_tx.go`: 事务绑定逻辑
  - `hooks.go`: UAS 钩子（OnResponseSent）

### 3. 会话层

#### `session/` - 呼叫会话
- **职责**: RTP 会话与媒体会话绑定
- **关键文件**:
  - `call_session.go`: `CallSession` 结构，管理 RTP ↔ MediaSession 桥接
  - `negotiate.go`: SDP 编解码协商（PCMA/PCMU/G722/Opus）
  - `medialeg.go`: 媒体腿管理
- **核心功能**:
  - 上行：RTP → 解码 → PCM → ASR
  - 下行：TTS PCM → 编码 → RTP
  - 录音：SN3 格式（支持 wallNs 时间戳）

#### `dialog/` - 对话管理
- **职责**: SIP 对话（Dialog）状态追踪
- **关键文件**:
  - `dialog.go`: 对话结构（Call-ID, local/remote tag, CSeq）
  - `registry.go`: 对话注册表
  - `tag.go`: Tag 生成器

### 4. 媒体层

#### `rtp/` - RTP 传输
- **职责**: RTP-over-UDP 会话管理
- **关键文件**:
  - `session.go`: RTP 会话（SSRC, SeqNum, Timestamp 管理）
  - `transport.go`: 传输层（发送/接收/镜像）
  - `rtp.go`: RTP 包头处理
- **特点**: 协议无关，时间戳增量由调用方提供

#### `sdp/` - SDP 协商
- **职责**: SDP 解析与生成
- **关键文件**: `sdp.go`
- **支持**: RTP/AVP, RTP/AVPF, a=rtpmap, 静态 payload type (0/8/9)
- **拒绝**: SRTP-only (SAVP/SAVPF)

#### `dtmf/` - DTMF 处理
- **职责**: 双音多频信号处理
- **关键文件**:
  - `rfc2833.go`: RTP  payload 方式 (telephone-event)
  - `sipinfo.go`: SIP INFO 方式 (application/dtmf-relay)
  - `processor.go`: DTMF 处理器

#### `vad/` - 语音活动检测
- **职责**: RMS 能量检测实现 barge-in（打断）
- **关键文件**: `detector.go`
- **配置**: `SIP_VAD_THRESHOLD` (默认 3200), `SIP_VAD_CONSEC_FRAMES` (默认 3)

### 5. 业务编排层

#### `conversation/` - AI 语音对话
- **职责**: ASR → LLM → TTS 管道编排
- **关键文件**:
  - `voice.go`: 核心管道（~1333 行），处理语音识别、LLM 对话、语音合成
  - `transfer.go`: 转人工逻辑
  - `transfer_bridge.go`: 转接桥接
  - `voice_intent_onnx.go`: ONNX 意图识别
  - `asr_state.go`: ASR 状态管理
  - `wav_playback.go`: WAV 文件播放
- **环境变量**: ASR_*, LLM_*, TTS_*, SIP_VAD_*, SIP_AI_HANGUP_PHRASES

#### `outbound/` - 外呼/出站
- **职责**: UAC（User Agent Client）逻辑，外呼活动管理
- **关键文件**:
  - `manager.go`: 外呼管理器，处理 Dial、BYE、Re-INVITE
  - `invite.go`: INVITE 构建
  - `ack.go/bye.go`: ACK/BYE 处理
  - `hybrid_script_*.go`: 混合脚本 LLM 路由
  - `acd_trunk.go`: ACD 中继线路
  - `transfer.go`: 转接目标解析
- **场景**: 外呼活动、AI 转人工、回拨

#### `voicedialog/` - HTTP/WebSocket 语音桥
- **职责**: 将 SIP 媒体桥接到 HTTP WebSocket 客户端
- **关键文件**:
  - `gateway_bridge.go`: 网关桥接核心
  - `hub.go`: WebSocket 连接管理
  - `gateway_events.go`: 事件定义
  - `dialog_audio.go`: 音频处理
  - `dialog_welcome.go`: 欢迎语播放
- **协议**: 双向 JSON over WebSocket (`tts.speak`, `asr.final`, `interrupt` 等)

#### `webseat/` - WebRTC 座席
- **职责**: 浏览器座席 WebRTC 接入
- **关键文件**:
  - `hub.go`: WebRTC 连接管理（~23KB）
  - `transport.go`: 传输层

### 6. 桥接层

#### `bridge/` - 两路桥接
- **职责**: SIP 两路呼叫桥接（转接场景）
- **关键文件**:
  - `two_leg.go`: 两路管理
  - `two_leg_relay.go`: RTP 中继

### 7. 持久化与工具

#### `persist/` - 数据持久化
- **职责**: SIP 用户与通话记录存储
- **关键文件**:
  - `sip_user.go`: SIP 用户（注册、在线状态）
  - `sip_call.go`: 通话记录（CDR）
  - `gorm_store.go`: GORM 后端
  - `json_store.go`: JSON 文件后端（开发模式）
- **模式**: `SIP_PERSIST=json` 启用文件存储，否则使用 GORM

#### `siputil/` - SIP 工具
- **职责**: 通用 SIP 辅助函数
- **关键文件**:
  - `cseq.go`: CSeq 解析
  - `pcm.go`: PCM 工具
  - `audio_env.go`: 音频环境变量

## 启动流程

```
cmd/server/main.go
    │
    ├─→ bootstrap.SetupDatabase() ──→ 数据库初始化
    │
    ├─→ sipserver.Start() ──────────→ 嵌入式 SIP 启动
    │       │
    │       ├─→ server.New() ────────→ SIPServer 创建
    │       ├─→ server.SetRegisterStore() ──→ 注册存储注入
    │       ├─→ server.SetCallPersist() ────→ 通话持久化注入
    │       ├─→ conversation.SetTransferDialer() ──→ 转接拨号器注入
    │       └─→ server.Start() ────→ UDP 监听开始
    │
    └─→ httpServer.ListenAndServe() ──→ HTTP API 启动
```

## 入站呼叫流程

```
SIP 设备
   │
   │ INVITE ─────────────────────┐
   │                             ▼
   │                    ┌─────────────┐
   │                    │   server    │
   │                    │ handleInvite│
   │                    └──────┬──────┘
   │                           │
   │                    ┌──────▼──────┐
   │  100 Trying        │ transaction │
   │◄───────────────────┤   层处理    │
   │                    └──────┬──────┘
   │                           │
   │                    ┌──────▼──────┐
   │  180 Ringing       │   session   │
   │◄───────────────────┤ 创建CallSession│
   │                    │  分配RTP端口  │
   │                    └──────┬──────┘
   │                           │
   │                    ┌──────▼──────┐
   │  200 OK (SDP)      │   server    │
   │◄───────────────────┤  构建响应   │
   │                    └──────┬──────┘
   │                           │
   │ ACK ──────────────────────►
   │                           │
   │                    ┌──────▼──────┐
   │                    │ conversation│
   │                    │ 或 voicedialog│
   │                    │  媒体管道启动  │
   │                    │ (ASR→LLM→TTS)│
   │                    └─────────────┘
```

## 代码问题与改进建议

### 当前问题

1. **代码重复**:
   - `conversation/` 与 `voicedialog/` 存在部分功能重叠（都处理 ASR/TTS 管道）
   - `protocol/` 和 `stack/` 的边界不够清晰

2. **依赖注入不够彻底**:
   - 部分模块仍使用全局变量（如 `logger.Lg`）
   - `conversation` 包的环境变量读取较分散

3. **测试覆盖不均**:
   - `stack/`, `transaction/` 有单元测试
   - `conversation/` 测试较少，主要依赖集成测试

4. **TODO/FIXME 遗留**:
   - `vad/detector.go`: 3 处待优化
   - `conversation/voice.go`: 2 处需要改进
   - `dialog/dialog.go`: 2 处边界情况处理

5. **文档不足**:
   - 包级文档较完善，但函数级注释不够
   - 缺少架构图和调用链说明

### 建议改进

1. **架构层面**:
   - 统一 `conversation` 与 `voicedialog` 的媒体管道抽象
   - 考虑将 `protocol/` 合并到 `stack/`，简化层级

2. **代码质量**:
   - 为 `conversation/` 添加单元测试，特别是 `voice.go`
   - 处理所有 TODO/FIXME
   - 增加接口抽象，减少包间直接依赖

3. **可观测性**:
   - 添加 Prometheus 指标（呼叫数、成功率、延迟）
   - 完善结构化日志字段

4. **配置管理**:
   - 将环境变量读取集中到 `pkg/config`
   - 支持配置文件热重载

## 如何入手

### 1. 快速理解（1-2 天）

```bash
# 阅读顺序
1. pkg/sip/stack/doc.go          # 了解协议栈定位
2. pkg/sip/server/sip_server.go   # 服务器主逻辑（前 100 行）
3. pkg/sip/conversation/voice.go  # AI 管道（前 100 行）
4. pkg/sip/rtp/session.go        # RTP 基础
5. pkg/sip/sdp/sdp.go            # SDP 协商
```

### 2. 调试启动（2-3 天）

```bash
# 启动 SIP 服务器
go run ./cmd/server \
  --sip-host=0.0.0.0 \
  --sip-port=6050 \
  --sip-local-ip=127.0.0.1

# 测试入站呼叫（使用 sipcmd 或类似工具）
sipcmd -u sip:1000@127.0.0.1:6050 -c sip:9999@127.0.0.1:6050
```

### 3. 修改扩展（按需）

| 需求 | 入口文件 | 关键函数 |
|------|----------|----------|
| 修改 SIP 消息处理 | `server/sip_server.go` | `handleInvite/handleBye` |
| 调整编解码优先级 | `session/negotiate.go` | `NegotiateOffer` |
| 修改 AI 对话逻辑 | `conversation/voice.go` | `AttachVoicePipeline` |
| 添加新外呼策略 | `outbound/manager.go` | `Dial` |
| 修改 WebSocket 协议 | `voicedialog/hub.go` | `handleWebSocket` |

### 4. 添加新 Provider

以添加新 ASR 为例：
1. 实现 `pkg/recognizer` 接口
2. 在 `conversation/voice.go` 的 `AttachVoicePipeline` 中添加分支
3. 添加环境变量配置（如 `ASR_NEW_PROVIDER_API_KEY`）

## 环境变量速查

| 变量 | 包 | 说明 |
|------|-----|------|
| `SIP_RTP_PORT` | server/session | 固定 RTP 端口 |
| `SIP_RTP_PORT_START/END` | server | RTP 端口范围 |
| `SIP_MEDIA_MAX_SECONDS` | session | 最大通话时长 |
| `SIP_VAD_THRESHOLD` | vad | VAD 阈值 |
| `SIP_VAD_BARGE_IN` | conversation | 是否允许打断 |
| `ASR_*` | conversation | ASR 配置 |
| `LLM_*` | conversation | LLM 配置 |
| `TTS_*` | conversation | TTS 配置 |
| `SIP_TRANSFER_*` | conversation/outbound | 转接目标 |
| `SIP_PERSIST` | persist | 存储模式 (json/gorm) |
| `VOICE_DIALOG_WS_TOKEN` | voicedialog | WebSocket Token |

## 目录统计

```
pkg/sip/
├── bridge/        (2 files)      # 两路桥接
├── conversation/  (17 files)     # AI 语音对话（核心）
├── dialog/        (6 files)      # 对话管理
├── dtmf/          (6 files)      # DTMF 处理
├── outbound/      (17 files)     # 外呼/出站
├── persist/       (10 files)     # 持久化
├── protocol/      (4 files)      # 底层协议
├── rtp/           (7 files)      # RTP 传输
├── sdp/           (3 files)      # SDP 协商
├── server/        (11 files)     # SIP 服务器（核心）
├── session/       (5 files)     # 呼叫会话
├── siputil/       (3 files)      # 工具函数
├── stack/         (12 files)     # 协议栈
├── transaction/   (16 files)    # 事务层
├── uas/           (8 files)      # UAS 事务
├── vad/           (3 files)      # 语音检测
├── voicedialog/   (11 files)    # HTTP/WS 桥
└── webseat/       (2 files)     # WebRTC 座席
```

---

**维护者**: LingEchoX Team  
**协议**: AGPL-3.0
