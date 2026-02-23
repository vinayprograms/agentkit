// Package heartbeat provides agent liveness detection for distributed swarms.
//
// # Overview
//
// Heartbeats enable swarm coordinators to detect dead agents and trigger
// failover. Agents periodically broadcast "I'm alive" signals containing
// status, load, and metadata. Monitors track these signals and invoke
// callbacks when agents are presumed dead.
//
// # Architecture
//
//	┌─────────────┐    heartbeat.<agent-id>    ┌─────────────┐
//	│   Sender    │ ────────────────────────>  │   Monitor   │
//	│  (Agent A)  │                            │ (Coordinator)│
//	└─────────────┘                            └─────────────┘
//
// # Usage
//
// Sending heartbeats from an agent:
//
//	sender, _ := heartbeat.NewBusSender(heartbeat.SenderConfig{
//	    Bus:      bus,
//	    AgentID:  "agent-1",
//	    Interval: 5 * time.Second,
//	})
//	sender.SetStatus("busy")
//	sender.SetLoad(0.75)
//	sender.Start(ctx)
//
// Monitoring heartbeats from a coordinator:
//
//	monitor, _ := heartbeat.NewBusMonitor(heartbeat.MonitorConfig{
//	    Bus:     bus,
//	    Timeout: 15 * time.Second, // 3 missed heartbeats
//	})
//	monitor.OnDead(func(agentID string) {
//	    log.Printf("Agent %s presumed dead", agentID)
//	})
//	monitor.WatchAll()
//
// # Subject Convention
//
// Heartbeats are published to: heartbeat.<agent-id>
// Monitor subscribes to: heartbeat.> (NATS wildcard)
//
// # Recommendations
//
//   - Set timeout to 2-3x the heartbeat interval
//   - Include meaningful metadata (version, capabilities)
//   - Use load metrics for intelligent task distribution
//   - Handle OnDead callbacks idempotently
package heartbeat
