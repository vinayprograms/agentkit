# Heartbeat Design

## Overview

The heartbeat package provides liveness detection for distributed agents. It consists of two components: senders that emit periodic heartbeats, and monitors that track agent liveness and detect failures.

## Goals

| Goal | Description |
|------|-------------|
| Failure detection | Identify dead or unresponsive agents |
| Status broadcasting | Share load and status across the swarm |
| Configurable timing | Adjustable intervals for different needs |
| Bus agnostic | Works over any MessageBus implementation |
| Registry integration | Update registry based on liveness |

## Non-Goals

| Non-Goal | Reason |
|----------|--------|
| Leader election | Separate concern; use consensus algorithms |
| Health checks | This is liveness, not deep health probing |
| Automatic recovery | Detection only; recovery is caller's job |
| Persistent history | Monitor keeps only last heartbeat per agent |

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                           Swarm                                   │
│                                                                   │
│   ┌─────────────────┐       MessageBus        ┌────────────────┐ │
│   │    Agent A      │                         │    Monitor     │ │
│   │  ┌───────────┐  │──heartbeat.agent-a─────▶│  (Supervisor)  │ │
│   │  │  Sender   │  │                         │                │ │
│   │  └───────────┘  │                         │  tracks:       │ │
│   └─────────────────┘                         │  - lastSeen[]  │ │
│                                               │  - callbacks   │ │
│   ┌─────────────────┐                         │                │ │
│   │    Agent B      │                         │  detects:      │ │
│   │  ┌───────────┐  │──heartbeat.agent-b─────▶│  - timeouts    │ │
│   │  │  Sender   │  │                         │  - deaths      │ │
│   │  └───────────┘  │                         │                │ │
│   └─────────────────┘                         └────────────────┘ │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

## Heartbeat Message Format

```go
type Heartbeat struct {
    AgentID   string            `json:"agent_id"`
    Timestamp time.Time         `json:"timestamp"`
    Status    string            `json:"status"`  // "idle", "busy", etc.
    Load      float64           `json:"load"`    // 0.0-1.0
    Metadata  map[string]string `json:"metadata,omitempty"`
}
```

**Subject format:** `heartbeat.<agent-id>`

Example: `heartbeat.agent-abc123`

## Sender Interface

```go
type Sender interface {
    // Start begins sending heartbeats at configured interval.
    Start(ctx context.Context) error

    // SetStatus updates the status field.
    SetStatus(status string)

    // SetLoad updates the load metric (clamped to 0.0-1.0).
    SetLoad(load float64)

    // SetMetadata updates a metadata field.
    SetMetadata(key, value string)

    // Stop stops sending heartbeats.
    Stop() error
}
```

**Configuration:**
- `Bus` - MessageBus for publishing
- `AgentID` - Unique agent identifier
- `Interval` - Time between heartbeats (default: 5s)
- `InitialStatus` - Starting status (default: "idle")

## Monitor Interface

```go
type Monitor interface {
    // Watch returns heartbeats for a specific agent.
    Watch(agentID string) (<-chan *Heartbeat, error)

    // WatchAll returns all heartbeats (starts monitoring).
    WatchAll() (<-chan *Heartbeat, error)

    // IsAlive checks if agent sent heartbeat within timeout.
    IsAlive(agentID string, timeout time.Duration) bool

    // LastHeartbeat returns most recent heartbeat, if any.
    LastHeartbeat(agentID string) *Heartbeat

    // OnDead registers callback for dead agent detection.
    OnDead(callback func(agentID string))

    // Stop stops monitoring.
    Stop() error
}
```

**Configuration:**
- `Bus` - MessageBus for subscribing
- `Timeout` - Dead threshold (default: 15s, should be 2-3x interval)
- `CheckInterval` - How often to check for deaths (default: 1s)

## Death Detection Algorithm

The monitor uses a simple timeout-based algorithm:

```
For each known agent:
    if (now - lastHeartbeat.Timestamp) > timeout:
        if not already_reported[agentID]:
            mark as reported
            invoke OnDead callbacks
```

**Timing recommendations:**
| Heartbeat Interval | Suggested Timeout | Rationale |
|--------------------|-------------------|-----------|
| 5 seconds | 15 seconds | Tolerates 2 missed beats |
| 1 second | 3-5 seconds | Fast detection for critical agents |
| 30 seconds | 90 seconds | Low-overhead for stable agents |

**Tradeoffs:**
- Shorter timeout = faster detection, more false positives
- Longer timeout = fewer false positives, slower detection

## Implementations

### BusSender

Production sender over MessageBus.

| Feature | Implementation |
|---------|----------------|
| Publishing | Periodic publish to `heartbeat.<id>` |
| Threading | Single goroutine with ticker |
| State updates | RWMutex-protected fields |
| Initial beat | Sent immediately on Start() |

### BusMonitor

Production monitor over MessageBus.

| Feature | Implementation |
|---------|----------------|
| Subscription | Subscribes to `heartbeat.*` (or equivalent) |
| Tracking | `map[string]*Heartbeat` for last seen |
| Death check | Periodic goroutine at `CheckInterval` |
| Callbacks | Slice of `func(string)`, called synchronously |

### Memory Implementations

For testing:
- `MemorySender` - Records sent heartbeats to slice
- `MemoryMonitor` - Manual heartbeat injection, manual death check

## Package Structure

```
heartbeat/
├── heartbeat.go   # Types, interfaces, config
├── sender.go      # BusSender + MemorySender
├── sender_test.go
├── monitor.go     # BusMonitor + MemoryMonitor
├── monitor_test.go
└── doc.go         # Package documentation
```

## Usage Patterns

### Agent Heartbeat

```go
sender, err := heartbeat.NewBusSender(heartbeat.SenderConfig{
    Bus:     msgBus,
    AgentID: agentID,
    Interval: 5 * time.Second,
})
if err != nil {
    return err
}

// Start heartbeating
ctx, cancel := context.WithCancel(context.Background())
sender.Start(ctx)

// Update as work happens
sender.SetStatus("busy")
sender.SetLoad(0.7)

// Graceful shutdown
sender.Stop()
cancel()
```

### Supervisor Monitoring

```go
monitor, err := heartbeat.NewBusMonitor(heartbeat.MonitorConfig{
    Bus:     msgBus,
    Timeout: 15 * time.Second,
})
if err != nil {
    return err
}

// Register death callback
monitor.OnDead(func(agentID string) {
    log.Printf("Agent %s is dead", agentID)
    registry.Deregister(agentID)
    rescheduleTasks(agentID)
})

// Start monitoring all agents
heartbeats, _ := monitor.WatchAll()

// Optionally process heartbeats
for hb := range heartbeats {
    log.Printf("Heartbeat from %s: load=%.2f", hb.AgentID, hb.Load)
}
```

### Query Agent Status

```go
// Check if specific agent is alive
if monitor.IsAlive("agent-123", 15*time.Second) {
    // Safe to assign work
}

// Get last known state
last := monitor.LastHeartbeat("agent-123")
if last != nil {
    fmt.Printf("Last status: %s, load: %.2f\n", last.Status, last.Load)
}
```

## Integration with Registry

Common pattern: heartbeat drives registry updates

```go
// In agent
go func() {
    ticker := time.NewTicker(5 * time.Second)
    for range ticker.C {
        // Update both
        sender.SetLoad(calculateLoad())
        registry.Register(AgentInfo{
            ID:     agentID,
            Load:   calculateLoad(),
            Status: currentStatus,
        })
    }
}()

// In supervisor
monitor.OnDead(func(agentID string) {
    registry.Deregister(agentID)
})
```

## Error Handling

| Error | Meaning | Recovery |
|-------|---------|----------|
| `ErrAlreadyStarted` | Sender/monitor already running | Ignore or fix logic |
| `ErrNotStarted` | Stop called before Start | Ignore or fix logic |
| `ErrInvalidConfig` | Missing Bus or AgentID | Fix configuration |

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Heartbeat serialization, timing |
| Integration | Full sender→bus→monitor flow |
| Timing | Death detection accuracy |
| Concurrency | Multiple senders, callback safety |
| Failure | Bus disconnection, recovery |
