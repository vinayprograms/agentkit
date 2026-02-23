// Package state provides shared state management for distributed agent coordination.
//
// The StateStore interface enables key-value storage with TTL, watch notifications,
// and distributed locking across various backends (NATS JetStream KV, in-memory).
//
// # Key Features
//
//   - Key-value operations: Get, Put, Delete with optional TTL
//   - Watch: Subscribe to changes on key patterns
//   - Distributed locks: Acquire/release with automatic expiry
//   - Multiple backends: NATS JetStream KV (production), in-memory (testing)
//
// # Usage
//
//	// Production: NATS JetStream KV
//	bus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
//	store, _ := state.NewNATSStore(state.NATSStoreConfig{
//	    Conn:   bus.Conn(),
//	    Bucket: "agent-state",
//	})
//
//	// Testing: In-memory
//	store := state.NewMemoryStore()
//
//	// Key-value operations
//	store.Put("config.timeout", []byte("30s"), time.Hour)
//	val, _ := store.Get("config.timeout")
//
//	// Watch for changes
//	ch, _ := store.Watch("config.*")
//	for kv := range ch {
//	    fmt.Printf("Key %s changed: %s\n", kv.Key, kv.Value)
//	}
//
//	// Distributed locking
//	lock, _ := store.Lock("resource.mutex", 30*time.Second)
//	defer lock.Unlock()
//	// ... critical section ...
package state
