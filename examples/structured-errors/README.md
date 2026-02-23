# Structured Errors Example

This example demonstrates the `agentkit/errors` package for structured error handling in agent-based systems.

## Overview

The `errors` package provides:

- **Error Categories**: Classify errors by their nature (transient, permanent, resource, internal)
- **Error Codes**: Specific codes identifying failure types (TIMEOUT, NOT_FOUND, RATE_LIMITED, etc.)
- **Retry Semantics**: Built-in logic to determine if operations should be retried
- **Rich Metadata**: Attach context like agent IDs, task IDs, and custom key-value pairs
- **JSON Serialization**: Full support for cross-agent communication

## Running the Example

```bash
cd examples/structured-errors
go run main.go
```

## Key Concepts

### Error Categories

| Category    | Description                          | Retryable |
|-------------|--------------------------------------|-----------|
| `transient` | Temporary failures (network, etc.)   | Yes       |
| `permanent` | Won't succeed on retry               | No        |
| `resource`  | Rate limits, quotas                  | Yes       |
| `internal`  | Bugs, system failures                | No        |

### Creating Errors

```go
// Using convenience functions
err := errors.Timeout("connection timed out")
err := errors.NotFound("user not found")
err := errors.RateLimited("quota exceeded")

// Using New() with options
err := errors.New(errors.ErrCodeInternal, "unexpected state",
    errors.WithAgentID("worker-1"),
    errors.WithTaskID("task-123"),
    errors.WithMetadata("key", "value"),
)
```

### Wrapping Errors

```go
// Wrap preserves the original error's code and category
wrapped := errors.Wrap(originalErr, "additional context")

// Wrap with a specific code
wrapped := errors.WrapWithCode(err, errors.ErrCodeUnavailable, "service down")
```

### Checking Errors

```go
// Check specific code
if errors.Is(err, errors.ErrCodeTimeout) {
    // Handle timeout
}

// Check category
if errors.IsTransient(err) {
    // Retry logic
}

// Check retryability directly
if errors.IsRetryable(err) {
    // Safe to retry
}

// Extract as AgentError for full access
if agentErr := errors.AsAgentError(err); agentErr != nil {
    fmt.Println(agentErr.Code(), agentErr.Metadata())
}
```

### Retry Pattern

```go
func executeWithRetry(maxRetries int) error {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        err := doOperation()
        if err == nil {
            return nil
        }
        
        if !errors.IsRetryable(err) {
            return err // Don't retry permanent errors
        }
        
        // Optional: check metadata for rate limit reset time
        if errors.Is(err, errors.ErrCodeRateLimit) {
            if meta := errors.GetMetadata(err); meta != nil {
                if resetAt, ok := meta["reset_at"]; ok {
                    waitUntil(resetAt)
                }
            }
        }
        
        time.Sleep(backoff(attempt))
    }
    return errors.New(errors.ErrCodeTimeout, "max retries exceeded")
}
```

### JSON Serialization

Errors serialize to JSON for cross-agent communication:

```go
// Serialize
jsonData, _ := json.Marshal(err)

// Deserialize
var restored errors.Error
json.Unmarshal(jsonData, &restored)
```

Example JSON output:

```json
{
  "code": "TASK_FAILED",
  "category": "permanent",
  "message": "task task-xyz-789 failed: execution timeout",
  "metadata": {
    "duration_ms": "30000",
    "stage": "data_processing"
  },
  "retryable": false,
  "timestamp": "2024-01-15T10:30:00Z",
  "agent_id": "executor-2",
  "task_id": "task-xyz-789"
}
```

## Agent-Specific Errors

The package includes error types specific to agent coordination:

```go
// Agent went offline
errors.AgentOffline("worker-3")

// Task execution failed
errors.TaskFailed("task-id", "reason")

// Coordination between agents failed
errors.CoordinationFailure("consensus not reached")
```

## See Also

- [`errors/doc.go`](../../errors/doc.go) - Package documentation
- [`errors/codes.go`](../../errors/codes.go) - All error codes and categories
- [`errors/wrap.go`](../../errors/wrap.go) - Error wrapping utilities
