package llm

import (
	"testing"
)

// =============================================================================
// OpenAI Provider Tests
// =============================================================================

func TestOpenAIProvider_Creation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     openAIConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: openAIConfig{
				APIKey:    "test-key",
				Model:     "gpt-4o",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			cfg: openAIConfig{
				Model:     "gpt-4o",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing model",
			cfg: openAIConfig{
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing max_tokens",
			cfg: openAIConfig{
				APIKey: "test-key",
				Model:  "gpt-4o",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOpenAI(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("newOpenAI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"o1-preview", true},
		{"o1-mini", true},
		{"o3-mini", true},
		{"o3", true},
		{"gpt-4o", false},
		{"gpt-4", false},
		{"claude-3-5-sonnet", false},
		{"a", false}, // too short
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := isReasoningModel(tt.model)
			if got != tt.want {
				t.Errorf("isReasoningModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
