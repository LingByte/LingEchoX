package conversation

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/LingByte/SoulNexus/pkg/intentonnx"
	"github.com/LingByte/SoulNexus/pkg/utils"
	"go.uber.org/zap"
)

// SIP ONNX intent env (optional; when disabled, voice stays LLM-only).
// Values are read via utils.GetEnv (SoulNexus .env / process env). Use absolute paths in production.
//
//	# 开关
//	SIP_INTENT_ONNX_ENABLED=1
//
//	# 模型与 tokenizer（与 cmd/onnx-intent-demo 用的是同一套文件即可）
//	SIP_INTENT_ONNX_MODEL=/Users/you/LingEchoX/model_q4.onnx
//	SIP_INTENT_ONNX_TOKENIZER=/Users/you/LingEchoX/tokenizer.json
//
//	# ONNX Runtime 动态库（二选一）
//	SIP_INTENT_ONNX_LIB=/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib   # Apple Silicon brew
//	# 或只设全局：ONNXRUNTIME_SHARED_LIBRARY_PATH=同上，可不写 SIP_INTENT_ONNX_LIB
//
//	# 可选
//	SIP_INTENT_ONNX_SEQ=128
//	SIP_INTENT_ONNX_CONFIG=/Users/you/LingEchoX/config/my_intents.json   # 不设则用 pkg/intentonnx 内置 default
//	SIP_INTENT_ONNX_COREML=1   # 仅 darwin；多数情况可不设
//
//	# 仅对下列「意图名」播放 ONNX 配置里的固定话术（不调用 LLM）；其它命中仍走 LLM 生成回答（单次 TTS，不双答）
//	SIP_INTENT_ONNX_CANNED_INTENTS=转人工,投诉建议
//
//	# 下列意图名的「固定话术」仅在用户话里出现显式用语时才允许（防模型误分类，如闲聊被判成转人工）
//	SIP_INTENT_STRICT_CANNED_NAMES=转人工   # 逗号分隔；设 none 或 - 关闭门禁（恢复仅看模型）
//	SIP_INTENT_TRANSFER_EXPLICIT_PHRASES=   # 可选；逗号分隔子串。不设时内置词表每条均含「人工」；自定义则不做「人工」子串校验
//
//	# 「转人工」意图：仅当规范化后的用户话里连续出现「转人工」三字时才采纳小模型该分类（默认开启）
//	SIP_INTENT_TRANSFER_LITERAL_ONLY=1   # 0/false 关闭，恢复完全相信分类器
//	SIP_INTENT_TRANSFER_LITERAL_INTENT_NAME=转人工   # 与 intents JSON 中 name 一致
//
//	# 走 LLM 时是否在用户话前附加 ONNX 意图（默认开启，让大模型「看见」小模型结果）
//	SIP_INTENT_ONNX_LLM_PREFIX=1   # 1/true 开启；0/false 关闭，仅传 ASR 原文
//	对 SIP_INTENT_STRICT_CANNED_NAMES 中的意图（默认「转人工」）：用户话里若无显式转接用语，不会把该意图名写进 LLM 前缀（避免误判仍当转人工处理）。
//
//	# 两段 TTS（顺序播放、不叠音）：先播 ONNX 配置里该意图的固定话术，再播 LLM 回答
//	SIP_INTENT_TWO_PHASE_TTS=1       # 默认开启；0 关闭
//	SIP_INTENT_TWO_PHASE_NAMES=查询   # 逗号分隔，与 intents JSON 里 name 一致
//
//	# 无座席 goodbye 播完后多等一会再发 BYE（毫秒），给对端缓冲播完
//	SIP_TRANSFER_GOODBYE_TAIL_MS=900
var (
	sipIntentMu     sync.Mutex
	sipIntentEng    *intentonnx.Engine
	sipIntentCfg    *intentonnx.IntentConfig
	sipIntentReady  bool
	sipIntentGaveUp bool
)

func sipIntentONNXEnabled() bool {
	v := strings.ToLower(utils.GetEnv("SIP_INTENT_ONNX_ENABLED"))
	return v == "1" || v == "true" || v == "yes"
}

func sipIntentONNXCoreML() bool {
	v := strings.ToLower(utils.GetEnv("SIP_INTENT_ONNX_COREML"))
	return v == "1" || v == "true" || v == "yes"
}

// sipIntentEngine returns (engine, config, true) when SIP intent routing is active.
func sipIntentEngine(lg *zap.Logger) (*intentonnx.Engine, *intentonnx.IntentConfig, bool) {
	sipIntentMu.Lock()
	defer sipIntentMu.Unlock()
	if sipIntentGaveUp {
		return nil, nil, false
	}
	if sipIntentReady && sipIntentEng != nil {
		return sipIntentEng, sipIntentCfg, true
	}
	if !sipIntentONNXEnabled() {
		sipIntentGaveUp = true
		return nil, nil, false
	}
	model := utils.GetEnv("SIP_INTENT_ONNX_MODEL")
	tok := utils.GetEnv("SIP_INTENT_ONNX_TOKENIZER")
	lib := utils.GetEnv("SIP_INTENT_ONNX_LIB")
	if lib == "" {
		lib = strings.TrimSpace(os.Getenv("ONNXRUNTIME_SHARED_LIBRARY_PATH"))
	}
	if model == "" || tok == "" || lib == "" {
		sipIntentGaveUp = true
		if lg != nil {
			lg.Info("sip intent onnx skipped (set SIP_INTENT_ONNX_ENABLED and SIP_INTENT_ONNX_MODEL, SIP_INTENT_ONNX_TOKENIZER, ORT lib path)")
		}
		return nil, nil, false
	}
	seq := 128
	if s := utils.GetEnv("SIP_INTENT_ONNX_SEQ"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			seq = n
		}
	}
	if err := intentonnx.InitRuntime(lib); err != nil {
		sipIntentGaveUp = true
		if lg != nil {
			lg.Warn("sip intent onnx InitRuntime failed", zap.Error(err))
		}
		return nil, nil, false
	}
	eng, err := intentonnx.NewEngine(intentonnx.Options{
		SharedLibraryPath: lib,
		ModelPath:         model,
		TokenizerPath:     tok,
		SeqLen:            seq,
		UseCoreML:         sipIntentONNXCoreML(),
	})
	if err != nil {
		sipIntentGaveUp = true
		if lg != nil {
			lg.Warn("sip intent onnx NewEngine failed", zap.Error(err))
		}
		return nil, nil, false
	}
	cfgPath := utils.GetEnv("SIP_INTENT_ONNX_CONFIG")
	cfg, err := intentonnx.LoadIntentConfig(cfgPath)
	if err != nil {
		_ = eng.Close()
		sipIntentGaveUp = true
		if lg != nil {
			lg.Warn("sip intent onnx LoadIntentConfig failed", zap.Error(err))
		}
		return nil, nil, false
	}
	if err := intentonnx.ValidateIntentConfig(cfg, eng.NumClasses()); err != nil {
		_ = eng.Close()
		sipIntentGaveUp = true
		if lg != nil {
			lg.Warn("sip intent onnx ValidateIntentConfig failed", zap.Error(err))
		}
		return nil, nil, false
	}
	sipIntentEng = eng
	sipIntentCfg = cfg
	sipIntentReady = true
	if lg != nil {
		lg.Info("sip intent onnx enabled",
			zap.String("model", model),
			zap.Int("classes", eng.NumClasses()),
			zap.Int("seq", seq),
		)
	}
	return sipIntentEng, sipIntentCfg, true
}

func sipIntentRouteOptions() intentonnx.RouteOptions {
	return intentonnx.RouteOptions{
		VoiceASRHints: true,
	}
}

func sipIntentTransferLiteralOnlyEnabled() bool {
	v := strings.ToLower(utils.GetEnv("SIP_INTENT_TRANSFER_LITERAL_ONLY"))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

func sipIntentTransferLiteralIntentName() string {
	s := utils.GetEnv("SIP_INTENT_TRANSFER_LITERAL_INTENT_NAME")
	if s == "" {
		return "转人工"
	}
	return s
}

// sipIntentEnforceTransferLiteral downgrades a 转人工 route to LLM-only unless routeText contains the substring 「转人工」.
func sipIntentEnforceTransferLiteral(routeText string, out *intentonnx.RouteOutput, lg *zap.Logger) *intentonnx.RouteOutput {
	if out == nil || !sipIntentTransferLiteralOnlyEnabled() {
		return out
	}
	if strings.TrimSpace(out.Prediction.IntentName) != sipIntentTransferLiteralIntentName() {
		return out
	}
	if strings.Contains(routeText, "转人工") {
		return out
	}
	if lg != nil {
		lg.Info("sip intent transfer literal gate: model 转人工 rejected (no 转人工 in transcript), llm-only turn",
			zap.String("route_text_preview", routeText),
			zap.Float64("model_confidence", out.Prediction.Confidence),
		)
	}
	adj := *out
	adj.Channel = intentonnx.AnswerChannelLLM
	adj.Reply = ""
	adj.Prediction.IntentName = ""
	adj.Prediction.Confidence = 0
	adj.Prediction.KeywordBiasApplied = false
	return &adj
}

// sipIntentCannedIntentNames lists intent names (from intents JSON "name") that use canned TTS only.
// Others (e.g. 查询) still run ONNX for routing/metrics but the user hears LLM once.
func sipIntentCannedIntentNames() []string {
	raw := utils.GetEnv("SIP_INTENT_ONNX_CANNED_INTENTS")
	if raw == "" {
		raw = "转人工,投诉建议"
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sipIntentReplyUsesCannedOnly(intentName string) bool {
	intentName = strings.TrimSpace(intentName)
	if intentName == "" {
		return false
	}
	for _, n := range sipIntentCannedIntentNames() {
		if n == intentName {
			return true
		}
	}
	return false
}

// sipIntentStrictCannedIntentNames lists intent names for which ONNX fixed-line TTS is allowed only when
// the user transcript matches an explicit phrase (see sipIntentExplicitTransferRequest).
func sipIntentStrictCannedIntentNames() []string {
	raw := utils.GetEnv("SIP_INTENT_STRICT_CANNED_NAMES")
	if raw == "" {
		return []string{"转人工"}
	}
	if strings.EqualFold(raw, "none") || raw == "-" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sipIntentRequiresExplicitTranscript(intentName string) bool {
	intentName = strings.TrimSpace(intentName)
	if intentName == "" {
		return false
	}
	for _, n := range sipIntentStrictCannedIntentNames() {
		if n == intentName {
			return true
		}
	}
	return false
}

func sipIntentTransferExplicitKeywords() []string {
	raw := utils.GetEnv("SIP_INTENT_TRANSFER_EXPLICIT_PHRASES")
	if raw != "" {
		var out []string
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	// 每条须含「人工」；不含「转/接/找/要」等动作词的「人工服务」等易与投诉混淆，不列入默认显式转接。
	return []string{
		"转人工", "转接人工", "人工客服", "真人客服", "找人工", "接人工",
		"人工帮我", "真人帮我", "能不能人工", "接个人工", "找个人工", "转个人工",
		"帮我转人工", "给我转人工", "我要转人工", "要人工",
	}
}

// sipIntentExplicitTransferRequest is true when the user transcript clearly asks for human/agent transfer.
// Used for SIP transfer after TTS and for gating ONNX canned 「转人工」 when the classifier misfires.
// When using the built-in keyword list, a match must also contain the substring 「人工」 (custom env list is not auto-filtered).
func sipIntentExplicitTransferRequest(transcript string) bool {
	s := strings.TrimSpace(transcript)
	if s == "" {
		return false
	}
	custom := utils.GetEnv("SIP_INTENT_TRANSFER_EXPLICIT_PHRASES") != ""
	for _, kw := range sipIntentTransferExplicitKeywords() {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if !strings.Contains(s, kw) {
			continue
		}
		if !custom && !strings.Contains(kw, "人工") {
			continue
		}
		return true
	}
	return false
}

// sipIntentExplicitCannedAllowed is false when the model picked a strict-listed intent but the user did not say an explicit phrase.
func sipIntentExplicitCannedAllowed(intentName, normalizedTranscript string) bool {
	if !sipIntentRequiresExplicitTranscript(intentName) {
		return true
	}
	return sipIntentExplicitTransferRequest(normalizedTranscript)
}

// sipIntentAugmentUserTextForLLM prepends ONNX route metadata so the LLM call uses both models in one turn
// (single user-visible answer from LLM/TTS, no second canned line). Disable with SIP_INTENT_ONNX_LLM_PREFIX=0.
func sipIntentTwoPhaseTTSEnabled() bool {
	v := strings.ToLower(utils.GetEnv("SIP_INTENT_TWO_PHASE_TTS"))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

func sipIntentTwoPhaseIntentNames() []string {
	raw := utils.GetEnv("SIP_INTENT_TWO_PHASE_NAMES")
	if raw == "" {
		return []string{"查询"}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// sipIntentUsesTwoPhaseQueuedTTS is true when we should play ONNX canned line first, then LLM TTS (sequential).
func sipIntentUsesTwoPhaseQueuedTTS(intentName string) bool {
	if !sipIntentTwoPhaseTTSEnabled() {
		return false
	}
	intentName = strings.TrimSpace(intentName)
	for _, n := range sipIntentTwoPhaseIntentNames() {
		if n == intentName {
			return true
		}
	}
	return false
}

func sipIntentAugmentUserTextForLLM(userText string, out *intentonnx.RouteOutput) string {
	if out == nil {
		return userText
	}
	v := strings.ToLower(utils.GetEnv("SIP_INTENT_ONNX_LLM_PREFIX"))
	if v == "0" || v == "false" || v == "no" {
		return userText
	}
	name := strings.TrimSpace(out.Prediction.IntentName)
	if name == "" {
		return userText
	}
	norm := intentonnx.NormalizeTranscript(userText)
	if sipIntentRequiresExplicitTranscript(name) && !sipIntentExplicitTransferRequest(norm) {
		// Model said e.g. 转人工 but user never asked for human — do not mislead the LLM with that label.
		return userText
	}
	conf := out.Prediction.Confidence
	kw := out.Prediction.KeywordBiasApplied
	return fmt.Sprintf("【本地意图模型：%s｜置信度%.3f｜关键词纠偏:%v】%s", name, conf, kw, userText)
}
