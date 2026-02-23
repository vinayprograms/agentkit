// Package main demonstrates a self-registering agent with heartbeat using agentkit's registry.
//
// Each agent registers itself with capabilities and metadata, then periodically
// sends heartbeats to keep its registration alive. If heartbeats stop, the monitor
// detects the agent as dead.
//
// Run: go run agent.go agent-1
// Monitor: go run monitor.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vinayprograms/agentkit/bus"
	"github.com/vinayprograms/agentkit/registry"
)

const (
	// SubjectHeartbeat is used to broadcast heartbeat notifications
	SubjectHeartbeat = "swarm.heartbeat"
	// SubjectAgentStatus is used for status announcements
	SubjectAgentStatus = "swarm.status"
)

// HeartbeatMessage is broadcast by agents on each heartbeat.
type HeartbeatMessage struct {
	AgentID   string    `json:"agent_id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Load      float64   `json:"load"`
	Timestamp time.Time `json:"timestamp"`
}

// Agent represents a self-registering swarm agent.
type Agent struct {
	id           string
	name         string
	capabilities []string
	bus          bus.MessageBus
	registry     registry.Registry

	// Configurable intervals
	heartbeatInterval time.Duration
	registryTTL       time.Duration
}

// AgentConfig holds agent configuration.
type AgentConfig struct {
	ID                string
	Name              string
	Capabilities      []string
	HeartbeatInterval time.Duration
	RegistryTTL       time.Duration
}

// NewAgent creates a new swarm agent.
func NewAgent(cfg AgentConfig, b bus.MessageBus, reg registry.Registry) *Agent {
	return &Agent{
		id:                cfg.ID,
		name:              cfg.Name,
		capabilities:      cfg.Capabilities,
		bus:               b,
		registry:          reg,
		heartbeatInterval: cfg.HeartbeatInterval,
		registryTTL:       cfg.RegistryTTL,
	}
}

// Start begins the agent lifecycle: register, heartbeat, and handle shutdown.
func (a *Agent) Start(ctx context.Context) error {
	// Initial registration
	if err := a.register(registry.StatusRunning); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}
	log.Printf("[%s] Registered with capabilities: %v", a.name, a.capabilities)

	// Update to idle after startup
	time.Sleep(100 * time.Millisecond)
	if err := a.register(registry.StatusIdle); err != nil {
		log.Printf("[%s] Warning: failed to update status: %v", a.name, err)
	}

	// Start heartbeat loop
	go a.heartbeatLoop(ctx)

	// Simulate work loop (optional - demonstrates status changes)
	go a.workLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Printf("[%s] Shutting down...", a.name)
	a.announceStatus("leaving")
	a.registry.Deregister(a.id)

	return nil
}

// register updates the agent's registry entry.
func (a *Agent) register(status registry.Status) error {
	info := registry.AgentInfo{
		ID:           a.id,
		Name:         a.name,
		Capabilities: a.capabilities,
		Status:       status,
		Load:         0,
		Metadata: map[string]string{
			"started":  time.Now().Format(time.RFC3339),
			"version":  "1.0.0",
			"pid":      fmt.Sprintf("%d", os.Getpid()),
			"hostname": getHostname(),
		},
	}
	return a.registry.Register(info)
}

// heartbeatLoop periodically refreshes the registry entry and broadcasts heartbeat.
func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	heartbeatCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatCount++

			// Refresh registry entry (prevents TTL expiry)
			info, err := a.registry.Get(a.id)
			if err != nil {
				log.Printf("[%s] Registry entry lost, re-registering...", a.name)
				a.register(registry.StatusIdle)
			} else {
				// Just refresh LastSeen by re-registering
				a.registry.Register(*info)
			}

			// Broadcast heartbeat to bus
			a.broadcastHeartbeat(info)

			if heartbeatCount%5 == 0 {
				log.Printf("[%s] Heartbeat #%d sent", a.name, heartbeatCount)
			}
		}
	}
}

// broadcastHeartbeat publishes a heartbeat message to the bus.
func (a *Agent) broadcastHeartbeat(info *registry.AgentInfo) {
	status := "idle"
	load := 0.0
	if info != nil {
		status = string(info.Status)
		load = info.Load
	}

	msg := HeartbeatMessage{
		AgentID:   a.id,
		Name:      a.name,
		Status:    status,
		Load:      load,
		Timestamp: time.Now(),
	}

	data, _ := marshalJSON(msg)
	a.bus.Publish(SubjectHeartbeat, data)
}

// announceStatus publishes a status change.
func (a *Agent) announceStatus(status string) {
	msg := HeartbeatMessage{
		AgentID:   a.id,
		Name:      a.name,
		Status:    status,
		Timestamp: time.Now(),
	}
	data, _ := marshalJSON(msg)
	a.bus.Publish(SubjectAgentStatus, data)
}

// workLoop simulates the agent doing work (status changes).
func (a *Agent) workLoop(ctx context.Context) {
	// Random work intervals
	for {
		// Wait random time before "starting work"
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(5+rand.Intn(10)) * time.Second):
		}

		// Simulate busy period
		if err := a.setStatus(registry.StatusBusy, 0.5+rand.Float64()*0.5); err == nil {
			log.Printf("[%s] Working...", a.name)

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(2+rand.Intn(5)) * time.Second):
			}

			a.setStatus(registry.StatusIdle, 0)
			log.Printf("[%s] Work complete, idle", a.name)
		}
	}
}

// setStatus updates the agent's status in the registry.
func (a *Agent) setStatus(status registry.Status, load float64) error {
	info, err := a.registry.Get(a.id)
	if err != nil {
		return err
	}
	info.Status = status
	info.Load = load
	return a.registry.Register(*info)
}

func main() {
	// Parse command-line flags
	var (
		agentName = flag.String("name", "", "Agent name (default: agent-<random>)")
		interval  = flag.Duration("interval", 5*time.Second, "Heartbeat interval")
		ttl       = flag.Duration("ttl", 15*time.Second, "Registry TTL")
	)
	flag.Parse()

	// Determine agent name from args or flag
	name := *agentName
	if name == "" && len(flag.Args()) > 0 {
		name = flag.Args()[0]
	}
	if name == "" {
		name = fmt.Sprintf("agent-%04d", rand.Intn(10000))
	}

	// Create in-memory bus and registry
	// NOTE: For distributed swarms, use NATS:
	//   natsBus, _ := bus.NewNATSBus("nats://localhost:4222", bus.DefaultConfig())
	//   natsReg, _ := registry.NewNATSRegistry(nc, registry.NATSConfig{...})
	memBus := bus.NewMemoryBus(bus.DefaultConfig())
	defer memBus.Close()

	memReg := registry.NewMemoryRegistry(registry.MemoryConfig{TTL: *ttl})
	defer memReg.Close()

	// Create agent
	agent := NewAgent(AgentConfig{
		ID:                name,
		Name:              name,
		Capabilities:      []string{"worker", "compute"},
		HeartbeatInterval: *interval,
		RegistryTTL:       *ttl,
	}, memBus, memReg)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start agent in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Start(ctx)
	}()

	log.Printf("Agent '%s' started (heartbeat: %v, TTL: %v)", name, *interval, *ttl)
	log.Println("Press Ctrl+C to stop")

	// Wait for signal or error
	select {
	case sig := <-sigCh:
		log.Printf("Received signal: %v", sig)
		cancel()
	case err := <-errCh:
		if err != nil {
			log.Printf("Agent error: %v", err)
		}
	}

	// Brief wait for cleanup
	time.Sleep(200 * time.Millisecond)
	log.Println("Agent stopped")
}

// Helper functions

func getHostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown"
}

func marshalJSON(v interface{}) ([]byte, error) {
	// Inline JSON marshal to avoid import
	// In production, use encoding/json
	switch m := v.(type) {
	case HeartbeatMessage:
		return []byte(fmt.Sprintf(
			`{"agent_id":"%s","name":"%s","status":"%s","load":%.2f,"timestamp":"%s"}`,
			m.AgentID, m.Name, m.Status, m.Load, m.Timestamp.Format(time.RFC3339),
		)), nil
	default:
		return nil, fmt.Errorf("unsupported type")
	}
}
