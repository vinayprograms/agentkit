package heartbeat

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// BusMonitor monitors heartbeats over a message bus.
type BusMonitor struct {
	bus           bus.MessageBus
	timeout       time.Duration
	checkInterval time.Duration

	mu         sync.RWMutex
	lastSeen   map[string]*Heartbeat
	deadCBs    []func(string)
	reported   map[string]bool // Track already-reported dead agents
	watcherChs []chan *Heartbeat

	running    atomic.Bool
	sub        bus.Subscription
	stopCh     chan struct{}
	doneCh     chan struct{}
}

// NewBusMonitor creates a new heartbeat monitor.
func NewBusMonitor(cfg MonitorConfig) (*BusMonitor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultMonitorConfig().Timeout
	}

	checkInterval := cfg.CheckInterval
	if checkInterval <= 0 {
		checkInterval = DefaultMonitorConfig().CheckInterval
	}

	return &BusMonitor{
		bus:           cfg.Bus,
		timeout:       timeout,
		checkInterval: checkInterval,
		lastSeen:      make(map[string]*Heartbeat),
		reported:      make(map[string]bool),
	}, nil
}

// Watch returns a channel of heartbeats for a specific agent.
func (m *BusMonitor) Watch(agentID string) (<-chan *Heartbeat, error) {
	subject := SubjectPrefix + agentID
	sub, err := m.bus.Subscribe(subject)
	if err != nil {
		return nil, err
	}

	ch := make(chan *Heartbeat, 16)
	go m.forwardMessages(sub, ch)
	return ch, nil
}

// WatchAll returns a channel of all heartbeats and starts monitoring.
func (m *BusMonitor) WatchAll() (<-chan *Heartbeat, error) {
	if m.running.Swap(true) {
		// Already running, just add a watcher
		ch := make(chan *Heartbeat, 64)
		m.mu.Lock()
		m.watcherChs = append(m.watcherChs, ch)
		m.mu.Unlock()
		return ch, nil
	}

	// Subscribe to all heartbeats (wildcard)
	// Note: MemoryBus doesn't support wildcards, so we use a workaround
	// In production with NATS, use "heartbeat.>"
	sub, err := m.bus.Subscribe(SubjectPrefix + "*")
	if err != nil {
		// Try without wildcard for MemoryBus compatibility
		sub, err = m.bus.Subscribe("heartbeat.all")
		if err != nil {
			m.running.Store(false)
			return nil, err
		}
	}
	m.sub = sub

	ch := make(chan *Heartbeat, 64)
	m.watcherChs = append(m.watcherChs, ch)

	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})

	go m.run()
	return ch, nil
}

// run processes incoming heartbeats and checks for dead agents.
func (m *BusMonitor) run() {
	defer close(m.doneCh)

	checkTicker := time.NewTicker(m.checkInterval)
	defer checkTicker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case msg, ok := <-m.sub.Messages():
			if !ok {
				return
			}
			m.processMessage(msg)
		case <-checkTicker.C:
			m.checkDeadAgents()
		}
	}
}

// processMessage handles an incoming heartbeat message.
func (m *BusMonitor) processMessage(msg *bus.Message) {
	hb, err := Unmarshal(msg.Data)
	if err != nil {
		return
	}

	// Extract agent ID from subject if not in payload
	if hb.AgentID == "" && strings.HasPrefix(msg.Subject, SubjectPrefix) {
		hb.AgentID = strings.TrimPrefix(msg.Subject, SubjectPrefix)
	}

	m.mu.Lock()
	m.lastSeen[hb.AgentID] = hb
	delete(m.reported, hb.AgentID) // Agent is alive, clear dead report
	watchers := make([]chan *Heartbeat, len(m.watcherChs))
	copy(watchers, m.watcherChs)
	m.mu.Unlock()

	// Notify watchers
	for _, ch := range watchers {
		select {
		case ch <- hb:
		default:
			// Buffer full, drop
		}
	}
}

// checkDeadAgents checks for agents that haven't sent heartbeats.
func (m *BusMonitor) checkDeadAgents() {
	now := time.Now()
	var deadAgents []string

	m.mu.RLock()
	for agentID, hb := range m.lastSeen {
		if now.Sub(hb.Timestamp) > m.timeout && !m.reported[agentID] {
			deadAgents = append(deadAgents, agentID)
		}
	}
	callbacks := make([]func(string), len(m.deadCBs))
	copy(callbacks, m.deadCBs)
	m.mu.RUnlock()

	// Mark as reported and invoke callbacks
	if len(deadAgents) > 0 {
		m.mu.Lock()
		for _, id := range deadAgents {
			m.reported[id] = true
		}
		m.mu.Unlock()

		for _, agentID := range deadAgents {
			for _, cb := range callbacks {
				cb(agentID)
			}
		}
	}
}

// forwardMessages forwards subscription messages to a heartbeat channel.
func (m *BusMonitor) forwardMessages(sub bus.Subscription, ch chan *Heartbeat) {
	defer close(ch)
	for msg := range sub.Messages() {
		hb, err := Unmarshal(msg.Data)
		if err != nil {
			continue
		}

		// Update last seen
		m.mu.Lock()
		m.lastSeen[hb.AgentID] = hb
		m.mu.Unlock()

		select {
		case ch <- hb:
		default:
		}
	}
}

// IsAlive checks if an agent has sent a heartbeat within timeout.
func (m *BusMonitor) IsAlive(agentID string, timeout time.Duration) bool {
	m.mu.RLock()
	hb, ok := m.lastSeen[agentID]
	m.mu.RUnlock()

	if !ok {
		return false
	}
	return time.Since(hb.Timestamp) <= timeout
}

// LastHeartbeat returns the last heartbeat from an agent.
func (m *BusMonitor) LastHeartbeat(agentID string) *Heartbeat {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSeen[agentID]
}

// OnDead registers a callback for when an agent is presumed dead.
func (m *BusMonitor) OnDead(callback func(agentID string)) {
	m.mu.Lock()
	m.deadCBs = append(m.deadCBs, callback)
	m.mu.Unlock()
}

// Stop stops monitoring.
func (m *BusMonitor) Stop() error {
	if !m.running.Swap(false) {
		return ErrNotStarted
	}

	if m.sub != nil {
		m.sub.Unsubscribe()
	}

	close(m.stopCh)
	<-m.doneCh

	// Close watcher channels
	m.mu.Lock()
	for _, ch := range m.watcherChs {
		close(ch)
	}
	m.watcherChs = nil
	m.mu.Unlock()

	return nil
}

// MemoryMonitor is a test implementation for monitoring heartbeats.
type MemoryMonitor struct {
	mu       sync.RWMutex
	lastSeen map[string]*Heartbeat
	deadCBs  []func(string)
	reported map[string]bool
	timeout  time.Duration
}

// NewMemoryMonitor creates a monitor for testing.
func NewMemoryMonitor(timeout time.Duration) *MemoryMonitor {
	if timeout <= 0 {
		timeout = DefaultMonitorConfig().Timeout
	}
	return &MemoryMonitor{
		lastSeen: make(map[string]*Heartbeat),
		reported: make(map[string]bool),
		timeout:  timeout,
	}
}

// Receive processes a heartbeat (for testing).
func (m *MemoryMonitor) Receive(hb *Heartbeat) {
	m.mu.Lock()
	m.lastSeen[hb.AgentID] = hb
	delete(m.reported, hb.AgentID)
	m.mu.Unlock()
}

// Watch is not implemented for MemoryMonitor.
func (m *MemoryMonitor) Watch(agentID string) (<-chan *Heartbeat, error) {
	ch := make(chan *Heartbeat)
	close(ch)
	return ch, nil
}

// WatchAll is not implemented for MemoryMonitor.
func (m *MemoryMonitor) WatchAll() (<-chan *Heartbeat, error) {
	ch := make(chan *Heartbeat)
	close(ch)
	return ch, nil
}

func (m *MemoryMonitor) IsAlive(agentID string, timeout time.Duration) bool {
	m.mu.RLock()
	hb, ok := m.lastSeen[agentID]
	m.mu.RUnlock()

	if !ok {
		return false
	}
	return time.Since(hb.Timestamp) <= timeout
}

func (m *MemoryMonitor) LastHeartbeat(agentID string) *Heartbeat {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSeen[agentID]
}

func (m *MemoryMonitor) OnDead(callback func(agentID string)) {
	m.mu.Lock()
	m.deadCBs = append(m.deadCBs, callback)
	m.mu.Unlock()
}

// CheckDead checks for dead agents (call periodically in tests).
func (m *MemoryMonitor) CheckDead() {
	now := time.Now()
	var deadAgents []string

	m.mu.RLock()
	for agentID, hb := range m.lastSeen {
		if now.Sub(hb.Timestamp) > m.timeout && !m.reported[agentID] {
			deadAgents = append(deadAgents, agentID)
		}
	}
	callbacks := make([]func(string), len(m.deadCBs))
	copy(callbacks, m.deadCBs)
	m.mu.RUnlock()

	if len(deadAgents) > 0 {
		m.mu.Lock()
		for _, id := range deadAgents {
			m.reported[id] = true
		}
		m.mu.Unlock()

		for _, agentID := range deadAgents {
			for _, cb := range callbacks {
				cb(agentID)
			}
		}
	}
}

func (m *MemoryMonitor) Stop() error {
	return nil
}

// Clear resets the monitor state.
func (m *MemoryMonitor) Clear() {
	m.mu.Lock()
	m.lastSeen = make(map[string]*Heartbeat)
	m.reported = make(map[string]bool)
	m.mu.Unlock()
}
