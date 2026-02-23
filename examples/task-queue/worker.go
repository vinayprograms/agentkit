// Package main demonstrates a task worker using agentkit's bus and registry.
//
// Workers claim tasks from a queue, process them, and report results.
// Multiple workers can run concurrently with automatic load balancing.
//
// Run: go run worker.go worker-1
// Requires coordinator or shared message bus
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vinayprograms/agentkit/bus"
	"github.com/vinayprograms/agentkit/registry"
)

// Subjects for task communication
const (
	SubjectTaskQueue    = "tasks.queue"
	SubjectTaskComplete = "tasks.complete"
	SubjectTaskFailed   = "tasks.failed"
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

// Worker processes tasks from the queue.
type Worker struct {
	id       string
	bus      bus.MessageBus
	registry registry.Registry

	tasksProcessed int
	tasksFailed    int
}

func main() {
	// Get worker ID from args
	workerID := "worker"
	if len(os.Args) > 1 {
		workerID = os.Args[1]
	} else {
		workerID = fmt.Sprintf("worker-%d", time.Now().UnixNano()%10000)
	}

	// Create in-memory message bus
	// NOTE: For distributed workers, use NATS:
	//   natsBus, _ := bus.NewNATSBus("nats://localhost:4222", bus.DefaultConfig())
	memBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer memBus.Close()

	// Create in-memory registry
	// NOTE: For distributed workers, use NATS registry:
	//   natsReg, _ := registry.NewNATSRegistry(nc, registry.NATSConfig{...})
	reg := registry.NewMemoryRegistry(registry.MemoryConfig{TTL: 30 * time.Second})
	defer reg.Close()

	worker := &Worker{
		id:       workerID,
		bus:      memBus,
		registry: reg,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Register with the swarm
	if err := worker.register(); err != nil {
		log.Fatalf("Failed to register: %v", err)
	}

	log.Printf("[%s] Worker started", workerID)
	log.Printf("[%s] Capabilities: compute, fetch, process", workerID)
	log.Println("Waiting for tasks...")

	// Start heartbeat to keep registration fresh
	go worker.heartbeat(ctx)

	// Start processing tasks
	go worker.processLoop(ctx)

	// Wait for shutdown signal
	<-sigCh
	log.Printf("[%s] Shutting down...", workerID)

	// Deregister from swarm
	worker.registry.Deregister(workerID)

	log.Printf("[%s] Processed: %d, Failed: %d", workerID, worker.tasksProcessed, worker.tasksFailed)
}

// register adds this worker to the registry.
func (w *Worker) register() error {
	info := registry.AgentInfo{
		ID:           w.id,
		Name:         w.id,
		Capabilities: []string{"compute", "fetch", "process"},
		Status:       registry.StatusIdle,
		Load:         0,
		Metadata: map[string]string{
			"started": time.Now().Format(time.RFC3339),
			"version": "1.0",
		},
	}
	return w.registry.Register(info)
}

// heartbeat periodically updates the registry entry.
func (w *Worker) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := w.registry.Get(w.id)
			if err != nil {
				w.register() // Re-register if entry expired
			} else {
				w.registry.Register(*info) // Refresh LastSeen
			}
		}
	}
}

// processLoop subscribes to the task queue and processes tasks.
func (w *Worker) processLoop(ctx context.Context) {
	// Use queue subscription for load balancing
	// All workers in the same queue group share the workload
	sub, err := w.bus.QueueSubscribe(SubjectTaskQueue, "workers")
	if err != nil {
		log.Printf("[%s] Failed to subscribe: %v", w.id, err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Messages():
			if !ok {
				return
			}
			w.processTask(msg)
		}
	}
}

// processTask handles a single task.
func (w *Worker) processTask(msg *bus.Message) {
	var task Task
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		log.Printf("[%s] Invalid task: %v", w.id, err)
		return
	}

	log.Printf("[%s] Processing: %s (%s)", w.id, task.ID, task.Type)

	// Update status to busy
	w.updateStatus(registry.StatusBusy, 0.8)

	start := time.Now()
	result := w.executeTask(&task)
	result.Duration = time.Since(start).String()

	// Update counters
	if result.Success {
		w.tasksProcessed++
	} else {
		w.tasksFailed++
	}

	// Report result
	w.reportResult(&result)

	// Update status to idle
	w.updateStatus(registry.StatusIdle, 0)

	log.Printf("[%s] Completed: %s (success=%v, %s)", w.id, task.ID, result.Success, result.Duration)
}

// executeTask performs the actual work (simulated).
func (w *Worker) executeTask(task *Task) TaskResult {
	result := TaskResult{
		TaskID:   task.ID,
		WorkerID: w.id,
	}

	// Simulate different work based on task type
	switch task.Type {
	case "compute":
		result = w.doCompute(task, result)
	case "fetch":
		result = w.doFetch(task, result)
	case "process":
		result = w.doProcess(task, result)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("unknown task type: %s", task.Type)
	}

	return result
}

// doCompute simulates computation tasks.
func (w *Worker) doCompute(task *Task, result TaskResult) TaskResult {
	operation, _ := task.Payload["operation"].(string)
	n, _ := task.Payload["n"].(float64)

	// Simulate CPU work
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)

	var output interface{}
	switch operation {
	case "fibonacci":
		output = fibonacci(int(n))
	case "factorial":
		output = factorial(int(n))
	case "prime":
		output = isPrime(int(n))
	default:
		output = fmt.Sprintf("computed with n=%v", n)
	}

	result.Success = true
	result.Result = output
	return result
}

// doFetch simulates network fetch tasks.
func (w *Worker) doFetch(task *Task, result TaskResult) TaskResult {
	url, _ := task.Payload["url"].(string)

	// Simulate network latency
	time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)

	// Random failure rate for network tasks
	if rand.Float64() < 0.15 {
		result.Success = false
		result.Error = "connection timeout"
		return result
	}

	result.Success = true
	result.Result = fmt.Sprintf("fetched %s (200 OK)", url)
	return result
}

// doProcess simulates data processing tasks.
func (w *Worker) doProcess(task *Task, result TaskResult) TaskResult {
	data, _ := task.Payload["data"].(string)

	// Simulate processing
	time.Sleep(time.Duration(150+rand.Intn(150)) * time.Millisecond)

	result.Success = true
	result.Result = fmt.Sprintf("transformed: %s -> processed_%s", data, data)
	return result
}

// reportResult publishes the task result.
func (w *Worker) reportResult(result *TaskResult) {
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("[%s] Failed to marshal result: %v", w.id, err)
		return
	}

	subject := SubjectTaskComplete
	if !result.Success {
		subject = SubjectTaskFailed
	}

	if err := w.bus.Publish(subject, data); err != nil {
		log.Printf("[%s] Failed to publish result: %v", w.id, err)
	}
}

// updateStatus updates the worker's registry entry.
func (w *Worker) updateStatus(status registry.Status, load float64) {
	info, err := w.registry.Get(w.id)
	if err != nil {
		return
	}
	info.Status = status
	info.Load = load
	w.registry.Register(*info)
}

// --- Helper functions for compute tasks ---

func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacci(n-1) + fibonacci(n-2)
}

func factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorial(n-1)
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	for i := 2; i*i <= n; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}
