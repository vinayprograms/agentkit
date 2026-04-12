// Package llm provides LLM provider interfaces and implementations.
package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Model is the interface for LLM access.
// Use llm.Prompt() to build simple requests from a text prompt.
type Model interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// Resolver looks up a Model by profile name.
type Resolver interface {
	Model(profile string) (Model, error)
}

// Config holds configuration for connecting to an LLM service.
type Config struct {
	Service      string         `json:"service"` // anthropic, openai, google, groq, mistral, openai-compat, etc.
	Model        string         `json:"model"`
	APIKey       string         `json:"api_key"`
	IsOAuthToken bool           `json:"is_oauth_token"` // True if APIKey is an OAuth access token (Anthropic)
	MaxTokens    int            `json:"max_tokens"`
	BaseURL      string         `json:"base_url"` // Custom API endpoint
	Thinking     ThinkingConfig `json:"thinking"`
	Retry        RetryConfig    `json:"retry"`
}

// RetryConfig holds retry settings for LLM calls.
type RetryConfig struct {
	MaxRetries  int           `json:"max_retries"`
	MaxBackoff  time.Duration `json:"max_backoff"`
	InitBackoff time.Duration `json:"init_backoff"`
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Service == "" {
		return fmt.Errorf("service is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	if c.APIKey == "" && !isLocalService(c.Service) {
		return fmt.Errorf("api key is required")
	}
	if c.MaxTokens == 0 {
		return fmt.Errorf("max_tokens is required")
	}
	return nil
}

func isLocalService(service string) bool {
	switch service {
	case "ollama", "ollama-local", "lmstudio":
		return true
	default:
		return false
	}
}

// New creates an LLM model based on the configuration.
// If Service is empty, it will be inferred from the Model name.
func New(cfg Config) (Model, error) {
	if cfg.Service == "" && cfg.Model != "" {
		cfg.Service = InferService(cfg.Model)
		if cfg.Service == "" {
			return nil, fmt.Errorf("cannot determine service for model %q; set service explicitly", cfg.Model)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	m, err := newModel(cfg)
	if err != nil {
		return nil, err
	}
	return instrument(m, cfg.Service, cfg.Model), nil
}

func newModel(cfg Config) (Model, error) {
	switch cfg.Service {
	case "anthropic":
		return newAnthropic(anthropicConfig{
			APIKey:       cfg.APIKey,
			IsOAuthToken: cfg.IsOAuthToken,
			BaseURL:      cfg.BaseURL,
			Model:        cfg.Model,
			MaxTokens:    cfg.MaxTokens,
			Thinking:     cfg.Thinking,
			Retry:        cfg.Retry,
		})

	case "openai":
		return newOpenAI(openAIConfig{
			APIKey:    cfg.APIKey,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			MaxTokens: cfg.MaxTokens,
			Thinking:  cfg.Thinking,
			Retry:     cfg.Retry,
		})

	case "google":
		return newGoogle(googleConfig{
			APIKey:    cfg.APIKey,
			Model:     cfg.Model,
			MaxTokens: cfg.MaxTokens,
			Thinking:  cfg.Thinking,
			Retry:     cfg.Retry,
		})

	case "ollama-cloud":
		return newOllamaCloud(ollamaCloudConfig{
			APIKey:    cfg.APIKey,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			MaxTokens: cfg.MaxTokens,
			Thinking:  cfg.Thinking,
			Retry:     cfg.Retry,
		})

	case "groq", "mistral", "xai", "openrouter", "ollama-local", "ollama",
		"lmstudio", "cerebras", "openai-compat", "litellm":
		return newOpenAICompat(cfg.Service, openAICompatConfig{
			APIKey:    cfg.APIKey,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			MaxTokens: cfg.MaxTokens,
			Thinking:  cfg.Thinking,
			Retry:     cfg.Retry,
		})

	default:
		return nil, fmt.Errorf("unsupported service: %s", cfg.Service)
	}
}

// InferService returns the provider name based on model name patterns.
func InferService(model string) string {
	model = strings.ToLower(model)

	if strings.HasPrefix(model, "claude") {
		return "anthropic"
	}
	if strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "chatgpt") {
		return "openai"
	}
	if strings.HasPrefix(model, "gemini") || strings.HasPrefix(model, "gemma") {
		return "google"
	}
	if strings.HasPrefix(model, "mistral") || strings.HasPrefix(model, "mixtral") ||
		strings.HasPrefix(model, "codestral") || strings.HasPrefix(model, "pixtral") {
		return "mistral"
	}
	if strings.HasPrefix(model, "grok") {
		return "xai"
	}
	if strings.HasPrefix(model, "cerebras") {
		return "cerebras"
	}
	return ""
}
