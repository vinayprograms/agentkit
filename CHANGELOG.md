# Changelog

All notable changes to agentkit will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.0] - 2026-06-12

### Changed (breaking)

- `mcp`: collapsed the `Client` interface's two tool-listing methods into one.
  `ListTools(ctx) ([]Tool, error)` is removed; `Tools() []Tool` is now the sole
  accessor — whether it serves a cache or a fresh load is the client's concern,
  not the caller's. Constructors load the tool list at connection time as
  before. Only affects code that called `ListTools` or implemented `Client`;
  callers that read `Tools()`/`Manager.AllTools()` are unaffected.

### Added — gaps found during first real-repo integration

The items in this section are additive and backward-compatible.

- `credentials`: `Resolve(Lookup, provider)` plus an `OAuthResolver` interface
  and `FileStore`/`UnionStore` `Resolve` methods, so callers can tell whether a
  resolved credential is an OAuth token (for `llm.Config.IsOAuthToken`).
- `credentials`: `StandardPaths`, `Load`, and `ClaudeCLICredentials` convenience
  helpers (search-path resolution, env+file+Claude-CLI composition).
- `credentials`: `FileStore.Save` now creates the parent directory; a
  nonexistent path passed to `NewFileStore` yields an empty usable store.
- `contentguard`: `Result.Related` exposes the untrusted content blocks in scope
  for a checked call (taint propagation); `Finding.Latency` carries per-stage
  timing; exported `ShannonEntropy` and `IsHighEntropy`.
- `memory`: `Extractor.Extract` accepts `WithSource` and `WithMaxInputChars`
  options (input truncation defaults to 4000 chars).
- `tools`: `SpawnBinder` allows late binding of a `SpawnFunc` after registration;
  `WithHTTPTimeout` option on `Fetch`/`Search`; fs tools accept additional
  allowed roots via a variadic `extraRoots` constructor parameter.
- `policy`: `FromTOMLWithUnknownKeys` reports unrecognized TOML keys so consumers
  can validate them without re-parsing.

### Documentation

- `policy`, `tools`: documented that policy is a model only and enforcement is
  the consumer's job, with a copy-paste `tools.Guard` example; documented
  default-deny and the legacy keys `FromTOML` ignores.
- `tools.Registry.Definitions`: documented that it is not policy-aware.
- `mcp`: documented the Connect → Register → Deny lifecycle with an example.

### Style

- `gofmt` applied across the repository.

## [1.0.2] - 2026-04-18

### Changed

- `shutdown.Shutdown` is now documented as idempotent: subsequent callers
  observe the first caller's result. An unreachable `default` branch that
  claimed to return `ErrAlreadyShutdown` was removed — `sync.Once` already
  blocks concurrent callers until `done` is closed, so that branch was
  never hit in practice. `ErrAlreadyShutdown` is retained as deprecated
  for API compatibility.

### Tests

- `errors`, `shellguard`, `shutdown` raised to 100% statement coverage.

## [1.0.1] - 2026-04-18

### Fixed

- `llm/retry`: exponential backoff now applies full jitter (uniform in
  `[0, backoff)`) so concurrent callers hitting the same rate limit spread
  their retries instead of retrying in lockstep.

## [1.0.0] - 2026-04-12

### Changed — kit is now single-agent focused

Multi-agent coordination primitives (`bus`, `registry`, `state`, `tasks`,
`results`, `heartbeat`, `ratelimit`, `resume`) moved to a separate toolkit.
Agentkit is now purely single-agent building blocks: LLM access, tools,
memory, content/shell guards, MCP, ACP, shutdown, errors, credentials, policy,
embedding.

### Removed

- `auth` — OAuth device flow was dead code. Anthropic banned third-party
  subscription OAuth use (Feb 2026). GitHub Copilot preset used VS Code's
  client ID (fragile). Use `golang.org/x/oauth2` for generic OAuth needs.
- `logging` — was consumer-specific, not kit material. Use stdlib `log/slog`.
- `telemetry` — was a wrapper around OpenTelemetry with agent-specific span
  helpers. Each package now imports `go.opentelemetry.io/otel` directly and
  emits spans scoped to its own package path. The consumer initializes
  OTel in main.
- `transport` — was unused scaffolding. MCP, ACP, and other protocols now
  own their own wire format.
- `types` — shared type dump package. Types moved to their domain packages.

### Refactored

Every remaining package refactored for clarity:
- Single-word type and method names wherever the abstraction allows
- Consumer-defined interfaces at module boundaries
- Ready-to-use constructors (no `Init()` or `Set*` methods)
- Pervasive OpenTelemetry instrumentation with scoped `trace()` helpers
- Hi-res span events via scoped `event()` helpers in `shellguard`,
  `contentguard`, and `acp/internal/rpc` — mark decision points and state
  transitions inside existing spans without creating extra sub-spans

### Added

- `acp/` is now a separate Go module with a complete Agent Client Protocol
  implementation. Agent-side and host-side orchestrators, all 17 spec
  feature areas covered, bidirectional JSON-RPC transport, proto types
  organized under `acp/proto/`.

## [0.1.0] - 2026-02-24

### Added

**Core Packages:**
- `llm` - Multi-provider LLM abstraction (Anthropic, OpenAI, Google, Ollama, Groq, Mistral, xAI)
- `bus` - Message bus with pub/sub, queue groups, request/reply (NATS + memory backends)
- `errors` - Structured error taxonomy with categories and retry semantics

**Swarm Coordination:**
- `registry` - Agent registration and capability-based discovery
- `heartbeat` - Liveness detection with death callbacks
- `state` - Distributed key-value store with locks (NATS JetStream + memory)

**Task Management:**
- `tasks` - Idempotent task handling with deduplication
- `results` - Task result publication with subscriptions
- `ratelimit` - Coordinated rate limiting across swarm

**Operations:**
- `shutdown` - Graceful shutdown with phases and signal handling
- `logging` - Structured real-time logging
- `telemetry` - OpenTelemetry tracing integration

**Specialized:**
- `transport` - JSON-RPC transports (stdio, WebSocket, SSE)
- `mcp` - Model Context Protocol client
- `acp` - Agent Client Protocol for editor integration
- `memory` - Semantic memory with BM25 search (FIL model)

**Documentation:**
- Design docs for all 16 packages
- 8 working examples
- README with learning path

### Notes

This is the initial release. API may change before 1.0.0.
