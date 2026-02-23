package registry

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// getNATSConn returns a NATS connection for testing, or skips the test.
func getNATSConn(t *testing.T) *nats.Conn {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}

	if testing.Short() {
		t.Skip("skipping NATS test in short mode")
	}

	conn, err := nats.Connect(url,
		nats.Timeout(2*time.Second),
		nats.MaxReconnects(0),
	)
	if err != nil {
		t.Skipf("skipping: NATS not available at %s: %v", url, err)
	}

	return conn
}

// uniqueBucket generates a unique bucket name for test isolation.
func uniqueBucket() string {
	return "test-" + time.Now().Format("150405") + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
}

// --- Integration Tests ---

func TestNATSRegistry_Register(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	info := AgentInfo{
		ID:           "agent-1",
		Name:         "Test Agent",
		Capabilities: []string{"code-review", "testing"},
		Status:       StatusIdle,
		Load:         0.5,
		Metadata:     map[string]string{"version": "1.0"},
	}

	err = r.Register(info)
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
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities = %v, want 2 items", got.Capabilities)
	}
}

func TestNATSRegistry_RegisterUpdate(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
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

func TestNATSRegistry_Deregister(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	r.Register(AgentInfo{ID: "agent-1"})

	err = r.Deregister("agent-1")
	if err != nil {
		t.Fatalf("Deregister error: %v", err)
	}

	_, err = r.Get("agent-1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNATSRegistry_DeregisterNotFound(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	err = r.Deregister("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNATSRegistry_List(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
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

func TestNATSRegistry_FindByCapability(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
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

func TestNATSRegistry_Watch(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	watch, err := r.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

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
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for event")
	}
}

// --- System Tests ---

func TestNATSRegistry_ConcurrentAccess(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	var wg sync.WaitGroup
	numAgents := 50

	// Concurrent registrations
	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r.Register(AgentInfo{
				ID:     string(rune('a'+id%26)) + string(rune('0'+id/26)),
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

func TestNATSRegistry_MultipleClients(t *testing.T) {
	conn1 := getNATSConn(t)
	defer conn1.Close()

	conn2 := getNATSConn(t)
	defer conn2.Close()

	bucket := uniqueBucket()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = bucket

	r1, err := NewNATSRegistry(conn1, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry r1 error: %v", err)
	}
	defer r1.Close()

	r2, err := NewNATSRegistry(conn2, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry r2 error: %v", err)
	}
	defer r2.Close()

	// Register via r1
	r1.Register(AgentInfo{ID: "agent-1", Status: StatusIdle})

	// Small delay for propagation
	time.Sleep(50 * time.Millisecond)

	// Should be visible from r2
	agent, err := r2.Get("agent-1")
	if err != nil {
		t.Fatalf("r2.Get error: %v", err)
	}
	if agent.ID != "agent-1" {
		t.Errorf("agent.ID = %q, want %q", agent.ID, "agent-1")
	}
}

// --- Performance Tests ---

func BenchmarkNATSRegistry_Register(b *testing.B) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		b.Skip("NATS_URL not set")
	}

	conn, err := nats.Connect(url)
	if err != nil {
		b.Skipf("NATS not available: %v", err)
	}
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = "bench-registry-" + time.Now().Format("150405")

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		b.Fatalf("NewNATSRegistry error: %v", err)
	}
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

func BenchmarkNATSRegistry_Get(b *testing.B) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		b.Skip("NATS_URL not set")
	}

	conn, err := nats.Connect(url)
	if err != nil {
		b.Skipf("NATS not available: %v", err)
	}
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = "bench-registry-get-" + time.Now().Format("150405")

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		b.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	r.Register(AgentInfo{ID: "agent-bench", Status: StatusIdle})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Get("agent-bench")
	}
}

// --- Failure Tests ---

func TestNATSRegistry_NilConnection(t *testing.T) {
	_, err := NewNATSRegistry(nil, DefaultNATSRegistryConfig())
	if err == nil {
		t.Error("expected error for nil connection")
	}
}

func TestNATSRegistry_OperationsAfterClose(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}

	r.Close()

	// Register after close
	err = r.Register(AgentInfo{ID: "agent-1"})
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

func TestNATSRegistry_WatchChannelClosedOnClose(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}

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

func TestNATSRegistry_InvalidData(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
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

func TestNATSRegistry_CapabilityInjection(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
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

// --- Additional Coverage Tests ---

func TestNATSRegistry_DeregisterEmptyID(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	err = r.Deregister("")
	if err != ErrInvalidID {
		t.Errorf("Deregister empty ID: expected ErrInvalidID, got %v", err)
	}
}

func TestNATSRegistry_DeregisterAfterClose(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}

	r.Register(AgentInfo{ID: "agent-1"})
	r.Close()

	err = r.Deregister("agent-1")
	if err != ErrClosed {
		t.Errorf("Deregister after close: expected ErrClosed, got %v", err)
	}
}

func TestNATSRegistry_GetEmptyID(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	_, err = r.Get("")
	if err != ErrInvalidID {
		t.Errorf("Get empty ID: expected ErrInvalidID, got %v", err)
	}
}

func TestNATSRegistry_FindByCapabilityAfterClose(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}

	r.Register(AgentInfo{ID: "agent-1", Capabilities: []string{"test"}})
	r.Close()

	_, err = r.FindByCapability("test")
	if err != ErrClosed {
		t.Errorf("FindByCapability after close: expected ErrClosed, got %v", err)
	}
}

func TestNATSRegistry_Conn(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	// Conn() should return the underlying connection
	gotConn := r.Conn()
	if gotConn != conn {
		t.Error("Conn() should return the underlying NATS connection")
	}
}

func TestNATSRegistry_ConfigDefaults(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	// Test that empty config values get defaults applied
	cfg := NATSRegistryConfig{
		BucketName: uniqueBucket(), // Use unique bucket for isolation
		Replicas:   0,              // Should default to 1
	}

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	// Should work with default-applied config
	err = r.Register(AgentInfo{ID: "agent-1"})
	if err != nil {
		t.Errorf("Register with default config failed: %v", err)
	}
}

func TestNATSRegistry_ListEmpty(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	// List on empty registry should return empty slice, not error
	agents, err := r.List(nil)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("List on empty registry returned %d agents, want 0", len(agents))
	}
}

func TestNATSRegistry_FindByCapabilityEmpty(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	// FindByCapability on empty registry should return empty slice
	agents, err := r.FindByCapability("nonexistent")
	if err != nil {
		t.Fatalf("FindByCapability error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("FindByCapability on empty registry returned %d agents, want 0", len(agents))
	}
}

func TestNATSRegistry_WatchEvents(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	watch, err := r.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Update triggers EventUpdated
	r.Register(AgentInfo{ID: "agent-update"})

	select {
	case event := <-watch:
		if event.Type != EventAdded {
			t.Errorf("First register: Type = %v, want %v", event.Type, EventAdded)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for first event")
	}

	// Update the same agent
	r.Register(AgentInfo{ID: "agent-update", Status: StatusBusy})

	select {
	case event := <-watch:
		if event.Type != EventUpdated {
			t.Errorf("Update: Type = %v, want %v", event.Type, EventUpdated)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for update event")
	}

	// Delete triggers EventRemoved
	r.Deregister("agent-update")

	select {
	case event := <-watch:
		if event.Type != EventRemoved {
			t.Errorf("Delete: Type = %v, want %v", event.Type, EventRemoved)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for remove event")
	}
}

func TestNATSRegistry_MultipleWatchers(t *testing.T) {
	conn := getNATSConn(t)
	defer conn.Close()

	cfg := DefaultNATSRegistryConfig()
	cfg.BucketName = uniqueBucket()

	r, err := NewNATSRegistry(conn, cfg)
	if err != nil {
		t.Fatalf("NewNATSRegistry error: %v", err)
	}
	defer r.Close()

	watch1, _ := r.Watch()
	watch2, _ := r.Watch()

	time.Sleep(100 * time.Millisecond)

	r.Register(AgentInfo{ID: "agent-1"})

	// Both should receive the event
	for i, watch := range []<-chan Event{watch1, watch2} {
		select {
		case event := <-watch:
			if event.Agent.ID != "agent-1" {
				t.Errorf("watcher %d: Agent.ID = %q, want %q", i, event.Agent.ID, "agent-1")
			}
		case <-time.After(2 * time.Second):
			t.Errorf("watcher %d: timeout waiting for event", i)
		}
	}
}

func TestDefaultNATSRegistryConfig(t *testing.T) {
	cfg := DefaultNATSRegistryConfig()

	if cfg.BucketName != "agent-registry" {
		t.Errorf("BucketName = %q, want %q", cfg.BucketName, "agent-registry")
	}
	if cfg.TTL != 30*time.Second {
		t.Errorf("TTL = %v, want %v", cfg.TTL, 30*time.Second)
	}
	if cfg.Replicas != 1 {
		t.Errorf("Replicas = %d, want %d", cfg.Replicas, 1)
	}
}
