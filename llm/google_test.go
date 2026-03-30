package llm

import (
	"testing"
)

// =============================================================================
// Google Provider Tests
// =============================================================================

func TestGoogleProvider_Creation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     googleConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: googleConfig{
				APIKey:    "test-key",
				Model:     "gemini-1.5-pro",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			cfg: googleConfig{
				Model:     "gemini-1.5-pro",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing model",
			cfg: googleConfig{
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing max_tokens",
			cfg: googleConfig{
				APIKey: "test-key",
				Model:  "gemini-1.5-pro",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newGoogle(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("newGoogle() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConvertPropertyToSchema(t *testing.T) {
	tests := []struct {
		name     string
		prop     map[string]interface{}
		wantType string
	}{
		{
			name:     "string type",
			prop:     map[string]interface{}{"type": "string"},
			wantType: "TypeString",
		},
		{
			name:     "number type",
			prop:     map[string]interface{}{"type": "number"},
			wantType: "TypeNumber",
		},
		{
			name:     "integer type",
			prop:     map[string]interface{}{"type": "integer"},
			wantType: "TypeInteger",
		},
		{
			name:     "boolean type",
			prop:     map[string]interface{}{"type": "boolean"},
			wantType: "TypeBoolean",
		},
		{
			name:     "array type",
			prop:     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			wantType: "TypeArray",
		},
		{
			name:     "object type",
			prop:     map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}},
			wantType: "TypeObject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := convertPropertyToSchema(tt.prop)
			if schema.Type.String() != tt.wantType {
				t.Errorf("convertPropertyToSchema() type = %v, want %v", schema.Type.String(), tt.wantType)
			}
		})
	}
}

func TestConvertPropertyToSchema_Description(t *testing.T) {
	prop := map[string]interface{}{
		"type":        "string",
		"description": "The user's name",
	}
	schema := convertPropertyToSchema(prop)
	if schema.Description != "The user's name" {
		t.Errorf("expected description 'The user's name', got %q", schema.Description)
	}
}

func TestConvertPropertyToSchema_Enum(t *testing.T) {
	prop := map[string]interface{}{
		"type": "string",
		"enum": []interface{}{"red", "green", "blue"},
	}
	schema := convertPropertyToSchema(prop)
	if len(schema.Enum) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(schema.Enum))
	}
}

func TestConvertToGeminiSchema(t *testing.T) {
	params := map[string]interface{}{
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Search query"},
		},
		"required": []interface{}{"query"},
	}
	schema := convertToGeminiSchema(params)
	if schema.Properties["query"] == nil {
		t.Error("expected query property")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Errorf("expected required [query], got %v", schema.Required)
	}
}

func TestToGeminiHistory(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "bye"},
	}
	history := toGeminiHistory(msgs)
	// system messages are skipped
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries (skipping system), got %d", len(history))
	}
	if history[0].Role != "user" {
		t.Errorf("expected first entry role 'user', got %q", history[0].Role)
	}
	if history[1].Role != "model" {
		t.Errorf("expected second entry role 'model', got %q", history[1].Role)
	}
}

func TestToGeminiTools(t *testing.T) {
	tools := []ToolDef{
		{Name: "search", Description: "Search", Parameters: map[string]any{
			"properties": map[string]any{"q": map[string]any{"type": "string"}},
		}},
	}
	result := toGeminiTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Name != "search" {
		t.Errorf("expected tool name 'search', got %q", result[0].Name)
	}
}
