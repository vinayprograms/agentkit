# Errors Package Design

## What This Package Does

The `errors` package provides structured error types for agent swarms. Instead of plain error strings, errors carry rich information: a code identifying what went wrong, a category for retry decisions, and metadata for debugging.

## Why It Exists

In distributed systems, error handling is complex:

- **Should I retry?** — Some errors are temporary (network blip), others are permanent (not found)
- **What went wrong?** — "error occurred" isn't helpful; "RATE_LIMITED on anthropic-api" is
- **Cross-agent errors** — When Agent A fails while calling Agent B, error context can be lost

Structured errors solve these by:
1. Classifying errors into categories with known retry behavior
2. Attaching codes that precisely identify the failure
3. Serializing to JSON for cross-agent propagation
4. Wrapping errors while preserving the original information

## When to Use It

**Use structured errors for:**
- Any error that might cross agent boundaries
- Errors where retry decisions matter
- Operations where you need to distinguish error types programmatically

**You might skip this for:**
- Simple internal errors that won't be transmitted
- Errors where you'll just log and fail anyway

## Core Concepts

### Error Categories

Every error belongs to a category that determines default retry behavior:

| Category | Retryable | Description |
|----------|-----------|-------------|
| **Transient** | Yes | Temporary failures — network timeout, service unavailable |
| **Resource** | Yes | Exhaustion — rate limit, quota exceeded |
| **Permanent** | No | Retry won't help — not found, invalid input |
| **Internal** | No | Bugs — panics, assertions, corruption |

When you receive an error, check its category to decide what to do. Transient and resource errors can be retried with backoff. Permanent and internal errors should fail immediately.

### Error Codes

Within each category, specific codes identify exactly what went wrong:

**Transient (retry with backoff):**
- `TIMEOUT` — Operation took too long
- `UNAVAILABLE` — Service temporarily down
- `NETWORK_ERR` — Connectivity issue
- `AGENT_OFFLINE` — Target agent unreachable
- `AGENT_BUSY` — Agent processing something else

**Resource (retry after waiting):**
- `RATE_LIMITED` — Too many requests
- `QUOTA_EXCEEDED` — Usage limit hit
- `RESOURCE_BUSY` — Resource locked
- `CAPACITY` — System overloaded

**Permanent (don't retry):**
- `NOT_FOUND` — Resource doesn't exist
- `INVALID_INPUT` — Bad request data
- `UNAUTHORIZED` — Auth failed
- `FORBIDDEN` — Permission denied
- `TASK_FAILED` — Task execution failed

**Internal (don't retry, investigate):**
- `INTERNAL` — Unexpected error
- `PANIC` — Recovered from crash
- `CORRUPTION` — Data integrity issue

### Error Wrapping

When you catch an error and need to add context, wrap it. Wrapping:
- Adds your context message
- Preserves the original error's code and category
- Maintains the error chain for `errors.Is` and `errors.As`

This lets errors bubble up through layers while retaining their original meaning.


### Retry Semantics

The package provides helpers to check if an error is retryable:


Your retry logic can simply ask "is this retryable?" without knowing the specific error. The category system encapsulates retry policy.

## Architecture


The error interface extends Go's standard `error` with:
- `Code()` — What specific failure occurred
- `Category()` — Classification for handling
- `Retryable()` — Should this be retried?
- `Metadata()` — Additional context
- `Unwrap()` — Access wrapped error

This integrates with Go's standard library — `errors.Is()` and `errors.As()` work as expected.

### JSON Serialization

Errors serialize to JSON for cross-agent communication. When Agent A calls Agent B and B fails, B's error can be serialized, sent back, and deserialized by A with full fidelity.

## Common Patterns

### Creating Errors

Use the convenience functions for common error types:
- `Timeout(msg)` — Creates transient timeout error
- `NotFound(msg)` — Creates permanent not found error
- `RateLimited(msg)` — Creates resource rate limit error
- And so on for each code

### Checking Errors

Check if an error matches a code or category:
- `IsTimeout(err)` — Is this a timeout?
- `IsRetryable(err)` — Should we retry?
- `IsTransient(err)` — Is this transient?

### Adding Context

Wrap errors to add context while preserving the original:
- `Wrap(err, "calling external API")` — Adds message
- `WithMetadata(err, "agent_id", "agent-123")` — Adds key-value data

### Retry Loops

A typical retry loop:
1. Attempt operation
2. If error, check `IsRetryable(err)`
3. If retryable, wait with backoff, retry
4. If not retryable or max attempts reached, fail

## Integration with Other Packages

All agentkit packages use this error taxonomy:

| Package | Error Examples |
|---------|----------------|
| Bus | `UNAVAILABLE` if connection lost |
| State | `NOT_FOUND` for missing keys |
| Tasks | `TASK_FAILED` when processing fails |
| Registry | `AGENT_OFFLINE` for unreachable agents |
| Ratelimit | `RATE_LIMITED` when quota exceeded |

This consistency means error handling code works the same across packages.

## Design Decisions

### Why categories instead of just codes?

Categories group similar errors for handling. You don't need to enumerate every possible code in your retry logic — just check the category.

### Why explicit retryable flag?

Most errors use their category's default, but sometimes you need to override. An error might be transient but known to be non-retryable in a specific context.

### Why no stack traces?

Errors are data, not logs. Stack traces belong in structured logging or tracing systems. Keeping errors focused makes them smaller and faster to serialize.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Error creation, category defaults, code matching |
| Wrapping | Context preservation, chain traversal |
| Serialization | JSON round-trip, metadata handling |
| Integration | Cross-package error handling |
| Retry | Retryable classification, backoff behavior |
