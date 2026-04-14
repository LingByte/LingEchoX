package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type AlibabaHandler struct {
	ctx          context.Context
	apiKey       string
	appID        string
	endpoint     string
	systemPrompt string
	client       *http.Client
	interruptCh  chan struct{}
}

func NewAlibabaHandler(ctx context.Context, llmOptions *LLMOptions) (*AlibabaHandler, error) {
	var opts LLMOptions
	if llmOptions != nil {
		opts = *llmOptions
	}
	timeout := 30 * time.Second
	if s := strings.TrimSpace(os.Getenv("ALIBABA_AI_TIMEOUT_SECONDS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	rawBase := strings.TrimSpace(opts.BaseURL)
	endpoint := "https://dashscope.aliyuncs.com"
	appID := ""
	// Compatibility:
	// - If BaseURL is a real URL, use it as endpoint and read AppID from env.
	// - If BaseURL is not a URL (legacy call-path), treat it as AppID.
	if rawBase != "" {
		if strings.Contains(rawBase, "://") {
			endpoint = rawBase
		} else {
			appID = rawBase
		}
	}
	if appID == "" {
		appID = strings.TrimSpace(os.Getenv("LLM_APP_ID"))
	}
	if appID == "" {
		appID = strings.TrimSpace(os.Getenv("ALIBABA_APP_ID"))
	}
	if appID == "" {
		return nil, errors.New("alibaba app id is required (set LLM BaseURL as app id or ALIBABA_APP_ID)")
	}
	return &AlibabaHandler{
		ctx:          ctx,
		apiKey:       strings.TrimSpace(opts.ApiKey),
		appID:        appID,
		endpoint:     endpoint,
		systemPrompt: opts.SystemPrompt,
		client:       &http.Client{Timeout: timeout},
		interruptCh:  make(chan struct{}, 1),
	}, nil
}

func (h *AlibabaHandler) Query(text, model string) (string, error) {
	resp, err := h.QueryWithOptions(text, &QueryOptions{Model: model})
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("empty response")
	}
	return resp.Choices[0].Content, nil
}

func (h *AlibabaHandler) QueryWithOptions(text string, options *QueryOptions) (*QueryResponse, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	select {
	case <-h.interruptCh:
		return nil, errors.New("interrupted")
	default:
	}
	var rewrite *QueryRewrite
	promptUser := text
	if options.EnableQueryRewrite {
		rw, err := h.rewriteQueryAlibaba(h.ctx, promptUser, options)
		if err == nil && rw != "" {
			rewrite = &QueryRewrite{Original: promptUser, Rewritten: rw}
			promptUser = rw
		}
	}

	var expansion *QueryExpansion
	if options.EnableQueryExpansion {
		expanded, terms, err := h.expandQueryAlibaba(h.ctx, promptUser, options)
		if err == nil {
			expansion = &QueryExpansion{
				Original: promptUser,
				Expanded: expanded,
				Terms:    terms,
				Debug:    map[string]any{},
			}
			promptUser = expanded
		}
	}

	reqBody := map[string]any{
		"input": map[string]string{
			"prompt": h.composePrompt(promptUser, options),
		},
		"parameters": map[string]any{},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", strings.TrimRight(h.endpoint, "/"), h.appID)
	req, err := http.NewRequestWithContext(h.ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alibaba request failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Output struct {
			Text string `json:"text"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	answer := strings.TrimSpace(parsed.Output.Text)
	return &QueryResponse{
		Provider:  h.Provider(),
		Model:     options.Model,
		Choices:   []QueryChoice{{Index: 0, Content: answer, FinishReason: "stop"}},
		Expansion: expansion,
		Rewrite:   rewrite,
	}, nil
}

func (h *AlibabaHandler) QueryStream(text string, options *QueryOptions, callback func(segment string, isComplete bool) error) (*QueryResponse, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	select {
	case <-h.interruptCh:
		return nil, errors.New("interrupted")
	default:
	}

	var rewrite *QueryRewrite
	promptUser := text
	if options.EnableQueryRewrite {
		rw, err := h.rewriteQueryAlibaba(h.ctx, promptUser, options)
		if err == nil && rw != "" {
			rewrite = &QueryRewrite{Original: promptUser, Rewritten: rw}
			promptUser = rw
		}
	}

	var expansion *QueryExpansion
	if options.EnableQueryExpansion {
		expanded, terms, err := h.expandQueryAlibaba(h.ctx, promptUser, options)
		if err == nil {
			expansion = &QueryExpansion{
				Original: promptUser,
				Expanded: expanded,
				Terms:    terms,
				Debug:    map[string]any{},
			}
			promptUser = expanded
		}
	}

	reqBody := map[string]any{
		"input": map[string]string{
			"prompt": h.composePrompt(promptUser, options),
		},
		"parameters": map[string]any{
			"incremental_output": true,
		},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", strings.TrimRight(h.endpoint, "/"), h.appID)
	reqCtx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-DashScope-SSE", "enable")

	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-h.interruptCh:
			cancel()
		}
	}()
	defer close(done)

	resp, err := h.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(reqCtx.Err(), context.Canceled) {
			return nil, errors.New("interrupted")
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("alibaba stream request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	extractText := func(m map[string]any) string {
		output, ok := m["output"].(map[string]any)
		if !ok {
			return ""
		}
		if v, ok := output["text"].(string); ok {
			return v
		}
		choices, ok := output["choices"].([]any)
		if !ok || len(choices) == 0 {
			return ""
		}
		first, ok := choices[0].(map[string]any)
		if !ok {
			return ""
		}
		msg, ok := first["message"].(map[string]any)
		if !ok {
			return ""
		}
		if v, ok := msg["content"].(string); ok {
			return v
		}
		return ""
	}

	var assembled strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 16*1024), 2*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}
		chunk := extractText(evt)
		if chunk == "" {
			continue
		}
		delta := chunk
		current := assembled.String()
		if strings.HasPrefix(chunk, current) {
			delta = chunk[len(current):]
		}
		if delta == "" {
			continue
		}
		assembled.WriteString(delta)
		if callback != nil {
			if err := callback(delta, false); err != nil {
				return nil, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(reqCtx.Err(), context.Canceled) {
			return nil, errors.New("interrupted")
		}
		return nil, err
	}
	if callback != nil {
		if err := callback("", true); err != nil {
			return nil, err
		}
	}
	finalText := strings.TrimSpace(assembled.String())
	return &QueryResponse{
		Provider:  h.Provider(),
		Model:     options.Model,
		Choices:   []QueryChoice{{Index: 0, Content: finalText, FinishReason: "stop"}},
		Expansion: expansion,
		Rewrite:   rewrite,
	}, nil
}

func (h *AlibabaHandler) Provider() string { return LLM_ALIBABA }

func (h *AlibabaHandler) Interrupt() {
	select {
	case h.interruptCh <- struct{}{}:
	default:
	}
}

func (h *AlibabaHandler) ResetMemory() {
	// no-op: in-memory conversation removed
}

func (h *AlibabaHandler) SummarizeMemory(model string) (string, error) {
	return "", nil
}

func (h *AlibabaHandler) SetMaxMemoryMessages(n int) {
	// no-op: in-memory conversation removed
}

func (h *AlibabaHandler) GetMaxMemoryMessages() int {
	return defaultMaxMemoryMessages
}

func (h *AlibabaHandler) composePrompt(currentUser string, opts *QueryOptions) string {
	currentUser = strings.TrimSpace(currentUser)
	var b strings.Builder
	if s := appendEmotionalStyle(strings.TrimSpace(h.systemPrompt), opts); s != "" {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("用户输入：")
	b.WriteString(currentUser)
	out := strings.TrimSpace(b.String())
	if out == "" {
		return currentUser
	}
	return out
}

func (h *AlibabaHandler) rewriteQueryAlibaba(ctx context.Context, text string, options *QueryOptions) (string, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	prompt := BuildQueryRewriteUserPrompt(text, options.QueryRewriteInstruction)
	reqBody := map[string]any{
		"input": map[string]string{
			"prompt": prompt,
		},
		"parameters": map[string]any{},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", strings.TrimRight(h.endpoint, "/"), h.appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("alibaba rewrite failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Output struct {
			Text string `json:"text"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	out := NormalizeRewrittenQuery(parsed.Output.Text)
	if out == "" {
		return strings.TrimSpace(text), nil
	}
	return out, nil
}

func (h *AlibabaHandler) expandQueryAlibaba(ctx context.Context, text string, options *QueryOptions) (string, []string, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	maxTerms := expansionMaxTerms(options)
	sep := expansionSeparator(options)
	prompt := BuildQueryExpansionUserPrompt(text, maxTerms)
	reqBody := map[string]any{
		"input": map[string]string{
			"prompt": prompt,
		},
		"parameters": map[string]any{},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}
	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", strings.TrimRight(h.endpoint, "/"), h.appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("alibaba expansion failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Output struct {
			Text string `json:"text"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", nil, err
	}
	out := strings.TrimSpace(parsed.Output.Text)
	expanded, terms := ExpandedQueryFromModelAnswer(text, out, maxTerms, sep)
	return expanded, terms, nil
}

func (h *AlibabaHandler) summarizeAlibaba(ctx context.Context, model string, transcript string, previousSummary string) (string, error) {
	_ = model
	system := "You are a conversation summarizer. Produce a concise, factual summary of the conversation so far. Preserve user preferences, facts, decisions, and open TODOs. Do not include any markdown."
	user := ""
	if strings.TrimSpace(previousSummary) != "" {
		user += "Existing summary:\n" + previousSummary + "\n\n"
	}
	user += "Conversation transcript:\n" + transcript + "\n\nReturn an updated summary in plain text."
	prompt := system + "\n\n" + user
	reqBody := map[string]any{
		"input": map[string]string{
			"prompt": prompt,
		},
		"parameters": map[string]any{},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/api/v1/apps/%s/completion", strings.TrimRight(h.endpoint, "/"), h.appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("alibaba summarize failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Output struct {
			Text string `json:"text"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	return strings.TrimSpace(parsed.Output.Text), nil
}
