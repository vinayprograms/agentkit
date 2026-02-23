// Package ratelimit provides rate limit coordination for agent swarms.
//
// Rate limiting is essential when multiple agents access shared resources
// (APIs, databases, external services) that have capacity constraints.
// This package provides both local and distributed rate limiters.
//
// # Local Rate Limiting
//
// The MemoryLimiter provides per-process rate limiting using token buckets:
//
//	limiter := ratelimit.NewMemoryLimiter()
//	limiter.SetCapacity("openai-api", 60, time.Minute) // 60 requests per minute
//
//	// Block until token available
//	if err := limiter.Acquire(ctx, "openai-api"); err != nil {
//	    return err // context cancelled
//	}
//	defer limiter.Release("openai-api")
//
//	// Non-blocking attempt
//	if limiter.TryAcquire("openai-api") {
//	    defer limiter.Release("openai-api")
//	    // Make request
//	}
//
// # Distributed Rate Limiting
//
// The DistributedLimiter coordinates rate limits across agents via the message bus:
//
//	limiter, err := ratelimit.NewDistributedLimiter(ratelimit.DistributedConfig{
//	    Bus:     nbus,
//	    AgentID: "agent-1",
//	})
//	limiter.SetCapacity("shared-api", 100, time.Minute)
//
//	// Announce reduced capacity (e.g., after 429 response)
//	limiter.AnnounceReduced("shared-api", "received 429 from API")
//
// When an agent announces reduced capacity, all agents in the swarm
// automatically adjust their local rate limits to share the remaining capacity.
//
// # Algorithm
//
// Both implementations use the token bucket algorithm with refill:
//   - Tokens are added at a fixed rate based on capacity/window
//   - Each Acquire consumes one token
//   - If no tokens available, Acquire blocks (or TryAcquire returns false)
//   - Release returns a token to the bucket (optional, for request tracking)
//
// # Best Practices
//
//   - Set capacity slightly below actual limits for safety margin
//   - Use Release to track in-flight requests, not just rate
//   - Watch for AnnounceReduced in production to detect limit issues
//   - Use TryAcquire with fallback for non-critical requests
package ratelimit
