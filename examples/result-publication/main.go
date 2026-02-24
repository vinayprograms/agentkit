// Package main demonstrates the results package for task result publication.
//
// This example shows the core workflow:
//   - Creating publishers (MemoryPublisher and BusPublisher)
//   - Publishing results with different statuses
//   - Subscribing to result updates
//   - Filtering and listing results
//
// Run: go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vinayprograms/agentkit/bus"
	"github.com/vinayprograms/agentkit/results"
)

func main() {
	fmt.Println("=== Result Publication Example ===")

	// Part 1: MemoryPublisher - simple in-process usage
	fmt.Println("--- Part 1: MemoryPublisher ---")
	demonstrateMemoryPublisher()

	// Part 2: BusPublisher - distributed notifications
	fmt.Println("\n--- Part 2: BusPublisher ---")
	demonstrateBusPublisher()

	// Part 3: Filtering and listing results
	fmt.Println("\n--- Part 3: Filtering Results ---")
	demonstrateFiltering()

	fmt.Println("\n=== Example Complete ===")
}

// demonstrateMemoryPublisher shows basic usage with the in-memory publisher.
func demonstrateMemoryPublisher() {
	ctx := context.Background()

	// Create a memory publisher - ideal for testing and single-process scenarios
	pub := results.NewMemoryPublisher()
	defer pub.Close()

	// Publish a pending result (task started)
	taskID := "task-001"
	err := pub.Publish(ctx, taskID, results.Result{
		TaskID: taskID,
		Status: results.StatusPending,
		Metadata: map[string]string{
			"type":   "compute",
			"worker": "agent-1",
		},
	})
	if err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}
	fmt.Printf("Published pending result for %s\n", taskID)

	// Retrieve the result
	result, err := pub.Get(ctx, taskID)
	if err != nil {
		log.Fatalf("Failed to get result: %v", err)
	}
	fmt.Printf("Retrieved: status=%s, worker=%s\n", result.Status, result.Metadata["worker"])

	// Update with success (task completed)
	err = pub.Publish(ctx, taskID, results.Result{
		TaskID: taskID,
		Status: results.StatusSuccess,
		Output: []byte(`{"answer": 42}`),
		Metadata: map[string]string{
			"type":     "compute",
			"worker":   "agent-1",
			"duration": "150ms",
		},
	})
	if err != nil {
		log.Fatalf("Failed to update result: %v", err)
	}
	fmt.Printf("Updated to success with output: %s\n", string(result.Output))

	// Verify the update
	result, _ = pub.Get(ctx, taskID)
	fmt.Printf("Final status: %s, output: %s\n", result.Status, string(result.Output))
}

// demonstrateBusPublisher shows distributed result publication.
func demonstrateBusPublisher() {
	ctx := context.Background()

	// Create an in-memory bus (for demo - use NATS in production)
	memBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer memBus.Close()

	// Create a bus-backed publisher
	pub := results.NewBusPublisher(memBus, results.BusPublisherConfig{
		SubjectPrefix: "results",
		BufferSize:    16,
	})
	defer pub.Close()

	taskID := "task-002"

	// Set up a subscriber BEFORE publishing
	// This simulates a coordinator waiting for task completion
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		ch, err := pub.Subscribe(taskID)
		if err != nil {
			log.Printf("Subscribe failed: %v", err)
			return
		}

		fmt.Println("Subscriber waiting for updates...")

		for result := range ch {
			fmt.Printf("  Subscriber received: status=%s\n", result.Status)
			if result.Status.IsTerminal() {
				fmt.Printf("  Task completed with output: %s\n", string(result.Output))
				return
			}
		}
	}()

	// Give subscriber time to set up
	time.Sleep(50 * time.Millisecond)

	// Simulate a worker publishing progress
	fmt.Println("Worker publishing pending...")
	pub.Publish(ctx, taskID, results.Result{
		TaskID: taskID,
		Status: results.StatusPending,
		Metadata: map[string]string{
			"progress": "10%",
		},
	})

	time.Sleep(100 * time.Millisecond)

	// Simulate task completion
	fmt.Println("Worker publishing success...")
	pub.Publish(ctx, taskID, results.Result{
		TaskID: taskID,
		Status: results.StatusSuccess,
		Output: []byte(`{"result": "computed value"}`),
	})

	// Wait for subscriber to process
	wg.Wait()
}

// demonstrateFiltering shows how to filter and list results.
func demonstrateFiltering() {
	ctx := context.Background()

	pub := results.NewMemoryPublisher()
	defer pub.Close()

	// Create a batch of results with different statuses and metadata
	tasks := []struct {
		id       string
		status   results.ResultStatus
		taskType string
		output   string
	}{
		{"batch-001", results.StatusSuccess, "compute", `{"value": 1}`},
		{"batch-002", results.StatusSuccess, "fetch", `{"url": "fetched"}`},
		{"batch-003", results.StatusFailed, "compute", ""},
		{"batch-004", results.StatusPending, "process", ""},
		{"batch-005", results.StatusSuccess, "compute", `{"value": 5}`},
	}

	for _, t := range tasks {
		r := results.Result{
			TaskID: t.id,
			Status: t.status,
			Metadata: map[string]string{
				"type": t.taskType,
			},
		}
		if t.output != "" {
			r.Output = []byte(t.output)
		}
		if t.status == results.StatusFailed {
			r.Error = "simulated failure"
		}
		pub.Publish(ctx, t.id, r)
	}
	fmt.Printf("Published %d results\n", len(tasks))

	// Filter by status
	fmt.Println("\nSuccessful results:")
	successful, _ := pub.List(results.ResultFilter{
		Status: results.StatusSuccess,
	})
	for _, r := range successful {
		fmt.Printf("  - %s (type=%s)\n", r.TaskID, r.Metadata["type"])
	}

	// Filter by task ID prefix
	fmt.Println("\nResults starting with 'batch-00':")
	prefixed, _ := pub.List(results.ResultFilter{
		TaskIDPrefix: "batch-00",
		Limit:        3, // Only first 3
	})
	for _, r := range prefixed {
		fmt.Printf("  - %s: %s\n", r.TaskID, r.Status)
	}

	// Filter by metadata
	fmt.Println("\nCompute tasks:")
	computeTasks, _ := pub.List(results.ResultFilter{
		Metadata: map[string]string{
			"type": "compute",
		},
	})
	for _, r := range computeTasks {
		fmt.Printf("  - %s: %s\n", r.TaskID, r.Status)
	}

	// Combined filter: successful compute tasks
	fmt.Println("\nSuccessful compute tasks:")
	successfulCompute, _ := pub.List(results.ResultFilter{
		Status: results.StatusSuccess,
		Metadata: map[string]string{
			"type": "compute",
		},
	})
	for _, r := range successfulCompute {
		fmt.Printf("  - %s: output=%s\n", r.TaskID, string(r.Output))
	}

	// Delete a result
	fmt.Println("\nDeleting batch-003...")
	if err := pub.Delete(ctx, "batch-003"); err != nil {
		fmt.Printf("Delete failed: %v\n", err)
	} else {
		fmt.Println("Deleted successfully")
	}

	// Verify deletion
	_, err := pub.Get(ctx, "batch-003")
	if err == results.ErrNotFound {
		fmt.Println("Confirmed: batch-003 no longer exists")
	}
}
