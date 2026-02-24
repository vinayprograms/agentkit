# Swarm Heartbeat Example

Demonstrates agent liveness detection using heartbeat and registry packages.

## Components

- **agent/** - Simulates an agent that sends heartbeats
- **monitor/** - Watches for heartbeats and detects dead agents

## Running

Start the monitor:
```bash
go run ./examples/swarm-heartbeat/monitor/
```

In another terminal, start one or more agents:
```bash
go run ./examples/swarm-heartbeat/agent/
```

Kill an agent (Ctrl+C) to see death detection.

## What This Shows

- Periodic heartbeat sending
- Status and load reporting
- Death detection with callbacks
- Registry integration
