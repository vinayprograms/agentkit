// Package shutdown provides graceful shutdown coordination for distributed agents.
//
// # Overview
//
// The shutdown package enables agents to shut down gracefully, ensuring that
// in-progress tasks complete, pending work is re-queued, and the system
// remains consistent. It handles OS signals (SIGTERM, SIGINT) and provides
// ordered shutdown with dependency management.
//
// # Architecture
//
//	┌──────────────────────────────────────────────────────────────────┐
//	│                      ShutdownCoordinator                         │
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
//	coord := shutdown.NewCoordinator(shutdown.DefaultConfig())
//	coord.HandleSignals() // SIGTERM, SIGINT
//
//	// Register handlers with phases (lower = earlier)
//	coord.RegisterWithPhase("database", dbHandler, 30)
//	coord.RegisterWithPhase("workers", workerHandler, 10)
//	coord.RegisterWithPhase("server", serverHandler, 20)
//
//	// Handlers are called in order: workers (10) → server (20) → database (30)
//
//	// Wait for shutdown
//	<-coord.Done()
//
// Implementing a shutdown handler:
//
//	type MyService struct {
//	    tasks chan Task
//	}
//
//	func (s *MyService) OnShutdown(ctx context.Context) error {
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
//	if err := coord.ShutdownWithTimeout(30 * time.Second); err != nil {
//	    log.Printf("Shutdown incomplete: %v", err)
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
// Handlers in the same phase are shut down concurrently.
//
// # Recommendations
//
//   - Always set a timeout for shutdown (30-60 seconds typical)
//   - Handlers should respect context cancellation
//   - Re-queue unfinished work rather than losing it
//   - Use phases to ensure dependencies shut down in order
//   - Log shutdown progress for debugging
package shutdown
