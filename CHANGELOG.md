# Changelog

All notable changes to agentkit will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
