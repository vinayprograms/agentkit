package registry

import (
	"sort"
	"sync"
	"time"
)

// MemoryRegistry is an in-memory implementation of Registry.
// Suitable for testing and single-node deployments.
type MemoryRegistry struct {
	mu       sync.RWMutex
	agents   map[string]AgentInfo
	watchers []chan Event
	closed   bool

	// TTL for stale entry detection. Zero means no expiry.
	ttl time.Duration
}

// MemoryConfig configures the in-memory registry.
type MemoryConfig struct {
	// TTL specifies how long before an agent is considered stale.
	// Zero means entries never expire.
	TTL time.Duration
}

// NewMemoryRegistry creates a new in-memory registry.
func NewMemoryRegistry(cfg MemoryConfig) *MemoryRegistry {
	r := &MemoryRegistry{
		agents:   make(map[string]AgentInfo),
		watchers: make([]chan Event, 0),
		ttl:      cfg.TTL,
	}

	// Start cleanup goroutine if TTL is set
	if cfg.TTL > 0 {
		go r.cleanupLoop()
	}

	return r
}

// Register adds or updates an agent in the registry.
func (r *MemoryRegistry) Register(info AgentInfo) error {
	if err := ValidateAgentInfo(info); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrClosed
	}

	// Set LastSeen to now
	info.LastSeen = time.Now()

	_, exists := r.agents[info.ID]
	r.agents[info.ID] = info

	// Notify watchers
	eventType := EventAdded
	if exists {
		eventType = EventUpdated
	}
	r.notifyWatchers(Event{Type: eventType, Agent: info})

	return nil
}

// Deregister removes an agent from the registry.
func (r *MemoryRegistry) Deregister(id string) error {
	if id == "" {
		return ErrInvalidID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrClosed
	}

	agent, exists := r.agents[id]
	if !exists {
		return ErrNotFound
	}

	delete(r.agents, id)
	r.notifyWatchers(Event{Type: EventRemoved, Agent: agent})

	return nil
}

// Get retrieves a specific agent by ID.
func (r *MemoryRegistry) Get(id string) (*AgentInfo, error) {
	if id == "" {
		return nil, ErrInvalidID
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrClosed
	}

	agent, exists := r.agents[id]
	if !exists {
		return nil, ErrNotFound
	}

	// Check if stale
	if r.ttl > 0 && time.Since(agent.LastSeen) > r.ttl {
		return nil, ErrNotFound
	}

	return &agent, nil
}

// List returns all agents matching the filter.
func (r *MemoryRegistry) List(filter *Filter) ([]AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrClosed
	}

	var result []AgentInfo
	now := time.Now()

	for _, agent := range r.agents {
		// Skip stale entries
		if r.ttl > 0 && now.Sub(agent.LastSeen) > r.ttl {
			continue
		}

		if MatchesFilter(agent, filter) {
			result = append(result, agent)
		}
	}

	// Sort by ID for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// FindByCapability returns agents with a specific capability.
func (r *MemoryRegistry) FindByCapability(capability string) ([]AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrClosed
	}

	var result []AgentInfo
	now := time.Now()

	for _, agent := range r.agents {
		// Skip stale entries
		if r.ttl > 0 && now.Sub(agent.LastSeen) > r.ttl {
			continue
		}

		if HasCapability(agent, capability) {
			result = append(result, agent)
		}
	}

	// Sort by load (lowest first) for load balancing
	sort.Slice(result, func(i, j int) bool {
		return result[i].Load < result[j].Load
	})

	return result, nil
}

// Watch returns a channel of registry events.
func (r *MemoryRegistry) Watch() (<-chan Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrClosed
	}

	ch := make(chan Event, 64)
	r.watchers = append(r.watchers, ch)

	return ch, nil
}

// Close shuts down the registry.
func (r *MemoryRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	// Close all watcher channels
	for _, ch := range r.watchers {
		close(ch)
	}
	r.watchers = nil

	return nil
}

// notifyWatchers sends an event to all watchers.
// Must be called with lock held.
func (r *MemoryRegistry) notifyWatchers(event Event) {
	for _, ch := range r.watchers {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
}

// cleanupLoop periodically removes stale entries.
func (r *MemoryRegistry) cleanupLoop() {
	ticker := time.NewTicker(r.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return
		}

		now := time.Now()
		var stale []string

		for id, agent := range r.agents {
			if now.Sub(agent.LastSeen) > r.ttl {
				stale = append(stale, id)
			}
		}

		for _, id := range stale {
			agent := r.agents[id]
			delete(r.agents, id)
			r.notifyWatchers(Event{Type: EventRemoved, Agent: agent})
		}

		r.mu.Unlock()
	}
}
