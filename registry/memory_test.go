package registry

import (
	"sync"
	"testing"
	"time"
)

// --- Unit Tests ---

func TestMemoryRegistry_Register(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	info := AgentInfo{
		ID:           "agent-1",
		Name:         "Test Agent",
		Capabilities: []string{"code-review", "testing"},
		Status:       StatusIdle,
		Load:         0.5,
	}

	err := r.Register(info)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Verify registration
	got, err := r.Get("agent-1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if got.Name != "Test Agent" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Agent")
	}
	if got.Load != 0.5 {
		t.Errorf("Load = %v, want %v", got.Load, 0.5)
	}
	if got.LastSeen.IsZero() {
		t.Error("LastSeen should be set")
	}
}

func TestMemoryRegistry_RegisterUpdate(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	// Initial registration
	r.Register(AgentInfo{ID: "agent-1", Status: StatusIdle, Load: 0.2})

	// Update
	r.Register(AgentInfo{ID: "agent-1", Status: StatusBusy, Load: 0.8})

	got, _ := r.Get("agent-1")
	if got.Status != StatusBusy {
		t.Errorf("Status = %v, want %v", got.Status, StatusBusy)
	}
	if got.Load != 0.8 {
		t.Errorf("Load = %v, want %v", got.Load, 0.8)
	}
}

func TestMemoryRegistry_RegisterInvalid(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	// Empty ID
	err := r.Register(AgentInfo{ID: ""})
	if err != ErrInvalidID {
		t.Errorf("expected ErrInvalidID, got %v", err)
	}

	// Invalid load
	err = r.Register(AgentInfo{ID: "agent-1", Load: 1.5})
	if err == nil {
		t.Error("expected error for load > 1.0")
	}

	err = r.Register(AgentInfo{ID: "agent-1", Load: -0.1})
	if err == nil {
		t.Error("expected error for load < 0.0")
	}
}

func TestMemoryRegistry_Deregister(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	r.Register(AgentInfo{ID: "agent-1"})

	err := r.Deregister("agent-1")
	if err != nil {
		t.Fatalf("Deregister error: %v", err)
	}

	_, err = r.Get("agent-1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryRegistry_DeregisterNotFound(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	err := r.Deregister("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryRegistry_List(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	r.Register(AgentInfo{ID: "agent-1", Status: StatusIdle, Load: 0.1})
	r.Register(AgentInfo{ID: "agent-2", Status: StatusBusy, Load: 0.8})
	r.Register(AgentInfo{ID: "agent-3", Status: StatusIdle, Load: 0.3})

	// List all
	agents, _ := r.List(nil)
	if len(agents) != 3 {
		t.Errorf("List() returned %d agents, want 3", len(agents))
	}

	// Filter by status
	agents, _ = r.List(&Filter{Status: StatusIdle})
	if len(agents) != 2 {
		t.Errorf("List(idle) returned %d agents, want 2", len(agents))
	}

	// Filter by max load
	agents, _ = r.List(&Filter{MaxLoad: 0.5})
	if len(agents) != 2 {
		t.Errorf("List(maxLoad=0.5) returned %d agents, want 2", len(agents))
	}
}

func TestMemoryRegistry_FindByCapability(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	r.Register(AgentInfo{
		ID:           "agent-1",
		Capabilities: []string{"code-review", "testing"},
		Load:         0.8,
	})
	r.Register(AgentInfo{
		ID:           "agent-2",
		Capabilities: []string{"code-review"},
		Load:         0.2,
	})
	r.Register(AgentInfo{
		ID:           "agent-3",
		Capabilities: []string{"deployment"},
		Load:         0.5,
	})

	// Find code-review capable agents
	agents, _ := r.FindByCapability("code-review")
	if len(agents) != 2 {
		t.Fatalf("FindByCapability returned %d agents, want 2", len(agents))
	}

	// Should be sorted by load
	if agents[0].ID != "agent-2" {
		t.Errorf("First agent should be agent-2 (lowest load), got %s", agents[0].ID)
	}
}

// --- Integration Tests ---

func TestMemoryRegistry_Watch(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	watch, err := r.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	// Register triggers event
	r.Register(AgentInfo{ID: "agent-1"})

	select {
	case event := <-watch:
		if event.Type != EventAdded {
			t.Errorf("Type = %v, want %v", event.Type, EventAdded)
		}
		if event.Agent.ID != "agent-1" {
			t.Errorf("Agent.ID = %q, want %q", event.Agent.ID, "agent-1")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for event")
	}

	// Update triggers event
	r.Register(AgentInfo{ID: "agent-1", Status: StatusBusy})

	select {
	case event := <-watch:
		if event.Type != EventUpdated {
			t.Errorf("Type = %v, want %v", event.Type, EventUpdated)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for update event")
	}

	// Deregister triggers event
	r.Deregister("agent-1")

	select {
	case event := <-watch:
		if event.Type != EventRemoved {
			t.Errorf("Type = %v, want %v", event.Type, EventRemoved)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for remove event")
	}
}

func TestMemoryRegistry_MultipleWatchers(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	watch1, _ := r.Watch()
	watch2, _ := r.Watch()

	r.Register(AgentInfo{ID: "agent-1"})

	// Both should receive the event
	for i, watch := range []<-chan Event{watch1, watch2} {
		select {
		case event := <-watch:
			if event.Agent.ID != "agent-1" {
				t.Errorf("watcher %d: Agent.ID = %q, want %q", i, event.Agent.ID, "agent-1")
			}
		case <-time.After(time.Second):
			t.Errorf("watcher %d: timeout waiting for event", i)
		}
	}
}

// --- System Tests ---

func TestMemoryRegistry_ConcurrentAccess(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	var wg sync.WaitGroup
	numAgents := 100

	// Concurrent registrations
	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r.Register(AgentInfo{
				ID:     string(rune('a' + id%26)) + string(rune('0'+id/26)),
				Status: StatusIdle,
			})
		}(i)
	}

	wg.Wait()

	agents, _ := r.List(nil)
	if len(agents) != numAgents {
		t.Errorf("List() returned %d agents, want %d", len(agents), numAgents)
	}
}

func TestMemoryRegistry_TTLExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TTL test in short mode")
	}

	r := NewMemoryRegistry(MemoryConfig{TTL: 100 * time.Millisecond})
	defer r.Close()

	watch, _ := r.Watch()

	r.Register(AgentInfo{ID: "agent-1"})

	// Wait for expiry
	time.Sleep(200 * time.Millisecond)

	// Should not be found
	_, err := r.Get("agent-1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after TTL, got %v", err)
	}

	// Should have received removal event
	select {
	case event := <-watch:
		// Skip the added event
		if event.Type == EventAdded {
			select {
			case event = <-watch:
			case <-time.After(500 * time.Millisecond):
				t.Error("timeout waiting for removal event")
			}
		}
		if event.Type != EventRemoved {
			t.Errorf("Type = %v, want %v", event.Type, EventRemoved)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for removal event")
	}
}

// --- Performance Tests ---

func BenchmarkMemoryRegistry_Register(b *testing.B) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	info := AgentInfo{
		ID:           "agent-bench",
		Capabilities: []string{"cap1", "cap2", "cap3"},
		Status:       StatusIdle,
		Load:         0.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Register(info)
	}
}

func BenchmarkMemoryRegistry_FindByCapability(b *testing.B) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		caps := []string{"common"}
		if i%10 == 0 {
			caps = append(caps, "rare")
		}
		r.Register(AgentInfo{
			ID:           string(rune(i)),
			Capabilities: caps,
			Load:         float64(i%100) / 100.0,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.FindByCapability("common")
	}
}

// --- Failure Tests ---

func TestMemoryRegistry_OperationsAfterClose(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	r.Close()

	// Register after close
	err := r.Register(AgentInfo{ID: "agent-1"})
	if err != ErrClosed {
		t.Errorf("Register: expected ErrClosed, got %v", err)
	}

	// Get after close
	_, err = r.Get("agent-1")
	if err != ErrClosed {
		t.Errorf("Get: expected ErrClosed, got %v", err)
	}

	// List after close
	_, err = r.List(nil)
	if err != ErrClosed {
		t.Errorf("List: expected ErrClosed, got %v", err)
	}

	// Watch after close
	_, err = r.Watch()
	if err != ErrClosed {
		t.Errorf("Watch: expected ErrClosed, got %v", err)
	}
}

func TestMemoryRegistry_WatchChannelClosedOnClose(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	watch, _ := r.Watch()

	r.Close()

	// Channel should be closed
	select {
	case _, ok := <-watch:
		if ok {
			t.Error("channel should be closed")
		}
	case <-time.After(time.Second):
		t.Error("timeout - channel not closed")
	}
}

// --- Security Tests ---

func TestMemoryRegistry_InvalidData(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	tests := []struct {
		name string
		info AgentInfo
	}{
		{"empty ID", AgentInfo{ID: ""}},
		{"negative load", AgentInfo{ID: "a", Load: -1}},
		{"load > 1", AgentInfo{ID: "a", Load: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.Register(tt.info)
			if err == nil {
				t.Error("expected error for invalid data")
			}
		})
	}
}

func TestMemoryRegistry_CapabilityInjection(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	// Register with suspicious capability names
	r.Register(AgentInfo{
		ID:           "agent-1",
		Capabilities: []string{"code-review", "admin;drop table", "../../../etc/passwd"},
	})

	// Should find by exact match only
	agents, _ := r.FindByCapability("admin;drop table")
	if len(agents) != 1 {
		t.Errorf("exact match should work, got %d agents", len(agents))
	}

	// Should not find partial match
	agents, _ = r.FindByCapability("admin")
	if len(agents) != 0 {
		t.Errorf("partial match should not work, got %d agents", len(agents))
	}
}

// Test helper functions
func TestHasCapability(t *testing.T) {
	agent := AgentInfo{
		Capabilities: []string{"a", "b", "c"},
	}

	if !HasCapability(agent, "b") {
		t.Error("HasCapability should return true for existing cap")
	}
	if HasCapability(agent, "d") {
		t.Error("HasCapability should return false for missing cap")
	}
}

func TestMatchesFilter(t *testing.T) {
	agent := AgentInfo{
		Status:       StatusIdle,
		Capabilities: []string{"test"},
		Load:         0.5,
	}

	// No filter
	if !MatchesFilter(agent, nil) {
		t.Error("nil filter should match")
	}

	// Status match
	if !MatchesFilter(agent, &Filter{Status: StatusIdle}) {
		t.Error("matching status should match")
	}
	if MatchesFilter(agent, &Filter{Status: StatusBusy}) {
		t.Error("non-matching status should not match")
	}

	// Capability match
	if !MatchesFilter(agent, &Filter{Capability: "test"}) {
		t.Error("matching capability should match")
	}
	if MatchesFilter(agent, &Filter{Capability: "other"}) {
		t.Error("non-matching capability should not match")
	}

	// Load filter
	if !MatchesFilter(agent, &Filter{MaxLoad: 0.6}) {
		t.Error("load under max should match")
	}
	if MatchesFilter(agent, &Filter{MaxLoad: 0.4}) {
		t.Error("load over max should not match")
	}
}

// --- Additional Coverage Tests ---

func TestMemoryRegistry_DeregisterEmptyID(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	err := r.Deregister("")
	if err != ErrInvalidID {
		t.Errorf("Deregister empty ID: expected ErrInvalidID, got %v", err)
	}
}

func TestMemoryRegistry_DeregisterAfterClose(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	r.Register(AgentInfo{ID: "agent-1"})
	r.Close()

	err := r.Deregister("agent-1")
	if err != ErrClosed {
		t.Errorf("Deregister after close: expected ErrClosed, got %v", err)
	}
}

func TestMemoryRegistry_GetEmptyID(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	_, err := r.Get("")
	if err != ErrInvalidID {
		t.Errorf("Get empty ID: expected ErrInvalidID, got %v", err)
	}
}

func TestMemoryRegistry_GetStaleEntry(t *testing.T) {
	// Use TTL > 0 but don't rely on cleanup loop timing
	// Instead, directly modify LastSeen to simulate a stale entry
	r := NewMemoryRegistry(MemoryConfig{TTL: 10 * time.Second}) // Long TTL to avoid cleanup
	defer r.Close()

	r.Register(AgentInfo{ID: "agent-1"})

	// Manually make the entry stale by setting LastSeen to the past
	r.mu.Lock()
	if agent, ok := r.agents["agent-1"]; ok {
		agent.LastSeen = time.Now().Add(-20 * time.Second) // 20s ago, way past TTL
		r.agents["agent-1"] = agent
	}
	r.mu.Unlock()

	// Get should return ErrNotFound for stale entry (TTL check in Get)
	_, err := r.Get("agent-1")
	if err != ErrNotFound {
		t.Errorf("Get stale entry: expected ErrNotFound, got %v", err)
	}
}

func TestMemoryRegistry_ListWithStaleEntries(t *testing.T) {
	// Use long TTL to avoid cleanup interference
	r := NewMemoryRegistry(MemoryConfig{TTL: 10 * time.Second})
	defer r.Close()

	// Register agents
	r.Register(AgentInfo{ID: "agent-1"})
	r.Register(AgentInfo{ID: "agent-2"})

	// Make agent-1 stale by setting LastSeen to the past
	r.mu.Lock()
	if agent, ok := r.agents["agent-1"]; ok {
		agent.LastSeen = time.Now().Add(-20 * time.Second) // 20s ago, past TTL
		r.agents["agent-1"] = agent
	}
	r.mu.Unlock()

	// List should only return fresh agent (stale entries filtered in List)
	agents, err := r.List(nil)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("List returned %d agents, want 1 (only fresh)", len(agents))
	}
	if len(agents) == 1 && agents[0].ID != "agent-2" {
		t.Errorf("List returned agent %s, want agent-2", agents[0].ID)
	}
}

func TestMemoryRegistry_FindByCapabilityAfterClose(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	r.Register(AgentInfo{ID: "agent-1", Capabilities: []string{"test"}})
	r.Close()

	_, err := r.FindByCapability("test")
	if err != ErrClosed {
		t.Errorf("FindByCapability after close: expected ErrClosed, got %v", err)
	}
}

func TestMemoryRegistry_FindByCapabilityWithStaleEntries(t *testing.T) {
	// Use long TTL to avoid cleanup interference
	r := NewMemoryRegistry(MemoryConfig{TTL: 10 * time.Second})
	defer r.Close()

	// Register agents with same capability
	r.Register(AgentInfo{ID: "agent-1", Capabilities: []string{"test"}, Load: 0.3})
	r.Register(AgentInfo{ID: "agent-2", Capabilities: []string{"test"}, Load: 0.5})

	// Make agent-1 stale by setting LastSeen to the past
	r.mu.Lock()
	if agent, ok := r.agents["agent-1"]; ok {
		agent.LastSeen = time.Now().Add(-20 * time.Second) // 20s ago, past TTL
		r.agents["agent-1"] = agent
	}
	r.mu.Unlock()

	// FindByCapability should only return fresh agent (stale entries filtered)
	agents, err := r.FindByCapability("test")
	if err != nil {
		t.Fatalf("FindByCapability error: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("FindByCapability returned %d agents, want 1 (only fresh)", len(agents))
	}
	if len(agents) == 1 && agents[0].ID != "agent-2" {
		t.Errorf("FindByCapability returned agent %s, want agent-2", agents[0].ID)
	}
}

func TestMemoryRegistry_DoubleClose(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})

	// First close
	err := r.Close()
	if err != nil {
		t.Errorf("First close: unexpected error %v", err)
	}

	// Second close should return nil (not panic or error)
	err = r.Close()
	if err != nil {
		t.Errorf("Second close: unexpected error %v", err)
	}
}

func TestMemoryRegistry_WatcherChannelOverflow(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	watch, err := r.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	// Fill the channel buffer (64) without reading
	for i := 0; i < 100; i++ {
		r.Register(AgentInfo{ID: "agent-overflow"})
	}

	// Verify we can still drain some events
	eventCount := 0
	timeout := time.After(100 * time.Millisecond)
drainLoop:
	for {
		select {
		case <-watch:
			eventCount++
		case <-timeout:
			break drainLoop
		}
	}

	// Channel should have received up to 64 events (buffer size)
	if eventCount == 0 {
		t.Error("Expected at least some events in the channel")
	}
	if eventCount > 64 {
		t.Errorf("Channel received %d events, but buffer is only 64", eventCount)
	}
}

func TestMemoryRegistry_CleanupLoopRemovesStaleEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup loop test in short mode")
	}

	// Create registry with TTL - cleanup runs at TTL/2 interval
	r := NewMemoryRegistry(MemoryConfig{TTL: 100 * time.Millisecond})
	defer r.Close()

	watch, _ := r.Watch()

	r.Register(AgentInfo{ID: "agent-stale"})

	// Drain added event
	select {
	case <-watch:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for added event")
	}

	// Wait for TTL + cleanup interval (TTL/2)
	time.Sleep(200 * time.Millisecond)

	// Entry should be removed by cleanup loop
	agents, _ := r.List(nil)
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents after cleanup, got %d", len(agents))
	}

	// Should receive removal event from cleanup
	select {
	case event := <-watch:
		if event.Type != EventRemoved {
			t.Errorf("Expected EventRemoved, got %v", event.Type)
		}
		if event.Agent.ID != "agent-stale" {
			t.Errorf("Expected agent-stale, got %s", event.Agent.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for removal event from cleanup")
	}
}

func TestMemoryRegistry_ListEmptyRegistry(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	agents, err := r.List(nil)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("List on empty registry returned %d agents, want 0", len(agents))
	}
}

func TestMemoryRegistry_FindByCapabilityEmpty(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	agents, err := r.FindByCapability("nonexistent")
	if err != nil {
		t.Fatalf("FindByCapability error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("FindByCapability on empty registry returned %d agents, want 0", len(agents))
	}
}

func TestMemoryRegistry_ConcurrentReadWrite(t *testing.T) {
	r := NewMemoryRegistry(MemoryConfig{})
	defer r.Close()

	var wg sync.WaitGroup

	// Start readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.List(nil)
				r.FindByCapability("test")
				r.Get("agent-1")
			}
		}()
	}

	// Start writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Register(AgentInfo{
					ID:           "agent-1",
					Capabilities: []string{"test"},
					Load:         float64(j%100) / 100.0,
				})
			}
		}(i)
	}

	wg.Wait()
}
