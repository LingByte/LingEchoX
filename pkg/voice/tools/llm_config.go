package tools

// Copyright (c) 2026 LingByte. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0

import (
	"context"
	"fmt"
	"sync"

	"github.com/LingByte/SoulNexus/pkg/llm"
	"go.uber.org/zap"
)

// LLMConfig LLM 配置
type LLMConfig struct {
	Provider     string // openai, coze, ollama
	APIKey       string
	BaseURL      string
	Model        string // 默认模型（如 gpt-4, qwen-plus 等）
	SystemPrompt string
	MaxTokens    int // 默认最大 token 数
}

// LLMService LLM 服务封装
type LLMService struct {
	provider llm.LLMHandler
	config   *LLMConfig
	logger   *zap.Logger
	tools    map[string]ToolRegistration
	toolsMu  sync.RWMutex
}

// ToolCallback defines a hardware tool callback signature.
type ToolCallback func(args map[string]interface{}, llmService interface{}) (string, error)

// ToolRegistration stores tool metadata for compatibility.
type ToolRegistration struct {
	Name        string
	Description string
	Parameters  interface{}
	Callback    ToolCallback
}

// NewLLMService 创建 LLM 服务
func NewLLMService(config *LLMConfig, logger *zap.Logger) (*LLMService, error) {
	if config == nil {
		return nil, fmt.Errorf("LLM 配置不能为空")
	}
	provider, err := llm.NewLLMProvider(
		context.Background(),
		config.Provider,
		config.APIKey,
		config.BaseURL,
		config.SystemPrompt,
	)
	if err != nil {
		return nil, fmt.Errorf("创建 LLM Provider 失败: %w", err)
	}

	return &LLMService{
		provider: provider,
		config:   config,
		logger:   logger,
		tools:    make(map[string]ToolRegistration),
	}, nil
}

// RegisterTool 注册工具
func (s *LLMService) RegisterTool(name, description string, parameters interface{}, callback ToolCallback) {
	s.toolsMu.Lock()
	s.tools[name] = ToolRegistration{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Callback:    callback,
	}
	s.toolsMu.Unlock()
	s.logger.Info("注册 LLM 工具",
		zap.String("name", name),
		zap.String("description", description))
}

// Query 同步查询
func (s *LLMService) Query(text string, model string) (string, error) {
	return s.provider.Query(text, model)
}

// QueryWithOptions 带选项的同步查询
func (s *LLMService) QueryWithOptions(text string, options llm.QueryOptions) (string, error) {
	opts := s.buildOptions(options)
	resp, err := s.provider.QueryWithOptions(text, &opts)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Content, nil
}

// QueryStream 流式查询
func (s *LLMService) QueryStream(text string, callback func(string, bool) error, options ...llm.QueryOptions) error {
	var opts llm.QueryOptions
	if len(options) > 0 {
		opts = options[0]
	}
	opts = s.buildOptions(opts)

	_, err := s.provider.QueryStream(text, &opts, callback)
	return err
}

// buildOptions 构建查询选项
func (s *LLMService) buildOptions(options llm.QueryOptions) llm.QueryOptions {
	if options.Model == "" {
		options.Model = s.config.Model
	}
	if options.MaxTokens <= 0 {
		options.MaxTokens = s.config.MaxTokens
	}
	return options
}

// GetProvider 获取底层 Provider（用于特殊场景）
func (s *LLMService) GetProvider() llm.LLMHandler {
	return s.provider
}

// Interrupt 中断当前 LLM 请求
func (s *LLMService) Interrupt() {
	if s.provider != nil {
		s.provider.Interrupt()
	}
}

// GetConfig 获取配置
func (s *LLMService) GetConfig() *LLMConfig {
	return s.config
}

// ListTools 列出已注册的工具
func (s *LLMService) ListTools() []string {
	s.toolsMu.RLock()
	defer s.toolsMu.RUnlock()
	tools := make([]string, 0, len(s.tools))
	for name := range s.tools {
		tools = append(tools, name)
	}
	return tools
}

// ResetMessages 重置对话历史
func (s *LLMService) ResetMessages() {
	s.provider.ResetMemory()
}

// SetSystemPrompt 设置系统提示词
func (s *LLMService) SetSystemPrompt(prompt string) {
	s.config.SystemPrompt = prompt
	s.logger.Debug("当前 LLM 实现不支持运行时修改 system prompt，已更新本地配置缓存")
}
