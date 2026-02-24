# Changelog

All notable changes to agentkit will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
