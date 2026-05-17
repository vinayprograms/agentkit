# embedding

Unified embedding interface across providers. Returns `nil` without error when no provider is configured, so callers that treat embeddings as optional don't need a nil-check on every code path.

## Providers

| Provider string | Backend |
|---|---|
| `"openai"` | OpenAI text-embedding models |
| `"google"` | Google Generative AI embedding |
| `"openai-compat"` / `"litellm"` | Any OpenAI-compatible endpoint |
| `"ollama"` / `"ollama-cloud"` / `"ollama-local"` | Ollama local or hosted |
| `"none"` / `""` | Returns nil — embeddings disabled |

## Usage

```go
embedder, err := embedding.New(embedding.Config{
    Provider: "openai",
    APIKey:   "sk-...",
    Model:    "text-embedding-3-small",
})
if err != nil { ... }
if embedder == nil {
    // embeddings not configured — proceed without them
}

vec, err := embedder.Embed(ctx, "some text to embed")
```

## Config

```go
type Config struct {
    Provider string // required
    Model    string // optional; provider default used when empty
    APIKey   string // required for cloud providers
    BaseURL  string // optional; for custom endpoints
}
```

All fields map directly to `[embedding]` in agent.toml:

```toml
[embedding]
provider = "openai"
model    = "text-embedding-3-small"
api_key  = "..."
```

## Interface

`Embedder` is compatible with `memory.Embedder` — any implementation satisfies both interfaces.
