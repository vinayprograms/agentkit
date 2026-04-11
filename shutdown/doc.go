// Package shutdown provides graceful shutdown coordination for distributed agents.
//
// # Overview
//
// The shutdown package enables agents to shut down gracefully, ensuring that
// in-progress tasks complete, pending work is re-queued, and the system
// remains consistent. It handles OS signals (SIGTERM, SIGINT) and provides
// ordered shutdown across multiple components.
//
// # Architecture
//
//	┌──────────────────────────────────────────────────────────────────┐
//	│                          Sequence                                │
//	├──────────────────────────────────────────────────────────────────┤
//	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
//	│  │  Handler A  │→ │  Handler B  │→ │  Handler C  │  (ordered)   │
//	│  │  (Phase 1)  │  │  (Phase 2)  │  │  (Phase 3)  │              │
//	│  └─────────────┘  └─────────────┘  └─────────────┘              │
//	└──────────────────────────────────────────────────────────────────┘
//	                              ↑
//	                    SIGTERM / SIGINT / Shutdown()
//
// # Usage
//
// Basic usage with signal handling:
//
//	seq := shutdown.New(shutdown.Defaults())
//	seq.HandleSignals() // SIGTERM, SIGINT
//
//	// Register handlers with phases (lower = earlier)
//	seq.RegisterWithPhase("workers", workerPool, 10)
//	seq.RegisterWithPhase("server", httpServer, 20)
//	seq.RegisterWithPhase("database", dbPool, 30)
//
//	// Handlers run in order: workers (10) → server (20) → database (30)
//
//	// Wait for shutdown
//	<-seq.Done()
//
// For simple functions, use Func (analogous to http.HandlerFunc):
//
//	seq.Register("cache", shutdown.Func(func(ctx context.Context) error {
//	    return cache.Flush(ctx)
//	}))
//
// Implementing a shutdown handler:
//
//	type MyService struct {
//	    tasks chan Task
//	}
//
//	func (s *MyService) Shutdown(ctx context.Context) error {
//	    // 1. Stop accepting new work
//	    close(s.tasks)
//
//	    // 2. Finish in-progress tasks (respect context deadline)
//	    for {
//	        select {
//	        case <-ctx.Done():
//	            return ctx.Err() // Timeout reached
//	        case task := <-s.tasks:
//	            task.Finish()
//	        default:
//	            return nil // All done
//	        }
//	    }
//	}
//
// Manual shutdown with timeout:
//
//	if err := seq.ShutdownWithTimeout(30 * time.Second); err != nil {
//	    log.Printf("shutdown incomplete: %v", err)
//	}
//
// # Phases
//
// Phases control shutdown order. Lower phase numbers are shut down first.
// Typical phase assignments:
//
//   - 10: Frontend (stop accepting requests)
//   - 20: Application services (drain queues)
//   - 30: Backend connections (close databases)
//
// Handlers in the same phase run concurrently.
//
// # Recommendations
//
//   - Always set a timeout for shutdown (30-60 seconds typical)
//   - Handlers should respect context cancellation
//   - Re-queue unfinished work rather than losing it
//   - Use phases to ensure dependencies shut down in order
package shutdown
