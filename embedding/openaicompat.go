package embedding

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// openAICompatEmbedder implements Embedder using an OpenAI-compatible endpoint.
// Works with Ollama, LiteLLM, LMStudio, vLLM, etc.
type openAICompatEmbedder struct {
	client *openai.Client
	model  string
}

func newOpenAICompat(cfg Config) (*openAICompatEmbedder, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai-compat embedding: base_url required")
	}
	model := cfg.Model
	if model == "" {
		return nil, fmt.Errorf("openai-compat embedding: model required")
	}

	opts := []option.RequestOption{
		option.WithBaseURL(cfg.BaseURL),
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	} else {
		// Many local providers don't need a key
		opts = append(opts, option.WithAPIKey("unused"))
	}

	client := openai.NewClient(opts...)
	return &openAICompatEmbedder{client: &client, model: model}, nil
}

func (e *openAICompatEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: e.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai-compat embedding: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai-compat embedding: empty response")
	}
	return resp.Data[0].Embedding, nil
}
