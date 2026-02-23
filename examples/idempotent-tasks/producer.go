// Package main demonstrates submitting idempotent tasks using agentkit's tasks package.
//
// The producer submits tasks with idempotency keys to prevent duplicate processing.
// Multiple submissions with the same key return the existing task ID.
//
// Run: go build producer.go && ./producer
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/vinayprograms/agentkit/state"
	"github.com/vinayprograms/agentkit/tasks"
)

// TaskPayload represents the work to be done.
type TaskPayload struct {
	Operation string      `json:"operation"`
	Data      interface{} `json:"data"`
}

func main() {
	// Create a shared state store (in production, use NATS KV for distribution)
	store := state.NewMemoryStore()
	defer store.Close()

	// Create task manager
	manager := tasks.NewManager(store)
	defer manager.Close()

	ctx := context.Background()

	fmt.Println("=== Idempotent Task Producer ===")
	fmt.Println()

	// --- Demo 1: Basic Task Submission ---
	fmt.Println("--- Demo 1: Basic Task Submission ---")
	payload1, _ := json.Marshal(TaskPayload{
		Operation: "process_order",
		Data:      map[string]interface{}{"order_id": 12345, "amount": 99.99},
	})

	taskID1, err := manager.Submit(ctx, tasks.Task{
		IdempotencyKey: "order-12345",
		Payload:        payload1,
		MaxAttempts:    3,
	})
	if err != nil {
		log.Fatalf("Failed to submit task: %v", err)
	}
	fmt.Printf("✓ Submitted task: %s (key: order-12345)\n", taskID1)

	// --- Demo 2: Idempotent Deduplication ---
	fmt.Println("\n--- Demo 2: Idempotent Deduplication ---")
	fmt.Println("Attempting to submit same order again (should dedupe)...")

	taskID2, err := manager.Submit(ctx, tasks.Task{
		IdempotencyKey: "order-12345", // Same key as before
		Payload:        payload1,
		MaxAttempts:    3,
	})
	if err != nil {
		log.Fatalf("Failed to submit task: %v", err)
	}

	if taskID1 == taskID2 {
		fmt.Printf("✓ Deduplicated! Returned existing task: %s\n", taskID2)
	} else {
		fmt.Printf("✗ Unexpected: Got different task ID: %s\n", taskID2)
	}

	// --- Demo 3: Multiple Unique Tasks ---
	fmt.Println("\n--- Demo 3: Submitting Multiple Unique Tasks ---")
	orders := []struct {
		orderID int
		amount  float64
	}{
		{12346, 49.99},
		{12347, 199.99},
		{12348, 29.99},
	}

	for _, order := range orders {
		payload, _ := json.Marshal(TaskPayload{
			Operation: "process_order",
			Data:      map[string]interface{}{"order_id": order.orderID, "amount": order.amount},
		})

		taskID, err := manager.Submit(ctx, tasks.Task{
			IdempotencyKey: fmt.Sprintf("order-%d", order.orderID),
			Payload:        payload,
			MaxAttempts:    3,
		})
		if err != nil {
			log.Printf("Failed to submit order %d: %v", order.orderID, err)
			continue
		}
		fmt.Printf("✓ Submitted: %s (order-%d, $%.2f)\n", taskID, order.orderID, order.amount)
	}

	// --- Demo 4: Task Without Idempotency Key ---
	fmt.Println("\n--- Demo 4: Task Without Idempotency Key ---")
	fmt.Println("Tasks without idempotency keys are always created as new...")

	for i := 1; i <= 3; i++ {
		payload, _ := json.Marshal(TaskPayload{
			Operation: "send_notification",
			Data:      map[string]interface{}{"message": fmt.Sprintf("Notification %d", i)},
		})

		taskID, err := manager.Submit(ctx, tasks.Task{
			// No IdempotencyKey - each submission creates a new task
			Payload:     payload,
			MaxAttempts: 1,
		})
		if err != nil {
			log.Printf("Failed to submit notification: %v", err)
			continue
		}
		fmt.Printf("✓ Created new task: %s\n", taskID)
	}

	// --- Demo 5: Lookup by Idempotency Key ---
	fmt.Println("\n--- Demo 5: Lookup by Idempotency Key ---")
	task, err := manager.GetByIdempotencyKey(ctx, "order-12345")
	if err != nil {
		log.Printf("Failed to lookup: %v", err)
	} else {
		fmt.Printf("✓ Found task by key 'order-12345': ID=%s, Status=%s\n", task.ID, task.Status)
	}

	// --- Demo 6: List Pending Tasks ---
	fmt.Println("\n--- Demo 6: List All Pending Tasks ---")
	pendingTasks, err := manager.List(ctx, tasks.StatusPending)
	if err != nil {
		log.Printf("Failed to list tasks: %v", err)
	} else {
		fmt.Printf("Found %d pending tasks:\n", len(pendingTasks))
		for _, t := range pendingTasks {
			var p TaskPayload
			json.Unmarshal(t.Payload, &p)
			fmt.Printf("  - %s: %s (attempts: %d/%d)\n",
				t.ID, p.Operation, t.Attempts, t.MaxAttempts)
		}
	}

	// --- Summary ---
	fmt.Println("\n=== Summary ===")
	fmt.Println("Key benefits of the tasks package over bus-based task-queue:")
	fmt.Println("  1. Idempotency: Duplicate submissions return existing task ID")
	fmt.Println("  2. State tracking: Task status persists across restarts")
	fmt.Println("  3. Claim semantics: Workers explicitly claim tasks")
	fmt.Println("  4. Retry support: Failed tasks automatically retry up to MaxAttempts")
	fmt.Println("  5. Result storage: Completed task results are stored for retrieval")
	fmt.Println()
	fmt.Printf("Total tasks in system: %d\n", len(pendingTasks)+1) // +1 for dedupe demo

	// Give workers time to pick up tasks (in real usage)
	fmt.Println("\nTasks are now ready for workers to claim and process.")
	fmt.Println("Run the worker to process these tasks.")

	// Keep manager open briefly to simulate waiting
	time.Sleep(100 * time.Millisecond)
}
