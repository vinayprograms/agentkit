package embedding

import (
	"context"
	"fmt"
	"strings"
)

// Config holds configuration for creating an Embedder.
type Config struct {
	// Provider name: "openai", "google", "openai-compat", "litellm", "none"
	Provider string `toml:"provider" json:"provider"`

	// Model name (e.g., "text-embedding-3-small", "text-embedding-004")
	Model string `toml:"model" json:"model"`

	// APIKey for the embedding provider
	APIKey string `toml:"api_key" json:"api_key"`

	// BaseURL for custom endpoints (OpenAI-compatible providers)
	BaseURL string `toml:"base_url" json:"base_url"`
}

// Embedder generates embedding vectors from text.
// Same interface as resume.Embedder — implementations satisfy both.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// New creates an Embedder from the given configuration.
// Returns nil if provider is "none" or empty.
func New(cfg Config) (Embedder, error) {
	switch strings.ToLower(cfg.Provider) {
	case "", "none":
		return nil, nil
	case "openai":
		return newOpenAI(cfg)
	case "google":
		return newGoogle(cfg)
	case "openai-compat", "litellm":
		return newOpenAICompat(cfg)
	case "ollama", "ollama-cloud", "ollama-local":
		return newOllama(cfg)
	default:
		return nil, fmt.Errorf("unknown embedding provider: %q", cfg.Provider)
	}
}
