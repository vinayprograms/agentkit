package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	Tools         []oaiTool         `json:"tools,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	ToolChoice    any               `json:"tool_choice,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

// oaiStreamOptions requests usage accounting on the final SSE chunk; without
// it most OpenAI-compatible servers omit token counts entirely from a
// streamed response.
type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// oaiNamedToolChoice is the OpenAI-compatible {"type":"function","function":{"name":...}}
// tool_choice shape.
type oaiNamedToolChoice struct {
	Type     string                     `json:"type"`
	Function oaiNamedToolChoiceFunction `json:"function"`
}

type oaiNamedToolChoiceFunction struct {
	Name string `json:"name"`
}

// toOAICompatToolChoice converts a ToolChoice to the OpenAI-compatible
// tool_choice value. Arbitrary OpenAI-compatible servers vary in support for
// this field; an unsupported field is expected to be ignored by well-behaved
// servers, so callers must keep a prose fallback rather than relying on it.
func toOAICompatToolChoice(choice ToolChoice) any {
	if name, ok := choice.ToolName(); ok {
		return oaiNamedToolChoice{Type: "function", Function: oaiNamedToolChoiceFunction{Name: name}}
	}
	if choice.IsRequired() {
		return "required"
	}
	return nil
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
	oaiReq := p.buildRequest(req)

	// Make request with retry
	resp, err := withRetry(ctx, p.retry, p.providerName, func() (*oaiResponse, error) {
		return p.doRequest(ctx, oaiReq)
	})
	if err != nil {
		return nil, err
	}

	return fromOAICompatResponse(resp)
}

// Stream implements Streamer for OpenAI-compatible providers using SSE
// chat.completions streaming.
func (p *openAICompatModel) Stream(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error) {
	oaiReq := p.buildRequest(req)
	oaiReq.Stream = true
	oaiReq.StreamOptions = &oaiStreamOptions{IncludeUsage: true}

	wrapped, delivered := deliveryTracker(on)

	return withStreamRetry(ctx, p.retry, p.providerName, delivered, func() (*ChatResponse, error) {
		return p.doStreamingRequest(ctx, oaiReq, wrapped)
	})
}

// buildRequest converts a ChatRequest into an OpenAI-compatible request,
// shared by Chat and Stream. Stream/StreamOptions are left at their zero
// value (omitted from the wire request); callers that want streaming set
// them after calling this.
func (p *openAICompatModel) buildRequest(req ChatRequest) oaiRequest {
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

	oaiReq.ToolChoice = toOAICompatToolChoice(req.ToolChoice)

	return oaiReq
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
		return nil, oaiStatusError(httpResp.StatusCode, respBody)
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

// oaiStatusError classifies a non-200 OpenAI-compatible HTTP response the
// same way for both the plain and streaming request paths.
func oaiStatusError(status int, body []byte) error {
	if status == 429 {
		return fmt.Errorf("rate limit exceeded: %s", string(body))
	}
	if status == 402 {
		return fmt.Errorf("payment required: %s", string(body))
	}
	return fmt.Errorf("API error (status %d): %s", status, string(body))
}

// oaiStreamChunk is one SSE "data:" line of a chat.completions stream.
type oaiStreamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content,omitempty"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
			Reasoning        string `json:"reasoning,omitempty"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

// oaiToolCallAccum accumulates one streamed tool call's fragments, keyed by
// its position in the delta.tool_calls array (servers send name/id once and
// arguments in pieces across multiple chunks).
type oaiToolCallAccum struct {
	id, name string
	args     strings.Builder
}

// doStreamingRequest executes a single SSE streaming request against an
// OpenAI-compatible chat.completions endpoint.
func (p *openAICompatModel) doStreamingRequest(ctx context.Context, req oaiRequest, on func(StreamEvent) error) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, oaiStatusError(httpResp.StatusCode, respBody)
	}

	result := &ChatResponse{Model: p.model}
	tools := map[int]*oaiToolCallAccum{}
	var toolOrder []int

	scanner := bufio.NewScanner(httpResp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("failed to parse stream chunk: %w", err)
		}

		if chunk.Model != "" {
			result.Model = chunk.Model
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]

			if choice.Delta.Content != "" {
				result.Content += choice.Delta.Content
				if on != nil {
					if err := on(StreamEvent{Type: StreamContent, Text: choice.Delta.Content}); err != nil {
						return nil, errStreamCallback(err)
					}
				}
			}

			thinking := choice.Delta.ReasoningContent
			if thinking == "" {
				thinking = choice.Delta.Reasoning
			}
			if thinking != "" {
				result.Thinking += thinking
				if on != nil {
					if err := on(StreamEvent{Type: StreamThinking, Text: thinking}); err != nil {
						return nil, errStreamCallback(err)
					}
				}
			}

			// Tool-call deltas are never surfaced as stream events;
			// buffer fragments until the stream ends.
			for _, tc := range choice.Delta.ToolCalls {
				acc, ok := tools[tc.Index]
				if !ok {
					acc = &oaiToolCallAccum{}
					tools[tc.Index] = acc
					toolOrder = append(toolOrder, tc.Index)
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args.WriteString(tc.Function.Arguments)
			}

			if choice.FinishReason != "" {
				result.StopReason = choice.FinishReason
			}
		}

		if chunk.Usage != nil {
			result.InputTokens = chunk.Usage.PromptTokens
			result.OutputTokens = chunk.Usage.CompletionTokens
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("stream read error: %w", err)
	}

	for _, idx := range toolOrder {
		acc := tools[idx]
		var args map[string]interface{}
		argsStr := acc.args.String()
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				return nil, fmt.Errorf("failed to parse tool call arguments for %s: %w", acc.name, err)
			}
		}
		result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
			ID:   acc.id,
			Name: acc.name,
			Args: args,
		})
	}

	return result, nil
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
