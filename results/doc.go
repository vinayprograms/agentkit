// Package results provides task result publication and retrieval for agent coordination.
//
// The ResultPublisher interface enables agents to publish task outputs to a shared
// store, allowing other agents to subscribe to and retrieve results. This decouples
// result production from consumption and enables async task pipelines.
//
// Two implementations are provided:
//   - MemoryPublisher: In-memory storage for testing and single-process scenarios
//   - BusPublisher: Bus-backed storage with pub/sub notification for distributed systems
//
// # Basic Usage
//
//	// Create a memory publisher for testing
//	pub := results.NewMemoryPublisher()
//
//	// Publish a result
//	err := pub.Publish(ctx, "task-123", results.Result{
//	    TaskID:   "task-123",
//	    Status:   results.StatusSuccess,
//	    Output:   []byte(`{"answer": 42}`),
//	    Metadata: map[string]string{"model": "gpt-4"},
//	})
//
//	// Retrieve a result
//	result, err := pub.Get(ctx, "task-123")
//
//	// Subscribe to result updates
//	ch, err := pub.Subscribe("task-123")
//	for result := range ch {
//	    fmt.Printf("Result received: %s\n", result.Status)
//	}
//
// # Bus-based Publisher
//
// For distributed systems, use the bus-backed publisher:
//
//	nbus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
//	pub := results.NewBusPublisher(nbus, results.BusPublisherConfig{
//	    SubjectPrefix: "results",
//	})
//
//	// Results are stored in memory but notifications go over the bus
//	pub.Publish(ctx, "task-123", result)
//
//	// Remote subscribers receive notifications via bus
//	ch, _ := pub.Subscribe("task-123")
//
// The bus publisher enables agents to be notified of result availability without
// polling, supporting efficient distributed task coordination.
package results
