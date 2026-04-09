# LingEchoX 项目现状说明

## 1. 项目定位

LingEchoX 是一个面向语音联络场景的 AI + SIP 平台，目标是把「呼叫中心能力」与「语音智能能力」整合在同一套系统中，支持从通话接入到语音识别、语音合成、脚本流程和运营管理的闭环。

## 2. 当前整体架构

- **后端（Go）**：`cmd` + `internal` + `pkg`
  - `cmd/server/main.go` 负责启动流程、配置加载、数据库初始化、HTTP 服务与嵌入式 SIP 服务。
  - `internal/handler` 提供 API 路由与业务入口。
  - `pkg` 提供可复用能力模块，如 SIP、媒体编解码、ASR、TTS、LLM、日志、配置、中间件等。
- **前端（React）**：`qiniu`
  - 基于 Vite + TypeScript + Tailwind。
  - 覆盖 SIP 用户、通话记录、号码池、外呼任务、脚本管理等核心页面。

## 3. 已有关键能力（基于当前代码扫描）

### 3.1 联络中心与 SIP 业务能力

- SIP 用户管理（列表、详情、删除等）。
- 呼叫记录查询（列表、详情）。
- ACD 池管理（增删改查、WebSeat 心跳）。
- 脚本模板管理（增删改查）。
- 外呼活动（Campaign）全生命周期管理：
  - 创建活动
  - 导入/管理联系人
  - 开始/暂停/恢复/停止
  - 指标与日志查询

### 3.2 语音智能能力

- ASR 多提供商适配（如 Deepgram、Google、QCloud、Qiniu、Baidu、本地等）。
- TTS 多提供商适配（如 OpenAI、Google、Azure、AWS、QCloud、Xunfei、Volcengine、本地等）。
- LLM Provider 抽象与适配（例如 OpenAI / Coze Provider）。
- RTP、DTMF、媒体会话与编解码相关基础能力。

### 3.3 平台化能力

- 多数据库支持：SQLite（默认）、PostgreSQL、MySQL。
- 会话、CORS、日志、监控等通用中间件。
- 搜索、备份、缓存、存储、SSL 等配置开关。
- 丰富的环境变量模板（`env.example`）支持部署环境扩展。

## 4. 前端当前页面能力

`qiniu/src/pages` 当前可见页面包含（部分）：

- `SIPUsers`
- `CallRecords`
- `NumberPool`
- `OutboundTasks`
- `ScriptManager`
- `WebAgents`
- `Settings`
- `Storage`
- `Users`
- `OperationLogs`
- `Notifications`
- `Documentation`

这说明前端已具备运营台形态，具备继续向统一客服/外呼工作台扩展的基础。

## 5. 当前优势与可改进点

### 优势

- 语音能力覆盖深（SIP + ASR + TTS + LLM）。
- 后端模块化程度较高，具备扩展新 Provider 的基础。
- 业务模型已经覆盖「联系人-活动-脚本-通话记录」主链路。

### 可改进点

- 文档体系刚起步，需补齐 API、部署、架构图、运维手册。
- 前后端工程规范（CI、测试矩阵、代码质量门禁）还可进一步强化。
- 产品层可增加更完善的可观测性、报表、运营自动化能力。

## 6. 建议的文档结构（下一步）

建议在 `docs` 下逐步增加：

- `architecture.md`：系统架构图与调用链路
- `api-overview.md`：核心 API 与认证说明
- `deployment.md`：单机/容器化/生产部署指南
- `troubleshooting.md`：常见问题与排障流程
- `contributing.md`：开发规范与提交流程

