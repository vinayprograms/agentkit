# Shutdown Coordination Design

## Overview

The shutdown package provides graceful shutdown coordination for distributed agents. It ensures that in-progress tasks complete, pending work is re-queued, and the system remains consistent during termination. The package handles OS signals (SIGTERM, SIGINT) and provides ordered shutdown with phase-based dependency management.

## Goals

| Goal | Description |
|------|-------------|
| Graceful termination | Complete in-flight work before exiting |
| Phase ordering | Shut down components in dependency order |
| Concurrent execution | Handlers in same phase run concurrently |
| Signal handling | Automatic SIGTERM/SIGINT handling |
| Timeout enforcement | Prevent indefinite shutdown hangs |
| Progress visibility | Track and report shutdown progress |

## Non-Goals

| Non-Goal | Reason |
|----------|--------|
| Restart coordination | Out of scope; handled by process supervisors |
| Distributed shutdown | Single-process coordination only |
| Work re-queueing logic | Handler responsibility, not coordinator |
| Health check integration | Separate concern (use registry package) |

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                      ShutdownCoordinator                         │
├──────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │  Handler A  │→ │  Handler B  │→ │  Handler C  │  (ordered)   │
│  │  (Phase 10) │  │  (Phase 20) │  │  (Phase 30) │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
└──────────────────────────────────────────────────────────────────┘
                              ↑
                    SIGTERM / SIGINT / Shutdown()
```

## Interface Design

### ShutdownHandler

Components that need graceful shutdown implement this interface:

```go
// ShutdownHandler is implemented by components that need graceful shutdown.
type ShutdownHandler interface {
    // OnShutdown is called when shutdown is initiated.
    // The context will be cancelled when the timeout is reached.
    // Implementations should:
    // - Stop accepting new work
    // - Finish in-progress tasks (if time permits)
    // - Re-queue pending work
    // - Release resources
    OnShutdown(ctx context.Context) error
}

// ShutdownFunc is a convenience type for simple shutdown functions.
type ShutdownFunc func(ctx context.Context) error
```

### ShutdownCoordinator

The coordinator manages the shutdown lifecycle:

```go
// ShutdownCoordinator manages graceful shutdown for multiple components.
type ShutdownCoordinator interface {
    // Register adds a handler to be called during shutdown.
    Register(name string, handler ShutdownHandler)

    // RegisterWithPhase adds a handler with a specific phase.
    // Lower phase numbers are shut down first.
    RegisterWithPhase(name string, handler ShutdownHandler, phase int)

    // Shutdown initiates graceful shutdown.
    Shutdown(ctx context.Context) error

    // ShutdownWithTimeout initiates shutdown with a timeout.
    ShutdownWithTimeout(timeout time.Duration) error

    // HandleSignals registers handlers for SIGTERM and SIGINT.
    HandleSignals()

    // Done returns a channel that is closed when shutdown is complete.
    Done() <-chan struct{}

    // Err returns any error that occurred during shutdown.
    Err() error
}
```

### Result Types

```go
// HandlerResult contains the result of a single handler's shutdown.
type HandlerResult struct {
    Name     string        // Handler name
    Phase    int           // Phase number
    Duration time.Duration // Time taken
    Err      error         // Any error
}

// ShutdownResult contains the complete shutdown result.
type ShutdownResult struct {
    TotalDuration time.Duration
    Results       []HandlerResult
    Err           error
}
```

## Phase-Based Ordering Design

### Phase Concept

Phases control shutdown order. Lower phase numbers execute first. This ensures dependencies are respected—components that depend on others shut down before their dependencies.

```
Phase 10 (Frontend)     ──▶  Phase 20 (Services)     ──▶  Phase 30 (Backend)
┌─────────────────────┐      ┌─────────────────────┐      ┌─────────────────────┐
│ HTTP Server         │      │ Worker Pool         │      │ Database            │
│ API Gateway         │      │ Task Queue          │      │ Cache               │
│ WebSocket Handler   │      │ Message Consumer    │      │ Message Bus         │
└─────────────────────┘      └─────────────────────┘      └─────────────────────┘
```

### Execution Model

| Behavior | Description |
|----------|-------------|
| Inter-phase | Phases execute sequentially (10 → 20 → 30) |
| Intra-phase | Handlers within same phase execute concurrently |
| Blocking | Each phase completes before next phase starts |
| Error handling | Configurable: continue or stop on error |

### Recommended Phase Assignments

| Phase | Category | Examples |
|-------|----------|----------|
| 10 | Frontend | HTTP servers, API gateways, load balancers |
| 20 | Application | Workers, task processors, message handlers |
| 30 | Backend | Databases, caches, message buses |
| 100 | Default | Handlers registered without explicit phase |

### Phase Grouping Algorithm

```go
// Handlers sorted by phase, then grouped
handlers := []registration{
    {name: "http", phase: 10},
    {name: "grpc", phase: 10},     // Same phase as http
    {name: "workers", phase: 20},
    {name: "db", phase: 30},
}

// Results in groups:
// Group 1 (phase 10): [http, grpc]      ← execute concurrently
// Group 2 (phase 20): [workers]         ← waits for Group 1
// Group 3 (phase 30): [db]              ← waits for Group 2
```

## Signal Handling Integration

### Supported Signals

| Signal | Behavior |
|--------|----------|
| SIGTERM | Initiates graceful shutdown with default timeout |
| SIGINT | Same as SIGTERM (Ctrl+C) |

### Signal Flow

```
OS Signal ──▶ signal.Notify() ──▶ signalChan ──▶ ShutdownWithTimeout()
                                                        │
                                                        ▼
                                            Execute handlers by phase
                                                        │
                                                        ▼
                                                 Close Done()
```

### Implementation

```go
func (c *Coordinator) HandleSignals() {
    signal.Notify(c.signalChan, syscall.SIGTERM, syscall.SIGINT)

    go func() {
        <-c.signalChan
        c.ShutdownWithTimeout(c.config.DefaultTimeout)
    }()
}
```

### Usage Pattern

```go
coord := shutdown.NewCoordinator(shutdown.DefaultConfig())
coord.HandleSignals() // Must call before signals expected

// Register handlers...

// Block until shutdown complete
<-coord.Done()
```

## Timeout and Error Handling

### Timeout Behavior

| Scenario | Behavior |
|----------|----------|
| Handler completes in time | Normal completion, no error |
| Handler exceeds timeout | Context cancelled, ErrTimeout returned |
| All handlers complete | Done() closed, Err() returns nil or aggregate error |

### Error Types

```go
var (
    ErrAlreadyShutdown = errors.New("shutdown already initiated")
    ErrTimeout         = errors.New("shutdown timeout exceeded")
    ErrHandlerFailed   = errors.New("one or more handlers failed")
    ErrInvalidConfig   = errors.New("invalid configuration")
)
```

### Error Handling Modes

The `ContinueOnError` configuration controls behavior:

| Mode | Behavior |
|------|----------|
| `ContinueOnError: true` (default) | All handlers execute; errors collected |
| `ContinueOnError: false` | Stop on first error; skip remaining handlers |

### Handler Error Contract

Handlers should:
1. Check context cancellation frequently
2. Return `ctx.Err()` if context is cancelled
3. Return other errors for handler-specific failures
4. Never block indefinitely

```go
func (s *Service) OnShutdown(ctx context.Context) error {
    for _, task := range s.pending {
        select {
        case <-ctx.Done():
            return ctx.Err() // Timeout reached
        default:
            task.Finish()
        }
    }
    return nil
}
```

## Configuration

```go
type Config struct {
    // DefaultTimeout for ShutdownWithTimeout (default: 30s)
    DefaultTimeout time.Duration

    // DefaultPhase for handlers without explicit phase (default: 100)
    DefaultPhase int

    // ContinueOnError: keep going if a handler fails (default: true)
    ContinueOnError bool

    // OnProgress: callback for each handler completion
    OnProgress func(result HandlerResult)
}

func DefaultConfig() Config {
    return Config{
        DefaultTimeout:  30 * time.Second,
        DefaultPhase:    100,
        ContinueOnError: true,
    }
}
```

## Usage Patterns

### Basic Signal Handling

```go
coord := shutdown.NewCoordinator(shutdown.DefaultConfig())
coord.HandleSignals()

coord.RegisterWithPhase("http-server", httpServer, 10)
coord.RegisterWithPhase("workers", workerPool, 20)
coord.RegisterWithPhase("database", db, 30)

// Run application...

<-coord.Done()
if err := coord.Err(); err != nil {
    log.Printf("Shutdown error: %v", err)
}
```

### Function-Based Handlers

```go
coord.RegisterFunc("cleanup", func(ctx context.Context) error {
    return os.RemoveAll(tempDir)
})

coord.RegisterFuncWithPhase("flush-logs", func(ctx context.Context) error {
    return logger.Sync()
}, 30)
```

### Progress Logging

```go
config := shutdown.DefaultConfig()
config.OnProgress = func(r shutdown.HandlerResult) {
    if r.Err != nil {
        log.Printf("✗ %s failed: %v (took %v)", r.Name, r.Err, r.Duration)
    } else {
        log.Printf("✓ %s completed (took %v)", r.Name, r.Duration)
    }
}

coord := shutdown.NewCoordinator(config)
```

### Manual Shutdown with Timeout

```go
// Programmatic shutdown (e.g., from admin endpoint)
if err := coord.ShutdownWithTimeout(60 * time.Second); err != nil {
    log.Printf("Shutdown incomplete: %v", err)
    // Force exit or log for investigation
}
```

### Checking Results

```go
<-coord.Done()

result := coord.Result()
log.Printf("Shutdown took %v", result.TotalDuration)

for _, hr := range result.Results {
    log.Printf("  %s (phase %d): %v", hr.Name, hr.Phase, hr.Duration)
}

if result.Failed() {
    log.Printf("Failed handlers: %v", result.FailedHandlers())
}
```

## Integration with Other Agentkit Packages

### Transport Package

Transports implement graceful shutdown via their `Close()` method:

```go
// In application startup
coord.RegisterFuncWithPhase("transport", func(ctx context.Context) error {
    return transport.Close()
}, 10)
```

### Bus Package

Message bus connections should close after handlers that use them:

```go
// Workers use the bus (phase 20)
coord.RegisterWithPhase("workers", workerPool, 20)

// Bus closes after workers (phase 30)
coord.RegisterFuncWithPhase("bus", func(ctx context.Context) error {
    return messageBus.Close()
}, 30)
```

### Registry Package

Deregister from registry early in shutdown:

```go
coord.RegisterFuncWithPhase("registry", func(ctx context.Context) error {
    return registry.Deregister(ctx, agentID)
}, 10) // Early phase - stop receiving work
```

### Typical Full Integration

```go
func main() {
    coord := shutdown.NewCoordinator(shutdown.DefaultConfig())
    coord.HandleSignals()

    // Phase 10: Stop accepting work
    coord.RegisterFuncWithPhase("http", httpServer.Shutdown, 10)
    coord.RegisterFuncWithPhase("registry", registry.Deregister, 10)

    // Phase 20: Drain in-flight work
    coord.RegisterWithPhase("task-processor", taskProcessor, 20)
    coord.RegisterWithPhase("message-handler", messageHandler, 20)

    // Phase 30: Close backends
    coord.RegisterFuncWithPhase("bus", messageBus.Close, 30)
    coord.RegisterFuncWithPhase("database", db.Close, 30)
    coord.RegisterFuncWithPhase("cache", cache.Close, 30)

    // Run application
    go httpServer.ListenAndServe()

    <-coord.Done()
    os.Exit(exitCode(coord.Err()))
}
```

## Package Structure

```
shutdown/
├── shutdown.go       # Interface definitions + errors + Config
├── coordinator.go    # Coordinator implementation
├── coordinator_test.go
└── doc.go            # Package documentation
```

## Thread Safety

| Operation | Safety |
|-----------|--------|
| Register/RegisterWithPhase | Safe during setup, not during shutdown |
| Shutdown | Safe to call from any goroutine |
| HandleSignals | Call once at startup |
| Done/Err/Result | Safe to call anytime |

## Testing Considerations

| Scenario | Approach |
|----------|----------|
| Unit tests | Use `Reset()` between tests |
| Manual trigger | Use `Trigger()` to simulate signal |
| Timeout testing | Use short timeouts, slow handlers |
| Error handling | Inject failing handlers |

```go
func TestShutdownOrder(t *testing.T) {
    coord := shutdown.NewCoordinator(shutdown.DefaultConfig())

    var order []string
    var mu sync.Mutex

    record := func(name string) shutdown.ShutdownFunc {
        return func(ctx context.Context) error {
            mu.Lock()
            order = append(order, name)
            mu.Unlock()
            return nil
        }
    }

    coord.RegisterFuncWithPhase("third", record("third"), 30)
    coord.RegisterFuncWithPhase("first", record("first"), 10)
    coord.RegisterFuncWithPhase("second", record("second"), 20)

    coord.ShutdownWithTimeout(time.Second)
    <-coord.Done()

    assert.Equal(t, []string{"first", "second", "third"}, order)
}
```

## Best Practices

| Practice | Rationale |
|----------|-----------|
| Always set a timeout | Prevent indefinite hangs (30-60s typical) |
| Respect context cancellation | Allow graceful timeout handling |
| Re-queue unfinished work | Prevent work loss |
| Use explicit phases | Make dependencies clear |
| Log shutdown progress | Aid debugging in production |
| Register early, before signals | Ensure all handlers are captured |
