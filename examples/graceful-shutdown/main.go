// Package main demonstrates graceful shutdown coordination using agentkit's shutdown package.
//
// This example shows a real-world pattern with:
// - HTTP server (accepts requests)
// - Worker pool (processes background jobs)
// - Resource cleanup (database connections, temp files)
// - Phase-ordered shutdown ensuring proper dependency cleanup
//
// Run: go run main.go
// Stop: Ctrl+C (SIGINT) or kill (SIGTERM)
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinayprograms/agentkit/shutdown"
)

// Phase constants for shutdown ordering.
// Lower phases shut down first.
const (
	PhaseFrontend = 10 // HTTP server - stop accepting requests first
	PhaseWorkers  = 20 // Worker pool - drain pending jobs
	PhaseBackend  = 30 // Cleanup - close DB connections, temp files
)

func main() {
	fmt.Println("=== Graceful Shutdown Example ===")
	fmt.Println()

	// Create shutdown coordinator with progress reporting
	coord := shutdown.NewCoordinator(shutdown.Config{
		DefaultTimeout:  15 * time.Second,
		ContinueOnError: true,
		OnProgress: func(result shutdown.HandlerResult) {
			status := "✓"
			if result.Err != nil {
				status = "✗"
			}
			log.Printf("  %s [Phase %d] %s shutdown complete (%v)",
				status, result.Phase, result.Name, result.Duration.Round(time.Millisecond))
		},
	})

	// Create application components
	server := newHTTPServer(":8080")
	workers := newWorkerPool(4)
	cleanup := newCleanupHandler()

	// Register shutdown handlers with phases
	// Order: server → workers → cleanup
	coord.RegisterWithPhase("http-server", server, PhaseFrontend)
	coord.RegisterWithPhase("worker-pool", workers, PhaseWorkers)
	coord.RegisterWithPhase("cleanup", cleanup, PhaseBackend)

	// Start components
	go server.Start()
	go workers.Start()

	// Enable signal handling (SIGINT, SIGTERM)
	coord.HandleSignals()

	fmt.Println("Application started!")
	fmt.Printf("  HTTP Server: http://localhost%s\n", server.addr)
	fmt.Printf("  Workers: %d running\n", workers.size)
	fmt.Println()
	fmt.Println("Press Ctrl+C to trigger graceful shutdown...")
	fmt.Println()

	// Simulate some work
	go simulateTraffic(server, workers)

	// Wait for shutdown to complete
	<-coord.Done()

	// Print final results
	fmt.Println()
	fmt.Println("=== Shutdown Complete ===")

	result := coord.Result()
	fmt.Printf("Total duration: %v\n", result.TotalDuration.Round(time.Millisecond))

	if result.Err != nil {
		fmt.Printf("Errors occurred: %v\n", result.Err)
		for _, r := range result.Results {
			if r.Err != nil {
				fmt.Printf("  - %s: %v\n", r.Name, r.Err)
			}
		}
	} else {
		fmt.Println("All handlers completed successfully!")
	}
}

// =============================================================================
// HTTP Server
// =============================================================================

type httpServer struct {
	addr       string
	server     *http.Server
	requests   atomic.Int64
	inFlight   atomic.Int64
	shutdownWg sync.WaitGroup
}

func newHTTPServer(addr string) *httpServer {
	s := &httpServer{addr: addr}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s
}

func (s *httpServer) Start() {
	log.Printf("[server] Starting on %s", s.addr)
	if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("[server] Error: %v", err)
	}
}

func (s *httpServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	s.requests.Add(1)

	// Simulate request processing (50-200ms)
	time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

	fmt.Fprintf(w, "Request #%d processed\n", s.requests.Load())
}

func (s *httpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK - Requests: %d, InFlight: %d\n",
		s.requests.Load(), s.inFlight.Load())
}

// OnShutdown implements shutdown.ShutdownHandler.
func (s *httpServer) OnShutdown(ctx context.Context) error {
	log.Printf("[server] Shutting down (in-flight: %d)...", s.inFlight.Load())

	// Gracefully shutdown HTTP server (waits for in-flight requests)
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}

	log.Printf("[server] Stopped. Total requests served: %d", s.requests.Load())
	return nil
}

// =============================================================================
// Worker Pool
// =============================================================================

type job struct {
	id   int
	work func()
}

type workerPool struct {
	size       int
	jobs       chan job
	wg         sync.WaitGroup
	running    atomic.Bool
	processed  atomic.Int64
	pending    atomic.Int64
	cancelFunc context.CancelFunc
}

func newWorkerPool(size int) *workerPool {
	return &workerPool{
		size: size,
		jobs: make(chan job, 100), // Buffered job queue
	}
}

func (p *workerPool) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel
	p.running.Store(true)

	log.Printf("[workers] Starting %d workers", p.size)

	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i+1)
	}
}

func (p *workerPool) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-p.jobs:
			if !ok {
				return
			}
			j.work()
			p.processed.Add(1)
			p.pending.Add(-1)
		}
	}
}

func (p *workerPool) Submit(work func()) {
	if !p.running.Load() {
		return
	}
	p.pending.Add(1)
	p.jobs <- job{work: work}
}

// OnShutdown implements shutdown.ShutdownHandler.
func (p *workerPool) OnShutdown(ctx context.Context) error {
	log.Printf("[workers] Shutting down (pending: %d)...", p.pending.Load())

	// Stop accepting new jobs
	p.running.Store(false)

	// Signal workers to drain and exit
	close(p.jobs)

	// Wait for workers to finish (with timeout)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[workers] Stopped. Total jobs processed: %d", p.processed.Load())
		return nil
	case <-ctx.Done():
		// Cancel workers if timeout reached
		if p.cancelFunc != nil {
			p.cancelFunc()
		}
		// Wait a bit more for cleanup
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
		return fmt.Errorf("worker shutdown timeout (pending: %d)", p.pending.Load())
	}
}

// =============================================================================
// Cleanup Handler (simulated database/resources)
// =============================================================================

type cleanupHandler struct {
	dbConnections int
	tempFiles     int
}

func newCleanupHandler() *cleanupHandler {
	return &cleanupHandler{
		dbConnections: 5,
		tempFiles:     10,
	}
}

// OnShutdown implements shutdown.ShutdownHandler.
func (c *cleanupHandler) OnShutdown(ctx context.Context) error {
	log.Printf("[cleanup] Releasing resources...")

	// Simulate closing database connections
	for i := 0; i < c.dbConnections; i++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cleanup interrupted: %d/%d connections closed",
				i, c.dbConnections)
		default:
			time.Sleep(50 * time.Millisecond) // Simulate close time
		}
	}
	log.Printf("[cleanup] Closed %d database connections", c.dbConnections)

	// Simulate cleaning up temp files
	for i := 0; i < c.tempFiles; i++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cleanup interrupted: %d/%d files removed",
				i, c.tempFiles)
		default:
			time.Sleep(20 * time.Millisecond) // Simulate file deletion
		}
	}
	log.Printf("[cleanup] Removed %d temporary files", c.tempFiles)

	return nil
}

// =============================================================================
// Traffic Simulation
// =============================================================================

func simulateTraffic(server *httpServer, workers *workerPool) {
	// Submit some background jobs
	for i := 0; i < 20; i++ {
		jobID := i + 1
		workers.Submit(func() {
			// Simulate job work (100-300ms)
			time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
			log.Printf("[job] Completed job #%d", jobID)
		})
		time.Sleep(200 * time.Millisecond)
	}
}
