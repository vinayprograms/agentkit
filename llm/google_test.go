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
