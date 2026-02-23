package ratelimit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// DistributedConfig configures a distributed rate limiter.
type DistributedConfig struct {
	// Bus is the message bus for coordination.
	Bus bus.MessageBus

	// AgentID is the unique identifier for this agent.
	AgentID string

	// ReduceFactor is the multiplier when reducing capacity (0-1).
	// Default: 0.5 (reduce by 50%)
	ReduceFactor float64

	// RecoveryInterval is how often to attempt capacity recovery.
	// Default: 30 seconds
	RecoveryInterval time.Duration

	// RecoveryFactor is the multiplier when recovering capacity (>1).
	// Default: 1.1 (increase by 10%)
	RecoveryFactor float64

	// MaxRecovery caps recovery at original capacity.
	// Default: true
	MaxRecovery bool
}

// Validate checks the configuration.
func (c *DistributedConfig) Validate() error {
	if c.Bus == nil {
		return ErrInvalidConfig
	}
	if c.AgentID == "" {
		return ErrInvalidConfig
	}
	return nil
}

// DefaultDistributedConfig returns configuration with sensible defaults.
func DefaultDistributedConfig() DistributedConfig {
	return DistributedConfig{
		ReduceFactor:     0.5,
		RecoveryInterval: 30 * time.Second,
		RecoveryFactor:   1.1,
		MaxRecovery:      true,
	}
}

// resourceConfig tracks per-resource configuration.
type resourceConfig struct {
	originalCapacity int           // initial capacity before reductions
	window           time.Duration // refill window
}

// DistributedLimiter coordinates rate limits across a swarm via message bus.
type DistributedLimiter struct {
	config DistributedConfig

	// local is the underlying memory limiter
	local *MemoryLimiter

	// originalCapacities tracks original settings for recovery
	mu                 sync.RWMutex
	resourceConfigs    map[string]*resourceConfig
	lastReduction      map[string]time.Time
	onCapacityCallback OnCapacityChange

	// subscription for capacity updates
	sub      bus.Subscription
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewDistributedLimiter creates a new distributed rate limiter.
func NewDistributedLimiter(config DistributedConfig) (*DistributedLimiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Apply defaults
	if config.ReduceFactor == 0 {
		config.ReduceFactor = DefaultDistributedConfig().ReduceFactor
	}
	if config.RecoveryInterval == 0 {
		config.RecoveryInterval = DefaultDistributedConfig().RecoveryInterval
	}
	if config.RecoveryFactor == 0 {
		config.RecoveryFactor = DefaultDistributedConfig().RecoveryFactor
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &DistributedLimiter{
		config:          config,
		local:           NewMemoryLimiter(),
		resourceConfigs: make(map[string]*resourceConfig),
		lastReduction:   make(map[string]time.Time),
		ctx:             ctx,
		cancel:          cancel,
	}

	// Subscribe to capacity updates (use specific subject, not wildcard)
	sub, err := config.Bus.Subscribe(SubjectPrefix + "capacity")
	if err != nil {
		cancel()
		return nil, err
	}
	d.sub = sub

	// Start listener goroutine
	d.wg.Add(1)
	go d.listenForUpdates()

	// Start recovery goroutine
	d.wg.Add(1)
	go d.recoveryLoop()

	return d, nil
}

// listenForUpdates processes capacity updates from other agents.
func (d *DistributedLimiter) listenForUpdates() {
	defer d.wg.Done()

	for {
		select {
		case <-d.ctx.Done():
			return
		case msg, ok := <-d.sub.Messages():
			if !ok {
				return
			}
			d.handleUpdate(msg)
		}
	}
}

// handleUpdate processes a single capacity update message.
func (d *DistributedLimiter) handleUpdate(msg *bus.Message) {
	var update CapacityUpdate
	if err := json.Unmarshal(msg.Data, &update); err != nil {
		return // Ignore malformed messages
	}

	// Ignore our own updates
	if update.AgentID == d.config.AgentID {
		return
	}

	// Apply the reduced capacity locally
	d.mu.Lock()
	rc, exists := d.resourceConfigs[update.Resource]
	if exists && update.NewCapacity < rc.originalCapacity {
		d.local.SetCapacity(update.Resource, update.NewCapacity, rc.window)
		d.lastReduction[update.Resource] = time.Now()
	}
	callback := d.onCapacityCallback
	d.mu.Unlock()

	// Notify callback if set
	if callback != nil {
		callback(&update)
	}
}

// recoveryLoop periodically attempts to recover capacity.
func (d *DistributedLimiter) recoveryLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.attemptRecovery()
		}
	}
}

// attemptRecovery tries to gradually restore reduced capacity.
func (d *DistributedLimiter) attemptRecovery() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	for resource, lastReduce := range d.lastReduction {
		// Wait at least one recovery interval before recovering
		if now.Sub(lastReduce) < d.config.RecoveryInterval {
			continue
		}

		rc, exists := d.resourceConfigs[resource]
		if !exists {
			continue
		}

		cap := d.local.GetCapacity(resource)
		if cap == nil {
			continue
		}

		// Gradually increase capacity
		newCapacity := int(float64(cap.Total) * d.config.RecoveryFactor)
		if d.config.MaxRecovery && newCapacity > rc.originalCapacity {
			newCapacity = rc.originalCapacity
		}

		if newCapacity > cap.Total {
			d.local.SetCapacity(resource, newCapacity, rc.window)
		}

		// If we've recovered to original, stop tracking
		if newCapacity >= rc.originalCapacity {
			delete(d.lastReduction, resource)
		}
	}
}

// SetCapacity configures the rate limit for a resource.
func (d *DistributedLimiter) SetCapacity(resource string, capacity int, window time.Duration) {
	d.mu.Lock()
	d.resourceConfigs[resource] = &resourceConfig{
		originalCapacity: capacity,
		window:           window,
	}
	d.mu.Unlock()

	d.local.SetCapacity(resource, capacity, window)
}

// GetCapacity returns the current capacity info for a resource.
func (d *DistributedLimiter) GetCapacity(resource string) *Capacity {
	return d.local.GetCapacity(resource)
}

// Acquire blocks until a token is available for the resource.
func (d *DistributedLimiter) Acquire(ctx context.Context, resource string) error {
	return d.local.Acquire(ctx, resource)
}

// TryAcquire attempts to acquire a token without blocking.
func (d *DistributedLimiter) TryAcquire(resource string) bool {
	return d.local.TryAcquire(resource)
}

// Release returns a token to the resource bucket.
func (d *DistributedLimiter) Release(resource string) {
	d.local.Release(resource)
}

// AnnounceReduced broadcasts that capacity should be reduced.
func (d *DistributedLimiter) AnnounceReduced(resource string, reason string) {
	d.mu.Lock()
	rc, exists := d.resourceConfigs[resource]
	if !exists {
		d.mu.Unlock()
		return
	}

	// Calculate reduced capacity
	cap := d.local.GetCapacity(resource)
	if cap == nil {
		d.mu.Unlock()
		return
	}

	newCapacity := int(float64(cap.Total) * d.config.ReduceFactor)
	if newCapacity < 1 {
		newCapacity = 1
	}

	// Apply locally
	d.local.SetCapacity(resource, newCapacity, rc.window)
	d.lastReduction[resource] = time.Now()
	d.mu.Unlock()

	// Broadcast to other agents
	update := CapacityUpdate{
		Resource:    resource,
		AgentID:     d.config.AgentID,
		NewCapacity: newCapacity,
		Reason:      reason,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(update)
	if err != nil {
		return
	}

	// Publish to the capacity subject
	_ = d.config.Bus.Publish(SubjectPrefix+"capacity", data)
}

// OnCapacityChange sets a callback for capacity change notifications.
func (d *DistributedLimiter) OnCapacityChange(cb OnCapacityChange) {
	d.mu.Lock()
	d.onCapacityCallback = cb
	d.mu.Unlock()
}

// Close shuts down the limiter.
func (d *DistributedLimiter) Close() error {
	d.cancel()

	if d.sub != nil {
		_ = d.sub.Unsubscribe()
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Timeout waiting for goroutines
	}

	return d.local.Close()
}

// Ensure DistributedLimiter implements RateLimiter.
var _ RateLimiter = (*DistributedLimiter)(nil)
