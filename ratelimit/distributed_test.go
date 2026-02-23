package ratelimit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

func TestDistributedLimiter_New(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()
}

func TestDistributedLimiter_InvalidConfig(t *testing.T) {
	// Missing bus
	_, err := NewDistributedLimiter(DistributedConfig{
		AgentID: "agent-1",
	})
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}

	// Missing agent ID
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	_, err = NewDistributedLimiter(DistributedConfig{
		Bus: mbus,
	})
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestDistributedLimiter_SetCapacity(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 100, time.Minute)

	cap := limiter.GetCapacity("api")
	if cap == nil {
		t.Fatal("expected capacity, got nil")
	}
	if cap.Total != 100 {
		t.Errorf("expected capacity 100, got %d", cap.Total)
	}
}

func TestDistributedLimiter_AcquireRelease(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 2, time.Minute)

	// Acquire
	if err := limiter.Acquire(context.Background(), "api"); err != nil {
		t.Errorf("Acquire failed: %v", err)
	}

	// TryAcquire
	if !limiter.TryAcquire("api") {
		t.Error("TryAcquire should succeed")
	}

	// Should fail now (capacity exhausted)
	if limiter.TryAcquire("api") {
		t.Error("TryAcquire should fail when capacity exhausted")
	}

	// Release one
	limiter.Release("api")

	cap := limiter.GetCapacity("api")
	if cap.InFlight != 1 {
		t.Errorf("expected inFlight 1, got %d", cap.InFlight)
	}
}

func TestDistributedLimiter_AnnounceReduced(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:          mbus,
		AgentID:      "agent-1",
		ReduceFactor: 0.5,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 100, time.Minute)

	// Announce reduced
	limiter.AnnounceReduced("api", "received 429")

	// Should be reduced to 50%
	cap := limiter.GetCapacity("api")
	if cap.Total != 50 {
		t.Errorf("expected capacity 50, got %d", cap.Total)
	}
}

func TestDistributedLimiter_ReceivesUpdates(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	// Create two limiters
	limiter1, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter1: %v", err)
	}
	defer limiter1.Close()

	limiter2, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-2",
	})
	if err != nil {
		t.Fatalf("failed to create limiter2: %v", err)
	}
	defer limiter2.Close()

	// Both set capacity
	limiter1.SetCapacity("shared-api", 100, time.Minute)
	limiter2.SetCapacity("shared-api", 100, time.Minute)

	// Set up callback on limiter2
	callbackCh := make(chan *CapacityUpdate, 1)
	limiter2.OnCapacityChange(func(update *CapacityUpdate) {
		callbackCh <- update
	})

	// Limiter1 announces reduced capacity
	limiter1.AnnounceReduced("shared-api", "rate limited")

	// Wait for limiter2 to receive the update
	select {
	case update := <-callbackCh:
		if update.AgentID != "agent-1" {
			t.Errorf("expected agent-1, got %s", update.AgentID)
		}
		if update.Resource != "shared-api" {
			t.Errorf("expected shared-api, got %s", update.Resource)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for capacity update")
	}

	// Verify limiter2's capacity was reduced
	time.Sleep(50 * time.Millisecond) // Small delay for processing
	cap := limiter2.GetCapacity("shared-api")
	if cap.Total >= 100 {
		t.Errorf("expected reduced capacity, got %d", cap.Total)
	}
}

func TestDistributedLimiter_IgnoresOwnUpdates(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 100, time.Minute)

	// Simulate receiving own update
	update := CapacityUpdate{
		Resource:    "api",
		AgentID:     "agent-1", // Same as limiter
		NewCapacity: 10,
		Reason:      "test",
		Timestamp:   time.Now(),
	}
	data, _ := json.Marshal(update)
	mbus.Publish(SubjectPrefix+"capacity", data)

	time.Sleep(50 * time.Millisecond)

	// Capacity should NOT be affected
	cap := limiter.GetCapacity("api")
	if cap.Total != 100 {
		t.Errorf("expected capacity 100, got %d", cap.Total)
	}
}

func TestDistributedConfig_Defaults(t *testing.T) {
	config := DefaultDistributedConfig()

	if config.ReduceFactor != 0.5 {
		t.Errorf("expected ReduceFactor 0.5, got %f", config.ReduceFactor)
	}
	if config.RecoveryInterval != 30*time.Second {
		t.Errorf("expected RecoveryInterval 30s, got %v", config.RecoveryInterval)
	}
	if config.RecoveryFactor != 1.1 {
		t.Errorf("expected RecoveryFactor 1.1, got %f", config.RecoveryFactor)
	}
	if !config.MaxRecovery {
		t.Error("expected MaxRecovery true")
	}
}

func TestDistributedLimiter_Recovery(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:              mbus,
		AgentID:          "agent-1",
		ReduceFactor:     0.5,
		RecoveryInterval: 50 * time.Millisecond, // Fast for testing
		RecoveryFactor:   2.0,                   // Double on recovery
		MaxRecovery:      true,
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 100, time.Minute)

	// Reduce capacity
	limiter.AnnounceReduced("api", "test reduction")

	cap := limiter.GetCapacity("api")
	if cap.Total != 50 {
		t.Errorf("expected reduced capacity 50, got %d", cap.Total)
	}

	// Wait for recovery to kick in (at least 2 intervals)
	time.Sleep(150 * time.Millisecond)

	cap = limiter.GetCapacity("api")
	if cap.Total <= 50 {
		t.Errorf("expected capacity to recover above 50, got %d", cap.Total)
	}
	// Should be capped at original (100)
	if cap.Total > 100 {
		t.Errorf("expected capacity capped at 100, got %d", cap.Total)
	}
}

func TestDistributedLimiter_Recovery_NoMaxRecovery(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:              mbus,
		AgentID:          "agent-1",
		ReduceFactor:     0.5,
		RecoveryInterval: 50 * time.Millisecond,
		RecoveryFactor:   3.0,    // Triple on recovery
		MaxRecovery:      false,  // Don't cap
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 10, time.Minute)

	// Reduce capacity
	limiter.AnnounceReduced("api", "test reduction")

	cap := limiter.GetCapacity("api")
	if cap.Total != 5 {
		t.Errorf("expected reduced capacity 5, got %d", cap.Total)
	}

	// Wait for recovery
	time.Sleep(150 * time.Millisecond)

	cap = limiter.GetCapacity("api")
	// With RecoveryFactor 3.0, should go 5 -> 15 after one recovery
	// Without MaxRecovery, can exceed original
	if cap.Total <= 10 {
		t.Errorf("expected capacity > 10, got %d", cap.Total)
	}
}

func TestDistributedLimiter_AnnounceReduced_UnknownResource(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Should not panic on unknown resource
	limiter.AnnounceReduced("unknown-api", "test")
}

func TestDistributedLimiter_AnnounceReduced_MinCapacity(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:          mbus,
		AgentID:      "agent-1",
		ReduceFactor: 0.1, // 10% of 1 = 0, should floor to 1
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 1, time.Minute)
	limiter.AnnounceReduced("api", "test")

	cap := limiter.GetCapacity("api")
	if cap.Total < 1 {
		t.Errorf("expected capacity >= 1, got %d", cap.Total)
	}
}

func TestDistributedLimiter_HandleUpdate_MalformedMessage(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 100, time.Minute)

	// Send malformed message - should be ignored
	mbus.Publish(SubjectPrefix+"capacity", []byte("not valid json"))

	time.Sleep(50 * time.Millisecond)

	// Capacity should be unchanged
	cap := limiter.GetCapacity("api")
	if cap.Total != 100 {
		t.Errorf("expected capacity 100, got %d", cap.Total)
	}
}

func TestDistributedLimiter_HandleUpdate_UnknownResource(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// Don't set capacity for "api"

	// Send update for unknown resource
	update := CapacityUpdate{
		Resource:    "api",
		AgentID:     "agent-2",
		NewCapacity: 50,
		Reason:      "test",
		Timestamp:   time.Now(),
	}
	data, _ := json.Marshal(update)
	mbus.Publish(SubjectPrefix+"capacity", data)

	time.Sleep(50 * time.Millisecond)

	// Should not have created a capacity entry
	cap := limiter.GetCapacity("api")
	if cap != nil {
		t.Error("expected nil capacity for unknown resource")
	}
}

func TestDistributedLimiter_HandleUpdate_HigherCapacity(t *testing.T) {
	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	limiter, err := NewDistributedLimiter(DistributedConfig{
		Bus:     mbus,
		AgentID: "agent-1",
	})
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	limiter.SetCapacity("api", 50, time.Minute)

	// Send update with higher capacity than original - should be ignored
	update := CapacityUpdate{
		Resource:    "api",
		AgentID:     "agent-2",
		NewCapacity: 100, // Higher than original 50
		Reason:      "test",
		Timestamp:   time.Now(),
	}
	data, _ := json.Marshal(update)
	mbus.Publish(SubjectPrefix+"capacity", data)

	time.Sleep(50 * time.Millisecond)

	// Capacity should be unchanged
	cap := limiter.GetCapacity("api")
	if cap.Total != 50 {
		t.Errorf("expected capacity 50, got %d", cap.Total)
	}
}

func TestWithCapacityCallback(t *testing.T) {
	var cfg watchConfig
	cb := func(update *CapacityUpdate) {}

	opt := WithCapacityCallback(cb)
	opt(&cfg)

	if cfg.callback == nil {
		t.Error("expected callback to be set")
	}
}

func TestDistributedConfig_Validate(t *testing.T) {
	// Test explicit Validate calls
	cfg := DistributedConfig{}
	if err := cfg.Validate(); err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig for empty config, got %v", err)
	}

	mbus := bus.NewMemoryBus(bus.DefaultConfig())
	defer mbus.Close()

	cfg = DistributedConfig{Bus: mbus}
	if err := cfg.Validate(); err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig for missing AgentID, got %v", err)
	}

	cfg = DistributedConfig{Bus: mbus, AgentID: "test"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected nil error for valid config, got %v", err)
	}
}
