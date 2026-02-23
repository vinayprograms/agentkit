package ratelimit

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	ErrClosed         = errors.New("limiter closed")
	ErrResourceUnknown = errors.New("unknown resource")
	ErrCapacityExhausted = errors.New("capacity exhausted")
	ErrInvalidCapacity = errors.New("invalid capacity")
	ErrInvalidWindow   = errors.New("invalid window")
	ErrInvalidConfig   = errors.New("invalid configuration")
)

// SubjectPrefix is the message bus subject prefix for rate limit messages.
const SubjectPrefix = "ratelimit."

// RateLimiter coordinates rate limits for shared resources.
type RateLimiter interface {
	// Acquire blocks until a token is available for the resource.
	// Returns context.Canceled or context.DeadlineExceeded if context ends.
	// Returns ErrResourceUnknown if the resource has no configured capacity.
	Acquire(ctx context.Context, resource string) error

	// TryAcquire attempts to acquire a token without blocking.
	// Returns true if a token was acquired, false otherwise.
	TryAcquire(resource string) bool

	// Release returns a token to the resource bucket.
	// This is optional and useful for tracking in-flight requests.
	// Has no effect if the resource is unknown or already at capacity.
	Release(resource string)

	// SetCapacity configures the rate limit for a resource.
	// capacity is the number of tokens per window.
	// window is the time period for refill (e.g., time.Minute).
	SetCapacity(resource string, capacity int, window time.Duration)

	// AnnounceReduced broadcasts that capacity should be reduced.
	// reason describes why (e.g., "received 429 response").
	// For distributed limiters, this notifies other agents.
	// For local limiters, this is a no-op.
	AnnounceReduced(resource string, reason string)

	// GetCapacity returns the current capacity info for a resource.
	// Returns nil if the resource is unknown.
	GetCapacity(resource string) *Capacity

	// Close shuts down the limiter and releases resources.
	Close() error
}

// Capacity describes the rate limit configuration for a resource.
type Capacity struct {
	// Resource is the unique identifier for the rate-limited resource.
	Resource string

	// Available is the current number of available tokens.
	Available int

	// Total is the maximum capacity (tokens per window).
	Total int

	// Window is the refill period.
	Window time.Duration

	// InFlight tracks requests currently in progress (if Release is used).
	InFlight int
}

// CapacityUpdate is broadcast when capacity changes in the swarm.
type CapacityUpdate struct {
	// Resource that changed.
	Resource string `json:"resource"`

	// AgentID that sent the update.
	AgentID string `json:"agent_id"`

	// NewCapacity is the suggested new total capacity.
	NewCapacity int `json:"new_capacity"`

	// Reason for the change.
	Reason string `json:"reason"`

	// Timestamp of the update.
	Timestamp time.Time `json:"timestamp"`
}

// OnCapacityChange is a callback for capacity change notifications.
type OnCapacityChange func(update *CapacityUpdate)

// WatchOption configures watching behavior.
type WatchOption func(*watchConfig)

type watchConfig struct {
	callback OnCapacityChange
}

// WithCapacityCallback sets a callback for capacity changes.
func WithCapacityCallback(cb OnCapacityChange) WatchOption {
	return func(c *watchConfig) {
		c.callback = cb
	}
}
