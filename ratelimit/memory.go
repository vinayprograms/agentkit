package ratelimit

import (
	"context"
	"sync"
	"time"
)

// bucket implements a token bucket rate limiter.
type bucket struct {
	capacity   int           // maximum tokens
	available  int           // current tokens
	window     time.Duration // refill window
	lastRefill time.Time     // last refill time
	inFlight   int           // requests in progress
	waiters    []chan struct{} // channels waiting for tokens
	cond       *sync.Cond    // condition variable for waiting
}

// refill adds tokens based on elapsed time since last refill.
// Returns true if tokens were added.
func (b *bucket) refill(now time.Time) bool {
	if b.window == 0 || b.capacity == 0 {
		return false
	}

	elapsed := now.Sub(b.lastRefill)
	if elapsed <= 0 {
		return false
	}

	// Calculate tokens to add based on elapsed time
	// rate = capacity / window
	tokensToAdd := int(float64(b.capacity) * float64(elapsed) / float64(b.window))
	if tokensToAdd > 0 {
		b.available += tokensToAdd
		if b.available > b.capacity {
			b.available = b.capacity
		}
		b.lastRefill = now
		return true
	}
	return false
}

// MemoryLimiter provides local rate limiting using token buckets.
// It is safe for concurrent use.
type MemoryLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	closed  bool
	nowFunc func() time.Time // for testing
}

// NewMemoryLimiter creates a new in-memory rate limiter.
func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{
		buckets: make(map[string]*bucket),
		nowFunc: time.Now,
	}
}

// SetCapacity configures the rate limit for a resource.
func (m *MemoryLimiter) SetCapacity(resource string, capacity int, window time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	if capacity <= 0 || window <= 0 {
		delete(m.buckets, resource)
		return
	}

	now := m.nowFunc()
	if b, exists := m.buckets[resource]; exists {
		// Update existing bucket
		b.capacity = capacity
		b.window = window
		if b.available > capacity {
			b.available = capacity
		}
	} else {
		// Create new bucket
		m.buckets[resource] = &bucket{
			capacity:   capacity,
			available:  capacity, // start full
			window:     window,
			lastRefill: now,
		}
	}
}

// GetCapacity returns the current capacity info for a resource.
func (m *MemoryLimiter) GetCapacity(resource string) *Capacity {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.buckets[resource]
	if !exists {
		return nil
	}

	b.refill(m.nowFunc())

	return &Capacity{
		Resource:  resource,
		Available: b.available,
		Total:     b.capacity,
		Window:    b.window,
		InFlight:  b.inFlight,
	}
}

// Acquire blocks until a token is available for the resource.
func (m *MemoryLimiter) Acquire(ctx context.Context, resource string) error {
	// Fast path: try to acquire immediately
	if m.TryAcquire(resource) {
		return nil
	}

	// Create a done channel for context cancellation
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			if b, exists := m.buckets[resource]; exists && b.cond != nil {
				b.cond.Broadcast()
			}
			m.mu.Unlock()
		case <-done:
		}
	}()
	defer close(done)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrClosed
	}

	b, exists := m.buckets[resource]
	if !exists {
		return ErrResourceUnknown
	}

	// Initialize cond if needed
	if b.cond == nil {
		b.cond = sync.NewCond(&m.mu)
	}

	// Wait for a token
	for {
		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Check if closed
		if m.closed {
			return ErrClosed
		}

		// Check if bucket still exists
		b, exists = m.buckets[resource]
		if !exists {
			return ErrResourceUnknown
		}

		// Try to refill and acquire
		b.refill(m.nowFunc())
		if b.available > 0 {
			b.available--
			b.inFlight++
			return nil
		}

		// Wait for signal (release or refill)
		// Use a short timeout to allow for refills and context checks
		go func() {
			time.Sleep(50 * time.Millisecond)
			m.mu.Lock()
			if b.cond != nil {
				b.cond.Broadcast()
			}
			m.mu.Unlock()
		}()
		b.cond.Wait()
	}
}

// TryAcquire attempts to acquire a token without blocking.
func (m *MemoryLimiter) TryAcquire(resource string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return false
	}

	b, exists := m.buckets[resource]
	if !exists {
		return false
	}

	// Refill tokens based on elapsed time
	b.refill(m.nowFunc())

	if b.available > 0 {
		b.available--
		b.inFlight++
		return true
	}

	return false
}

// Release returns a token to the resource bucket.
func (m *MemoryLimiter) Release(resource string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	b, exists := m.buckets[resource]
	if !exists {
		return
	}

	if b.inFlight > 0 {
		b.inFlight--
	}

	// Return a token to the bucket (for semaphore-style rate limiting)
	// This allows immediate reuse without waiting for time-based refill
	if b.available < b.capacity {
		b.available++
	}

	// Notify waiters
	if b.cond != nil {
		b.cond.Signal()
	}
}

// AnnounceReduced is a no-op for the memory limiter.
// It logs locally but doesn't broadcast to other agents.
func (m *MemoryLimiter) AnnounceReduced(resource string, reason string) {
	// No-op for local limiter - just reduce local capacity
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.buckets[resource]
	if !exists {
		return
	}

	// Reduce capacity by 25% (configurable in real implementation)
	newCapacity := int(float64(b.capacity) * 0.75)
	if newCapacity < 1 {
		newCapacity = 1
	}
	b.capacity = newCapacity
	if b.available > newCapacity {
		b.available = newCapacity
	}
}

// Close shuts down the limiter.
func (m *MemoryLimiter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrClosed
	}

	m.closed = true

	// Wake up all waiters so they can exit
	for _, b := range m.buckets {
		if b.cond != nil {
			b.cond.Broadcast()
		}
	}

	return nil
}

// Ensure MemoryLimiter implements RateLimiter.
var _ RateLimiter = (*MemoryLimiter)(nil)
