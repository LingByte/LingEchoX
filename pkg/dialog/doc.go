// Package dialog 是 LingEchoX 的对话引擎抽象层。
//
// 它把"如何与用户进行语音对话"这件事从 SIP 协议层 (pkg/sip) 完全
// 解耦,让上层业务可以在多种实现之间切换:
//
//   - cascaded — 经典三段式:ASR → LLM → TTS
//   - realtime — 多模态实时:Qwen-Omni / GPT-4o realtime / Doubao
//   - hybrid   — 未来:cascaded + realtime 协同(如 ASR 备份 + realtime 主流)
//
// 子包:
//
//   - engine        — Engine 接口 + Mode 注册表 (Strategy / Factory)
//   - provider      — ASR / TTS / LLM / Realtime Provider 接口 + Registry
//   - pipeline      — 媒体处理链 (Chain of Responsibility / Decorator)
//   - tools         — 跨模式共享的工具调用 (transfer / hangup / lookup)
//   - turn          — 对话轮抽象与持久化模型
//
// 设计目标:
//
//   - 零 SIP 协议依赖:本包不 import pkg/sip/*,只通过 MediaPort 接口
//     和上层交互。pkg/sip/session.CallSession 通过 Adapter 模式实现
//     MediaPort,反向不成立。
//   - Provider 插件化:新增 ASR/TTS/LLM 实现只需 init() 自注册,
//     业务代码无需修改。
//   - 引擎可热切换:同一个进程可以为不同租户/不同 DID 选择不同 Mode,
//     由 ModeDecider 决策。
//
// 详见 docs/refactor-architecture.md。
package dialog
