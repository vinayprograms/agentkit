package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// Common errors.
var (
	ErrAlreadyStarted = errors.New("heartbeat already started")
	ErrNotStarted     = errors.New("heartbeat not started")
	ErrInvalidConfig  = errors.New("invalid configuration")
)

// SubjectPrefix is the subject prefix for heartbeat messages.
const SubjectPrefix = "heartbeat."

// Heartbeat represents a single heartbeat message from an agent.
type Heartbeat struct {
	// AgentID uniquely identifies the sending agent.
	AgentID string `json:"agent_id"`

	// Timestamp when the heartbeat was generated.
	Timestamp time.Time `json:"timestamp"`

	// Status of the agent (e.g., "idle", "busy", "draining").
	Status string `json:"status"`

	// Load is a normalized load metric (0.0 to 1.0).
	Load float64 `json:"load"`

	// Metadata contains additional key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Marshal serializes a heartbeat to JSON.
func (h *Heartbeat) Marshal() ([]byte, error) {
	return json.Marshal(h)
}

// Unmarshal deserializes a heartbeat from JSON.
func Unmarshal(data []byte) (*Heartbeat, error) {
	var h Heartbeat
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// Subject returns the subject for this heartbeat.
func (h *Heartbeat) Subject() string {
	return SubjectPrefix + h.AgentID
}

// Sender sends periodic heartbeats.
type Sender interface {
	// Start begins sending heartbeats at the configured interval.
	// Returns ErrAlreadyStarted if already running.
	Start(ctx context.Context) error

	// SetStatus updates the status included in heartbeats.
	SetStatus(status string)

	// SetLoad updates the load metric (0.0 to 1.0).
	SetLoad(load float64)

	// SetMetadata updates a metadata field.
	SetMetadata(key, value string)

	// Stop stops sending heartbeats.
	// Returns ErrNotStarted if not running.
	Stop() error
}

// Monitor monitors heartbeats and detects dead agents.
type Monitor interface {
	// Watch returns a channel of heartbeats for a specific agent.
	Watch(agentID string) (<-chan *Heartbeat, error)

	// WatchAll returns a channel of all heartbeats.
	WatchAll() (<-chan *Heartbeat, error)

	// IsAlive checks if an agent has sent a heartbeat within timeout.
	IsAlive(agentID string, timeout time.Duration) bool

	// LastHeartbeat returns the last heartbeat from an agent, if any.
	LastHeartbeat(agentID string) *Heartbeat

	// OnDead registers a callback for when an agent is presumed dead.
	// The callback receives the agent ID.
	OnDead(callback func(agentID string))

	// Stop stops monitoring.
	Stop() error
}

// SenderConfig configures a heartbeat sender.
type SenderConfig struct {
	// Bus is the message bus for publishing heartbeats.
	Bus bus.MessageBus

	// AgentID is the unique identifier for this agent.
	AgentID string

	// Interval between heartbeats.
	// Default: 5 seconds
	Interval time.Duration

	// InitialStatus is the starting status.
	// Default: "idle"
	InitialStatus string
}

// Validate checks the configuration.
func (c *SenderConfig) Validate() error {
	if c.Bus == nil {
		return ErrInvalidConfig
	}
	if c.AgentID == "" {
		return ErrInvalidConfig
	}
	return nil
}

// DefaultSenderConfig returns configuration with sensible defaults.
func DefaultSenderConfig() SenderConfig {
	return SenderConfig{
		Interval:      5 * time.Second,
		InitialStatus: "idle",
	}
}

// MonitorConfig configures a heartbeat monitor.
type MonitorConfig struct {
	// Bus is the message bus for subscribing to heartbeats.
	Bus bus.MessageBus

	// Timeout for considering an agent dead.
	// Should be 2-3x the expected heartbeat interval.
	// Default: 15 seconds
	Timeout time.Duration

	// CheckInterval for the dead agent checker.
	// Default: 1 second
	CheckInterval time.Duration
}

// Validate checks the configuration.
func (c *MonitorConfig) Validate() error {
	if c.Bus == nil {
		return ErrInvalidConfig
	}
	return nil
}

// DefaultMonitorConfig returns configuration with sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		Timeout:       15 * time.Second,
		CheckInterval: 1 * time.Second,
	}
}
