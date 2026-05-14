package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/pkg/logger"
	"go.uber.org/zap"
)

// AlibabaProvider 阿里云百炼 LLM 提供者实现
type AlibabaProvider struct {
	config          AlibabaAIConfig
	client          *http.Client
	ctx             context.Context
	systemMsg       string
	pendingAction   string
	mutex           sync.Mutex
	messages        []Message
	hangupChan      chan struct{}
	interruptCh     chan struct{}
	functionManager *FunctionToolManager
	lastUsage       Usage
	lastUsageValid  bool
}

// ConsumePendingAction returns and clears the latest resolved action.
func (p *AlibabaProvider) ConsumePendingAction() string {
	if p == nil {
		return ""
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	a := p.pendingAction
	p.pendingAction = ""
	return a
}

type alibabaMessagePayload struct {
	Message    string `json:"message"`
	NeedPerson int    `json:"needperson"`
	NeedHangup int    `json:"needhangup"`
	Action     string `json:"action"`
}

func parseAlibabaPayload(raw string) (alibabaMessagePayload, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return alibabaMessagePayload{}, false
	}
	var msg alibabaMessagePayload
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return alibabaMessagePayload{}, false
	}
	// Some app templates nest the real JSON in message as a JSON string/object.
	nestedRaw := strings.TrimSpace(msg.Message)
	if nestedRaw != "" && strings.HasPrefix(nestedRaw, "{") && strings.HasSuffix(nestedRaw, "}") {
		var nested alibabaMessagePayload
		if err := json.Unmarshal([]byte(nestedRaw), &nested); err == nil {
			if strings.TrimSpace(nested.Message) != "" {
				msg.Message = nested.Message
			}
			if strings.TrimSpace(nested.Action) != "" {
				msg.Action = nested.Action
			}
			if nested.NeedPerson != 0 {
				msg.NeedPerson = nested.NeedPerson
			}
			if nested.NeedHangup != 0 {
				msg.NeedHangup = nested.NeedHangup
			}
		}
	}
	if strings.TrimSpace(msg.Message) == "" && strings.TrimSpace(msg.Action) == "" && msg.NeedPerson == 0 && msg.NeedHangup == 0 {
		// Generic fallback for templates with different key casing/naming.
		var anyMap map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &anyMap); err == nil {
			if v, ok := anyMap["message"].(string); ok && strings.TrimSpace(v) != "" {
				msg.Message = v
			}
			if v, ok := anyMap["action"].(string); ok {
				msg.Action = v
			}
			if v, ok := anyMap["needperson"].(float64); ok {
				msg.NeedPerson = int(v)
			}
			if v, ok := anyMap["needhangup"].(float64); ok {
				msg.NeedHangup = int(v)
			}
		}
	}
	if strings.TrimSpace(msg.Message) == "" && strings.TrimSpace(msg.Action) == "" && msg.NeedPerson == 0 && msg.NeedHangup == 0 {
		return alibabaMessagePayload{}, false
	}
	return msg, true
}

func previewText(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// AlibabaAIConfig 阿里云百炼配置
type AlibabaAIConfig struct {
	APIKey    string
	AppID     string
	Endpoint  string
	Timeout   time.Duration // 总超时时间
	FirstByte time.Duration // 首字节超时时间（默认10秒）
}

// NewAlibabaProvider 创建阿里云百炼提供者
func NewAlibabaProvider(ctx context.Context, apiKey, appID, systemPrompt string, endpoint ...string) *AlibabaProvider {
	timeout := 30 * time.Second
	firstByte := 10 * time.Second
	if s := strings.TrimSpace(os.Getenv("ALIBABA_AI_TIMEOUT_SECONDS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	if s := strings.TrimSpace(os.Getenv("ALIBABA_AI_FIRST_BYTE_TIMEOUT_SECONDS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			firstByte = time.Duration(n) * time.Second
		}
	}
	config := AlibabaAIConfig{
		APIKey:    apiKey,
		AppID:     appID,
		Timeout:   timeout,
		FirstByte: firstByte,
	}

	if len(endpoint) > 0 && endpoint[0] != "" {
		config.Endpoint = endpoint[0]
	} else {
		config.Endpoint = "https://dashscope.aliyuncs.com"
	}

	return &AlibabaProvider{
		config:          config,
		client:          &http.Client{Timeout: config.Timeout},
		ctx:             ctx,
		systemMsg:       systemPrompt,
		messages:        make([]Message, 0),
		hangupChan:      make(chan struct{}),
		interruptCh:     make(chan struct{}, 1),
		functionManager: NewFunctionToolManager(),
	}
}

// Query 执行非流式查询
func (p *AlibabaProvider) Query(text, model string) (string, error) {
	return p.QueryWithOptions(text, QueryOptions{Model: model})
}

// QueryWithOptions 执行带完整参数的非流式查询
func (p *AlibabaProvider) QueryWithOptions(text string, options QueryOptions) (string, error) {
	startTime := time.Now()

	p.mutex.Lock()
	// 添加用户消息到历史
	p.messages = append(p.messages, Message{
		Role:    "user",
		Content: text,
	})
	p.mutex.Unlock()

	ctx, cancel := context.WithTimeout(p.ctx, p.config.Timeout)
	defer cancel()

	// 构建请求
	reqBody := map[string]interface{}{
		"input": map[string]string{
			"prompt":     p.composePrompt(text),
			"session_id": options.SessionID,
		},
		"parameters": map[string]interface{}{},
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", p.config.Endpoint, p.config.AppID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))
	req.Header.Set("Content-Type", "application/json")

	logger.Debug("Alibaba AI request started",
		zap.String("url", url),
		zap.String("prompt", text))

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	logger.Debug("Alibaba AI response headers received",
		zap.Int("status_code", resp.StatusCode),
		zap.String("url", url),
	)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// 读取完整响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Output struct {
			Text         string `json:"text"`
			FinishReason string `json:"finish_reason"`
		} `json:"output"`
		Usage struct {
			Models []struct {
				ModelID      string `json:"model_id"`
				InputTokens  int    `json:"input_tokens"`
				OutputTokens int    `json:"output_tokens"`
			} `json:"models"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	finalResponse := result.Output.Text
	if msgResp, ok := parseAlibabaPayload(finalResponse); ok {
		logger.Info("Alibaba AI json parsed",
			zap.String("action", strings.TrimSpace(msgResp.Action)),
			zap.Int("needperson", msgResp.NeedPerson),
			zap.Int("needhangup", msgResp.NeedHangup),
			zap.String("message_preview", previewText(msgResp.Message, 120)),
		)
		if msgResp.Message != "" {
			finalResponse = msgResp.Message
		}
		_ = p.maybeInvokeActions(msgResp)
	} else {
		logger.Warn("Alibaba AI json parse failed (non-structured response)",
			zap.String("output_preview", previewText(finalResponse, 300)),
		)
	}

	// 更新消息历史
	p.mutex.Lock()
	p.messages = append(p.messages, Message{
		Role:    "assistant",
		Content: finalResponse,
	})
	p.mutex.Unlock()

	// 记录使用统计
	if len(result.Usage.Models) > 0 {
		p.lastUsage = Usage{
			PromptTokens:     result.Usage.Models[0].InputTokens,
			CompletionTokens: result.Usage.Models[0].OutputTokens,
			TotalTokens:      result.Usage.Models[0].InputTokens + result.Usage.Models[0].OutputTokens,
		}
		p.lastUsageValid = true
	}

	logger.Info("Alibaba AI request completed",
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("response_length", len(finalResponse)))

	return finalResponse, nil
}

// QueryStream 执行流式查询。
//
// 阿里百炼 App 模式的流式响应是按 JSON 包装格式返回的（`{"action":...,"message":"..."}`），
// 早期实现在每个 SSE chunk 上做整体 JSON parse —— JSON 完整闭合前 callback 无法拿到任何
// 文本，相当于退化成非流式，首响延迟被 LLM 端到端时长拖累。
//
// 现在统一走渐进式提取：维护一个累积的原始文本缓冲，每个 chunk 进来后从 `"message":"...`
// 处把已写出的字符串字段抽取成 rune，相对上一次 emitted 的尾部增量回调出去。这样首响
// 一旦 LLM 写完 `"message":"<首字>` 即可下发。
//
// callback(segment, isComplete) 的语义变为：segment 为 message 字段的「新增 rune 文本」，
// isComplete=true 仅在流末尾被调用一次（segment="")。这与 OpenAI/Coze 等的 delta 语义对齐。
func (p *AlibabaProvider) QueryStream(text string, options QueryOptions, callback func(segment string, isComplete bool) error) (string, error) {
	return p.queryStreamProgressive(p.ctx, text, options, callback)
}

// queryStreamProgressive 是 QueryStream 的实际实现。
//
// 注意：本算法（cumulative buffer + 渐进式抽取 message 字段）只适用于「外层包了一层 JSON
// 契约」的 provider。当前只有阿里百炼 App 接口会强制返回 `{"action":...,"message":"..."}`
// 这样的格式，所以这套逻辑放在 alibaba_provider.go 内即可，OpenAI / Coze / Ollama 的
// SSE 本身就直接 yield delta 文本，不需要这一层处理。
func (p *AlibabaProvider) queryStreamProgressive(
	ctx context.Context,
	text string,
	options QueryOptions,
	onDelta func(segment string, isComplete bool) error,
) (string, error) {
	startTime := time.Now()

	p.mutex.Lock()
	p.messages = append(p.messages, Message{Role: "user", Content: text})
	p.mutex.Unlock()

	if ctx == nil {
		ctx = p.ctx
	}
	cctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	reqBody := map[string]interface{}{
		"input": map[string]string{
			"prompt":     p.composePrompt(text),
			"session_id": options.SessionID,
		},
		"parameters": map[string]interface{}{
			"incremental_output": true,
		},
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", p.config.Endpoint, p.config.AppID)
	req, err := http.NewRequestWithContext(cctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-SSE", "enable")
	req.Header.Set("Accept", "text/event-stream")

	logger.Debug("Alibaba AI stream request started",
		zap.String("url", url),
		zap.String("prompt", text),
	)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	logger.Debug("Alibaba AI stream response headers received",
		zap.Int("status_code", resp.StatusCode),
		zap.String("url", url),
	)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	var (
		cumulativeRaw  strings.Builder
		prevRaw        string
		emittedRunes   int
		firstChunkAt   time.Time
		lineCh         = make(chan string, 1)
		errCh          = make(chan error, 1)
		scannerStarted bool
	)

	startScanner := func() {
		if scannerStarted {
			return
		}
		scannerStarted = true
		go func() {
			for scanner.Scan() {
				lineCh <- scanner.Text()
			}
			if serr := scanner.Err(); serr != nil {
				errCh <- serr
				return
			}
			close(lineCh)
		}()
	}
	startScanner()

	firstByteTimer := time.NewTimer(p.config.FirstByte)
	defer firstByteTimer.Stop()

	emitDelta := func(currentMsg string) error {
		curRunes := []rune(currentMsg)
		if len(curRunes) <= emittedRunes {
			return nil
		}
		delta := string(curRunes[emittedRunes:])
		emittedRunes = len(curRunes)
		if onDelta != nil && delta != "" {
			return onDelta(delta, false)
		}
		return nil
	}

LOOP:
	for {
		select {
		case <-cctx.Done():
			return cumulativeRaw.String(), cctx.Err()
		case <-p.interruptCh:
			return cumulativeRaw.String(), fmt.Errorf("interrupted")
		case <-p.hangupChan:
			return cumulativeRaw.String(), fmt.Errorf("hangup")
		case <-firstByteTimer.C:
			if firstChunkAt.IsZero() {
				logger.Warn("Alibaba AI: First byte timeout",
					zap.Duration("timeout", p.config.FirstByte))
				const fallback = "您好,非常抱歉,您的这个问题我暂时无法解答,建议您提交工单申请处理。"
				if onDelta != nil {
					_ = onDelta(fallback, false)
					_ = onDelta("", true)
				}
				return fallback, nil
			}
		case serr := <-errCh:
			return cumulativeRaw.String(), fmt.Errorf("scan error: %w", serr)
		case line, ok := <-lineCh:
			if !ok {
				break LOOP
			}
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			var chunk struct {
				Output struct {
					Text         string `json:"text"`
					FinishReason string `json:"finish_reason"`
				} `json:"output"`
			}
			if jerr := json.Unmarshal([]byte(data), &chunk); jerr != nil {
				continue
			}
			if chunk.Output.Text == "" {
				continue
			}
			if firstChunkAt.IsZero() {
				firstChunkAt = time.Now()
				logger.Info("Alibaba AI first SSE chunk",
					zap.Duration("elapsed", time.Since(startTime)),
				)
			}
			// 自动判别 cumulative vs delta：若新文本以已有 prevRaw 为前缀，说明是
			// cumulative 模式 → 用新文本整体替换；否则视为 delta → 直接拼接。
			newText := chunk.Output.Text
			if prevRaw != "" && strings.HasPrefix(newText, prevRaw) {
				cumulativeRaw.Reset()
				cumulativeRaw.WriteString(newText)
			} else {
				cumulativeRaw.WriteString(newText)
			}
			prevRaw = cumulativeRaw.String()

			// 渐进式抽取 message 字段并发出增量。
			if currentMsg, _ := extractJSONStringField(prevRaw, "message"); currentMsg != "" {
				if cberr := emitDelta(currentMsg); cberr != nil {
					return currentMsg, cberr
				}
			}
		}
	}

	finalRaw := cumulativeRaw.String()
	finalMsg := finalRaw
	if msgResp, ok := parseAlibabaPayload(finalRaw); ok {
		logger.Info("Alibaba AI stream json parsed",
			zap.String("action", strings.TrimSpace(msgResp.Action)),
			zap.Int("needperson", msgResp.NeedPerson),
			zap.Int("needhangup", msgResp.NeedHangup),
			zap.String("message_preview", previewText(msgResp.Message, 120)),
		)
		finalMsg = msgResp.Message
		_ = p.maybeInvokeActions(msgResp)
	} else if firstChunkAt.IsZero() {
		// 完全没收到任何 chunk
		return "", fmt.Errorf("no data received")
	} else {
		logger.Warn("Alibaba AI stream json parse failed (returning raw cumulative text)",
			zap.String("raw_preview", previewText(finalRaw, 240)),
		)
	}

	// flush 任何未发出的尾部（罕见：JSON 收尾后才 parse 成功，但 progressive 抽取因
	// 转义不完整提前停下）。
	finalRunes := []rune(finalMsg)
	if len(finalRunes) > emittedRunes && onDelta != nil {
		delta := string(finalRunes[emittedRunes:])
		if delta != "" {
			if cberr := onDelta(delta, false); cberr != nil {
				return finalMsg, cberr
			}
		}
		emittedRunes = len(finalRunes)
	}
	if onDelta != nil {
		_ = onDelta("", true)
	}

	p.mutex.Lock()
	p.messages = append(p.messages, Message{Role: "assistant", Content: finalMsg})
	p.mutex.Unlock()

	logger.Info("Alibaba AI stream request completed",
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("response_runes", len([]rune(finalMsg))),
	)
	return finalMsg, nil
}

func (p *AlibabaProvider) maybeInvokeActions(msg alibabaMessagePayload) error {
	action := strings.TrimSpace(strings.ToLower(msg.Action))
	if msg.NeedPerson == 1 || action == "transfer_to_agent" || action == "transfer" {
		logger.Info("Alibaba AI action resolved", zap.String("action", "transfer_to_agent"))
		p.mutex.Lock()
		p.pendingAction = "transfer_to_agent"
		p.mutex.Unlock()
		return nil
	}
	// hangup action intentionally ignored in SIP voice flow (no hangup tool registered).
	logger.Debug("Alibaba AI no actionable command in payload",
		zap.String("action", action),
		zap.Int("needperson", msg.NeedPerson),
		zap.Int("needhangup", msg.NeedHangup),
	)
	return nil
}

func (p *AlibabaProvider) invokeToolByName(name string, args map[string]interface{}) error {
	if p == nil || p.functionManager == nil {
		return nil
	}
	def, ok := p.functionManager.GetTool(name)
	if !ok || def == nil || def.Callback == nil {
		return nil
	}
	_, err := def.Callback(args, p.functionManager.GetLLMService())
	return err
}

func (p *AlibabaProvider) composePrompt(userText string) string {
	sys := strings.TrimSpace(p.systemMsg)
	userText = strings.TrimSpace(userText)
	contract := `你必须只输出单行JSON，不要输出任何额外文本、markdown或代码块。JSON结构：
{"message":"给用户播报的自然语言","action":"none|transfer_to_agent|hangup_call","needperson":0或1,"needhangup":0或1}
规则：
1) 若用户要求转人工，action=transfer_to_agent, needperson=1, needhangup=0。
2) 若用户要求挂断/结束通话（如“再见、挂了”），action=hangup_call, needhangup=1, needperson=0。
3) 其他情况 action=none, needperson=0, needhangup=0。
4) message 使用中文，简短自然。`
	if sys == "" {
		return fmt.Sprintf("%s\n\n用户输入：%s", contract, userText)
	}
	return fmt.Sprintf("系统指令：%s\n\n%s\n\n用户输入：%s", sys, contract, userText)
}

// RegisterFunctionTool 注册函数工具
func (p *AlibabaProvider) RegisterFunctionTool(name, description string, parameters interface{}, callback FunctionToolCallback) {
	var params json.RawMessage
	if parameters != nil {
		if raw, ok := parameters.(json.RawMessage); ok {
			params = raw
		} else {
			bytes, _ := json.Marshal(parameters)
			params = json.RawMessage(bytes)
		}
	}
	p.functionManager.RegisterTool(name, description, params, callback)
}

// RegisterFunctionToolDefinition 通过定义结构注册工具
func (p *AlibabaProvider) RegisterFunctionToolDefinition(def *FunctionToolDefinition) {
	p.functionManager.RegisterToolDefinition(def)
}

// GetFunctionTools 获取所有可用的函数工具
func (p *AlibabaProvider) GetFunctionTools() []interface{} {
	return []interface{}{}
}

// ListFunctionTools 列出所有已注册的工具名称
func (p *AlibabaProvider) ListFunctionTools() []string {
	return p.functionManager.ListTools()
}

// GetLastUsage 获取最后一次调用的使用统计信息
func (p *AlibabaProvider) GetLastUsage() (Usage, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.lastUsage, p.lastUsageValid
}

// ResetMessages 重置对话历史
func (p *AlibabaProvider) ResetMessages() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.messages = make([]Message, 0)
}

// SetSystemPrompt 设置系统提示词
func (p *AlibabaProvider) SetSystemPrompt(systemPrompt string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.systemMsg = systemPrompt
}

// GetMessages 获取当前对话历史
func (p *AlibabaProvider) GetMessages() []Message {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.messages
}

// Interrupt 中断当前请求
func (p *AlibabaProvider) Interrupt() {
	select {
	case p.interruptCh <- struct{}{}:
	default:
	}
}

// Hangup 挂断（清理资源）
func (p *AlibabaProvider) Hangup() {
	close(p.hangupChan)
}

// extractJSONStringField 在可能不完整的 JSON 文本 raw 中渐进式抽取指定字符串字段的当前值。
// 只要找到 `"key":"...` 的起始引号即可开始返回，遇到收尾引号或缓冲尾部停下；
// done=true 表示已读到完整收尾引号。处理常见 JSON 转义（\n \t \r \" \\ \uXXXX）。
//
// 仅供阿里百炼应用接口（外包 JSON 契约）流式输出时按 message 字段做渐进式抽取使用。
func extractJSONStringField(raw, key string) (string, bool) {
	if raw == "" || key == "" {
		return "", false
	}
	needle := "\"" + key + "\""
	idx := strings.Index(raw, needle)
	if idx < 0 {
		return "", false
	}
	rest := raw[idx+len(needle):]
	i := 0
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' || rest[i] == '\r') {
		i++
	}
	if i >= len(rest) || rest[i] != ':' {
		return "", false
	}
	i++
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' || rest[i] == '\r') {
		i++
	}
	if i >= len(rest) || rest[i] != '"' {
		return "", false
	}
	i++
	var b strings.Builder
	closed := false
	for i < len(rest) {
		c := rest[i]
		if c == '\\' && i+1 < len(rest) {
			nc := rest[i+1]
			switch nc {
			case 'n':
				b.WriteByte('\n')
				i += 2
			case 't':
				b.WriteByte('\t')
				i += 2
			case 'r':
				b.WriteByte('\r')
				i += 2
			case '"':
				b.WriteByte('"')
				i += 2
			case '\\':
				b.WriteByte('\\')
				i += 2
			case '/':
				b.WriteByte('/')
				i += 2
			case 'u':
				if i+5 < len(rest) {
					hex := rest[i+2 : i+6]
					if r, err := strconv.ParseUint(hex, 16, 32); err == nil {
						b.WriteRune(rune(r))
					}
					i += 6
				} else {
					return b.String(), false
				}
			default:
				b.WriteByte(nc)
				i += 2
			}
			continue
		}
		if c == '"' {
			closed = true
			break
		}
		b.WriteByte(c)
		i++
	}
	return b.String(), closed
}
