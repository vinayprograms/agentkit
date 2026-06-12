package llm

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// googleModel implements the Provider interface using the official Google Gemini SDK.
type googleModel struct {
	client    *genai.Client
	modelName string
	maxTokens int
	thinking  ThinkingConfig
	retry     RetryConfig
}

// googleConfig holds configuration for the Google provider.
type googleConfig struct {
	APIKey    string
	Model     string
	MaxTokens int
	Thinking  ThinkingConfig
	Retry     RetryConfig
}

// newGoogle creates a new Google Gemini provider using the official SDK.
func newGoogle(cfg googleConfig) (*googleModel, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for google")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required for google")
	}
	if cfg.MaxTokens == 0 {
		return nil, fmt.Errorf("max_tokens is required for google")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.APIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create google client: %w", err)
	}

	return &googleModel{
		client:    client,
		modelName: cfg.Model,
		maxTokens: cfg.MaxTokens,
		thinking:  cfg.Thinking,
		retry:     cfg.Retry,
	}, nil
}

// Close closes the underlying client.
func (p *googleModel) Close() error {
	return p.client.Close()
}

// Chat implements the Provider interface.
func (p *googleModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Create a per-request model to avoid data races on concurrent Chat calls.
	model := p.client.GenerativeModel(p.modelName)
	maxTokens := int32(p.maxTokens)
	model.MaxOutputTokens = &maxTokens

	// Set system instruction if present
	for _, m := range req.Messages {
		if m.Role == "system" {
			model.SystemInstruction = &genai.Content{
				Parts: []genai.Part{genai.Text(m.Content)},
			}
			break
		}
	}

	// Convert tools to Gemini format
	if len(req.Tools) > 0 {
		model.Tools = []*genai.Tool{{FunctionDeclarations: toGeminiTools(req.Tools)}}
	}

	// Build chat session with history and extract the final prompt
	cs := model.StartChat()
	cs.History = toGeminiHistory(req.Messages)

	// Remove last user message from history (will be sent as the prompt)
	var prompt string
	if len(cs.History) > 0 && cs.History[len(cs.History)-1].Role == "user" {
		lastContent := cs.History[len(cs.History)-1]
		cs.History = cs.History[:len(cs.History)-1]
		if len(lastContent.Parts) > 0 {
			if text, ok := lastContent.Parts[0].(genai.Text); ok {
				prompt = string(text)
			}
		}
	}

	// Make request with retry
	resp, err := withRetry(ctx, p.retry, "google", func() (*genai.GenerateContentResponse, error) {
		return cs.SendMessage(ctx, genai.Text(prompt))
	})
	if err != nil {
		return nil, err
	}

	return fromGeminiResponse(p.modelName, resp), nil
}

func fromGeminiResponse(modelName string, resp *genai.GenerateContentResponse) *ChatResponse {
	result := &ChatResponse{Model: modelName}

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		if candidate.FinishReason != 0 {
			result.StopReason = candidate.FinishReason.String()
		}
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				switch p := part.(type) {
				case genai.Text:
					result.Content += string(p)
				case genai.FunctionCall:
					result.ToolCalls = append(result.ToolCalls, ToolCallResponse{
						ID:   fmt.Sprintf("call_%s", p.Name),
						Name: p.Name,
						Args: p.Args,
					})
				}
			}
		}
	}

	if resp.UsageMetadata != nil {
		result.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
		result.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return result
}

// toGeminiHistory converts generic messages to Gemini chat history format,
// skipping system messages (handled separately).
func toGeminiHistory(msgs []Message) []*genai.Content {
	history := make([]*genai.Content, 0, len(msgs))

	for _, m := range msgs {
		switch m.Role {
		case "system":
			continue
		case "user":
			history = append(history, &genai.Content{
				Role:  "user",
				Parts: []genai.Part{genai.Text(m.Content)},
			})
		case "assistant":
			content := &genai.Content{
				Role:  "model",
				Parts: []genai.Part{},
			}
			if m.Content != "" {
				content.Parts = append(content.Parts, genai.Text(m.Content))
			}
			for _, tc := range m.ToolCalls {
				content.Parts = append(content.Parts, genai.FunctionCall{
					Name: tc.Name,
					Args: tc.Args,
				})
			}
			history = append(history, content)
		case "tool":
			history = append(history, &genai.Content{
				Role: "user",
				Parts: []genai.Part{
					genai.FunctionResponse{
						Name:     m.ToolCallID,
						Response: map[string]interface{}{"result": m.Content},
					},
				},
			})
		}
	}

	return history
}

// toGeminiTools converts generic tool definitions to Gemini FunctionDeclarations.
func toGeminiTools(tools []ToolDef) []*genai.FunctionDeclaration {
	funcDecls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		schema := convertToGeminiSchema(t.Parameters)
		funcDecls = append(funcDecls, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}
	return funcDecls
}

// convertToGeminiSchema converts a JSON Schema map to Gemini's Schema type.
func convertToGeminiSchema(params map[string]interface{}) *genai.Schema {
	schema := &genai.Schema{
		Type: genai.TypeObject,
	}

	if props, ok := params["properties"].(map[string]interface{}); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for name, prop := range props {
			if propMap, ok := prop.(map[string]interface{}); ok {
				schema.Properties[name] = convertPropertyToSchema(propMap)
			}
		}
	}

	if required, ok := params["required"].([]interface{}); ok {
		for _, r := range required {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}

	return schema
}

// convertPropertyToSchema converts a single property to Gemini Schema.
func convertPropertyToSchema(prop map[string]interface{}) *genai.Schema {
	schema := &genai.Schema{}

	if typ, ok := prop["type"].(string); ok {
		switch typ {
		case "string":
			schema.Type = genai.TypeString
		case "number":
			schema.Type = genai.TypeNumber
		case "integer":
			schema.Type = genai.TypeInteger
		case "boolean":
			schema.Type = genai.TypeBoolean
		case "array":
			schema.Type = genai.TypeArray
			if items, ok := prop["items"].(map[string]interface{}); ok {
				schema.Items = convertPropertyToSchema(items)
			}
		case "object":
			schema.Type = genai.TypeObject
			if props, ok := prop["properties"].(map[string]interface{}); ok {
				schema.Properties = make(map[string]*genai.Schema)
				for name, p := range props {
					if propMap, ok := p.(map[string]interface{}); ok {
						schema.Properties[name] = convertPropertyToSchema(propMap)
					}
				}
			}
		}
	}

	if desc, ok := prop["description"].(string); ok {
		schema.Description = desc
	}

	if enum, ok := prop["enum"].([]interface{}); ok {
		for _, e := range enum {
			if s, ok := e.(string); ok {
				schema.Enum = append(schema.Enum, s)
			}
		}
	}

	return schema
}
