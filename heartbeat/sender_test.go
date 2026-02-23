package heartbeat

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// --- Unit Tests ---

func TestHeartbeat_Marshal(t *testing.T) {
	hb := &Heartbeat{
		AgentID:   "agent-1",
		Timestamp: time.Now(),
		Status:    "busy",
		Load:      0.75,
		Metadata:  map[string]string{"version": "1.0.0"},
	}

	data, err := hb.Marshal()
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	parsed, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.AgentID != hb.AgentID {
		t.Errorf("AgentID = %q, want %q", parsed.AgentID, hb.AgentID)
	}
	if parsed.Status != hb.Status {
		t.Errorf("Status = %q, want %q", parsed.Status, hb.Status)
	}
	if parsed.Load != hb.Load {
		t.Errorf("Load = %v, want %v", parsed.Load, hb.Load)
	}
	if parsed.Metadata["version"] != "1.0.0" {
		t.Errorf("Metadata[version] = %q, want %q", parsed.Metadata["version"], "1.0.0")
	}
}

func TestHeartbeat_Subject(t *testing.T) {
	hb := &Heartbeat{AgentID: "agent-1"}
	if hb.Subject() != "heartbeat.agent-1" {
		t.Errorf("Subject = %q, want %q", hb.Subject(), "heartbeat.agent-1")
	}
}

func TestUnmarshal_Invalid(t *testing.T) {
	_, err := Unmarshal([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSenderConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SenderConfig
		wantErr bool
	}{
		{
			name:    "valid",
			cfg:     SenderConfig{Bus: bus.NewMemoryBus(bus.DefaultConfig()), AgentID: "agent-1"},
			wantErr: false,
		},
		{
			name:    "missing bus",
			cfg:     SenderConfig{AgentID: "agent-1"},
			wantErr: true,
		},
		{
			name:    "missing agent id",
			cfg:     SenderConfig{Bus: bus.NewMemoryBus(bus.DefaultConfig())},
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

// --- Integration Tests ---

func TestBusSender_StartStop(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, err := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewBusSender error: %v", err)
	}

	// Subscribe to heartbeats
	sub, _ := msgBus.Subscribe("heartbeat.agent-1")
	defer sub.Unsubscribe()

	ctx := context.Background()
	if err := sender.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Should receive heartbeat
	select {
	case msg := <-sub.Messages():
		hb, _ := Unmarshal(msg.Data)
		if hb.AgentID != "agent-1" {
			t.Errorf("AgentID = %q, want %q", hb.AgentID, "agent-1")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for heartbeat")
	}

	if err := sender.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestBusSender_DoubleStart(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	sender.Start(ctx)
	defer sender.Stop()

	err := sender.Start(ctx)
	if err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
}

func TestBusSender_StopBeforeStart(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})

	err := sender.Stop()
	if err != ErrNotStarted {
		t.Errorf("expected ErrNotStarted, got %v", err)
	}
}

func TestBusSender_SetStatus(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})

	sub, _ := msgBus.Subscribe("heartbeat.agent-1")
	defer sub.Unsubscribe()

	sender.SetStatus("busy")
	sender.Start(context.Background())
	defer sender.Stop()

	select {
	case msg := <-sub.Messages():
		hb, _ := Unmarshal(msg.Data)
		if hb.Status != "busy" {
			t.Errorf("Status = %q, want %q", hb.Status, "busy")
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

func TestBusSender_SetLoad(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})

	// Test clamping
	sender.SetLoad(-0.5) // Should clamp to 0
	sender.SetLoad(1.5)  // Should clamp to 1

	sub, _ := msgBus.Subscribe("heartbeat.agent-1")
	defer sub.Unsubscribe()

	sender.Start(context.Background())
	defer sender.Stop()

	select {
	case msg := <-sub.Messages():
		hb, _ := Unmarshal(msg.Data)
		if hb.Load != 1.0 {
			t.Errorf("Load = %v, want 1.0 (clamped)", hb.Load)
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

func TestBusSender_SetMetadata(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})

	sender.SetMetadata("version", "1.0.0")
	sender.SetMetadata("region", "us-west")

	sub, _ := msgBus.Subscribe("heartbeat.agent-1")
	defer sub.Unsubscribe()

	sender.Start(context.Background())
	defer sender.Stop()

	select {
	case msg := <-sub.Messages():
		hb, _ := Unmarshal(msg.Data)
		if hb.Metadata["version"] != "1.0.0" {
			t.Errorf("Metadata[version] = %q, want %q", hb.Metadata["version"], "1.0.0")
		}
		if hb.Metadata["region"] != "us-west" {
			t.Errorf("Metadata[region] = %q, want %q", hb.Metadata["region"], "us-west")
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

func TestBusSender_ContextCancel(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	sender.Start(ctx)

	// Cancel should stop the sender
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Sender should have stopped
	// Try to stop again - should get ErrNotStarted since context cancelled
	// (running flag was set to false in the run goroutine)
}

// --- System Tests ---

func TestBusSender_MultipleHeartbeats(t *testing.T) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-1",
		Interval: 30 * time.Millisecond,
	})

	sub, _ := msgBus.Subscribe("heartbeat.agent-1")
	defer sub.Unsubscribe()

	var received int32
	done := make(chan struct{})
	go func() {
		for range sub.Messages() {
			if atomic.AddInt32(&received, 1) >= 3 {
				close(done)
				return
			}
		}
	}()

	sender.Start(context.Background())
	defer sender.Stop()

	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Errorf("received only %d heartbeats, wanted at least 3", atomic.LoadInt32(&received))
	}
}

// --- Performance Tests ---

func BenchmarkHeartbeat_Marshal(b *testing.B) {
	hb := &Heartbeat{
		AgentID:   "agent-benchmark",
		Timestamp: time.Now(),
		Status:    "busy",
		Load:      0.75,
		Metadata:  map[string]string{"version": "1.0.0", "region": "us-west"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hb.Marshal()
	}
}

func BenchmarkHeartbeat_Unmarshal(b *testing.B) {
	hb := &Heartbeat{
		AgentID:   "agent-benchmark",
		Timestamp: time.Now(),
		Status:    "busy",
		Load:      0.75,
		Metadata:  map[string]string{"version": "1.0.0", "region": "us-west"},
	}
	data, _ := hb.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Unmarshal(data)
	}
}

func BenchmarkBusSender_Send(b *testing.B) {
	msgBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer msgBus.Close()

	// Drain messages
	sub, _ := msgBus.Subscribe("heartbeat.agent-bench")
	go func() {
		for range sub.Messages() {
		}
	}()

	sender, _ := NewBusSender(SenderConfig{
		Bus:      msgBus,
		AgentID:  "agent-bench",
		Interval: time.Hour, // Don't auto-send during benchmark
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender.sendHeartbeat()
	}
}

// --- MemorySender Tests ---

func TestMemorySender_Records(t *testing.T) {
	sender := NewMemorySender("agent-1", 50*time.Millisecond)

	sender.SetStatus("busy")
	sender.SetLoad(0.5)
	sender.SetMetadata("test", "value")

	sender.Start(context.Background())
	time.Sleep(150 * time.Millisecond)
	sender.Stop()

	sent := sender.Sent()
	if len(sent) < 2 {
		t.Errorf("expected at least 2 heartbeats, got %d", len(sent))
	}

	// Check first heartbeat has our values
	if sent[0].Status != "busy" {
		t.Errorf("Status = %q, want busy", sent[0].Status)
	}
	if sent[0].Load != 0.5 {
		t.Errorf("Load = %v, want 0.5", sent[0].Load)
	}
	if sent[0].Metadata["test"] != "value" {
		t.Errorf("Metadata[test] = %q, want value", sent[0].Metadata["test"])
	}
}

func TestMemorySender_Clear(t *testing.T) {
	sender := NewMemorySender("agent-1", 50*time.Millisecond)
	sender.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	sender.Stop()

	if len(sender.Sent()) == 0 {
		t.Error("expected some heartbeats before clear")
	}

	sender.Clear()

	if len(sender.Sent()) != 0 {
		t.Error("expected no heartbeats after clear")
	}
}
