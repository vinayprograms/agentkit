// Package main demonstrates rate limiting using agentkit's ratelimit package.
//
// This example shows:
// - Local rate limiting with MemoryLimiter
// - Distributed rate limiting with DistributedLimiter
// - Acquiring and releasing tokens
// - Handling rate limit announcements (capacity reduction)
// - Coordination across multiple agents via message bus
//
// Run: go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinayprograms/agentkit/bus"
	"github.com/vinayprograms/agentkit/ratelimit"
)

func main() {
	fmt.Println("=== Rate Limiting Example ===")
	fmt.Println()

	// Part 1: Local rate limiting with MemoryLimiter
	demonstrateMemoryLimiter()

	fmt.Println()

	// Part 2: Distributed rate limiting across agents
	demonstrateDistributedLimiter()
}

// =============================================================================
// Part 1: Local Rate Limiting with MemoryLimiter
// =============================================================================

func demonstrateMemoryLimiter() {
	fmt.Println("--- Part 1: MemoryLimiter (Local Rate Limiting) ---")
	fmt.Println()

	// Create a local rate limiter
	limiter := ratelimit.NewMemoryLimiter()
	defer limiter.Close()

	// Configure different resources with different capacities
	limiter.SetCapacity("openai-api", 5, time.Second)        // 5 requests per second
	limiter.SetCapacity("database", 10, time.Second)         // 10 queries per second
	limiter.SetCapacity("external-service", 3, time.Second)  // 3 requests per second

	fmt.Println("Configured rate limits:")
	printCapacity(limiter, "openai-api")
	printCapacity(limiter, "database")
	printCapacity(limiter, "external-service")
	fmt.Println()

	// Demonstrate TryAcquire (non-blocking)
	fmt.Println("TryAcquire demo (non-blocking):")
	for i := 0; i < 7; i++ {
		if limiter.TryAcquire("openai-api") {
			fmt.Printf("  ✓ Request %d: acquired token\n", i+1)
		} else {
			fmt.Printf("  ✗ Request %d: rate limited (no token available)\n", i+1)
		}
	}
	fmt.Println()

	// Show capacity after consuming tokens
	fmt.Println("Capacity after consuming tokens:")
	printCapacity(limiter, "openai-api")
	fmt.Println()

	// Demonstrate Release (returning tokens)
	fmt.Println("Releasing tokens...")
	for i := 0; i < 3; i++ {
		limiter.Release("openai-api")
	}
	fmt.Println("Capacity after releasing 3 tokens:")
	printCapacity(limiter, "openai-api")
	fmt.Println()

	// Demonstrate blocking Acquire with timeout
	fmt.Println("Blocking Acquire demo (with timeout):")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := limiter.Acquire(ctx, "openai-api")
	if err != nil {
		fmt.Printf("  Acquire failed after %v: %v\n", time.Since(start).Round(time.Millisecond), err)
	} else {
		fmt.Printf("  ✓ Acquired token in %v\n", time.Since(start).Round(time.Millisecond))
		limiter.Release("openai-api")
	}
	fmt.Println()

	// Demonstrate AnnounceReduced (capacity reduction)
	fmt.Println("Simulating 429 response - announcing reduced capacity:")
	fmt.Println("  Before:")
	printCapacity(limiter, "external-service")
	limiter.AnnounceReduced("external-service", "received 429 Too Many Requests")
	fmt.Println("  After (capacity reduced by 25%):")
	printCapacity(limiter, "external-service")
	fmt.Println()

	// Demonstrate concurrent access pattern
	fmt.Println("Concurrent access pattern (5 workers, 10 requests each):")
	demonstrateConcurrentAccess(limiter)
}

func printCapacity(limiter ratelimit.RateLimiter, resource string) {
	cap := limiter.GetCapacity(resource)
	if cap == nil {
		fmt.Printf("  %s: not configured\n", resource)
		return
	}
	fmt.Printf("  %s: %d/%d available, %d in-flight, window=%v\n",
		resource, cap.Available, cap.Total, cap.InFlight, cap.Window)
}

func demonstrateConcurrentAccess(limiter ratelimit.RateLimiter) {
	var wg sync.WaitGroup
	var success, failed atomic.Int64

	resource := "database"
	workers := 5
	requestsPerWorker := 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < requestsPerWorker; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				err := limiter.Acquire(ctx, resource)
				cancel()

				if err == nil {
					success.Add(1)
					// Simulate work
					time.Sleep(10 * time.Millisecond)
					limiter.Release(resource)
				} else {
					failed.Add(1)
				}
			}
		}(w)
	}

	wg.Wait()
	total := workers * requestsPerWorker
	fmt.Printf("  Results: %d/%d succeeded, %d failed (timeout)\n",
		success.Load(), total, failed.Load())
}

// =============================================================================
// Part 2: Distributed Rate Limiting with DistributedLimiter
// =============================================================================

func demonstrateDistributedLimiter() {
	fmt.Println("--- Part 2: DistributedLimiter (Swarm Coordination) ---")
	fmt.Println()

	// Create in-memory bus for this demo
	// In production, use NATS or another distributed bus
	memBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer memBus.Close()

	// Create two agents sharing the same rate limit
	agent1, err := createAgent(memBus, "agent-1")
	if err != nil {
		log.Fatalf("Failed to create agent-1: %v", err)
	}
	defer agent1.Close()

	agent2, err := createAgent(memBus, "agent-2")
	if err != nil {
		log.Fatalf("Failed to create agent-2: %v", err)
	}
	defer agent2.Close()

	// Both agents configure the same resource
	resource := "shared-api"
	agent1.SetCapacity(resource, 10, time.Second)
	agent2.SetCapacity(resource, 10, time.Second)

	fmt.Println("Two agents sharing 'shared-api' rate limit (10/sec each):")
	fmt.Printf("  Agent-1: ")
	printCapacity(agent1, resource)
	fmt.Printf("  Agent-2: ")
	printCapacity(agent2, resource)
	fmt.Println()

	// Simulate both agents making requests
	fmt.Println("Both agents making requests concurrently...")
	var wg sync.WaitGroup
	var agent1Success, agent2Success atomic.Int64

	// Agent 1 makes requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 8; i++ {
			if agent1.TryAcquire(resource) {
				agent1Success.Add(1)
				time.Sleep(50 * time.Millisecond) // Simulate API call
				agent1.Release(resource)
			}
		}
	}()

	// Agent 2 makes requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 8; i++ {
			if agent2.TryAcquire(resource) {
				agent2Success.Add(1)
				time.Sleep(50 * time.Millisecond) // Simulate API call
				agent2.Release(resource)
			}
		}
	}()

	wg.Wait()
	fmt.Printf("  Agent-1 successful requests: %d\n", agent1Success.Load())
	fmt.Printf("  Agent-2 successful requests: %d\n", agent2Success.Load())
	fmt.Println()

	// Demonstrate distributed capacity reduction
	fmt.Println("Agent-1 receives 429 response and announces reduced capacity...")
	fmt.Println("  Before announcement:")
	fmt.Printf("    Agent-1: ")
	printCapacity(agent1, resource)
	fmt.Printf("    Agent-2: ")
	printCapacity(agent2, resource)

	// Agent 1 announces reduced capacity
	agent1.AnnounceReduced(resource, "received 429 from shared-api")

	// Give time for the message to propagate
	time.Sleep(100 * time.Millisecond)

	fmt.Println("  After announcement (both agents reduce capacity):")
	fmt.Printf("    Agent-1: ")
	printCapacity(agent1, resource)
	fmt.Printf("    Agent-2: ")
	printCapacity(agent2, resource)
	fmt.Println()

	// Demonstrate capacity change callback
	fmt.Println("Setting up capacity change callback on Agent-2...")
	agent2.OnCapacityChange(func(update *ratelimit.CapacityUpdate) {
		log.Printf("  [Agent-2 callback] Received capacity update from %s: %s reduced to %d (reason: %s)",
			update.AgentID, update.Resource, update.NewCapacity, update.Reason)
	})

	// Agent 1 announces another reduction
	fmt.Println("Agent-1 announces another reduction...")
	agent1.AnnounceReduced(resource, "still getting rate limited")

	// Wait for callback to fire
	time.Sleep(100 * time.Millisecond)
	fmt.Println()

	fmt.Println("=== Example Complete ===")
}

func createAgent(b bus.MessageBus, agentID string) (*ratelimit.DistributedLimiter, error) {
	config := ratelimit.DefaultDistributedConfig()
	config.Bus = b
	config.AgentID = agentID
	config.ReduceFactor = 0.5        // Reduce by 50% on announcement
	config.RecoveryInterval = time.Minute // Recover slowly

	return ratelimit.NewDistributedLimiter(config)
}
