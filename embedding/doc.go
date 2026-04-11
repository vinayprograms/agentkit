// Package embedding provides text-to-vector implementations for semantic
// search and similarity. Supports OpenAI, Google, Ollama, and any
// OpenAI-compatible endpoint (LiteLLM, LMStudio, vLLM, etc.).
//
// Create an embedder from configuration:
//
//	e, err := embedding.New(embedding.Config{
//	    Provider: "openai",
//	    Model:    "text-embedding-3-small",
//	    APIKey:   key,
//	})
//	vec, err := e.Embed(ctx, "search query")
//
// Provider "none" or empty returns nil (useful for testing without embeddings).
package embedding
