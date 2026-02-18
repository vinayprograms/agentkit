package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements the Provider interface using the official Anthropic SDK.
type AnthropicProvider struct {
	client    *anthropic.Client
	model     string
	maxTokens int
	thinking  ThinkingConfig
	retry     RetryConfig
}

// AnthropicConfig holds configuration for the Anthropic provider.
type AnthropicConfig struct {
	APIKey    string
	BaseURL   string // Optional custom endpoint
	Model     string
	MaxTokens int
	Thinking  ThinkingConfig
	Retry     RetryConfig
}

// NewAnthropicProvider creates a new Anthropic provider using the official SDK.
func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for anthropic")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required for anthropic")
	}
	if cfg.MaxTokens == 0 {
		return nil, fmt.Errorf("max_tokens is required for anthropic")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := anthropic.NewClient(opts...)

	return &AnthropicProvider{
		client:    &client,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		thinking:  cfg.Thinking,
		retry:     cfg.Retry,
	}, nil
}

// getRetryConfig returns effective retry settings with defaults.
func (p *AnthropicProvider) getRetryConfig() (maxRetries int, initBackoff, maxBackoff time.Duration) {
	maxRetries = p.retry.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	initBackoff = p.retry.InitBackoff
	if initBackoff <= 0 {
		initBackoff = defaultInitBackoff
	}
	maxBackoff = p.retry.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = defaultMaxBackoff
	}
	return
}

// Chat implements the Provider interface.
func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Convert messages to Anthropic format
	var systemPrompt string
	messages := make([]anthropic.MessageParam, 0, len(req.Messages))

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			systemPrompt = m.Content
		case "user":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(m.Content),
			))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				// Assistant message with tool calls
				blocks := make([]anthropic.ContentBlockParamUnion, 0)
				if m.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(m.Content))
				}
				for _, tc := range m.ToolCalls {
					blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Args, tc.Name))
				}
				messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			} else {
				messages = append(messages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(m.Content),
				))
			}
		case "tool":
			// Tool result message
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(m.ToolCallID, m.Content, false),
			))
		}
	}

	// Convert tools to Anthropic format
	tools := make([]anthropic.ToolUnionParam, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: t.Parameters["properties"],
				},
			},
		})
	}

	maxTokens := int64(p.maxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}

	// Build request params
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	// Add thinking if configured
	thinkingLevel := ResolveThinkingLevel(p.thinking, req.Messages, req.Tools)
	useStreaming := false
	if thinkingLevel != ThinkingOff {
		budget := ThinkingLevelToAnthropicBudget(thinkingLevel, p.thinking.BudgetTokens)
		if budget > 0 {
			// Anthropic requires max_tokens > thinking.budget_tokens
			// Add minimum 1024 for response beyond thinking
			minMaxTokens := budget + 1024
			if maxTokens < minMaxTokens {
				maxTokens = minMaxTokens
				params.MaxTokens = maxTokens
			}
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfEnabled: &anthropic.ThinkingConfigEnabledParam{
					BudgetTokens: int64(budget),
				},
			}
			// Anthropic requires streaming for extended thinking
			useStreaming = true
		}
	}

	maxRetries, initBackoff, maxBackoff := p.getRetryConfig()

	if useStreaming {
		return p.chatStreaming(ctx, params, maxRetries, initBackoff, maxBackoff)
	}
	return p.chatNonStreaming(ctx, params, maxRetries, initBackoff, maxBackoff)
}

// chatNonStreaming makes a non-streaming request with retry.
func (p *AnthropicProvider) chatNonStreaming(
	ctx context.Context,
	params anthropic.MessageNewParams,
	maxRetries int,
	initBackoff, maxBackoff time.Duration,
) (*ChatResponse, error) {
	var resp *anthropic.Message
	var err error
	backoff := initBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = p.client.Messages.New(ctx, params)
		if err == nil {
			break
		}

		if isBillingError(err) {
			return nil, fmt.Errorf("billing/payment error (fatal): %w", err)
		}

		if !isRetryableError(err) {
			return nil, fmt.Errorf("anthropic request failed: %w", err)
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("anthropic request failed after %d retries: %w", maxRetries, err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// Convert response
	result := &ChatResponse{
		StopReason:   string(resp.StopReason),
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		Model:        string(resp.Model),
	}

	// Extract content and tool calls from response
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Thinking += block.Thinking
		case "tool_use":
			// Input is json.RawMessage
			var args map[string]interface{}
			if block.Input != nil {
				json.Unmarshal(block.Input, &args)
			}
			result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
				ID:   block.ID,
				Name: block.Name,
				Args: args,
			})
		}
	}

	return result, nil
}

// chatStreaming makes a streaming request (required for extended thinking).
func (p *AnthropicProvider) chatStreaming(
	ctx context.Context,
	params anthropic.MessageNewParams,
	maxRetries int,
	initBackoff, maxBackoff time.Duration,
) (*ChatResponse, error) {
	backoff := initBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := p.doStreamingRequest(ctx, params)
		if err == nil {
			return result, nil
		}

		if isBillingError(err) {
			return nil, fmt.Errorf("billing/payment error (fatal): %w", err)
		}

		if !isRetryableError(err) {
			return nil, fmt.Errorf("anthropic streaming request failed: %w", err)
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("anthropic streaming request failed after %d retries: %w", maxRetries, err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return nil, fmt.Errorf("unreachable")
}

// doStreamingRequest executes a single streaming request.
func (p *AnthropicProvider) doStreamingRequest(
	ctx context.Context,
	params anthropic.MessageNewParams,
) (*ChatResponse, error) {
	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	result := &ChatResponse{}

	// Track content blocks by index
	type blockState struct {
		blockType   string // "text", "thinking", "tool_use"
		toolID      string
		toolName    string
		textBuilder strings.Builder
	}
	blocks := make(map[int64]*blockState)

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			msg := event.AsMessageStart()
			result.Model = string(msg.Message.Model)
			result.InputTokens = int(msg.Message.Usage.InputTokens)

		case "content_block_start":
			evt := event.AsContentBlockStart()
			cb := evt.ContentBlock
			state := &blockState{blockType: cb.Type}

			switch cb.Type {
			case "tool_use":
				state.toolID = cb.ID
				state.toolName = cb.Name
			}

			blocks[evt.Index] = state

		case "content_block_delta":
			evt := event.AsContentBlockDelta()
			state, ok := blocks[evt.Index]
			if !ok {
				continue
			}

			delta := evt.Delta
			switch delta.Type {
			case "text_delta":
				state.textBuilder.WriteString(delta.Text)
			case "thinking_delta":
				state.textBuilder.WriteString(delta.Thinking)
			case "input_json_delta":
				state.textBuilder.WriteString(delta.PartialJSON)
			}

		case "content_block_stop":
			evt := event.AsContentBlockStop()
			state, ok := blocks[evt.Index]
			if !ok {
				continue
			}

			text := state.textBuilder.String()
			switch state.blockType {
			case "text":
				result.Content += text
			case "thinking":
				result.Thinking += text
			case "tool_use":
				var args map[string]interface{}
				if text != "" {
					json.Unmarshal([]byte(text), &args)
				}
				result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
					ID:   state.toolID,
					Name: state.toolName,
					Args: args,
				})
			}

		case "message_delta":
			evt := event.AsMessageDelta()
			result.StopReason = string(evt.Delta.StopReason)
			result.OutputTokens = int(evt.Usage.OutputTokens)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
