// Package bus provides message bus clients for agent-to-agent communication.
//
// The [Bus] interface enables pub/sub and request/reply patterns over
// various backends (NATS, in-memory). Subscriptions support wildcards:
//
//   - [Wildcard] (*) matches exactly one token: "foo.*" matches "foo.bar"
//   - [WildcardAll] (>) matches trailing tokens: "foo.>" matches "foo.bar.baz"
//
// # Pub/Sub
//
//	b := bus.Memory(bus.Config{})
//	b.Publish("events.user", data)
//	sub, _ := b.Subscribe("events.*")
//	for msg := range sub.Messages() { ... }
//
// # Queue Groups (load balanced)
//
//	sub, _ := b.QueueSubscribe("tasks", "workers")
//	// Only one worker in the group receives each message
//
// # Request/Reply
//
//	reply, _ := b.Request("service", data, timeout)
//
// # Implementations
//
//   - [Memory]: in-memory channels for testing and single-process use
//   - [NATS]: production-grade messaging via NATS server
package bus
