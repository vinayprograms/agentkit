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

// ollamaCloudModel implements the Provider interface for Ollama's cloud API.
// This uses Ollama's native /api/chat endpoint, not the OpenAI-compatible endpoint.
type ollamaCloudModel struct {
	apiKey    string
	baseURL   string
	model     string
	maxTokens int
	thinking  ThinkingConfig
	retry     RetryConfig
	client    *http.Client
}

// ollamaCloudConfig holds configuration for the Ollama Cloud provider.
type ollamaCloudConfig struct {
	APIKey    string
	BaseURL   string // defaults to https://ollama.com
	Model     string
	MaxTokens int
	Thinking  ThinkingConfig
	Retry     RetryConfig
}

// newOllamaCloud creates a new Ollama Cloud provider.
func newOllamaCloud(cfg ollamaCloudConfig) (*ollamaCloudModel, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for ollama-cloud")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required for ollama-cloud")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://ollama.com"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	return &ollamaCloudModel{
		apiKey:    cfg.APIKey,
		baseURL:   baseURL,
		model:     cfg.Model,
		maxTokens: maxTokens,
		thinking:  cfg.Thinking,
		retry:     cfg.Retry,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

// ollamaMessage represents a message in Ollama's API format.
type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Thinking  string           `json:"thinking,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

// ollamaToolCall represents a tool call in Ollama's format.
type ollamaToolCall struct {
	Function ollamaFunction `json:"function"`
}

// ollamaFunction represents a function call in Ollama's format.
type ollamaFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ollamaTool represents a tool definition in Ollama's format.
type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

// ollamaToolFunction represents a function definition in Ollama's format.
type ollamaToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ollamaChatRequest represents a chat request to Ollama's API.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	Think    interface{}     `json:"think,omitempty"` // bool or "low"/"medium"/"high"
	Options  *ollamaOptions  `json:"options,omitempty"`
}

// ollamaOptions represents generation options.
type ollamaOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

// ollamaChatResponse represents a response from Ollama's API.
type ollamaChatResponse struct {
	Model      string        `json:"model"`
	Message    ollamaMessage `json:"message"`
	Done       bool          `json:"done"`
	DoneReason string        `json:"done_reason,omitempty"`

	// Token counts
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

// Chat implements the Provider interface.
func (p *ollamaCloudModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	ollamaReq := p.buildRequest(req)

	// Make request with retry
	resp, err := withRetry(ctx, p.retry, "ollama-cloud", func() (*ollamaChatResponse, error) {
		return p.doRequest(ctx, ollamaReq)
	})
	if err != nil {
		return nil, err
	}

	return fromOllamaResponse(resp), nil
}

// Stream implements Streamer for the Ollama Cloud provider using the native
// /api/chat NDJSON streaming mode.
func (p *ollamaCloudModel) Stream(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error) {
	ollamaReq := p.buildRequest(req)
	ollamaReq.Stream = true

	wrapped, delivered := deliveryTracker(on)

	return withStreamRetry(ctx, p.retry, "ollama-cloud", delivered, func() (*ChatResponse, error) {
		return p.doStreamingRequest(ctx, ollamaReq, wrapped)
	})
}

// buildRequest converts a ChatRequest into an Ollama chat request, shared by
// Chat and Stream. Stream is left false; callers that want streaming set it
// after calling this.
func (p *ollamaCloudModel) buildRequest(req ChatRequest) ollamaChatRequest {
	messages := toOllamaMessages(req.Messages)
	tools := toOllamaTools(req.Tools)

	maxTokens := p.maxTokens
	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}

	// Determine thinking level
	thinkingLevel := ResolveThinkingLevel(p.thinking, req)
	var thinkParam interface{}
	if thinkingLevel != ThinkingOff {
		// GPT-OSS uses string levels, others use bool
		if isGPTOSSModel(p.model) {
			thinkParam = string(thinkingLevel)
		} else {
			thinkParam = true
		}
	}

	// req.ToolChoice is intentionally not wired here: Ollama's native
	// /api/chat has no documented tool_choice/forced-tool-call field (see
	// https://docs.ollama.com/capabilities/tool-calling, checked 2026-08-23).
	// Any ToolChoice degrades to Ollama's own auto behavior; callers must
	// keep a prose fallback for this provider.
	return ollamaChatRequest{
		Model:    p.model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
		Think:    thinkParam,
		Options: &ollamaOptions{
			NumPredict: maxTokens,
		},
	}
}

func fromOllamaResponse(resp *ollamaChatResponse) *ChatResponse {
	result := &ChatResponse{
		Content:      resp.Message.Content,
		Thinking:     resp.Message.Thinking,
		StopReason:   resp.DoneReason,
		InputTokens:  resp.PromptEvalCount,
		OutputTokens: resp.EvalCount,
		Model:        resp.Model,
	}
	for i, tc := range resp.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
			ID:   fmt.Sprintf("call_%d", i),
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}
	return result
}

// doRequest makes the HTTP request to Ollama's API.
func (p *ollamaCloudModel) doRequest(ctx context.Context, req ollamaChatRequest) (*ollamaChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

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
		return nil, ollamaStatusError(httpResp.StatusCode, respBody)
	}

	var resp ollamaChatResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// ollamaStatusError classifies a non-200 Ollama HTTP response the same way
// for both the plain and streaming request paths.
func ollamaStatusError(status int, body []byte) error {
	if status == 429 {
		return fmt.Errorf("rate limit exceeded: %s", string(body))
	}
	if status == 402 {
		return fmt.Errorf("payment required: %s", string(body))
	}
	return fmt.Errorf("ollama API error (status %d): %s", status, string(body))
}

// doStreamingRequest executes a single streaming request against Ollama's
// native NDJSON /api/chat mode: one JSON object per line, message.content
// and message.thinking carrying incremental fragments, and the final line
// (done=true) carrying the finish reason and eval counts.
func (p *ollamaCloudModel) doStreamingRequest(ctx context.Context, req ollamaChatRequest, on func(StreamEvent) error) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, ollamaStatusError(httpResp.StatusCode, respBody)
	}

	result := &ChatResponse{}
	scanner := bufio.NewScanner(httpResp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var chunk ollamaChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return nil, fmt.Errorf("failed to parse stream chunk: %w", err)
		}

		if chunk.Model != "" {
			result.Model = chunk.Model
		}

		if chunk.Message.Content != "" {
			result.Content += chunk.Message.Content
			if on != nil {
				if err := on(StreamEvent{Type: StreamContent, Text: chunk.Message.Content}); err != nil {
					return nil, errStreamCallback(err)
				}
			}
		}

		if chunk.Message.Thinking != "" {
			result.Thinking += chunk.Message.Thinking
			if on != nil {
				if err := on(StreamEvent{Type: StreamThinking, Text: chunk.Message.Thinking}); err != nil {
					return nil, errStreamCallback(err)
				}
			}
		}

		// Tool calls are never surfaced as stream events; Ollama sends
		// them whole (not as incremental fragments), so buffer as-is.
		for i, tc := range chunk.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
				ID:   fmt.Sprintf("call_%d", i),
				Name: tc.Function.Name,
				Args: tc.Function.Arguments,
			})
		}

		if chunk.Done {
			result.StopReason = chunk.DoneReason
			result.InputTokens = chunk.PromptEvalCount
			result.OutputTokens = chunk.EvalCount
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("stream read error: %w", err)
	}

	return result, nil
}

// toOllamaMessages converts generic messages to Ollama format.
func toOllamaMessages(msgs []Message) []ollamaMessage {
	messages := make([]ollamaMessage, 0, len(msgs))
	for _, m := range msgs {
		msg := ollamaMessage{
			Role:    m.Role,
			Content: m.Content,
		}

		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, ollamaToolCall{
					Function: ollamaFunction{
						Name:      tc.Name,
						Arguments: tc.Args,
					},
				})
			}
		}

		messages = append(messages, msg)
	}
	return messages
}

// toOllamaTools converts generic tool definitions to Ollama format.
func toOllamaTools(tools []ToolDef) []ollamaTool {
	result := make([]ollamaTool, 0, len(tools))
	for _, t := range tools {
		result = append(result, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return result
}

// isGPTOSSModel checks if the model is GPT-OSS which uses string think levels.
func isGPTOSSModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "gpt-oss")
}
