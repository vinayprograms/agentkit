package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic beta feature headers. These are versioned and may change as features graduate.
const (
	anthropicOAuthBetaHeader = "oauth-2025-04-20"
)

// anthropicAuthOptions returns the SDK options for authenticating with Anthropic.
func anthropicAuthOptions(cfg anthropicConfig) []option.RequestOption {
	if cfg.IsOAuthToken {
		return []option.RequestOption{
			option.WithAuthToken(cfg.APIKey),
			option.WithHeader("anthropic-beta", anthropicOAuthBetaHeader),
		}
	}
	return []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
}

// anthropicModel implements the Provider interface using the official Anthropic SDK.
type anthropicModel struct {
	client    *anthropic.Client
	model     string
	maxTokens int
	thinking  ThinkingConfig
	retry     RetryConfig
}

// anthropicConfig holds configuration for the Anthropic provider.
type anthropicConfig struct {
	APIKey       string
	IsOAuthToken bool   // True if APIKey is an OAuth access token (uses Bearer auth)
	BaseURL      string // Optional custom endpoint
	Model        string
	MaxTokens    int
	Thinking     ThinkingConfig
	Retry        RetryConfig
}

// newAnthropic creates a new Anthropic provider using the official SDK.
func newAnthropic(cfg anthropicConfig) (*anthropicModel, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for anthropic")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required for anthropic")
	}
	if cfg.MaxTokens == 0 {
		return nil, fmt.Errorf("max_tokens is required for anthropic")
	}

	opts := anthropicAuthOptions(cfg)
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := anthropic.NewClient(opts...)

	return &anthropicModel{
		client:    &client,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		thinking:  cfg.Thinking,
		retry:     cfg.Retry,
	}, nil
}

// Chat implements the Provider interface.
func (p *anthropicModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	systemPrompt, messages := toAnthropicMessages(req.Messages)
	tools := toAnthropicTools(req.Tools)

	maxTokens := int64(p.maxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{
				Text:         systemPrompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		}
	}

	if len(tools) > 0 {
		tools[len(tools)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		params.Tools = tools
	}

	applyAnthropicThinking(p.thinking, req, &params, &maxTokens)

	return p.chatStreaming(ctx, params)
}

// toAnthropicMessages converts generic messages to Anthropic format,
// extracting the system prompt separately.
func toAnthropicMessages(msgs []Message) (string, []anthropic.MessageParam) {
	var systemPrompt string
	messages := make([]anthropic.MessageParam, 0, len(msgs))

	for _, m := range msgs {
		switch m.Role {
		case "system":
			systemPrompt = m.Content
		case "user":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(m.Content),
			))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				blocks := make([]anthropic.ContentBlockParamUnion, 0)
				if m.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(m.Content))
				}
				for _, tc := range m.ToolCalls {
					args := tc.Args
					if args == nil {
						args = make(map[string]interface{})
					}
					blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, args, tc.Name))
				}
				messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			} else {
				messages = append(messages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(m.Content),
				))
			}
		case "tool":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(m.ToolCallID, m.Content, false),
			))
		}
	}

	return systemPrompt, messages
}

// toAnthropicTools converts generic tool definitions to Anthropic format.
func toAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: t.Parameters["properties"],
				},
			},
		})
	}
	return result
}

// thinkingLevelToAnthropicBudget converts a thinking level to Anthropic budget tokens.
func thinkingLevelToAnthropicBudget(level ThinkingLevel, configBudget int64) int64 {
	if configBudget > 0 {
		return configBudget
	}
	switch level {
	case ThinkingHigh:
		return 16000
	case ThinkingMedium:
		return 8000
	case ThinkingLow:
		return 4000
	default:
		return 0
	}
}

// applyAnthropicThinking configures extended thinking on the request params.
func applyAnthropicThinking(cfg ThinkingConfig, req ChatRequest, params *anthropic.MessageNewParams, maxTokens *int64) {
	level := ResolveThinkingLevel(cfg, req.Messages, req.Tools)
	if level == ThinkingOff {
		return
	}
	budget := thinkingLevelToAnthropicBudget(level, cfg.BudgetTokens)
	if budget <= 0 {
		return
	}
	// Anthropic requires max_tokens > thinking.budget_tokens
	minMaxTokens := budget + 1024
	if *maxTokens < minMaxTokens {
		*maxTokens = minMaxTokens
		params.MaxTokens = *maxTokens
	}
	params.Thinking = anthropic.ThinkingConfigParamUnion{
		OfEnabled: &anthropic.ThinkingConfigEnabledParam{
			BudgetTokens: int64(budget),
		},
	}
}

// chatStreaming makes a streaming request (required for extended thinking).
func (p *anthropicModel) chatStreaming(
	ctx context.Context,
	params anthropic.MessageNewParams,
) (*ChatResponse, error) {
	return withRetry(ctx, p.retry, "anthropic", func() (*ChatResponse, error) {
		return p.doStreamingRequest(ctx, params)
	})
}

// doStreamingRequest executes a single streaming request.
func (p *anthropicModel) doStreamingRequest(
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
			result.CacheCreationInputTokens = int(msg.Message.Usage.CacheCreationInputTokens)
			result.CacheReadInputTokens = int(msg.Message.Usage.CacheReadInputTokens)

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
					if err := json.Unmarshal([]byte(text), &args); err != nil {
						return nil, fmt.Errorf("failed to parse tool call arguments for %s: %w", state.toolName, err)
					}
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
