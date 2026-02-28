package resume

import "context"

// Embedder generates embedding vectors from text.
// Implementations can use OpenAI, Google, Cohere, Voyage, Ollama, etc.
type Embedder interface {
	// Embed generates a vector representation of the given text.
	Embed(ctx context.Context, text string) ([]float64, error)
}

// EmbedResume generates and attaches an embedding vector to a resume.
// Uses the resume's ToText() representation as input.
func EmbedResume(ctx context.Context, r *Resume, embedder Embedder) error {
	text := r.ToText()
	vec, err := embedder.Embed(ctx, text)
	if err != nil {
		return err
	}
	r.Embedding = vec
	return nil
}
