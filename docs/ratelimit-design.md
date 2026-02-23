# Rate Limiter Design

## What This Package Does

The `ratelimit` package prevents agents from overwhelming shared resources (like APIs) by coordinating request rates across a swarm. When one agent hits a rate limit, it tells the others to slow down too.

## Why It Exists

External APIs have rate limits. If you're allowed 100 requests per minute, and you have 10 agents each making requests, they need to share that quota. Without coordination:

- Each agent thinks it has the full 100 requests
- They collectively send 1000 requests
- Most requests get rejected (HTTP 429)
- You waste resources and annoy the API provider

This package solves that by:
1. Giving each agent a local token bucket (limits its own rate)
2. When an agent hits a 429, it broadcasts "slow down" to all agents
3. All agents reduce their limits simultaneously
4. Over time, limits gradually recover

## When to Use It

**Use rate limiting for:**
- Coordinating access to external APIs with quotas
- Protecting shared resources from overload
- Handling 429 (Too Many Requests) responses gracefully
- Ensuring fair resource sharing across agents

**Don't use rate limiting for:**
- Exact quota tracking (this is approximate)
- Complex tiered limits (this is simple token bucket)
- Persistent configuration (limits are in-memory)

## Core Concepts

### Token Bucket

Each resource has a **token bucket** — a pool of tokens that refills over time. To make a request, you take a token. If no tokens are available, you wait.

![Token Bucket State Transitions](images/ratelimit-token-bucket.png)

**Example:** 100 tokens, refills every minute. You can make 100 requests per minute. If you use all tokens, you wait for refill.

### Acquire and Release

**Acquire** — Take a token before making a request. Blocks if none available.

**TryAcquire** — Try to take a token without blocking. Returns success/failure.

**Release** — Return a token (optional). Useful for tracking in-flight requests.

### Capacity Reduction

When an agent receives a 429 response, it calls `AnnounceReduced()`. This:
1. Cuts the local capacity (e.g., by 50%)
2. Broadcasts to other agents via the message bus
3. Other agents also cut their capacity

Now the entire swarm is making fewer requests, reducing 429s.

### Gradual Recovery

After a reduction, a background process slowly increases capacity back to normal. This typically happens over minutes:
- Every 30 seconds, increase capacity by 10%
- Stop when you reach the original limit (or exceed it, configurable)

This prevents the swarm from immediately hammering the API again after a brief pause.

## Architecture

![Rate Limiter Architecture](images/ratelimit-architecture.png)

Two implementations:

**MemoryLimiter** — Local token buckets only. Each agent limits itself independently. Good for testing or single-agent scenarios.

**DistributedLimiter** — Local buckets + bus coordination. When one agent announces a reduction, all agents hear it. Good for production swarms.

## Distributed Coordination

When using DistributedLimiter:

1. **Initial setup** — Each agent configures the same resources with the same limits
2. **Normal operation** — Each agent uses its local bucket
3. **Rate limit hit** — One agent calls `AnnounceReduced(resource, reason)`
4. **Broadcast** — Message goes out on `ratelimit.capacity` subject
5. **All agents receive** — Each reduces its local capacity
6. **Recovery** — Background process gradually restores capacity

![Distributed Coordination Message Flow](images/ratelimit-message-flow.png)

This is **eventually consistent**. There's a brief window where some agents haven't received the reduction yet. That's acceptable — the goal is approximate coordination, not perfect synchronization.

## Common Patterns

### API Client Integration

Before making an API call, acquire a token. If the call returns 429, announce reduction. This integrates rate limiting into your HTTP client.

### Multiple Resources

You can limit multiple resources independently. Set up separate capacities for different APIs or endpoints. Each has its own bucket and coordination.

### Shared vs Per-Agent Limits

If an API gives you 1000 requests/minute for your entire organization, divide among agents: 10 agents × 100 each. If each agent has its own quota, configure accordingly.

### Backoff on Reduction

When announcing a reduction, you can specify the reason. Other agents might use this to decide how aggressively to back off.

## Error Scenarios

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| Unknown resource | Acquire fails | Configure capacity first |
| No tokens available | Acquire blocks | Wait or use TryAcquire |
| Context cancelled | Acquire returns error | Handle timeout |
| Bus down | Reduction not broadcast | Local reduction still works |
| Agent ignores reduction | That agent gets more 429s | Ensure all agents use same config |

## Design Decisions

### Why token bucket?

Simple, well-understood, good enough. More complex algorithms (leaky bucket, sliding window) add complexity without much benefit for this use case.

### Why broadcast reductions?

When one agent hits a limit, others probably will too. Proactive reduction prevents wasted requests. The first agent to hit the limit warns the others.

### Why gradual recovery?

Instant recovery would immediately restore full capacity, potentially causing another wave of 429s. Gradual recovery tests whether the API is ready for more traffic.

### Why no persistence?

Rate limits are operational state, not configuration. After restart, agents re-learn limits quickly (first 429 triggers reduction). Persisting would add complexity for little benefit.

## Integration Notes

### With LLM Package

LLM API calls should acquire tokens before sending requests. On 429 response, call `AnnounceReduced()`. This coordinates across all agents using the same LLM provider.

### With Bus Package

DistributedLimiter requires a MessageBus for coordination. Uses subject `ratelimit.capacity` for broadcasts.

### With Shutdown

Call `Close()` to stop the recovery goroutine and release resources.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Token bucket mechanics, acquire/release |
| Concurrency | Multiple goroutines acquiring same resource |
| Distributed | Reduction broadcast and reception |
| Recovery | Gradual capacity restoration |
| Timeout | Context cancellation during acquire |
