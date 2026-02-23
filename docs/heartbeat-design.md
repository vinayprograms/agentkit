# Heartbeat Design

## What This Package Does

The `heartbeat` package detects when agents die or become unresponsive. Agents periodically send "I'm alive" signals. A monitor watches for these signals and raises alerts when they stop.

## Why It Exists

In a distributed swarm, agents can crash, hang, or lose network connectivity. Without heartbeats, the system has no way to know an agent is gone — tasks assigned to it would wait forever.

Heartbeats solve this by creating a simple contract: agents promise to send a signal every few seconds. If signals stop, the agent is presumed dead, and the system can respond (reassign tasks, alert operators, spin up replacements).

## When to Use It

**Use heartbeats for:**
- Detecting crashed or frozen agents
- Tracking which agents are currently available
- Monitoring agent load for work distribution
- Triggering cleanup when agents disappear

**Don't use heartbeats for:**
- Deep health checks (this is liveness, not correctness)
- Leader election (use consensus algorithms)
- Automatic recovery (detection only — recovery is your job)

## Core Concepts

### Heartbeat Signals

A heartbeat is a small message containing:
- **Agent ID** — Who sent it
- **Timestamp** — When it was sent
- **Status** — What the agent is doing ("idle", "busy", etc.)
- **Load** — How busy the agent is (0.0 to 1.0)
- **Metadata** — Optional extra info

Heartbeats are published to the message bus on a subject like `heartbeat.agent-abc123`.

### Senders and Monitors

**Sender** — Runs inside each agent. Sends heartbeats at a regular interval (e.g., every 5 seconds). The agent can update its status and load between beats.

**Monitor** — Runs in a supervisor or coordinator. Subscribes to all heartbeat subjects and tracks when each agent last checked in. If an agent misses too many beats, the monitor fires a callback.

## Architecture


The sender and monitor communicate indirectly through the message bus. They never talk directly to each other. This decoupling means:
- Agents don't need to know who's monitoring them
- Multiple monitors can watch the same agents
- Adding/removing agents doesn't require configuration changes

## Death Detection

The monitor uses a simple timeout algorithm:

1. Track the last heartbeat time for each known agent
2. Periodically check: has any agent exceeded the timeout?
3. If yes, fire the "dead" callback (once per death)

### Timing Tradeoffs

| Heartbeat Interval | Timeout | Detection Speed | False Positives |
|--------------------|---------|-----------------|-----------------|
| 1 second | 3 seconds | Fast | Higher (network blips) |
| 5 seconds | 15 seconds | Medium | Low |
| 30 seconds | 90 seconds | Slow | Very low |

**Rule of thumb:** Set timeout to 2-3x the heartbeat interval. This tolerates a missed beat or two without false alarms.

Shorter intervals catch failures faster but cost more network traffic and risk false positives from temporary network issues. Longer intervals are cheaper but slower to detect problems.

## Common Patterns

### Basic Agent Heartbeat

An agent starts a sender when it boots. The sender automatically publishes heartbeats until stopped. The agent updates its status as work progresses ("idle" → "busy" → "idle").

### Supervisor Monitoring

A supervisor starts a monitor and registers a callback for dead agents. When an agent dies, the callback fires — the supervisor can then deregister the agent, reassign its tasks, or alert operators.

### Load-Aware Work Distribution

Heartbeats include load metrics. A coordinator can watch heartbeats and preferentially assign work to agents with lower load. This enables simple load balancing without complex protocols.

### Registry Integration

Heartbeats and the registry often work together:
- Heartbeat detects the agent is gone
- Callback removes the agent from the registry
- Other components query the registry and no longer see the dead agent

## What Heartbeats Don't Do

**No recovery** — The monitor only detects failures. What to do about them (restart, reassign, alert) is up to you.

**No health checks** — An agent might be alive but misbehaving (returning wrong results, stuck in a loop). Heartbeats only prove the process is running and can reach the network.

**No history** — The monitor keeps only the most recent heartbeat per agent. If you need historical data, log the heartbeats yourself.

## Error Scenarios

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| Network partition | Heartbeats stop arriving | Monitor triggers death callback (may be false positive) |
| Agent frozen | Heartbeats stop | Monitor detects correctly |
| Agent crash | Heartbeats stop | Monitor detects correctly |
| Bus down | Nothing works | Fix the bus first |
| High load delays heartbeat | May trigger false death | Increase timeout or fix the load |

## Implementation Notes

### Thread Safety

Both sender and monitor are safe to use from multiple goroutines. Status/load updates don't block heartbeat publishing.

### Startup Behavior

Senders publish immediately on start (don't wait for first interval). This lets monitors detect new agents right away.

### Graceful Shutdown

Call `Stop()` before exiting. This ensures a clean shutdown — the final heartbeat is sent and resources are released.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Heartbeat serialization, timing accuracy |
| Integration | Full sender → bus → monitor flow |
| Timing | Death detection fires at correct threshold |
| Concurrency | Multiple senders, callback thread safety |
| Failure | Bus disconnection, recovery behavior |
