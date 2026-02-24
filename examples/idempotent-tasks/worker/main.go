// Package main demonstrates a task worker using agentkit's tasks package.
//
// Workers claim pending tasks, process them, and mark them complete or failed.
// The tasks package provides proper state management with retry support.
//
// Run: go build worker.go && ./worker [worker-id]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vinayprograms/agentkit/state"
	"github.com/vinayprograms/agentkit/tasks"
)

// TaskPayload represents the work to be done.
type TaskPayload struct {
	Operation string      `json:"operation"`
	Data      interface{} `json:"data"`
}

// TaskResult is the output of task processing.
type TaskResult struct {
	ProcessedBy string    `json:"processed_by"`
	ProcessedAt time.Time `json:"processed_at"`
	Output      string    `json:"output"`
}

// Worker processes tasks from the task manager.
type Worker struct {
	id      string
	manager tasks.TaskManager

	// Stats
	processed int
	failed    int
	retried   int
}

func main() {
	// Get worker ID from args
	workerID := "worker"
	if len(os.Args) > 1 {
		workerID = os.Args[1]
	} else {
		workerID = fmt.Sprintf("worker-%d", time.Now().UnixNano()%10000)
	}

	// Create a shared state store
	// NOTE: In production, use NATS KV for distributed state:
	//   natsStore, _ := state.NewNATSStore(nc, state.NATSConfig{Bucket: "tasks"})
	store := state.NewMemoryStore()
	defer store.Close()

	// Create task manager
	manager := tasks.NewManager(store)
	defer manager.Close()

	worker := &Worker{
		id:      workerID,
		manager: manager,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("[%s] Task Worker started", workerID)
	log.Println("Processing loop active - waiting for tasks...")

	// Start processing loop
	go worker.processLoop(ctx)

	// Wait for shutdown
	<-sigCh
	log.Printf("[%s] Shutting down...", workerID)
	cancel()

	// Print stats
	log.Printf("[%s] Stats: processed=%d, failed=%d, retried=%d",
		workerID, worker.processed, worker.failed, worker.retried)
}

// processLoop continuously claims and processes pending tasks.
func (w *Worker) processLoop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.claimAndProcess(ctx)
		}
	}
}

// claimAndProcess finds a pending task, claims it, and processes it.
func (w *Worker) claimAndProcess(ctx context.Context) {
	// List pending tasks
	pendingTasks, err := w.manager.List(ctx, tasks.StatusPending)
	if err != nil {
		log.Printf("[%s] Failed to list tasks: %v", w.id, err)
		return
	}

	if len(pendingTasks) == 0 {
		return // No work available
	}

	// Try to claim the first available task
	for _, task := range pendingTasks {
		// Attempt to claim
		err := w.manager.Claim(ctx, task.ID, w.id)
		if err != nil {
			if errors.Is(err, tasks.ErrTaskAlreadyClaimed) {
				// Another worker got it first, try next
				continue
			}
			log.Printf("[%s] Failed to claim %s: %v", w.id, task.ID, err)
			continue
		}

		// Successfully claimed - process it
		w.processTask(ctx, task)
		return // Process one task per iteration
	}
}

// processTask handles a claimed task.
func (w *Worker) processTask(ctx context.Context, task *tasks.Task) {
	var payload TaskPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		log.Printf("[%s] Invalid payload for %s: %v", w.id, task.ID, err)
		w.failTask(ctx, task.ID, fmt.Errorf("invalid payload: %v", err))
		return
	}

	log.Printf("[%s] Processing: %s (%s) - attempt %d/%d",
		w.id, task.ID, payload.Operation, task.Attempts+1, task.MaxAttempts)

	// Execute the task
	result, err := w.executeTask(ctx, &payload)
	if err != nil {
		w.failTask(ctx, task.ID, err)
		return
	}

	// Complete successfully
	w.completeTask(ctx, task.ID, result)
}

// executeTask performs the actual work (simulated).
func (w *Worker) executeTask(ctx context.Context, payload *TaskPayload) (*TaskResult, error) {
	// Simulate processing time (100-400ms)
	processingTime := time.Duration(100+rand.Intn(300)) * time.Millisecond
	time.Sleep(processingTime)

	// Simulate random failures (20% chance)
	if rand.Float64() < 0.2 {
		return nil, fmt.Errorf("processing failed: temporary error")
	}

	// Build result based on operation
	result := &TaskResult{
		ProcessedBy: w.id,
		ProcessedAt: time.Now(),
	}

	switch payload.Operation {
	case "process_order":
		data, _ := payload.Data.(map[string]interface{})
		orderID := data["order_id"]
		amount := data["amount"]
		result.Output = fmt.Sprintf("Order %v processed, charged $%.2f", orderID, amount)

	case "send_notification":
		data, _ := payload.Data.(map[string]interface{})
		message := data["message"]
		result.Output = fmt.Sprintf("Notification sent: %v", message)

	default:
		result.Output = fmt.Sprintf("Executed operation: %s", payload.Operation)
	}

	return result, nil
}

// completeTask marks a task as successfully completed.
func (w *Worker) completeTask(ctx context.Context, taskID string, result *TaskResult) {
	resultData, _ := json.Marshal(result)

	err := w.manager.Complete(ctx, taskID, resultData)
	if err != nil {
		log.Printf("[%s] Failed to complete %s: %v", w.id, taskID, err)
		return
	}

	w.processed++
	log.Printf("[%s] ✓ Completed: %s - %s", w.id, taskID, result.Output)
}

// failTask handles task failure with potential retry.
func (w *Worker) failTask(ctx context.Context, taskID string, taskErr error) {
	err := w.manager.Fail(ctx, taskID, taskErr)
	if err != nil {
		log.Printf("[%s] Failed to mark %s as failed: %v", w.id, taskID, err)
		return
	}

	// Check if the task was retried or permanently failed
	task, getErr := w.manager.Get(ctx, taskID)
	if getErr != nil {
		log.Printf("[%s] ✗ Failed: %s - %v", w.id, taskID, taskErr)
		w.failed++
		return
	}

	switch task.Status {
	case tasks.StatusPending:
		// Task moved back to pending for retry
		w.retried++
		log.Printf("[%s] ↻ Retry queued: %s - %v (attempt %d/%d)",
			w.id, taskID, taskErr, task.Attempts, task.MaxAttempts)

	case tasks.StatusFailed:
		// Task permanently failed (max attempts reached)
		w.failed++
		log.Printf("[%s] ✗ Permanently failed: %s - %v (exhausted %d attempts)",
			w.id, taskID, taskErr, task.Attempts)

	default:
		log.Printf("[%s] ✗ Failed: %s - %v (status: %s)",
			w.id, taskID, taskErr, task.Status)
		w.failed++
	}
}
