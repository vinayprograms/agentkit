// Package main demonstrates a swarm monitor that watches for agent registration events
// and detects dead agents based on missed heartbeats.
//
// The monitor uses agentkit's registry Watch() to receive real-time events when agents
// join, update, or leave the swarm.
//
// Run: go run monitor.go
// Then start agents: go run agent.go agent-1
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/vinayprograms/agentkit/bus"
	"github.com/vinayprograms/agentkit/registry"
)

const (
	SubjectHeartbeat   = "swarm.heartbeat"
	SubjectAgentStatus = "swarm.status"
)

// HeartbeatMessage matches the agent's broadcast format.
type HeartbeatMessage struct {
	AgentID   string    `json:"agent_id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Load      float64   `json:"load"`
	Timestamp time.Time `json:"timestamp"`
}

// AgentState tracks the last known state of an agent.
type AgentState struct {
	ID           string
	Name         string
	Status       string
	Load         float64
	LastSeen     time.Time
	LastHeartbeat time.Time
	Alive        bool
}

// Monitor watches the swarm for agent events.
type Monitor struct {
	bus      bus.MessageBus
	registry registry.Registry

	mu     sync.RWMutex
	agents map[string]*AgentState

	deadThreshold time.Duration // How long before considering agent dead
}

// NewMonitor creates a new swarm monitor.
func NewMonitor(b bus.MessageBus, reg registry.Registry, deadThreshold time.Duration) *Monitor {
	return &Monitor{
		bus:           b,
		registry:      reg,
		agents:        make(map[string]*AgentState),
		deadThreshold: deadThreshold,
	}
}

// Start begins monitoring the swarm.
func (m *Monitor) Start(ctx context.Context) error {
	log.Println("Swarm Monitor starting...")
	log.Printf("Dead threshold: %v", m.deadThreshold)

	// Load existing agents from registry
	m.loadExistingAgents()

	// Watch registry for events
	go m.watchRegistry(ctx)

	// Subscribe to heartbeat messages
	go m.watchHeartbeats(ctx)

	// Subscribe to status announcements
	go m.watchStatus(ctx)

	// Periodic dead agent check
	go m.deadAgentChecker(ctx)

	// Periodic status display
	go m.statusDisplay(ctx)

	<-ctx.Done()
	return nil
}

// loadExistingAgents populates state from current registry.
func (m *Monitor) loadExistingAgents() {
	agents, err := m.registry.List(nil)
	if err != nil {
		log.Printf("Failed to list agents: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, agent := range agents {
		m.agents[agent.ID] = &AgentState{
			ID:       agent.ID,
			Name:     agent.Name,
			Status:   string(agent.Status),
			Load:     agent.Load,
			LastSeen: agent.LastSeen,
			Alive:    true,
		}
	}

	if len(agents) > 0 {
		log.Printf("Loaded %d existing agents from registry", len(agents))
	}
}

// watchRegistry listens for registry events.
func (m *Monitor) watchRegistry(ctx context.Context) {
	events, err := m.registry.Watch()
	if err != nil {
		log.Printf("Failed to watch registry: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			m.handleRegistryEvent(event)
		}
	}
}

// handleRegistryEvent processes a registry event.
func (m *Monitor) handleRegistryEvent(event registry.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent := event.Agent

	switch event.Type {
	case registry.EventAdded:
		m.agents[agent.ID] = &AgentState{
			ID:       agent.ID,
			Name:     agent.Name,
			Status:   string(agent.Status),
			Load:     agent.Load,
			LastSeen: agent.LastSeen,
			Alive:    true,
		}
		log.Printf("🟢 JOINED: %s (capabilities: %v)", agent.Name, agent.Capabilities)

	case registry.EventUpdated:
		if state, ok := m.agents[agent.ID]; ok {
			state.Status = string(agent.Status)
			state.Load = agent.Load
			state.LastSeen = agent.LastSeen
			state.Alive = true
		}

	case registry.EventRemoved:
		if state, ok := m.agents[agent.ID]; ok {
			state.Alive = false
			log.Printf("🔴 LEFT: %s", agent.Name)
		}
		delete(m.agents, agent.ID)
	}
}

// watchHeartbeats subscribes to heartbeat messages.
func (m *Monitor) watchHeartbeats(ctx context.Context) {
	sub, err := m.bus.Subscribe(SubjectHeartbeat)
	if err != nil {
		log.Printf("Failed to subscribe to heartbeats: %v", err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Messages():
			if !ok {
				return
			}
			m.handleHeartbeat(msg)
		}
	}
}

// handleHeartbeat processes a heartbeat message.
func (m *Monitor) handleHeartbeat(msg *bus.Message) {
	var hb HeartbeatMessage
	if err := json.Unmarshal(msg.Data, &hb); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.agents[hb.AgentID]
	if !ok {
		// Agent not in registry yet - might be race condition
		state = &AgentState{
			ID:   hb.AgentID,
			Name: hb.Name,
		}
		m.agents[hb.AgentID] = state
	}

	// Check if agent was marked dead and is now alive again
	wasAlive := state.Alive
	state.Status = hb.Status
	state.Load = hb.Load
	state.LastHeartbeat = hb.Timestamp
	state.Alive = true

	if !wasAlive {
		log.Printf("🟡 REVIVED: %s (was thought dead)", hb.Name)
	}
}

// watchStatus subscribes to status announcements.
func (m *Monitor) watchStatus(ctx context.Context) {
	sub, err := m.bus.Subscribe(SubjectAgentStatus)
	if err != nil {
		log.Printf("Failed to subscribe to status: %v", err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Messages():
			if !ok {
				return
			}
			var status HeartbeatMessage
			if err := json.Unmarshal(msg.Data, &status); err == nil {
				log.Printf("📢 STATUS: %s -> %s", status.Name, status.Status)
			}
		}
	}
}

// deadAgentChecker periodically checks for agents that stopped sending heartbeats.
func (m *Monitor) deadAgentChecker(ctx context.Context) {
	ticker := time.NewTicker(m.deadThreshold / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkDeadAgents()
		}
	}
}

// checkDeadAgents marks agents as dead if they haven't sent heartbeats.
func (m *Monitor) checkDeadAgents() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, state := range m.agents {
		if !state.Alive {
			continue
		}

		// Use LastHeartbeat if available, otherwise LastSeen
		lastContact := state.LastHeartbeat
		if lastContact.IsZero() {
			lastContact = state.LastSeen
		}

		if !lastContact.IsZero() && now.Sub(lastContact) > m.deadThreshold {
			state.Alive = false
			log.Printf("💀 DEAD: %s (no heartbeat for %v)", state.Name, now.Sub(lastContact).Round(time.Second))

			// Remove from registry (cleanup)
			go m.registry.Deregister(id)
		}
	}
}

// statusDisplay periodically prints swarm status.
func (m *Monitor) statusDisplay(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.printStatus()
		}
	}
}

// printStatus shows current swarm state.
func (m *Monitor) printStatus() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.agents) == 0 {
		log.Println("📊 Swarm Status: No agents registered")
		return
	}

	// Sort agents by name
	var ids []string
	for id := range m.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var alive, dead int
	for _, id := range ids {
		if m.agents[id].Alive {
			alive++
		} else {
			dead++
		}
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  SWARM STATUS: %d alive, %d dead                              \n", alive, dead)
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  %-12s │ %-8s │ %-6s │ %-20s ║\n", "AGENT", "STATUS", "LOAD", "LAST HEARTBEAT")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")

	for _, id := range ids {
		state := m.agents[id]
		status := state.Status
		icon := "🟢"
		if !state.Alive {
			status = "DEAD"
			icon = "💀"
		}

		lastHB := "never"
		if !state.LastHeartbeat.IsZero() {
			ago := time.Since(state.LastHeartbeat).Round(time.Second)
			lastHB = fmt.Sprintf("%v ago", ago)
		}

		fmt.Printf("║ %s %-10s │ %-8s │ %5.1f%% │ %-20s ║\n",
			icon, truncate(state.Name, 10), truncate(status, 8), state.Load*100, lastHB)
	}
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// truncate limits string length.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func main() {
	var (
		deadThreshold = flag.Duration("dead", 30*time.Second, "Time without heartbeat before agent considered dead")
		ttl           = flag.Duration("ttl", 15*time.Second, "Registry TTL")
	)
	flag.Parse()

	// Create in-memory bus and registry
	memBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer memBus.Close()

	memReg := registry.NewMemoryRegistry(registry.MemoryConfig{TTL: *ttl})
	defer memReg.Close()

	monitor := NewMonitor(memBus, memReg, *deadThreshold)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down monitor...")
		cancel()
	}()

	log.Println("╔════════════════════════════════════════╗")
	log.Println("║        SWARM HEARTBEAT MONITOR         ║")
	log.Println("╚════════════════════════════════════════╝")

	if err := monitor.Start(ctx); err != nil {
		log.Printf("Monitor error: %v", err)
		os.Exit(1)
	}

	log.Println("Monitor stopped")
}
