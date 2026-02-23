# Swarm Heartbeat Example

A distributed agent swarm demonstrating agentkit's registry for self-registration, heartbeats, and dead agent detection.

## Overview

This example shows how to build a monitored agent swarm using:
- **Registry** for agent self-registration with capabilities and metadata
- **Heartbeats** to keep registrations fresh and detect failures
- **Registry Watch** for real-time agent join/leave events
- **Message Bus** for heartbeat broadcasts

## Architecture

```
                    ┌─────────────────┐
                    │     Monitor     │
                    │  (monitor.go)   │
                    │                 │
                    │ • Watches registry events
                    │ • Detects dead agents
                    │ • Displays swarm status
                    └────────┬────────┘
                             │ Registry Watch
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼────┐         ┌────▼────┐         ┌────▼────┐
    │ Agent 1 │         │ Agent 2 │         │ Agent N │
    │         │         │         │         │         │
    │ • Self-register   │ • Self-register   │ • Self-register
    │ • Heartbeat       │ • Heartbeat       │ • Heartbeat
    │ • Work simulation │ • Work simulation │ • Work simulation
    └─────────┘         └─────────┘         └─────────┘
```

## Components

### Agent (`agent.go`)
- Self-registers with the registry on startup
- Sends periodic heartbeats to refresh registration
- Broadcasts heartbeat messages to the bus
- Simulates work (status changes between idle/busy)
- Gracefully deregisters on shutdown

### Monitor (`monitor.go`)
- Watches registry for agent join/leave events
- Subscribes to heartbeat broadcasts
- Tracks last heartbeat time for each agent
- Marks agents as dead if heartbeats stop
- Displays real-time swarm status

## Registry Features Used

| Feature | Purpose |
|---------|---------|
| `Register()` | Add agent with capabilities and metadata |
| `Deregister()` | Remove agent cleanly on shutdown |
| `Get()` | Retrieve agent's current state |
| `List()` | Get all registered agents |
| `Watch()` | Stream real-time add/update/remove events |

## How to Run

### Quick Demo (Single Process)

For testing, run the demo that includes embedded agents:

```bash
cd examples/swarm-heartbeat
go run agent.go demo
```

### Multi-Process Mode

**Terminal 1 - Monitor:**
```bash
go run monitor.go -dead 20s
```

**Terminals 2, 3, ... - Agents:**
```bash
go run agent.go agent-1 -interval 5s
go run agent.go agent-2 -interval 5s
go run agent.go agent-3 -interval 5s
```

### Test Dead Detection

1. Start the monitor
2. Start an agent
3. Kill the agent with Ctrl+C (clean shutdown - should show "LEFT")
4. Start another agent, then kill it with `kill -9` (hard kill - should show "DEAD")

## Command-Line Options

### Agent
```
go run agent.go [name] [flags]

Flags:
  -name string      Agent name (default: random)
  -interval duration   Heartbeat interval (default: 5s)
  -ttl duration        Registry TTL (default: 15s)
```

### Monitor
```
go run monitor.go [flags]

Flags:
  -dead duration    Time without heartbeat before dead (default: 30s)
  -ttl duration     Registry TTL (default: 15s)
```

## Example Output

**Monitor output:**
```
╔════════════════════════════════════════╗
║        SWARM HEARTBEAT MONITOR         ║
╚════════════════════════════════════════╝
Swarm Monitor starting...
Dead threshold: 30s
🟢 JOINED: agent-1 (capabilities: [worker compute])
🟢 JOINED: agent-2 (capabilities: [worker compute])
📢 STATUS: agent-1 -> busy
📢 STATUS: agent-1 -> idle

╔══════════════════════════════════════════════════════════════╗
║  SWARM STATUS: 2 alive, 0 dead                              
╠══════════════════════════════════════════════════════════════╣
║  AGENT        │ STATUS   │ LOAD   │ LAST HEARTBEAT           ║
╠══════════════════════════════════════════════════════════════╣
║ 🟢 agent-1    │ idle     │   0.0% │ 2s ago                   ║
║ 🟢 agent-2    │ busy     │  75.3% │ 4s ago                   ║
╚══════════════════════════════════════════════════════════════╝

🔴 LEFT: agent-2
💀 DEAD: agent-3 (no heartbeat for 35s)
```

**Agent output:**
```
Agent 'agent-1' started (heartbeat: 5s, TTL: 15s)
Press Ctrl+C to stop
[agent-1] Registered with capabilities: [worker compute]
[agent-1] Heartbeat #5 sent
[agent-1] Working...
[agent-1] Work complete, idle
^C
Received signal: interrupt
[agent-1] Shutting down...
Agent stopped
```

## Key Concepts Demonstrated

1. **Self-Registration** - Agents register themselves with capabilities and metadata
2. **TTL-Based Expiry** - Registry entries expire if not refreshed
3. **Heartbeat Pattern** - Periodic refresh to prove liveness
4. **Watch Events** - Real-time notifications without polling
5. **Graceful vs Hard Failure** - Clean shutdown vs crash detection
6. **Load Reporting** - Agents report their current workload

## Distributed Operation

For production use across multiple machines, replace in-memory components with NATS:

```go
// In agent.go and monitor.go, replace:
memBus := bus.NewMemoryBus(bus.DefaultConfig())
memReg := registry.NewMemoryRegistry(registry.MemoryConfig{TTL: ttl})

// With:
nc, _ := nats.Connect("nats://your-nats-server:4222")
natsBus, _ := bus.NewNATSBus(nc, bus.DefaultConfig())
natsReg, _ := registry.NewNATSRegistry(nc, registry.NATSConfig{
    Prefix: "swarm",
    TTL:    ttl,
})
```

## Notes

- The in-memory bus/registry only work within a single process
- For multi-process demos, agents and monitor would share a NATS server
- Heartbeat interval should be less than TTL/2 for reliability
- Dead threshold should be >= 2 * heartbeat interval
