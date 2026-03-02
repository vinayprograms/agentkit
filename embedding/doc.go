// Package embedding provides concrete Embedder implementations for generating
// text embedding vectors. Supports OpenAI, Google, and OpenAI-compatible
// endpoints (Ollama, LiteLLM, etc.).
//
// Embedders satisfy the resume.Embedder interface and can be used for
// resume embedding, query embedding, and semantic similarity search.
package embedding
