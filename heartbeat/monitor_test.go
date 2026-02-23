package heartbeat

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// --- Unit Tests ---

func TestMonitorConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     MonitorConfig
		wantErr bool
	}{
		{
			name:    "valid",
			cfg:     MonitorConfig{Bus: bus.NewMemoryBus(bus.DefaultConfig())},
			wantErr: false,
		},
		{
			name:    "missing bus",
			cfg:     MonitorConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultMonitorConfig(t *testing.T) {
	cfg := DefaultMonitorConfig()
	if cfg.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want 15s", cfg.Timeout)
	}
	if cfg.CheckInterval != time.Second {
		t.Errorf("CheckInterval = %v, want 1s", cfg.CheckInterval)
	}
}

// --- Integration Tests ---

func TestBusMonitor_Watch(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	monitor, err := NewBusMonitor(MonitorConfig{
		Bus:     msgBus,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewBusMonitor error: %v", err)
	}
	defer monitor.Stop()

	// Watch specific agent
	ch, err := monitor.Watch("agent-1")
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	// Publish heartbeat
	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now(),
		Status:    "idle",
	}
	data, _ := hb.Marshal()
	msgBus.Publish("heartbeat.agent-1", data)

	select {
	case received := <-ch:
		if received.AgentID != "agent-1" {
			t.Errorf("AgentID = %q, want agent-1", received.AgentID)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for heartbeat")
	}
}

func TestBusMonitor_IsAlive(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	monitor, _ := NewBusMonitor(MonitorConfig{
		Bus:     msgBus,
		Timeout: time.Second,
	})
	defer monitor.Stop()

	// Initially not alive
	if monitor.IsAlive("agent-1", time.Second) {
		t.Error("expected agent to not be alive initially")
	}

	// Receive a heartbeat via the watch mechanism
	ch, _ := monitor.Watch("agent-1")

	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now(),
		Status:    "idle",
	}
	data, _ := hb.Marshal()
	msgBus.Publish("heartbeat.agent-1", data)

	// Wait for heartbeat to be processed
	<-ch

	// Should be alive now
	if !monitor.IsAlive("agent-1", time.Second) {
		t.Error("expected agent to be alive after heartbeat")
	}
}

func TestBusMonitor_LastHeartbeat(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	monitor, _ := NewBusMonitor(MonitorConfig{
		Bus:     msgBus,
		Timeout: time.Second,
	})
	defer monitor.Stop()

	ch, _ := monitor.Watch("agent-1")

	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now(),
		Status:    "busy",
		Load:      0.8,
	}
	data, _ := hb.Marshal()
	msgBus.Publish("heartbeat.agent-1", data)

	<-ch

	last := monitor.LastHeartbeat("agent-1")
	if last == nil {
		t.Fatal("expected last heartbeat to exist")
	}
	if last.Status != "busy" {
		t.Errorf("Status = %q, want busy", last.Status)
	}
	if last.Load != 0.8 {
		t.Errorf("Load = %v, want 0.8", last.Load)
	}
}

// --- System Tests: Multiple Agents ---

func TestBusMonitor_MultipleAgents(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	monitor, _ := NewBusMonitor(MonitorConfig{
		Bus:     msgBus,
		Timeout: time.Second,
	})
	defer monitor.Stop()

	// Watch multiple agents
	ch1, _ := monitor.Watch("agent-1")
	ch2, _ := monitor.Watch("agent-2")

	// Send heartbeats
	for _, id := range []string{"agent-1", "agent-2"} {
		hb := &Heartbeat{AgentID: id, Timestamp: time.Now(), Status: "idle"}
		data, _ := hb.Marshal()
		msgBus.Publish("heartbeat."+id, data)
	}

	// Both should receive
	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case hb := <-ch1:
			received[hb.AgentID] = true
		case hb := <-ch2:
			received[hb.AgentID] = true
		case <-time.After(time.Second):
			t.Fatalf("timeout, received: %v", received)
		}
	}

	if !received["agent-1"] || !received["agent-2"] {
		t.Errorf("expected both agents, got: %v", received)
	}
}

// --- Failure Tests: Death Detection ---

func TestMemoryMonitor_DeathDetection(t *testing.T) {
	monitor := NewMemoryMonitor(100 * time.Millisecond)

	var deadAgents []string
	var mu sync.Mutex
	monitor.OnDead(func(agentID string) {
		mu.Lock()
		deadAgents = append(deadAgents, agentID)
		mu.Unlock()
	})

	// Send heartbeat with old timestamp
	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now().Add(-200 * time.Millisecond), // Already expired
		Status:    "idle",
	}
	monitor.Receive(hb)

	// Check for dead agents
	monitor.CheckDead()

	mu.Lock()
	if len(deadAgents) != 1 || deadAgents[0] != "agent-1" {
		t.Errorf("expected [agent-1] dead, got %v", deadAgents)
	}
	mu.Unlock()
}

func TestMemoryMonitor_DeathReportedOnce(t *testing.T) {
	monitor := NewMemoryMonitor(100 * time.Millisecond)

	var count int32
	monitor.OnDead(func(agentID string) {
		atomic.AddInt32(&count, 1)
	})

	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now().Add(-200 * time.Millisecond),
	}
	monitor.Receive(hb)

	// Multiple checks should only report once
	monitor.CheckDead()
	monitor.CheckDead()
	monitor.CheckDead()

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected 1 death callback, got %d", count)
	}
}

func TestMemoryMonitor_Resurrection(t *testing.T) {
	monitor := NewMemoryMonitor(100 * time.Millisecond)

	var deaths int32
	monitor.OnDead(func(agentID string) {
		atomic.AddInt32(&deaths, 1)
	})

	// First death
	oldHB := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now().Add(-200 * time.Millisecond),
	}
	monitor.Receive(oldHB)
	monitor.CheckDead()

	// Resurrection (new heartbeat)
	newHB := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	monitor.Receive(newHB)

	// Should be alive again
	if !monitor.IsAlive("agent-1", time.Second) {
		t.Error("expected agent to be alive after resurrection")
	}

	// Die again (old timestamp)
	oldHB.Timestamp = time.Now().Add(-200 * time.Millisecond)
	monitor.Receive(oldHB)
	monitor.CheckDead()

	// Should have been reported twice
	if atomic.LoadInt32(&deaths) != 2 {
		t.Errorf("expected 2 deaths, got %d", deaths)
	}
}

// --- Performance Tests ---

func BenchmarkBusMonitor_ProcessHeartbeat(b *testing.B) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	monitor, _ := NewBusMonitor(MonitorConfig{
		Bus:     msgBus,
		Timeout: time.Second,
	})
	defer monitor.Stop()

	ch, _ := monitor.Watch("agent-bench")

	// Drain the channel
	go func() {
		for range ch {
		}
	}()

	hb := &Heartbeat{
		AgentID:   "agent-bench",
		Timestamp: time.Now(),
		Status:    "busy",
		Load:      0.75,
	}
	data, _ := hb.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgBus.Publish("heartbeat.agent-bench", data)
	}
}

func BenchmarkMemoryMonitor_IsAlive(b *testing.B) {
	monitor := NewMemoryMonitor(time.Second)

	// Add some agents
	for i := 0; i < 100; i++ {
		hb := &Heartbeat{
			AgentID:   "agent-" + string(rune(i)),
			Timestamp: time.Now(),
		}
		monitor.Receive(hb)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		monitor.IsAlive("agent-A", time.Second)
	}
}

// --- Security Tests ---

func TestHeartbeat_SpoofedAgentID(t *testing.T) {
	// Test that heartbeat carries its own agent ID
	hb := &Heartbeat{
		AgentID:   "legitimate-agent",
		Timestamp: time.Now(),
		Status:    "idle",
	}

	data, _ := hb.Marshal()
	parsed, _ := Unmarshal(data)

	// Verification: the parsed heartbeat should have the original agent ID
	// A spoofed message would need to be detected at a higher level (signing)
	if parsed.AgentID != "legitimate-agent" {
		t.Errorf("AgentID = %q, expected legitimate-agent", parsed.AgentID)
	}
}

func TestHeartbeat_ReplayAttack(t *testing.T) {
	monitor := NewMemoryMonitor(100 * time.Millisecond)

	// Original heartbeat
	originalHB := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now(),
		Status:    "idle",
	}
	monitor.Receive(originalHB)

	if !monitor.IsAlive("agent-1", time.Second) {
		t.Error("expected agent to be alive")
	}

	// Time passes
	time.Sleep(150 * time.Millisecond)

	// Replay the same heartbeat (old timestamp)
	monitor.Receive(originalHB)

	// Agent should still be considered dead because timestamp is old
	if monitor.IsAlive("agent-1", 100*time.Millisecond) {
		t.Error("replayed heartbeat should not extend liveness")
	}
}

func TestHeartbeat_FutureTimestamp(t *testing.T) {
	monitor := NewMemoryMonitor(time.Second)

	// Heartbeat with future timestamp (clock skew attack)
	futureHB := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now().Add(10 * time.Minute),
		Status:    "idle",
	}
	monitor.Receive(futureHB)

	// Future timestamps should be handled - in this case, IsAlive
	// will return true because the timestamp is within the timeout
	// A production system might want to reject future timestamps
	last := monitor.LastHeartbeat("agent-1")
	if last.Timestamp.Before(time.Now()) {
		t.Error("expected future timestamp to be stored")
	}
}

// --- MemoryMonitor Additional Tests ---

func TestMemoryMonitor_Clear(t *testing.T) {
	monitor := NewMemoryMonitor(time.Second)

	hb := &Heartbeat{AgentID: "agent-1", Timestamp: time.Now()}
	monitor.Receive(hb)

	if !monitor.IsAlive("agent-1", time.Second) {
		t.Error("expected agent to be alive before clear")
	}

	monitor.Clear()

	if monitor.IsAlive("agent-1", time.Second) {
		t.Error("expected agent to not be alive after clear")
	}
}

func TestMemoryMonitor_MultipleCallbacks(t *testing.T) {
	monitor := NewMemoryMonitor(100 * time.Millisecond)

	var cb1, cb2 int32
	monitor.OnDead(func(agentID string) { atomic.AddInt32(&cb1, 1) })
	monitor.OnDead(func(agentID string) { atomic.AddInt32(&cb2, 1) })

	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now().Add(-200 * time.Millisecond),
	}
	monitor.Receive(hb)
	monitor.CheckDead()

	if atomic.LoadInt32(&cb1) != 1 || atomic.LoadInt32(&cb2) != 1 {
		t.Errorf("expected both callbacks to fire, got cb1=%d cb2=%d", cb1, cb2)
	}
}

// --- End-to-End Integration Test ---

func TestSenderMonitor_Integration(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	// Create sender
	sender, _ := NewBusSender(SenderConfig{
		Bus:           msgBus,
		AgentID:       "integration-agent",
		Interval:      50 * time.Millisecond,
		InitialStatus: "ready",
	})
	sender.SetLoad(0.5)
	sender.SetMetadata("test", "integration")

	// Create monitor watching specific agent
	monitor, _ := NewBusMonitor(MonitorConfig{
		Bus:     msgBus,
		Timeout: time.Second,
	})

	ch, _ := monitor.Watch("integration-agent")

	// Start sender
	sender.Start(nil)
	defer sender.Stop()

	// Receive heartbeat
	select {
	case hb := <-ch:
		if hb.AgentID != "integration-agent" {
			t.Errorf("AgentID = %q, want integration-agent", hb.AgentID)
		}
		if hb.Status != "ready" {
			t.Errorf("Status = %q, want ready", hb.Status)
		}
		if hb.Load != 0.5 {
			t.Errorf("Load = %v, want 0.5", hb.Load)
		}
		if hb.Metadata["test"] != "integration" {
			t.Errorf("Metadata[test] = %q, want integration", hb.Metadata["test"])
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for heartbeat")
	}

	// Verify IsAlive
	if !monitor.IsAlive("integration-agent", time.Second) {
		t.Error("expected agent to be alive")
	}
}
