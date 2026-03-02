package embedding

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// googleEmbedder implements Embedder using Google's Generative AI embedding API.
type googleEmbedder struct {
	apiKey string
	model  string
}

func newGoogle(cfg Config) (*googleEmbedder, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("google embedding: api_key required")
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-004"
	}
	return &googleEmbedder{apiKey: cfg.APIKey, model: model}, nil
}

func (e *googleEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(e.apiKey))
	if err != nil {
		return nil, fmt.Errorf("google embedding client: %w", err)
	}
	defer client.Close()

	em := client.EmbeddingModel(e.model)
	resp, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("google embedding: %w", err)
	}
	if resp.Embedding == nil || len(resp.Embedding.Values) == 0 {
		return nil, fmt.Errorf("google embedding: empty response")
	}

	// Convert float32 → float64
	vec := make([]float64, len(resp.Embedding.Values))
	for i, v := range resp.Embedding.Values {
		vec[i] = float64(v)
	}
	return vec, nil
}
