// Package tasks provides idempotent task handling with deduplication.
//
// Tasks enables safe retry semantics for distributed agent workloads.
// Key features:
//
//   - Idempotency keys prevent duplicate task creation
//   - Claim-based task ownership with worker tracking
//   - Configurable retry limits and attempt counting
//   - State-backed persistence for durability
//
// # Basic Usage
//
// Create a task manager backed by a state store:
//
//	store := state.NewMemoryStore()
//	mgr := tasks.NewManager(store)
//
//	// Submit a task with an idempotency key
//	task := tasks.Task{
//	    IdempotencyKey: "order-123-process",
//	    Payload:        []byte(`{"order_id": "123"}`),
//	    MaxAttempts:    3,
//	}
//	taskID, err := mgr.Submit(ctx, task)
//
//	// Worker claims and processes the task
//	err = mgr.Claim(ctx, taskID, "worker-1")
//	// ... do work ...
//	err = mgr.Complete(ctx, taskID, []byte("result"))
//
// # Idempotency
//
// The idempotency key ensures duplicate submissions return the existing task:
//
//	id1, _ := mgr.Submit(ctx, Task{IdempotencyKey: "unique-key", Payload: []byte("data")})
//	id2, _ := mgr.Submit(ctx, Task{IdempotencyKey: "unique-key", Payload: []byte("data")})
//	// id1 == id2, no duplicate task created
//
// This is essential for retry-safe operations where a task submission
// might be retried due to network failures.
//
// # Task Lifecycle
//
// Tasks move through the following states:
//
//	Pending → Claimed → Completed
//	              ↓
//	           Failed ↔ Pending (via Retry)
//
// A task can be retried up to MaxAttempts times before being
// permanently marked as failed.
//
// # Thread Safety
//
// All TaskManager implementations are safe for concurrent use.
// The underlying state store handles synchronization.
package tasks
