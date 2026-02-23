# Telemetry Design

## What This Package Does

The `telemetry` package provides distributed tracing for agent workflows using OpenTelemetry. It creates spans for LLM calls, tool executions, and other operations, letting you visualize agent behavior in tracing backends like Jaeger or Tempo.

## Why It Exists

Agents are complex distributed systems:
- Multiple LLM calls with varying latency
- Tool executions that may call external services
- MCP servers that spawn subprocesses
- Security checks and policy evaluations

Without observability, debugging is guesswork. Telemetry provides:
- **Traces** — See the full execution path of a request
- **Timing** — Know where time is spent
- **Context** — Understand relationships between operations
- **Debugging** — Optional content capture for troubleshooting

## When to Use It

**Use telemetry for:**
- Production agents where you need visibility
- Debugging latency or performance issues
- Understanding agent execution flow
- Compliance and audit trails

**You might skip telemetry for:**
- Simple local scripts
- Development without a tracing backend
- Extremely latency-sensitive operations (small overhead)

## Core Concepts

### OpenTelemetry

OpenTelemetry is the industry standard for observability. This package integrates with it:
- **Traces** are made of spans
- **Spans** represent operations (LLM call, tool execution)
- **Context** propagates trace IDs across boundaries

You export traces to a backend (Jaeger, Tempo, Honeycomb, etc.) for visualization.

### Agent-Specific Spans

The package provides helpers for common agent operations:

| Span Type | What It Tracks |
|-----------|----------------|
| LLM | Model, tokens, latency, prompt/response (debug only) |
| Tool | Tool name, arguments, result |
| MCP | Server, tool name, execution |
| Security | Check path, verdict, flags |
| Policy | Step, allowed/denied, reason |

Each span type has structured attributes appropriate for that operation.

### Debug Mode

By default, telemetry captures metadata but not content (prompts, responses, tool arguments). This protects user privacy.

Enable debug mode to capture content for troubleshooting. Only do this in development or when investigating specific issues.

### Event Export

Besides tracing, the package supports event export — writing agent events to files or HTTP endpoints. This enables custom analysis pipelines.

## Configuration

### Provider Setup

Configure the OpenTelemetry provider:

| Setting | Purpose |
|---------|---------|
| ServiceName | Identifies your agent in traces |
| Endpoint | Where to send traces (e.g., `localhost:4317`) |
| Protocol | `grpc` or `http` |
| Debug | Enable content capture |

If no endpoint is configured, the package checks the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable.

### OTLP Export

Traces export via OTLP (OpenTelemetry Protocol):
- **gRPC** (port 4317) — Default, more efficient
- **HTTP** (port 4318) — Works through more proxies

Configure based on your infrastructure.

## Common Patterns

### Wrapping LLM Calls

Before calling an LLM:
1. Start an LLM span
2. Make the call
3. End the span with token counts and latency

The span captures everything needed to understand LLM behavior.

### Tracing Tool Execution

For each tool call:
1. Start a tool span
2. Execute the tool
3. End the span with result (or error)

Child spans nest under parent spans, showing the execution hierarchy.

### Cross-Process Tracing

When calling MCP servers or other processes, propagate trace context. This links spans across process boundaries into a single trace.

Use the context propagation helpers to inject/extract trace IDs.

### Event Logging

For offline analysis, export events to files:
- Each event is a JSON line
- Events include session, workflow, goal context
- Import into analysis tools later

## Error Handling

| Situation | What Happens |
|-----------|--------------|
| No backend configured | Traces are dropped (noop) |
| Backend unavailable | Export fails, logged, agent continues |
| Invalid config | Initialization fails with error |
| Context missing | Spans created without parent |

Telemetry failures don't crash the agent. Observability is best-effort.

## Privacy Considerations

**Default (debug=false):**
- Captures: model, tokens, latency, tool names
- Does NOT capture: prompts, responses, tool arguments

**Debug mode (debug=true):**
- Captures everything including content
- Use only for troubleshooting
- May contain sensitive user data

Production systems should default to debug=false.

## Integration Notes

### With LLM Package

LLM providers can accept a tracer. Each Chat() call creates a span with model, tokens, and timing.

### With MCP Package

MCP tool calls create spans linking agent to MCP server execution. Use cross-process propagation for full traces.

### With Shutdown

Call the shutdown function before exiting. This flushes pending spans to ensure nothing is lost.

## Design Decisions

### Why OpenTelemetry?

Industry standard with broad backend support. Write once, export to any compatible system.

### Why explicit spans?

Auto-instrumentation misses agent-specific context. Explicit spans let us capture exactly what matters (tokens, tool names, security verdicts).

### Why optional content?

Privacy by default. Users shouldn't need to worry about prompts being logged. Debug mode is opt-in for troubleshooting.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Span creation, attribute population |
| Export | OTLP encoding, batch handling |
| Integration | Full trace through agent workflow |
| Debug mode | Content capture on/off |
| Failure | Backend unavailable, graceful degradation |
