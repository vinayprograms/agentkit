// Package registry provides agent registration and discovery for swarm coordination.
//
// # Overview
//
// The Registry interface enables agents to self-register with capabilities,
// status, and load. Other agents discover and route to appropriate handlers
// based on capabilities and availability.
//
// # Available Implementations
//
//   - MemoryRegistry: In-memory implementation for testing and single-node use
//   - NATSRegistry: Distributed registry using NATS JetStream KV store
//
// # Basic Usage
//
// Register an agent:
//
//	reg := registry.NewMemoryRegistry(registry.MemoryConfig{})
//	err := reg.Register(registry.AgentInfo{
//	    ID:           "agent-1",
//	    Name:         "Code Review Agent",
//	    Capabilities: []string{"code-review", "testing"},
//	    Status:       registry.StatusIdle,
//	    Load:         0.3,
//	})
//
// Discover agents by capability:
//
//	agents, _ := reg.FindByCapability("code-review")
//	// Returns agents sorted by load (lowest first)
//	if len(agents) > 0 {
//	    target := agents[0] // Pick the least loaded agent
//	}
//
// Watch for changes:
//
//	events, _ := reg.Watch()
//	for event := range events {
//	    switch event.Type {
//	    case registry.EventAdded:
//	        fmt.Printf("New agent: %s\n", event.Agent.ID)
//	    case registry.EventUpdated:
//	        fmt.Printf("Agent updated: %s (load=%.2f)\n", event.Agent.ID, event.Agent.Load)
//	    case registry.EventRemoved:
//	        fmt.Printf("Agent removed: %s\n", event.Agent.ID)
//	    }
//	}
//
// # NATS Registry
//
// For distributed deployments, use NATSRegistry with a shared NATS cluster:
//
//	import "github.com/vinayprograms/agentkit/bus"
//
//	// Reuse bus connection
//	natsBus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
//	reg, _ := registry.NewNATSRegistry(natsBus.Conn(), registry.NATSRegistryConfig{
//	    BucketName: "my-swarm-registry",
//	    TTL:        30 * time.Second,
//	})
//
// Multiple agents across different nodes share the same registry, enabling
// discovery and load balancing across the swarm.
//
// # TTL and Stale Entries
//
// Both implementations support TTL-based expiry. Agents should periodically
// re-register (heartbeat) to prevent being marked stale:
//
//	// Heartbeat every 10 seconds
//	ticker := time.NewTicker(10 * time.Second)
//	for range ticker.C {
//	    reg.Register(AgentInfo{
//	        ID:     myID,
//	        Status: currentStatus,
//	        Load:   currentLoad,
//	    })
//	}
//
// # Load Balancing
//
// FindByCapability returns agents sorted by load (lowest first), enabling
// simple load-aware routing:
//
//	agents, _ := reg.FindByCapability("code-review")
//	agents, _ = reg.List(&registry.Filter{
//	    Capability: "code-review",
//	    Status:     registry.StatusIdle,
//	    MaxLoad:    0.8,  // Only agents with load <= 80%
//	})
package registry
