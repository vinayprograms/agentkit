# LLM Package Design

## What This Package Does

The `llm` package provides a unified interface for calling Large Language Models from multiple providers. Write your code once, then switch between Anthropic (Claude), OpenAI (GPT), Google (Gemini), and others by changing configuration.

## Why It Exists

Each LLM provider has its own API:
- Anthropic uses `Messages` with different tool formats
- OpenAI uses `ChatCompletions` with function calling
- Google uses `GenerateContent` with FunctionDeclaration
- Ollama has its own REST API

Without abstraction, you'd need provider-specific code everywhere. This package hides those differences behind a common interface.

It also handles:
- **Provider inference** — Figure out which provider from model name
- **Retry logic** — Exponential backoff for rate limits
- **Thinking modes** — Adaptive reasoning level selection
- **Tool calling** — Unified tool format across providers

## When to Use It

**Use this package for:**
- Building agents that call LLMs
- Applications that need provider flexibility
- Projects that use tool calling with LLMs

**You might not need this for:**
- Simple one-off scripts (direct SDK might be simpler)
- Highly provider-specific features
- Projects locked to a single provider forever

## Core Concepts

### Provider Interface

All LLM providers implement one method: send a chat request, get a response. The request contains messages (conversation history) and optionally tools. The response contains the LLM's reply and metadata (tokens used, stop reason).

### Messages

Messages form the conversation:
- **User** — Human input
- **Assistant** — LLM output
- **System** — Instructions to the LLM
- **Tool** — Results from tool execution

Each message has a role and content. Tool calls and results have additional fields for the call ID and arguments.

### Tool Calling

LLMs can request tool calls. When the LLM wants to use a tool:
1. Response includes tool call (name + arguments)
2. You execute the tool
3. You send a tool result message
4. LLM continues with the result

This enables agentic behavior — the LLM can take actions, not just generate text.

### Thinking/Reasoning

Some models support extended thinking (Claude) or reasoning effort (OpenAI o1/o3). The package provides:
- **Auto mode** — Heuristically choose thinking level based on prompt
- **Manual levels** — Off, low, medium, high
- **Provider translation** — Each provider implements thinking differently

## Architecture

![LLM Package Architecture](images/llm-architecture.png)

### Provider Implementations

**AnthropicProvider** — Official SDK for Claude models. Supports extended thinking via budget tokens. Uses streaming internally when thinking is enabled.

**OpenAIProvider** — Official SDK for GPT models. Supports ReasoningEffort for o1/o3 models.

**GoogleProvider** — Official SDK for Gemini models. Translates JSON Schema to Gemini's schema format.

**OllamaCloudProvider** — Native API for hosted Ollama models. Supports think parameter.

**OpenAICompatProvider** — Generic provider for OpenAI-compatible APIs (Groq, Mistral, xAI, local Ollama). Any endpoint that accepts OpenAI format works.

### Provider Inference

The package can automatically detect the provider from the model name:

| Pattern | Provider |
|---------|----------|
| `claude-*` | Anthropic |
| `gpt-*`, `o1-*`, `o3-*` | OpenAI |
| `gemini-*`, `gemma-*` | Google |
| `llama*`, `mistral*` | Ollama Cloud |
| `groq/*` | Groq |

You can also specify the provider explicitly.

## Common Patterns

### Basic Chat

Send messages, get response. The simplest use case:
1. Create provider with config
2. Build request with messages
3. Call Chat()
4. Use response content

### Tool Use Loop

For agentic behavior:
1. Define tools (name, description, parameters)
2. Send request with tools
3. Check if response has tool calls
4. If yes, execute tools, add results, send again
5. Repeat until no more tool calls

### Streaming (Internal)

Some operations require streaming (extended thinking). The provider handles this internally — you still get a complete response.

### Retry on Rate Limit

The package automatically retries on rate limits with exponential backoff. Configure max retries and initial delay.

## Error Handling

| Error Type | Meaning | Retry? |
|------------|---------|--------|
| Rate limit (429) | Too many requests | Yes, with backoff |
| Server error (5xx) | Provider issue | Yes, with backoff |
| Invalid request | Bad input | No |
| Auth error | Bad API key | No |
| Network error | Connectivity | Yes, with backoff |

The package classifies errors and applies appropriate retry behavior automatically.

## Configuration

### Provider Config

| Field | Purpose |
|-------|---------|
| Provider | Which provider (anthropic, openai, google, etc.) |
| Model | Model name |
| APIKey | Authentication |
| BaseURL | Custom endpoint (for proxies or self-hosted) |
| MaxTokens | Response length limit |

### Thinking Config

| Field | Purpose |
|-------|---------|
| Mode | Auto, off, low, medium, high |
| BudgetTokens | For Anthropic extended thinking |

### Retry Config

| Field | Purpose |
|-------|---------|
| MaxRetries | How many times to retry |
| InitialDelay | Starting backoff duration |
| MaxDelay | Cap on backoff |

## Integration Notes

### With MCP Package

Tools from MCP servers become ToolDefs for the LLM. The agent translates between MCP tool format and LLM tool format.

### With Ratelimit Package

Use the rate limiter before LLM calls to prevent hitting provider limits. On 429 response, announce reduction.

### With Telemetry Package

LLM calls can be traced — model, tokens, latency. Use spans to track LLM operations in your traces.

## Design Decisions

### Why one method (Chat)?

Simplicity. All LLM interactions are fundamentally "here's context, give me completion." More complex operations (embeddings, fine-tuning) would be separate interfaces.

### Why auto-detect provider?

Convenience. Most of the time, `claude-3-5-sonnet` obviously means Anthropic. Explicit configuration is still available when needed.

### Why unified tool format?

Tool calling is essential for agents. Having one format means your tool definitions work across providers without modification.

### Why internal retry?

Rate limits are common and handling them is boilerplate. Built-in retry makes the default behavior correct.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Message formatting, tool translation |
| Provider | Each provider's specific behavior |
| Retry | Backoff timing, error classification |
| Thinking | Mode selection, budget application |
| Integration | Full request/response with real providers |
