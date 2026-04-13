package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

var emojiRegex = regexp.MustCompile(`([\x{00A9}\x{00AE}\x{203C}\x{2049}\x{2122}\x{2139}\x{2194}-\x{2199}\x{21A9}-\x{21AA}\x{231A}-\x{231B}\x{2328}\x{23CF}\x{23E9}-\x{23F3}\x{23F8}-\x{23FA}\x{24C2}\x{25AA}-\x{25AB}\x{25B6}\x{25C0}\x{25FB}-\x{25FE}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{2B05}-\x{2B07}\x{2B1B}-\x{2B1C}\x{2B50}\x{2B55}\x{3030}\x{303D}\x{3297}\x{3299}\x{1F004}\x{1F0CF}\x{1F170}-\x{1F251}\x{1F300}-\x{1F5FF}\x{1F600}-\x{1F64F}\x{1F680}-\x{1F6FF}\x{1F910}-\x{1F93E}\x{1F940}-\x{1F94C}\x{1F950}-\x{1F96B}\x{1F980}-\x{1F997}\x{1F9C0}-\x{1F9E6}\x{1FA70}-\x{1FA74}\x{1FA78}-\x{1FA7A}\x{1FA80}-\x{1FA86}\x{1FA90}-\x{1FAA8}\x{1FAB0}-\x{1FAB6}\x{1FAC0}-\x{1FAC2}\x{1FAD0}-\x{1FAD6}\x{1F1E6}-\x{1F1FF}\x{200D}\x{FE0F}])`)

const defaultMaxMemoryMessages = 40

const maxSummarizeInputChars = 12000

func buildTranscriptForSummary(messages []openai.ChatCompletionMessage) string {
	if len(messages) == 0 {
		return ""
	}
	b := strings.Builder{}
	for _, m := range messages {
		role := m.Role
		if role == "" {
			role = "unknown"
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
		if b.Len() >= maxSummarizeInputChars {
			break
		}
	}
	s := b.String()
	if len(s) > maxSummarizeInputChars {
		s = s[len(s)-maxSummarizeInputChars:]
	}
	return s
}

type OpenaiHandler struct {
	ctx             context.Context
	client          *openai.Client
	provider        string
	systemPrompt    string
	fewShotExamples []FewShotExample
	baseUrl         string
	logger          *zap.Logger
	mutex           sync.Mutex
	cancelMu        sync.Mutex
	currentCancel   context.CancelFunc
	currentCancelID uint64
	cancelSeq       uint64
}

func NewOpenaiHandler(ctx context.Context, llmOptions *LLMOptions) (*OpenaiHandler, error) {
	if llmOptions == nil {
		return nil, errors.New("options cannot be nil")
	}
	log := llmOptions.Logger
	if log == nil {
		log = zap.NewNop()
	}
	config := openai.DefaultConfig(llmOptions.ApiKey)
	config.BaseURL = llmOptions.BaseURL
	client := openai.NewClientWithConfig(config)
	return &OpenaiHandler{
		ctx:             ctx,
		client:          client,
		provider:        LLM_OPENAI,
		baseUrl:         llmOptions.BaseURL,
		systemPrompt:    llmOptions.SystemPrompt,
		fewShotExamples: llmOptions.FewShotExamples,
		logger:          log,
	}, nil
}

func newOpenAICompatibleHandler(ctx context.Context, llmOptions *LLMOptions, provider string) (*OpenaiHandler, error) {
	h, err := NewOpenaiHandler(ctx, llmOptions)
	if err != nil {
		return nil, err
	}
	h.provider = provider
	return h, nil
}

func (oh *OpenaiHandler) ResetMemory() {
	// no-op: in-memory conversation removed
}

func (oh *OpenaiHandler) SummarizeMemory(model string) (string, error) {
	return "", nil
}

func (oh *OpenaiHandler) generateSummary(model string, messages []openai.ChatCompletionMessage, previousSummary string) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}
	transcript := buildTranscriptForSummary(messages)
	requestID := GenerateLingRequestID()

	system := "You are a conversation summarizer. Produce a concise, factual summary of the conversation so far. Preserve user preferences, facts, decisions, and open TODOs. Do not include any markdown."
	user := ""
	if previousSummary != "" {
		user += "Existing summary:\n" + previousSummary + "\n\n"
	}
	user += "Conversation transcript:\n" + transcript + "\n\n"
	user += "Return an updated summary in plain text."

	req := openai.ChatCompletionRequest{
		Model: model,
		User:  requestID,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		MaxTokens:   512,
		Temperature: 0,
		TopP:        1,
	}

	resp, err := oh.client.CreateChatCompletion(oh.ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	newSummary := strings.TrimSpace(resp.Choices[0].Message.Content)
	if newSummary == "" {
		return "", nil
	}
	if emojiRegex != nil {
		newSummary = emojiRegex.ReplaceAllString(newSummary, "")
	}
	return newSummary, nil
}

func (oh *OpenaiHandler) startAsyncSummarizeIfNeeded(model string, snapshot []openai.ChatCompletionMessage, previousSummary string, seq uint64) {
	// no-op: in-memory conversation removed
}

func (oh *OpenaiHandler) compactMessagesKeepNewer(snapshotLen int) {
	// no-op: in-memory conversation removed
}

func (oh *OpenaiHandler) SetMaxMemoryMessages(n int) {
	// no-op: in-memory conversation removed
}

func (oh *OpenaiHandler) GetMaxMemoryMessages() int {
	return defaultMaxMemoryMessages
}

func (oh *OpenaiHandler) Provider() string {
	if strings.TrimSpace(oh.provider) == "" {
		return LLM_OPENAI
	}
	return oh.provider
}

func (oh *OpenaiHandler) Interrupt() {
	oh.cancelMu.Lock()
	cancel := oh.currentCancel
	oh.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (oh *OpenaiHandler) setCurrentCancel(cancel context.CancelFunc) uint64 {
	id := atomic.AddUint64(&oh.cancelSeq, 1)
	oh.cancelMu.Lock()
	oh.currentCancel = cancel
	oh.currentCancelID = id
	oh.cancelMu.Unlock()
	return id
}

func (oh *OpenaiHandler) clearCurrentCancel(id uint64) {
	oh.cancelMu.Lock()
	if oh.currentCancelID == id {
		oh.currentCancel = nil
		oh.currentCancelID = 0
	}
	oh.cancelMu.Unlock()
}

func (oh *OpenaiHandler) Query(text, model string) (string, error) {
	resp, err := oh.QueryWithOptions(text, &QueryOptions{
		Model: model,
	})
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("empty response choices")
	}
	return resp.Choices[0].Content, nil
}

func (oh *OpenaiHandler) QueryWithOptions(text string, options *QueryOptions) (*QueryResponse, error) {
	if options == nil {
		options = &QueryOptions{}
	}

	var rewrite *QueryRewrite
	if options.EnableQueryRewrite {
		before := text
		rwOut, err := oh.rewriteQueryStateless(before, options)
		if err != nil && oh.logger != nil {
			oh.logger.Warn("Query rewrite failed", zap.Error(err))
		} else if err == nil && rwOut != "" {
			rewrite = &QueryRewrite{Original: before, Rewritten: rwOut}
			text = rwOut
		}
	}

	var expansion *QueryExpansion
	if options.EnableQueryExpansion {
		expandedText, terms, err := oh.expandQueryStateless(text, options)
		if err != nil && oh.logger != nil {
			oh.logger.Warn("Query expansion failed", zap.Error(err))
		} else if err == nil {
			expansion = &QueryExpansion{
				Original: text,
				Expanded: expandedText,
				Terms:    terms,
				Debug:    map[string]any{},
			}
			text = expandedText
		}
	}

	n := options.N
	if n <= 0 {
		n = 1
	}
	model := options.Model
	if model == "" {
		model = "qwen-plus"
	}
	estimatedMaxOutputChars := 0
	if options.MaxTokens > 0 {
		estimatedMaxOutputChars = options.MaxTokens * 4
	}

	requestID := GenerateLingRequestID()
	requestedOutputFormat := options.OutputFormat
	if requestedOutputFormat == "" && (options.EnableJSONOutput || options.EnableSelfQueryJSONOutput) {
		requestedOutputFormat = OutputFormatJSONObject
	}
	requestedOutputFormatLower := strings.ToLower(requestedOutputFormat)
	requiresJSONWrapper := requestedOutputFormatLower == OutputFormatXML ||
		requestedOutputFormatLower == OutputFormatHTML ||
		requestedOutputFormatLower == OutputFormatSQL
	responseFormatApplied := false
	appliedResponseFormat := ""
	var responseFormat *openai.ChatCompletionResponseFormat
	switch requestedOutputFormatLower {
	case "", OutputFormatText:
		// default
	case OutputFormatJSON, OutputFormatJSONObject:
		responseFormatApplied = true
		appliedResponseFormat = OutputFormatJSONObject
		responseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
	case OutputFormatJSONSchema:
		// json_schema requires a schema object; since QueryOptions doesn't carry it yet, fallback to json_object.
		responseFormatApplied = true
		appliedResponseFormat = OutputFormatJSONObject
		responseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
	default:
		if requiresJSONWrapper {
			responseFormatApplied = true
			appliedResponseFormat = OutputFormatJSONObject
			responseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
		}
	}

	formatInstruction := ""
	if requiresJSONWrapper {
		formatInstruction = "Return a JSON object with keys: format, content. format must be \"" + requestedOutputFormatLower + "\". content must be strictly " + requestedOutputFormatLower + " and must not be wrapped in markdown."
	}

	extractStructuredContent := func(raw string) string {
		var obj map[string]any
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			return raw
		}
		if v, ok := obj["content"]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return raw
	}

	sanitizedMessages := make([]openai.ChatCompletionMessage, 0)
	sysContent := appendEmotionalStyle(oh.systemPrompt, options)
	if strings.TrimSpace(sysContent) != "" {
		sanitizedMessages = append(sanitizedMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: sysContent,
		})
	}
	if len(oh.fewShotExamples) > 0 {
		for _, ex := range oh.fewShotExamples {
			u := strings.TrimSpace(ex.User)
			a := strings.TrimSpace(ex.Assistant)
			if u != "" {
				sanitizedMessages = append(sanitizedMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: u})
			}
			if a != "" {
				sanitizedMessages = append(sanitizedMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: a})
			}
		}
	}

	if formatInstruction != "" {
		sanitizedMessages = append(sanitizedMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: formatInstruction,
		})
	}
	if estimatedMaxOutputChars > 0 {
		sanitizedMessages = append(sanitizedMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: fmt.Sprintf("Limit the assistant output to at most %d characters.", estimatedMaxOutputChars),
		})
	}

	sanitizedMessages = append(sanitizedMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: text,
	})
	request := openai.ChatCompletionRequest{
		Model:          model,
		N:              n,
		User:           requestID,
		ResponseFormat: responseFormat,
		Messages:       sanitizedMessages,
	}
	if options.MaxTokens > 0 {
		request.MaxTokens = options.MaxTokens
	}
	if options.Temperature != 0 {
		request.Temperature = options.Temperature
	}
	if options.TopP != 0 {
		request.TopP = options.TopP
	}
	if options.LogitBias != nil {
		request.LogitBias = options.LogitBias
	}
	reqCtx, cancel := context.WithCancel(oh.ctx)
	cancelID := oh.setCurrentCancel(cancel)
	defer func() {
		oh.clearCurrentCancel(cancelID)
		cancel()
	}()
	response, err := oh.client.CreateChatCompletion(reqCtx, request)
	if err != nil {
		return nil, err
	}
	choices := make([]QueryChoice, 0, len(response.Choices))
	for i, c := range response.Choices {
		content := c.Message.Content
		if requiresJSONWrapper {
			content = extractStructuredContent(content)
		}
		if options.FilterEmoji {
			content = emojiRegex.ReplaceAllString(content, "")
		}
		choices = append(choices, QueryChoice{
			Index:        i,
			Content:      content,
			FinishReason: string(c.FinishReason),
		})
	}
	resp := &QueryResponse{
		Provider: oh.Provider(),
		Model:    response.Model,
		Choices:  choices,
		Usage: &TokenUsage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
			PromptTokensDetails: func() *PromptTokensDetails {
				if response.Usage.PromptTokensDetails == nil {
					return nil
				}
				return &PromptTokensDetails{
					AudioTokens:  response.Usage.PromptTokensDetails.AudioTokens,
					CachedTokens: response.Usage.PromptTokensDetails.CachedTokens,
				}
			}(),
			CompletionTokensDetails: func() *CompletionTokensDetails {
				if response.Usage.CompletionTokensDetails == nil {
					return nil
				}
				return &CompletionTokensDetails{
					AudioTokens:              response.Usage.CompletionTokensDetails.AudioTokens,
					ReasoningTokens:          response.Usage.CompletionTokensDetails.ReasoningTokens,
					AcceptedPredictionTokens: response.Usage.CompletionTokensDetails.AcceptedPredictionTokens,
					RejectedPredictionTokens: response.Usage.CompletionTokensDetails.RejectedPredictionTokens,
				}
			}(),
		},
		Expansion: expansion,
		Rewrite:   rewrite,
	}

	llmDetails := &LLMDetails{
		RequestID:               requestID,
		Provider:                oh.Provider(),
		BaseURL:                 oh.baseUrl,
		Model:                   response.Model,
		Input:                   text,
		SystemPrompt:            oh.systemPrompt,
		N:                       n,
		MaxTokens:               options.MaxTokens,
		EstimatedMaxOutputChars: estimatedMaxOutputChars,
		FilterEmoji:             options.FilterEmoji,
		RequestedOutputFormat:   requestedOutputFormatLower,
		AppliedResponseFormat:   appliedResponseFormat,
		ResponseFormatApplied:   responseFormatApplied,
		ResponseID:              response.ID,
		Object:                  response.Object,
		Created:                 response.Created,
		SystemFingerprint:       response.SystemFingerprint,
		ChoicesCount:            len(response.Choices),
		Choices:                 resp.Choices,
		Usage:                   resp.Usage,
	}
	if b, e := json.Marshal(response.PromptFilterResults); e == nil {
		llmDetails.PromptFilterResultsJSON = string(b)
	}
	if b, e := json.Marshal(response.ServiceTier); e == nil {
		llmDetails.ServiceTierJSON = string(b)
	}
	if b, e := json.Marshal(response.Usage); e == nil {
		llmDetails.UsageRawJSON = string(b)
	}
	if b, e := json.Marshal(response.Choices); e == nil {
		llmDetails.ChoicesRawJSON = string(b)
	}
	if b, e := json.Marshal(response); e == nil {
		llmDetails.RawResponseJSON = string(b)
	}
	return resp, nil
}

func (oh *OpenaiHandler) QueryStream(text string, options *QueryOptions, callback func(segment string, isComplete bool) error) (*QueryResponse, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	var streamRewrite *QueryRewrite
	if options.EnableQueryRewrite {
		before := text
		rwOut, err := oh.rewriteQueryStateless(before, options)
		if err != nil && oh.logger != nil {
			oh.logger.Warn("Query rewrite failed", zap.Error(err))
		} else if err == nil && rwOut != "" {
			streamRewrite = &QueryRewrite{Original: before, Rewritten: rwOut}
			text = rwOut
		}
	}
	var streamExpansion *QueryExpansion
	if options.EnableQueryExpansion {
		expandedText, terms, err := oh.expandQueryStateless(text, options)
		if err != nil && oh.logger != nil {
			oh.logger.Warn("Query expansion failed", zap.Error(err))
		} else if err == nil {
			streamExpansion = &QueryExpansion{
				Original: text,
				Expanded: expandedText,
				Terms:    terms,
				Debug:    map[string]any{},
			}
			text = expandedText
		}
	}
	n := options.N
	if n <= 0 {
		n = 1
	}
	model := options.Model
	if model == "" {
		model = "qwen-plus"
	}

	requestID := GenerateLingRequestID()
	messages := make([]openai.ChatCompletionMessage, 0)
	sysContent := appendEmotionalStyle(oh.systemPrompt, options)
	if strings.TrimSpace(sysContent) != "" {
		messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: sysContent})
	}
	if len(oh.fewShotExamples) > 0 {
		for _, ex := range oh.fewShotExamples {
			u := strings.TrimSpace(ex.User)
			a := strings.TrimSpace(ex.Assistant)
			if u != "" {
				messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: u})
			}
			if a != "" {
				messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: a})
			}
		}
	}

	messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: text})

	request := openai.ChatCompletionRequest{
		Model:    model,
		N:        n,
		User:     requestID,
		Messages: messages,
		Stream:   true,
	}
	if options.MaxTokens > 0 {
		request.MaxTokens = options.MaxTokens
	}
	if options.Temperature != 0 {
		request.Temperature = options.Temperature
	}
	if options.TopP != 0 {
		request.TopP = options.TopP
	}
	if options.LogitBias != nil {
		request.LogitBias = options.LogitBias
	}

	reqCtx, cancel := context.WithCancel(oh.ctx)
	cancelID := oh.setCurrentCancel(cancel)
	defer func() {
		oh.clearCurrentCancel(cancelID)
		cancel()
	}()
	stream, err := oh.client.CreateChatCompletionStream(reqCtx, request)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var content strings.Builder
	for {
		chunkResp, e := stream.Recv()
		if errors.Is(e, context.Canceled) {
			return nil, e
		}
		if e != nil {
			if errors.Is(e, io.EOF) {
				break
			}
			return nil, e
		}
		if len(chunkResp.Choices) == 0 {
			continue
		}
		delta := chunkResp.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		if options.FilterEmoji {
			delta = emojiRegex.ReplaceAllString(delta, "")
		}
		content.WriteString(delta)
		if callback != nil {
			if err := callback(delta, false); err != nil {
				return nil, err
			}
		}
	}
	if callback != nil {
		if err := callback("", true); err != nil {
			return nil, err
		}
	}

	finalText := strings.TrimSpace(content.String())
	return &QueryResponse{
		Provider: oh.Provider(),
		Model:    model,
		Choices: []QueryChoice{
			{Index: 0, Content: finalText, FinishReason: "stop"},
		},
		Rewrite:   streamRewrite,
		Expansion: streamExpansion,
	}, nil
}

// expandQueryStateless runs expansion via a one-shot completion without touching conversation memory or oh.mutex.
func (oh *OpenaiHandler) expandQueryStateless(text string, options *QueryOptions) (string, []string, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	maxTerms := expansionMaxTerms(options)
	separator := expansionSeparator(options)
	prompt := BuildQueryExpansionUserPrompt(text, maxTerms)
	model := strings.TrimSpace(options.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}
	req := openai.ChatCompletionRequest{
		Model: model,
		User:  GenerateLingRequestID(),
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   256,
		Temperature: 0.2,
		TopP:        1,
	}
	resp, err := oh.client.CreateChatCompletion(oh.ctx, req)
	if err != nil {
		return "", nil, err
	}
	if len(resp.Choices) == 0 {
		return strings.TrimSpace(text), nil, nil
	}
	out := strings.TrimSpace(resp.Choices[0].Message.Content)
	expanded, terms := ExpandedQueryFromModelAnswer(text, out, maxTerms, separator)
	return expanded, terms, nil
}

func (oh *OpenaiHandler) rewriteQueryStateless(text string, options *QueryOptions) (string, error) {
	if options == nil {
		options = &QueryOptions{}
	}
	prompt := BuildQueryRewriteUserPrompt(text, options.QueryRewriteInstruction)
	model := queryRewriteModel(options, "gpt-4o-mini")
	req := openai.ChatCompletionRequest{
		Model: model,
		User:  GenerateLingRequestID(),
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   128,
		Temperature: 0.2,
		TopP:        1,
	}
	resp, err := oh.client.CreateChatCompletion(oh.ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return strings.TrimSpace(text), nil
	}
	out := NormalizeRewrittenQuery(resp.Choices[0].Message.Content)
	if out == "" {
		return strings.TrimSpace(text), nil
	}
	return out, nil
}
