# Logging Design

## What This Package Does

The `logging` package provides real-time console output for monitoring agent execution. It produces structured, human-readable logs showing what the agent is doing as it happens.

## Why It Exists

When developing and debugging agents, you need visibility:
- What goal is the agent working on?
- Which tools is it calling?
- Where did things go wrong?

Logs provide this real-time window into agent behavior.

**Important distinction:** Logs are for real-time monitoring. The session JSON file is the forensic record. Don't confuse them:
- **Logs** — Ephemeral console output for watching execution
- **Session JSON** — Persistent record for replay and analysis

## When to Use It

**Use logging for:**
- Watching agent execution in real-time
- Debugging during development
- Monitoring production agents
- Troubleshooting failures

**Don't use logging for:**
- Forensic analysis (use session JSON)
- Log aggregation (use external systems like ELK)
- Persistent storage (logs go to console, not files)

## Core Concepts

### Log Levels

Levels control verbosity:

| Level | Use Case |
|-------|----------|
| DEBUG | Verbose tracing — every detail |
| INFO | Normal operations — what's happening |
| WARN | Potential issues — something's off |
| ERROR | Failures — something broke |

Each level includes all higher-severity levels. Setting INFO shows INFO, WARN, and ERROR but hides DEBUG.

### Structured Fields

Logs include key-value pairs for context:

```
INFO  2026-02-23T12:00:00Z tool_call tool=bash duration=150ms exit=0
```

This format is:
- Human readable (makes sense at a glance)
- Machine parseable (can grep or filter)
- Compact (fits in terminal width)

### Components

Loggers can have a component name for context:

```
INFO  2026-02-23T12:00:00Z [executor] starting goal goal=analyze-code
```

The `[executor]` tag shows which subsystem generated the log.

### Trace Correlation

Loggers can carry a trace ID that appears in every log line. This helps correlate logs from the same request across components.

## Architecture


The logger writes to an output stream (default: stdout). Multiple goroutines can log concurrently — a mutex ensures lines don't interleave.

## Output Format

```
LEVEL TIMESTAMP [component] message key=value key=value
```

Example lines:

```
INFO  2026-02-23T12:00:00Z [executor] goal_start goal=review-pr
DEBUG 2026-02-23T12:00:01Z [security] checking_policy path=/etc/passwd
WARN  2026-02-23T12:00:02Z [supervisor] high_latency duration=5.2s
ERROR 2026-02-23T12:00:03Z [llm] request_failed error="rate limited"
```

The format prioritizes readability. For JSON structured logging, use a separate log aggregation system.

## Common Patterns

### Basic Logging

Log with a message and optional fields:
- `Info("starting", fields)` — Normal event
- `Warn("slow response", fields)` — Something concerning
- `Error("failed", fields)` — Something broke

### Component Logger

Create a logger scoped to a component:
- Component name appears in brackets
- Helps identify which part of the system logged

### Conditional Debug

Debug logs are verbose. Enable them during development, disable in production:
- Development: DEBUG level shows everything
- Production: INFO level hides noise

### Event-Derived Logging

The logger provides methods for common agent events:
- Tool calls (start, result, error)
- Goal transitions (start, complete, fail)
- Security events (warnings, verdicts)

These ensure consistent formatting across the codebase.

## Relationship to Session Forensics


| Aspect | Logging | Session JSON |
|--------|---------|--------------|
| Purpose | Real-time monitoring | Post-hoc analysis |
| Persistence | Ephemeral | Durable |
| Format | Human-readable text | Structured JSON |
| Completeness | Filtered by level | Complete record |
| Use case | Watch execution | Replay, audit |

The session JSON derives logs (you can replay events as log lines), but logging doesn't derive session JSON. Keep them separate in your mental model.

## Error Handling

The logger follows the "logging should never crash the app" principle:
- Write errors are silently ignored
- Nil fields are handled gracefully
- Malformed input produces ugly output but doesn't panic

Logging is best-effort. If it fails, the agent keeps running.

## Thread Safety

All logging operations are safe for concurrent use:
- Multiple goroutines can log simultaneously
- Lines won't interleave (mutex protection)
- Creating child loggers is thread-safe

## Design Decisions

### Why not JSON output?

Readability. Humans read logs in terminals. JSON is for machines. If you need JSON, pipe logs through a formatter or use a dedicated log aggregation system.

### Why synchronous writes?

Simplicity. Console logging doesn't need buffering. For high-throughput scenarios, use async logging externally.

### Why no log rotation?

That's an OS concern. Use `logrotate` or similar. The logging package writes to a stream; what happens to that stream is outside its scope.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Level filtering, field formatting |
| Output | Correct format, timestamp generation |
| Concurrency | No interleaving, no races |
| Integration | End-to-end agent logging |
