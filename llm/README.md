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

## Streaming

```go
type StreamEventType string

const (
    StreamContent  StreamEventType = "content"
    StreamThinking StreamEventType = "thinking"
)

type StreamEvent struct {
    Type StreamEventType
    Text string
}

// Streamer is an optional capability a Model may implement.
type Streamer interface {
    Stream(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error)
}

func Stream(ctx context.Context, m Model, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error)
```

`llm.Stream()` works with any `Model`. Call it instead of `Chat()` to also receive token-by-token
deltas as they arrive:

```go
resp, err := llm.Stream(ctx, model, llm.ChatRequest{
    Messages: []llm.Message{{Role: "user", Content: "Explain this code."}},
}, func(ev llm.StreamEvent) error {
    switch ev.Type {
    case llm.StreamThinking:
        fmt.Print(ev.Text) // extended-thinking text, as it arrives
    case llm.StreamContent:
        fmt.Print(ev.Text) // reply text, as it arrives
    }
    return nil
})
```

`resp` is the same aggregated `*ChatResponse` `Chat()` would return for the same exchange
(`Content`, `Thinking`, `ToolCalls`, `StopReason`, token counts) — a caller that ignores every
delta gets Chat-equivalent results. Tool-call deltas are never surfaced as events; they are
buffered internally and only appear in `resp.ToolCalls`.

**Fallback behavior.** `anthropic`, `ollama-cloud`, and `openai-compat` (and everything built on
it: Groq, Mistral, xAI, OpenRouter, Ollama Local, LM Studio, Cerebras, LiteLLM) implement
`Streamer` with real token-by-token delivery. `openai` and `google` do not implement `Streamer`
this round; `llm.Stream()` falls back to a single `Chat()` call for them and synthesizes at most
two deltas from the result — one `StreamThinking` event if `resp.Thinking != ""`, then one
`StreamContent` event if `resp.Content != ""`. Every model returned by `llm.New()` satisfies
`Streamer` regardless (the tracing wrapper forwards to real streaming when the underlying provider
supports it, and to this fallback otherwise), so callers can always type-assert or always call
`llm.Stream()` without checking first.

**Callback errors abort the stream.** A non-nil error returned by `on` stops the provider from
reading/writing further and `Stream` returns an error that wraps it — `errors.Is(err, yourErr)`
finds it through the wrapping.

**Retry and partial delivery.** Streaming requests use the same backoff policy as `Chat()`
(see Retry Behavior below), with one added rule: retries are only attempted while zero deltas have
reached `on`. Once any delta has been delivered, a later failure (a dropped connection, a 5xx
mid-stream) is returned as an error immediately, without retrying — retrying at that point would
re-run the request and deliver the same text to the callback a second time.

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

For `Stream()` calls, retries additionally stop being attempted once the callback has received any
delta — see Streaming above.

Defaults: 5 retries, 1s initial backoff, 60s max backoff, 2x factor.

```go
llm.RetryConfig{
    MaxRetries:  3,
    InitBackoff: 2 * time.Second,
    MaxBackoff:  30 * time.Second,
}
```

