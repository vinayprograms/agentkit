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
