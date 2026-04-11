package bus

import (
	"errors"
	"strings"
	"time"
)

// Sentinel errors.
var (
	ErrClosed         = errors.New("bus closed")
	ErrTimeout        = errors.New("request timeout")
	ErrNoResponders   = errors.New("no responders")
	ErrInvalidSubject = errors.New("invalid subject")
)

// Subject wildcards for pattern-based subscriptions.
const (
	// Wildcard matches exactly one token in a subject.
	// "foo.*" matches "foo.bar" but not "foo.bar.baz".
	Wildcard = "*"

	// WildcardAll matches one or more trailing tokens (must be last).
	// "foo.>" matches "foo.bar" and "foo.bar.baz".
	WildcardAll = ">"
)

// Bus provides pub/sub and request/reply messaging.
// Safe for concurrent use.
type Bus interface {
	// Publish sends a message to all subscribers of a subject.
	Publish(subject string, data []byte) error

	// Subscribe creates a subscription to a subject.
	// Supports Wildcard (*) and WildcardAll (>) in subject patterns.
	Subscribe(subject string) (Subscription, error)

	// QueueSubscribe creates a queue subscription.
	// Messages are load-balanced across queue members with the same queue name.
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
	// Channel is closed when the subscription ends.
	Messages() <-chan *Message

	// Unsubscribe cancels the subscription and closes the Messages channel.
	Unsubscribe() error
}

// Message represents a message received from the bus.
type Message struct {
	Subject string // subject the message was published to
	Data    []byte // message payload
	Reply   string // reply subject for request/reply (empty for pub/sub)
}

// Config holds bus configuration. Zero value uses sensible defaults.
type Config struct {
	// BufferSize for subscription channels. Default: 256.
	BufferSize int
}

func (c Config) bufferSize() int {
	if c.BufferSize <= 0 {
		return 256
	}
	return c.BufferSize
}

// validateSubject checks if a subject is valid.
func validateSubject(subject string) error {
	if subject == "" {
		return ErrInvalidSubject
	}
	return nil
}

// subjectMatch reports whether subject matches pattern,
// supporting Wildcard (*) and WildcardAll (>) tokens.
func subjectMatch(pattern, subject string) bool {
	ptokens := strings.Split(pattern, ".")
	stokens := strings.Split(subject, ".")

	for i, pt := range ptokens {
		if pt == WildcardAll {
			return i <= len(stokens)-1 // > must match at least one token
		}
		if i >= len(stokens) {
			return false
		}
		if pt != Wildcard && pt != stokens[i] {
			return false
		}
	}
	return len(ptokens) == len(stokens)
}
