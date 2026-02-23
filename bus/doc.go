// Package bus provides message bus clients for agent-to-agent communication.
//
// # Overview
//
// The MessageBus interface enables pub/sub and request/reply patterns for
// distributed agent communication. All implementations use channel-based APIs
// for Go-idiomatic concurrent use.
//
// # Available Implementations
//
//   - NATSBus: Production-grade messaging using NATS
//   - MemoryBus: In-memory implementation for testing and single-process use
//
// # Patterns
//
// Pub/Sub - broadcast to all subscribers:
//
//	bus.Publish("events.user", data)
//	sub, _ := bus.Subscribe("events.user")
//	for msg := range sub.Messages() {
//	    // Handle message
//	}
//
// Queue Groups - load balanced across workers:
//
//	sub, _ := bus.QueueSubscribe("tasks", "workers")
//	// Only one worker in the group receives each message
//
// Request/Reply - synchronous RPC:
//
//	// Responder
//	sub, _ := bus.Subscribe("service")
//	for msg := range sub.Messages() {
//	    bus.Publish(msg.Reply, response)
//	}
//
//	// Requester
//	reply, _ := bus.Request("service", data, timeout)
//
// # Queue Groups for Swarm Agents
//
// Queue subscriptions enable load balancing across agent instances:
//
//   - Multiple agents subscribe to same subject with same queue name
//   - Each message delivered to exactly one agent in the queue group
//   - Natural scaling: add more agents to handle more load
//   - No coordination needed between agents
package bus
