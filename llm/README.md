# llm

Unified interface for LLM services (Anthropic, OpenAI, Google, Groq, Mistral, Ollama, and OpenAI-compatible APIs).

## Usage

### Simple prompt

```go
model, err := llm.New(llm.Config{
    Service:   "anthropic",
    Model:     "claude-sonnet-4-20250514",
    APIKey:    apiKey,
    MaxTokens: 4096,
})

resp, err := model.Chat(ctx, llm.Prompt("Summarize this document...",
    llm.SystemPrompt("You are a helpful assistant."),
    llm.MaxTokens(500),
))
fmt.Println(resp.Content)
```

### Full chat with tool calling

```go
resp, err := model.Chat(ctx, llm.ChatRequest{
    Messages: []llm.Message{
        {Role: "system", Content: "You are a coding assistant."},
        {Role: "user", Content: "List files in the current directory."},
    },
    Tools: []llm.ToolDef{
        {Name: "ls", Description: "List directory contents", Parameters: schema},
    },
})

for _, tc := range resp.ToolCalls {
    fmt.Printf("Tool: %s, Args: %v\n", tc.Name, tc.Args)
}
```

## Interface

```go
type Model interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
```

One interface, one method. Use `llm.Prompt()` for simple requests, `llm.ChatRequest{}` for full control.

### Prompt helper

`llm.Prompt()` builds a `ChatRequest` from a text prompt with optional settings:

```go
llm.Prompt("explain this")
llm.Prompt("explain this", llm.SystemPrompt("Be concise."))
llm.Prompt("explain this", llm.SystemPrompt("Be concise."), llm.MaxTokens(500))
```

### Resolver

`Resolver` looks up models by profile name (for multi-model systems):

```go
type Resolver interface {
    Model(profile string) (Model, error)
}
```

## Supported Services

| Service | Config value | Models |
|---|---|---|
| Anthropic | `anthropic` | claude-* |
| OpenAI | `openai` | gpt-*, o1-*, o3-* |
| Google | `google` | gemini-*, gemma-* |
| Groq | `groq` | (set explicitly) |
| Mistral | `mistral` | mistral-*, mixtral-*, codestral-* |
| xAI | `xai` | grok-* |
| Ollama Cloud | `ollama-cloud` | any Ollama model |
| Ollama Local | `ollama-local` | any local model |
| LM Studio | `lmstudio` | any local model |
| OpenRouter | `openrouter` | any model |
| Cerebras | `cerebras` | cerebras-* |
| OpenAI-compat | `openai-compat` | any (requires `base_url`) |

The service can be auto-inferred from the model name via `llm.InferService()`.

## Configuration

```go
llm.Config{
    Service:      "anthropic",                // required (or inferred from model)
    Model:        "claude-sonnet-4-20250514", // required
    APIKey:       "sk-...",                   // required (except local services)
    MaxTokens:    4096,                       // required
    BaseURL:      "",                         // optional custom endpoint
    IsOAuthToken: false,                      // true for Anthropic OAuth
    Thinking:     llm.ThinkingConfig{...},    // optional thinking/reasoning config
    Retry:        llm.RetryConfig{...},       // optional retry settings
}
```

## Retry Behavior

All services retry on rate limits and transient server errors with exponential backoff. Billing errors are fatal (no retry).

Defaults: 5 retries, 1s initial backoff, 60s max backoff, 2x factor.

```go
llm.RetryConfig{
    MaxRetries:  3,
    InitBackoff: 2 * time.Second,
    MaxBackoff:  30 * time.Second,
}
```

