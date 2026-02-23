// Package registry provides agent registration and discovery for swarm coordination.
//
// Agents self-register with capabilities, status, and load. Other agents
// discover and route to appropriate handlers based on capabilities and availability.
package registry

import (
	"errors"
	"time"
)

// Common errors.
var (
	ErrNotFound   = errors.New("agent not found")
	ErrClosed     = errors.New("registry closed")
	ErrInvalidID  = errors.New("invalid agent ID")
	ErrDuplicateID = errors.New("duplicate agent ID")
)

// Status represents an agent's operational state.
type Status string

const (
	StatusIdle     Status = "idle"
	StatusBusy     Status = "busy"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
)

// AgentInfo contains registration information for an agent.
type AgentInfo struct {
	// ID uniquely identifies the agent.
	ID string

	// Name is a human-readable name for the agent.
	Name string

	// Capabilities lists what the agent can do (e.g., "code-review", "testing").
	Capabilities []string

	// Status is the agent's current operational state.
	Status Status

	// Load is the agent's current load (0.0-1.0).
	Load float64

	// Metadata contains additional key-value pairs.
	Metadata map[string]string

	// LastSeen is when the agent last updated its registration.
	LastSeen time.Time
}

// Filter specifies criteria for listing agents.
type Filter struct {
	// Status filters by operational state. Empty means all.
	Status Status

	// Capability filters to agents with this capability.
	Capability string

	// MaxLoad filters to agents with load at or below this value.
	// Zero means no filter.
	MaxLoad float64
}

// EventType represents the type of registry event.
type EventType string

const (
	EventAdded   EventType = "added"
	EventUpdated EventType = "updated"
	EventRemoved EventType = "removed"
)

// Event represents a change in the registry.
type Event struct {
	// Type indicates what happened.
	Type EventType

	// Agent contains the agent information.
	// For removal events, this contains the last known state.
	Agent AgentInfo
}

// Registry provides agent registration and discovery.
type Registry interface {
	// Register adds or updates an agent in the registry.
	// If an agent with the same ID exists, it updates the entry.
	Register(info AgentInfo) error

	// Deregister removes an agent from the registry.
	// Returns ErrNotFound if the agent doesn't exist.
	Deregister(id string) error

	// Get retrieves a specific agent by ID.
	// Returns nil, ErrNotFound if not found.
	Get(id string) (*AgentInfo, error)

	// List returns all agents matching the optional filter.
	// Pass nil for no filtering.
	List(filter *Filter) ([]AgentInfo, error)

	// FindByCapability returns agents with a specific capability.
	// Results are sorted by load (lowest first).
	FindByCapability(capability string) ([]AgentInfo, error)

	// Watch returns a channel of registry events.
	// The channel is closed when the registry is closed.
	// Multiple watchers are supported.
	Watch() (<-chan Event, error)

	// Close shuts down the registry client.
	Close() error
}

// ValidateAgentInfo checks if agent info is valid.
func ValidateAgentInfo(info AgentInfo) error {
	if info.ID == "" {
		return ErrInvalidID
	}
	if info.Load < 0 || info.Load > 1 {
		return errors.New("load must be between 0.0 and 1.0")
	}
	return nil
}

// HasCapability checks if an agent has a specific capability.
func HasCapability(info AgentInfo, capability string) bool {
	for _, cap := range info.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}

// MatchesFilter checks if an agent matches the filter criteria.
func MatchesFilter(info AgentInfo, filter *Filter) bool {
	if filter == nil {
		return true
	}

	if filter.Status != "" && info.Status != filter.Status {
		return false
	}

	if filter.Capability != "" && !HasCapability(info, filter.Capability) {
		return false
	}

	if filter.MaxLoad > 0 && info.Load > filter.MaxLoad {
		return false
	}

	return true
}
