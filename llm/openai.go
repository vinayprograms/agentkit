package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// openAIModel implements the Provider interface using the official OpenAI SDK.
type openAIModel struct {
	client    *openai.Client
	model     string
	maxTokens int
	thinking  ThinkingConfig
	retry     RetryConfig
}

// openAIConfig holds configuration for the OpenAI provider.
type openAIConfig struct {
	APIKey    string
	BaseURL   string // Optional custom endpoint
	Model     string
	MaxTokens int
	Thinking  ThinkingConfig
	Retry     RetryConfig
}

// newOpenAI creates a new OpenAI provider using the official SDK.
func newOpenAI(cfg openAIConfig) (*openAIModel, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for openai")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required for openai")
	}
	if cfg.MaxTokens == 0 {
		return nil, fmt.Errorf("max_tokens is required for openai")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	return &openAIModel{
		client:    &client,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		thinking:  cfg.Thinking,
		retry:     cfg.Retry,
	}, nil
}

// Chat implements the Provider interface.
func (p *openAIModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	messages := toOpenAIMessages(req.Messages)
	tools := toOpenAITools(req.Tools)

	maxTokens := int64(p.maxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}

	params := openai.ChatCompletionNewParams{
		Model:     shared.ChatModel(p.model),
		Messages:  messages,
		MaxTokens: openai.Int(maxTokens),
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	// Add reasoning effort for o1/o3 models
	thinkingLevel := ResolveThinkingLevel(p.thinking, req)
	if thinkingLevel != ThinkingOff && isReasoningModel(p.model) {
		var effort shared.ReasoningEffort
		switch thinkingLevel {
		case ThinkingHigh:
			effort = shared.ReasoningEffortHigh
		case ThinkingMedium:
			effort = shared.ReasoningEffortMedium
		case ThinkingLow:
			effort = shared.ReasoningEffortLow
		}
		params.ReasoningEffort = effort
	}

	applyOpenAIToolChoice(req.ToolChoice, &params)

	// Make request with retry
	resp, err := withRetry(ctx, p.retry, "openai", func() (*openai.ChatCompletion, error) {
		return p.client.Chat.Completions.New(ctx, params)
	})
	if err != nil {
		return nil, err
	}

	return fromOpenAIResponse(resp)
}

func fromOpenAIResponse(resp *openai.ChatCompletion) (*ChatResponse, error) {
	result := &ChatResponse{
		Model:        resp.Model,
		InputTokens:  int(resp.Usage.PromptTokens),
		OutputTokens: int(resp.Usage.CompletionTokens),
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.StopReason = string(choice.FinishReason)

		for _, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("failed to parse tool call arguments for %s: %w", tc.Function.Name, err)
			}
			result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			})
		}
	}

	return result, nil
}

// toOpenAIMessages converts generic messages to OpenAI format.
func toOpenAIMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, m := range msgs {
		switch m.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(m.Content))
		case "user":
			messages = append(messages, openai.UserMessage(m.Content))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					argsJSON, _ := json.Marshal(tc.Args)
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: string(argsJSON),
						},
					})
				}
				messages = append(messages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(m.Content)},
						ToolCalls: toolCalls,
					},
				})
			} else {
				messages = append(messages, openai.AssistantMessage(m.Content))
			}
		case "tool":
			messages = append(messages, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}

	return messages
}

// toOpenAITools converts generic tool definitions to OpenAI format.
func toOpenAITools(tools []ToolDef) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		schemaJSON, _ := json.Marshal(t.Parameters)
		var schema shared.FunctionParameters
		json.Unmarshal(schemaJSON, &schema)

		result = append(result, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  schema,
			},
		})
	}
	return result
}

// applyOpenAIToolChoice sets tool_choice on the request params. OpenAI
// natively supports "required" (call some tool) and a named function;
// ToolChoiceAuto leaves the field unset (OpenAI's own default).
func applyOpenAIToolChoice(choice ToolChoice, params *openai.ChatCompletionNewParams) {
	if name, ok := choice.ToolName(); ok {
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionParamOfChatCompletionNamedToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{Name: name},
		)
		return
	}
	if choice.IsRequired() {
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String(string(openai.ChatCompletionToolChoiceOptionAutoRequired)),
		}
	}
}

// isReasoningModel checks if the model supports reasoning effort (o1, o3 models).
func isReasoningModel(model string) bool {
	return len(model) >= 2 && (model[:2] == "o1" || model[:2] == "o3")
}
