// Package bus provides message bus clients for agent-to-agent communication.
//
// The MessageBus interface enables pub/sub and request/reply patterns over
// various backends (NATS, in-memory). All implementations use channel-based
// APIs for Go-idiomatic concurrent use.
package bus

import (
	"errors"
	"time"
)

// Common errors.
var (
	ErrClosed       = errors.New("bus closed")
	ErrTimeout      = errors.New("request timeout")
	ErrNoResponders = errors.New("no responders")
	ErrInvalidSubject = errors.New("invalid subject")
)

// Message represents a message received from the bus.
type Message struct {
	// Subject the message was published to.
	Subject string

	// Data is the message payload.
	Data []byte

	// Reply is the reply subject for request/reply pattern.
	// Empty for regular pub/sub messages.
	Reply string
}

// MessageBus provides pub/sub and request/reply messaging.
type MessageBus interface {
	// Publish sends a message to all subscribers of a subject.
	Publish(subject string, data []byte) error

	// Subscribe creates a subscription to a subject.
	// All subscribers receive all messages.
	Subscribe(subject string) (Subscription, error)

	// QueueSubscribe creates a queue subscription.
	// Messages are load-balanced across queue members.
	QueueSubscribe(subject, queue string) (Subscription, error)

	// Request sends a request and waits for a single reply.
	// Returns ErrTimeout if no reply within timeout.
	Request(subject string, data []byte, timeout time.Duration) (*Message, error)

	// Close shuts down the bus connection.
	Close() error
}

// Subscription represents an active subscription.
type Subscription interface {
	// Messages returns the channel for incoming messages.
	// Channel is closed when subscription ends.
	Messages() <-chan *Message

	// Unsubscribe cancels the subscription.
	Unsubscribe() error
}

// Config holds common bus configuration.
type Config struct {
	// BufferSize for subscription channels.
	// Default: 256
	BufferSize int
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BufferSize: 256,
	}
}

// ValidateSubject checks if a subject is valid.
func ValidateSubject(subject string) error {
	if subject == "" {
		return ErrInvalidSubject
	}
	// Add more validation as needed (NATS wildcards, etc.)
	return nil
}
