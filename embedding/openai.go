package embedding

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// openAIEmbedder implements Embedder using the OpenAI embeddings API.
type openAIEmbedder struct {
	client *openai.Client
	model  string
}

func newOpenAI(cfg Config) (*openAIEmbedder, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai embedding: api_key required")
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)
	return &openAIEmbedder{client: &client, model: model}, nil
}

func (e *openAIEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: e.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai embedding: empty response")
	}
	return resp.Data[0].Embedding, nil
}
