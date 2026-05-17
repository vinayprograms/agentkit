# shutdown

Ordered, phased graceful shutdown for multi-component applications. Handlers registered at the same phase run concurrently; phases run sequentially lowest-first. `Shutdown` is idempotent — safe to call from multiple goroutines.

## Usage

```go
seq := shutdown.New(shutdown.Defaults())

seq.Register("mcp",      mcpManager)   // phase 0 (default)
seq.Register("workflow", workflowRunner)
seq.RegisterWithPhase("db", dbPool, 1) // phase 1, after phase 0 completes

seq.HandleSignals() // shuts down on SIGINT / SIGTERM

// in main — wait for shutdown to complete
<-seq.Done()
if err := seq.Err(); err != nil {
    log.Fatal(err)
}
```

## Handler interface

```go
type Handler interface {
    Shutdown(ctx context.Context) error
}
```

Any type with a `Shutdown(ctx) error` method satisfies this — including swarmkit primitives (duck-typed, no import needed). For simple cases use `shutdown.Func`:

```go
seq.Register("cache", shutdown.Func(func(ctx context.Context) error {
    return cache.Flush(ctx)
}))
```

## Config

```go
cfg := shutdown.Config{
    Timeout:         30 * time.Second, // total budget across all phases
    PhaseTimeout:    10 * time.Second, // per-phase budget
    ContinueOnError: false,            // abort remaining phases on failure
}
```

`shutdown.Defaults()` returns sensible values (30s total, 10s per phase, abort on error).

## Summary

```go
summary := seq.Summary()
for _, r := range summary.Results {
    fmt.Printf("%s: %v (%s)\n", r.Name, r.Err, r.Duration)
}
```
