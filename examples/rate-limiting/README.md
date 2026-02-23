# Rate Limiting Example

Demonstrates coordinated rate limiting using agentkit's `ratelimit` package for both single-agent and multi-agent swarm scenarios.

## Overview

This example shows how to:
- **Configure rate limits** for different resources with token bucket semantics
- **Acquire and release tokens** for request tracking
- **Handle 429 responses** by announcing reduced capacity
- **Coordinate across agents** using a distributed limiter and message bus

## Architecture

### Local Rate Limiting (MemoryLimiter)

```
┌─────────────────────────────────────────────────────────┐
│                    MemoryLimiter                        │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─────────────────┐  ┌─────────────────┐              │
│  │  openai-api     │  │  database       │              │
│  │  5 tokens/sec   │  │  10 tokens/sec  │              │
│  │  ████░░░░░ 4/5  │  │  ██████████ 10  │              │
│  └─────────────────┘  └─────────────────┘              │
│                                                         │
│  TryAcquire()  →  Get token immediately or fail        │
│  Acquire(ctx)  →  Block until token available          │
│  Release()     →  Return token to bucket               │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Distributed Rate Limiting (DistributedLimiter)

```
┌────────────────────┐          ┌────────────────────┐
│      Agent-1       │          │      Agent-2       │
│  ┌──────────────┐  │          │  ┌──────────────┐  │
│  │ Distributed  │  │          │  │ Distributed  │  │
│  │   Limiter    │  │          │  │   Limiter    │  │
│  │              │  │          │  │              │  │
│  │ shared-api:  │  │          │  │ shared-api:  │  │
│  │   10/sec     │  │          │  │   10/sec     │  │
│  └──────────────┘  │          │  └──────────────┘  │
└────────┬───────────┘          └────────┬───────────┘
         │                               │
         │     ┌─────────────────┐       │
         └────►│   Message Bus   │◄──────┘
               │  (NATS/Memory)  │
               └─────────────────┘
                       │
                       ▼
           AnnounceReduced("shared-api")
                       │
          ┌────────────┴────────────┐
          ▼                         ▼
    Agent-1: 5/sec            Agent-2: 5/sec
    (local reduction)         (received via bus)
```

## How to Run

```bash
cd examples/rate-limiting
go run main.go
```

## Example Output

```
=== Rate Limiting Example ===

--- Part 1: MemoryLimiter (Local Rate Limiting) ---

Configured rate limits:
  openai-api: 5/5 available, 0 in-flight, window=1s
  database: 10/10 available, 0 in-flight, window=1s
  external-service: 3/3 available, 0 in-flight, window=1s

TryAcquire demo (non-blocking):
  ✓ Request 1: acquired token
  ✓ Request 2: acquired token
  ✓ Request 3: acquired token
  ✓ Request 4: acquired token
  ✓ Request 5: acquired token
  ✗ Request 6: rate limited (no token available)
  ✗ Request 7: rate limited (no token available)

Capacity after consuming tokens:
  openai-api: 0/5 available, 5 in-flight, window=1s

Releasing tokens...
Capacity after releasing 3 tokens:
  openai-api: 3/5 available, 2 in-flight, window=1s

Blocking Acquire demo (with timeout):
  ✓ Acquired token in 0ms

Simulating 429 response - announcing reduced capacity:
  Before:
  external-service: 3/3 available, 0 in-flight, window=1s
  After (capacity reduced by 25%):
  external-service: 2/2 available, 0 in-flight, window=1s

Concurrent access pattern (5 workers, 10 requests each):
  Results: 47/50 succeeded, 3 failed (timeout)

--- Part 2: DistributedLimiter (Swarm Coordination) ---

Two agents sharing 'shared-api' rate limit (10/sec each):
  Agent-1: 10/10 available, 0 in-flight, window=1s
  Agent-2: 10/10 available, 0 in-flight, window=1s

Both agents making requests concurrently...
  Agent-1 successful requests: 8
  Agent-2 successful requests: 8

Agent-1 receives 429 response and announces reduced capacity...
  Before announcement:
    Agent-1: 10/10 available, 0 in-flight, window=1s
    Agent-2: 10/10 available, 0 in-flight, window=1s
  After announcement (both agents reduce capacity):
    Agent-1: 5/5 available, 0 in-flight, window=1s
    Agent-2: 5/5 available, 0 in-flight, window=1s

=== Example Complete ===
```

## Key Concepts

### Token Bucket Algorithm

The rate limiter uses the token bucket algorithm:
- Bucket starts full (capacity tokens)
- Each `Acquire`/`TryAcquire` consumes one token
- Tokens refill over time based on `window` (e.g., 5 tokens/second = 1 token every 200ms)
- `Release` returns a token immediately (useful for tracking in-flight requests)

### Creating a MemoryLimiter

```go
limiter := ratelimit.NewMemoryLimiter()
defer limiter.Close()

// Configure: 60 requests per minute
limiter.SetCapacity("openai-api", 60, time.Minute)
```

### Acquiring Tokens

```go
// Non-blocking: returns immediately
if limiter.TryAcquire("openai-api") {
    defer limiter.Release("openai-api")
    // Make API call
}

// Blocking: waits for token or context cancellation
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := limiter.Acquire(ctx, "openai-api"); err != nil {
    // Handle timeout or cancellation
    return err
}
defer limiter.Release("openai-api")
// Make API call
```

### Handling Rate Limit Responses

When you receive a 429 (Too Many Requests) response:

```go
if resp.StatusCode == 429 {
    // Announce reduced capacity to the swarm
    limiter.AnnounceReduced("openai-api", "received 429 from API")
    // Retry with backoff...
}
```

### Distributed Coordination

```go
// Create distributed limiter with message bus
limiter, err := ratelimit.NewDistributedLimiter(ratelimit.DistributedConfig{
    Bus:     natsClient,           // or bus.NewMemoryBus() for testing
    AgentID: "agent-123",          // unique identifier
    ReduceFactor: 0.5,             // reduce by 50% on announcement
    RecoveryInterval: 30 * time.Second,
    RecoveryFactor: 1.1,           // recover by 10% each interval
})

// Set up callback for capacity changes from other agents
limiter.OnCapacityChange(func(update *ratelimit.CapacityUpdate) {
    log.Printf("Agent %s reduced %s capacity to %d: %s",
        update.AgentID, update.Resource, update.NewCapacity, update.Reason)
})
```

### Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `ReduceFactor` | 0.5 | Multiplier when reducing capacity (0.5 = 50% reduction) |
| `RecoveryInterval` | 30s | How often to attempt capacity recovery |
| `RecoveryFactor` | 1.1 | Multiplier when recovering (1.1 = 10% increase) |
| `MaxRecovery` | true | Cap recovery at original capacity |

## Patterns

### API Client with Rate Limiting

```go
type APIClient struct {
    limiter ratelimit.RateLimiter
    client  *http.Client
}

func (c *APIClient) Request(ctx context.Context, url string) (*http.Response, error) {
    // Wait for rate limit token
    if err := c.limiter.Acquire(ctx, "api"); err != nil {
        return nil, fmt.Errorf("rate limit: %w", err)
    }
    defer c.limiter.Release("api")

    resp, err := c.client.Get(url)
    if err != nil {
        return nil, err
    }

    // Handle rate limit response
    if resp.StatusCode == 429 {
        c.limiter.AnnounceReduced("api", "429 response")
        return nil, ErrRateLimited
    }

    return resp, nil
}
```

### Worker Pool with Shared Rate Limit

```go
func worker(ctx context.Context, limiter ratelimit.RateLimiter, jobs <-chan Job) {
    for job := range jobs {
        // Get rate limit token (blocks if exhausted)
        if err := limiter.Acquire(ctx, "database"); err != nil {
            continue // Context cancelled
        }

        // Process job
        processJob(job)

        // Return token when done
        limiter.Release("database")
    }
}
```

### Multiple Resources with Different Limits

```go
limiter := ratelimit.NewMemoryLimiter()

// Different limits for different resources
limiter.SetCapacity("openai-gpt4", 20, time.Minute)   // 20/min for GPT-4
limiter.SetCapacity("openai-gpt3", 60, time.Minute)  // 60/min for GPT-3.5
limiter.SetCapacity("embeddings", 200, time.Minute)  // 200/min for embeddings

// Use appropriate limit for each call
if limiter.TryAcquire("openai-gpt4") {
    defer limiter.Release("openai-gpt4")
    callGPT4()
}
```

## Notes

- Always call `Release()` after `Acquire()` to track in-flight requests accurately
- Use `TryAcquire()` for non-critical requests that can be skipped
- Use `Acquire()` with a context timeout for critical requests
- In production, use NATS (`bus.NewNATSClient()`) for distributed coordination
- Set capacity slightly below actual API limits for safety margin
- The distributed limiter automatically recovers capacity over time after reductions
