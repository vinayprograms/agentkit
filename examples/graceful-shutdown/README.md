# Graceful Shutdown Example

Demonstrates coordinated graceful shutdown using agentkit's `shutdown` package with a real-world pattern: HTTP server + worker pool + resource cleanup.

## Overview

This example shows how to:
- **Register handlers with phases** for ordered shutdown
- **Handle OS signals** (Ctrl+C / SIGTERM)
- **Drain in-flight work** before shutting down
- **Timeout handling** with configurable limits
- **Progress reporting** during shutdown

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                    Shutdown Coordinator                         │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Phase 10          Phase 20           Phase 30                 │
│  ┌──────────┐      ┌─────────────┐    ┌─────────────┐         │
│  │  HTTP    │  →   │   Worker    │  →  │  Cleanup    │         │
│  │  Server  │      │   Pool      │     │  (DB/Files) │         │
│  └──────────┘      └─────────────┘    └─────────────┘         │
│                                                                 │
│  Stop accepting     Drain pending      Close connections       │
│  requests           jobs               Delete temp files        │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
                            ↑
                  SIGINT / SIGTERM / Shutdown()
```

## Components

### HTTP Server (Phase 10)
- Accepts HTTP requests on `:8080`
- Tracks in-flight requests
- Uses `http.Server.Shutdown()` for graceful drain
- Shuts down first to stop accepting new work

### Worker Pool (Phase 20)
- 4 concurrent workers processing background jobs
- Buffered job queue (100 jobs)
- Drains pending jobs before shutting down
- Respects timeout context

### Cleanup Handler (Phase 30)
- Simulates closing database connections
- Simulates deleting temporary files
- Runs last to ensure all work is complete

## How to Run

```bash
cd examples/graceful-shutdown
go run main.go
```

Then press `Ctrl+C` to trigger graceful shutdown.

## Example Output

```
=== Graceful Shutdown Example ===

Application started!
  HTTP Server: http://localhost:8080
  Workers: 4 running

Press Ctrl+C to trigger graceful shutdown...

2024/01/15 10:00:01 [job] Completed job #1
2024/01/15 10:00:01 [job] Completed job #2
2024/01/15 10:00:02 [job] Completed job #3
2024/01/15 10:00:02 [job] Completed job #4
^C
2024/01/15 10:00:03 [server] Shutting down (in-flight: 0)...
2024/01/15 10:00:03 [server] Stopped. Total requests served: 0
2024/01/15 10:00:03   ✓ [Phase 10] http-server shutdown complete (5ms)
2024/01/15 10:00:03 [workers] Shutting down (pending: 2)...
2024/01/15 10:00:03 [job] Completed job #5
2024/01/15 10:00:03 [job] Completed job #6
2024/01/15 10:00:03 [workers] Stopped. Total jobs processed: 6
2024/01/15 10:00:03   ✓ [Phase 20] worker-pool shutdown complete (312ms)
2024/01/15 10:00:03 [cleanup] Releasing resources...
2024/01/15 10:00:03 [cleanup] Closed 5 database connections
2024/01/15 10:00:04 [cleanup] Removed 10 temporary files
2024/01/15 10:00:04   ✓ [Phase 30] cleanup shutdown complete (456ms)

=== Shutdown Complete ===
Total duration: 773ms
All handlers completed successfully!
```

## Key Concepts

### Phases
Lower phase numbers shut down first. Handlers in the same phase run concurrently.

```go
const (
    PhaseFrontend = 10 // Stop accepting requests
    PhaseWorkers  = 20 // Drain work queues
    PhaseBackend  = 30 // Close connections
)

coord.RegisterWithPhase("server", server, PhaseFrontend)
coord.RegisterWithPhase("workers", workers, PhaseWorkers)
coord.RegisterWithPhase("cleanup", cleanup, PhaseBackend)
```

### Implementing ShutdownHandler

```go
func (s *myService) OnShutdown(ctx context.Context) error {
    // 1. Stop accepting new work
    s.running.Store(false)
    
    // 2. Drain existing work (respect timeout)
    select {
    case <-s.drained:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Signal Handling

```go
coord.HandleSignals()  // Handles SIGINT, SIGTERM
<-coord.Done()         // Wait for shutdown to complete
```

### Progress Callback

```go
coord := shutdown.NewCoordinator(shutdown.Config{
    OnProgress: func(result shutdown.HandlerResult) {
        log.Printf("%s completed in %v", result.Name, result.Duration)
    },
})
```

### Timeout Configuration

```go
// Default timeout for signal-triggered shutdowns
coord := shutdown.NewCoordinator(shutdown.Config{
    DefaultTimeout: 30 * time.Second,
})

// Or manual shutdown with specific timeout
coord.ShutdownWithTimeout(10 * time.Second)
```

## Testing Scenarios

### Normal Shutdown
```bash
# Start the app
go run main.go

# Wait a few seconds, then Ctrl+C
# All handlers complete successfully
```

### Timeout Scenario
Modify `DefaultTimeout` to a very short duration (e.g., 100ms) and watch handlers get interrupted.

### Manual Shutdown
You can trigger shutdown programmatically:
```go
coord.Trigger()  // Sends SIGTERM to signal handler
// or
coord.ShutdownWithTimeout(5 * time.Second)
```

## Notes

- Handlers should respect the context deadline
- `ContinueOnError: true` ensures all handlers run even if one fails
- Use phases to ensure dependencies shut down in the correct order
- The `Result()` method provides detailed timing and error information after shutdown
