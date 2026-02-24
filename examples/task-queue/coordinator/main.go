// Package main demonstrates a distributed task coordinator using agentkit's bus and registry.
//
// The coordinator publishes tasks to a message bus and tracks their completion.
// Workers claim and process tasks using queue subscriptions for load balancing.
//
// Run: go run coordinator.go
// In other terminals: go run worker.go worker-1
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/vinayprograms/agentkit/bus"
	"github.com/vinayprograms/agentkit/registry"
)

// Subjects for task communication
const (
	SubjectTaskQueue    = "tasks.queue"      // Where new tasks are published
	SubjectTaskComplete = "tasks.complete"   // Where workers report completion
	SubjectTaskFailed   = "tasks.failed"     // Where workers report failures
)

// Task represents a unit of work.
type Task struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Priority  int                    `json:"priority"`
	Payload   map[string]interface{} `json:"payload"`
	CreatedAt time.Time              `json:"created_at"`
}

// TaskResult is sent when a task completes.
type TaskResult struct {
	TaskID   string      `json:"task_id"`
	WorkerID string      `json:"worker_id"`
	Success  bool        `json:"success"`
	Result   interface{} `json:"result,omitempty"`
	Error    string      `json:"error,omitempty"`
	Duration string      `json:"duration"`
}

// Coordinator manages task distribution and tracking.
type Coordinator struct {
	bus      bus.MessageBus
	registry registry.Registry

	mu       sync.RWMutex
	pending  map[string]*Task       // Tasks not yet completed
	results  map[string]*TaskResult // Completed task results

	taskCount int
}

func main() {
	// Create in-memory message bus
	memBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer memBus.Close()

	// Create in-memory registry with 30s TTL
	reg := registry.NewMemoryRegistry(registry.MemoryConfig{TTL: 30 * time.Second})
	defer reg.Close()

	coord := &Coordinator{
		bus:      memBus,
		registry: reg,
		pending:  make(map[string]*Task),
		results:  make(map[string]*TaskResult),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start coordinator components
	go coord.watchWorkers(ctx)
	go coord.handleResults(ctx)

	log.Println("Task Coordinator started")
	log.Println("Waiting for workers to connect...")

	// For demo: also run embedded workers in this process
	// In production, workers would be separate processes
	go runEmbeddedWorker(ctx, memBus, reg, "worker-1")
	go runEmbeddedWorker(ctx, memBus, reg, "worker-2")

	// Wait a moment for workers to register
	time.Sleep(500 * time.Millisecond)

	// Submit sample tasks
	coord.submitSampleTasks()

	// Main loop - print status periodically
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			log.Println("Shutting down...")
			cancel()
			time.Sleep(500 * time.Millisecond)
			coord.printFinalStats()
			return
		case <-ticker.C:
			coord.printStatus()
		}
	}
}

// watchWorkers monitors worker registrations.
func (c *Coordinator) watchWorkers(ctx context.Context) {
	events, err := c.registry.Watch()
	if err != nil {
		log.Printf("Failed to watch registry: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			switch event.Type {
			case registry.EventAdded:
				log.Printf("Worker joined: %s (capabilities: %v)", event.Agent.Name, event.Agent.Capabilities)
			case registry.EventRemoved:
				log.Printf("Worker left: %s", event.Agent.Name)
			}
		}
	}
}

// handleResults processes task completion notifications.
func (c *Coordinator) handleResults(ctx context.Context) {
	// Subscribe to completion channel
	completeSub, err := c.bus.Subscribe(SubjectTaskComplete)
	if err != nil {
		log.Printf("Failed to subscribe to completions: %v", err)
		return
	}

	failedSub, err := c.bus.Subscribe(SubjectTaskFailed)
	if err != nil {
		log.Printf("Failed to subscribe to failures: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			completeSub.Unsubscribe()
			failedSub.Unsubscribe()
			return
		case msg := <-completeSub.Messages():
			c.processResult(msg)
		case msg := <-failedSub.Messages():
			c.processResult(msg)
		}
	}
}

// processResult handles a task completion message.
func (c *Coordinator) processResult(msg *bus.Message) {
	var result TaskResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		log.Printf("Invalid result: %v", err)
		return
	}

	c.mu.Lock()
	delete(c.pending, result.TaskID)
	c.results[result.TaskID] = &result
	c.mu.Unlock()

	status := "✓"
	if !result.Success {
		status = "✗"
	}
	log.Printf("%s Task %s completed by %s (%s)", status, result.TaskID, result.WorkerID, result.Duration)
}

// submitTask publishes a task to the queue.
func (c *Coordinator) submitTask(task *Task) error {
	c.mu.Lock()
	c.taskCount++
	task.ID = fmt.Sprintf("task-%d", c.taskCount)
	task.CreatedAt = time.Now()
	c.pending[task.ID] = task
	c.mu.Unlock()

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	if err := c.bus.Publish(SubjectTaskQueue, data); err != nil {
		c.mu.Lock()
		delete(c.pending, task.ID)
		c.mu.Unlock()
		return err
	}

	log.Printf("Submitted: %s (%s)", task.ID, task.Type)
	return nil
}

// submitSampleTasks creates demo tasks.
func (c *Coordinator) submitSampleTasks() {
	log.Println("\n--- Submitting sample tasks ---")

	tasks := []Task{
		{Type: "compute", Priority: 1, Payload: map[string]interface{}{"operation": "fibonacci", "n": 10}},
		{Type: "compute", Priority: 2, Payload: map[string]interface{}{"operation": "factorial", "n": 5}},
		{Type: "fetch", Priority: 1, Payload: map[string]interface{}{"url": "https://example.com"}},
		{Type: "process", Priority: 3, Payload: map[string]interface{}{"data": "transform-me"}},
		{Type: "compute", Priority: 1, Payload: map[string]interface{}{"operation": "prime", "n": 100}},
	}

	for i := range tasks {
		if err := c.submitTask(&tasks[i]); err != nil {
			log.Printf("Failed to submit task: %v", err)
		}
		time.Sleep(100 * time.Millisecond) // Stagger submissions
	}
}

// printStatus shows current state.
func (c *Coordinator) printStatus() {
	c.mu.RLock()
	pending := len(c.pending)
	completed := len(c.results)
	c.mu.RUnlock()

	workers, _ := c.registry.List(nil)

	log.Printf("Status: %d pending, %d completed, %d workers active", pending, completed, len(workers))
}

// printFinalStats shows completion summary.
func (c *Coordinator) printFinalStats() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fmt.Println("\n=== Final Statistics ===")
	fmt.Printf("Tasks submitted: %d\n", c.taskCount)
	fmt.Printf("Tasks completed: %d\n", len(c.results))
	fmt.Printf("Tasks pending:   %d\n", len(c.pending))

	successCount := 0
	for _, r := range c.results {
		if r.Success {
			successCount++
		}
	}
	fmt.Printf("Success rate:    %.1f%%\n", float64(successCount)/float64(len(c.results))*100)
}

// --- Embedded Worker (for demo) ---
// In production, use worker.go as a separate process

func runEmbeddedWorker(ctx context.Context, b bus.MessageBus, reg registry.Registry, workerID string) {
	// Register with the registry
	info := registry.AgentInfo{
		ID:           workerID,
		Name:         workerID,
		Capabilities: []string{"compute", "fetch", "process"},
		Status:       registry.StatusIdle,
		Load:         0,
		Metadata:     map[string]string{"started": time.Now().Format(time.RFC3339)},
	}
	if err := reg.Register(info); err != nil {
		log.Printf("[%s] Failed to register: %v", workerID, err)
		return
	}

	// Queue subscribe for load balancing
	sub, err := b.QueueSubscribe(SubjectTaskQueue, "workers")
	if err != nil {
		log.Printf("[%s] Failed to subscribe: %v", workerID, err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			reg.Deregister(workerID)
			return
		case msg := <-sub.Messages():
			processTask(b, reg, workerID, msg)
		}
	}
}

func processTask(b bus.MessageBus, reg registry.Registry, workerID string, msg *bus.Message) {
	var task Task
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		return
	}

	// Update status to busy
	info, _ := reg.Get(workerID)
	if info != nil {
		info.Status = registry.StatusBusy
		info.Load = 0.5 + rand.Float64()*0.5
		reg.Register(*info)
	}

	start := time.Now()

	// Simulate work
	result := TaskResult{
		TaskID:   task.ID,
		WorkerID: workerID,
	}

	// Random processing time (100-500ms)
	time.Sleep(time.Duration(100+rand.Intn(400)) * time.Millisecond)

	// Random failure (10% chance)
	if rand.Float64() < 0.1 {
		result.Success = false
		result.Error = "simulated failure"
	} else {
		result.Success = true
		result.Result = fmt.Sprintf("processed %s:%v", task.Type, task.Payload)
	}

	result.Duration = time.Since(start).String()

	// Report result
	data, _ := json.Marshal(result)
	subject := SubjectTaskComplete
	if !result.Success {
		subject = SubjectTaskFailed
	}
	b.Publish(subject, data)

	// Update status to idle
	if info != nil {
		info.Status = registry.StatusIdle
		info.Load = 0
		reg.Register(*info)
	}
}
