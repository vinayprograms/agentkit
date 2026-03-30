package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Provider-specific base URLs
const (
	GroqBaseURL       = "https://api.groq.com/openai/v1"
	MistralBaseURL    = "https://api.mistral.ai/v1"
	XAIBaseURL        = "https://api.x.ai/v1"
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
	CerebrasBaseURL   = "https://api.cerebras.ai/v1"
	OllamaLocalURL    = "http://localhost:11434/v1"
	LMStudioLocalURL  = "http://localhost:1234/v1"
)

// openAICompatModel implements the Provider interface for OpenAI-compatible APIs.
// This includes Groq, Mistral, LiteLLM, OpenRouter, local Ollama, LMStudio, etc.
type openAICompatModel struct {
	apiKey       string
	baseURL      string
	model        string
	maxTokens    int
	providerName string
	thinking     ThinkingConfig
	retry        RetryConfig
	client       *http.Client
}

// openAICompatConfig holds configuration for OpenAI-compatible providers.
type openAICompatConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	MaxTokens int
	Thinking  ThinkingConfig
	Retry     RetryConfig
}

// defaultBaseURLs maps provider names to their default API base URLs.
var defaultBaseURLs = map[string]string{
	"groq":         GroqBaseURL,
	"mistral":      MistralBaseURL,
	"xai":          XAIBaseURL,
	"openrouter":   OpenRouterBaseURL,
	"ollama-local": OllamaLocalURL,
	"ollama":       OllamaLocalURL,
	"lmstudio":     LMStudioLocalURL,
	"cerebras":     CerebrasBaseURL,
}


// newOpenAICompat creates a new OpenAI-compatible provider.
// providerName is used for logging and to resolve default base URLs.
func newOpenAICompat(providerName string, cfg openAICompatConfig) (*openAICompatModel, error) {
	if cfg.BaseURL == "" {
		if defaultURL, ok := defaultBaseURLs[providerName]; ok {
			cfg.BaseURL = defaultURL
		} else {
			return nil, fmt.Errorf("base_url is required for provider %s", providerName)
		}
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if cfg.MaxTokens == 0 {
		return nil, fmt.Errorf("max_tokens is required")
	}

	return &openAICompatModel{
		apiKey:       cfg.APIKey,
		baseURL:      cfg.BaseURL,
		model:        cfg.Model,
		maxTokens:    cfg.MaxTokens,
		providerName: providerName,
		thinking:     cfg.Thinking,
		retry:        cfg.Retry,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

// OpenAI-compatible request/response types

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string            `json:"type"`
	Function oaiToolDefinition `json:"function"`
}

type oaiToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
}

type oaiResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int        `json:"index"`
		Message      oaiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Chat implements the Provider interface.
func (p *openAICompatModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	messages := toOAICompatMessages(req.Messages)
	tools := toOAICompatTools(req.Tools)

	maxTokens := p.maxTokens
	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}

	oaiReq := oaiRequest{
		Model:     p.model,
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	if len(tools) > 0 {
		oaiReq.Tools = tools
	}

	// Make request with retry
	resp, err := withRetry(ctx, p.retry, p.providerName, func() (*oaiResponse, error) {
		return p.doRequest(ctx, oaiReq)
	})
	if err != nil {
		return nil, err
	}

	return fromOAICompatResponse(resp)
}

func fromOAICompatResponse(resp *oaiResponse) (*ChatResponse, error) {
	result := &ChatResponse{
		Model:        resp.Model,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.StopReason = choice.FinishReason

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

// doRequest makes the HTTP request.
func (p *openAICompatModel) doRequest(ctx context.Context, req oaiRequest) (*oaiResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		if httpResp.StatusCode == 429 {
			return nil, fmt.Errorf("rate limit exceeded: %s", string(respBody))
		}
		if httpResp.StatusCode == 402 {
			return nil, fmt.Errorf("payment required: %s", string(respBody))
		}
		return nil, fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var resp oaiResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("API error: %s", resp.Error.Message)
	}

	return &resp, nil
}

// toOAICompatMessages converts generic messages to OpenAI-compatible format.
func toOAICompatMessages(msgs []Message) []oaiMessage {
	messages := make([]oaiMessage, 0, len(msgs))

	for _, m := range msgs {
		msg := oaiMessage{
			Role:    m.Role,
			Content: m.Content,
		}

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Args)
				msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: oaiFunction{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		if m.Role == "tool" {
			msg.ToolCallID = m.ToolCallID
		}

		messages = append(messages, msg)
	}

	return messages
}

// toOAICompatTools converts generic tool definitions to OpenAI-compatible format.
func toOAICompatTools(tools []ToolDef) []oaiTool {
	result := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		result = append(result, oaiTool{
			Type: "function",
			Function: oaiToolDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return result
}
