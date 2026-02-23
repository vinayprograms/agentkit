package state

import (
	"errors"
	"strings"
	"time"
)

// Common errors.
var (
	ErrNotFound      = errors.New("key not found")
	ErrClosed        = errors.New("store closed")
	ErrLockHeld      = errors.New("lock already held")
	ErrLockNotHeld   = errors.New("lock not held")
	ErrLockExpired   = errors.New("lock expired")
	ErrInvalidKey    = errors.New("invalid key")
	ErrInvalidTTL    = errors.New("invalid TTL")
	ErrWatchClosed   = errors.New("watch closed")
)

// Operation represents the type of change to a key.
type Operation int

const (
	// OpPut indicates a key was created or updated.
	OpPut Operation = iota
	// OpDelete indicates a key was deleted.
	OpDelete
)

// String returns the operation name.
func (o Operation) String() string {
	switch o {
	case OpPut:
		return "put"
	case OpDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// KeyValue represents a key-value entry with metadata.
type KeyValue struct {
	// Key is the entry key.
	Key string

	// Value is the entry value.
	Value []byte

	// Revision is a monotonic version number.
	Revision uint64

	// Operation indicates the type of change.
	Operation Operation

	// Created is when the key was first created.
	Created time.Time

	// Modified is when the key was last modified.
	Modified time.Time
}

// StateStore provides distributed key-value storage with locking.
type StateStore interface {
	// Get retrieves a value by key.
	// Returns ErrNotFound if the key does not exist.
	Get(key string) ([]byte, error)

	// GetKeyValue retrieves the full KeyValue entry.
	// Returns ErrNotFound if the key does not exist.
	GetKeyValue(key string) (*KeyValue, error)

	// Put stores a value with an optional TTL.
	// If ttl is 0, the key never expires.
	Put(key string, value []byte, ttl time.Duration) error

	// Delete removes a key.
	// Returns nil if the key does not exist.
	Delete(key string) error

	// Keys returns all keys matching a pattern.
	// Pattern supports * wildcard at the end (e.g., "config.*").
	Keys(pattern string) ([]string, error)

	// Watch watches for changes to keys matching a pattern.
	// Pattern supports * wildcard at the end (e.g., "config.*").
	// The channel is closed when the watch ends or store closes.
	Watch(pattern string) (<-chan *KeyValue, error)

	// Lock acquires a distributed lock with the given TTL.
	// Returns ErrLockHeld if the lock is already held.
	Lock(key string, ttl time.Duration) (Lock, error)

	// Close shuts down the store and releases resources.
	Close() error
}

// Lock represents a distributed lock.
type Lock interface {
	// Unlock releases the lock.
	// Returns ErrLockNotHeld if already released.
	Unlock() error

	// Refresh extends the lock TTL.
	// Returns ErrLockExpired if the lock has expired.
	Refresh() error

	// Key returns the lock key.
	Key() string
}

// ValidateKey checks if a key is valid.
func ValidateKey(key string) error {
	if key == "" {
		return ErrInvalidKey
	}
	if strings.Contains(key, " ") {
		return ErrInvalidKey
	}
	if strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") {
		return ErrInvalidKey
	}
	if len(key) > 1024 {
		return ErrInvalidKey
	}
	return nil
}

// ValidateTTL checks if a TTL is valid.
func ValidateTTL(ttl time.Duration) error {
	if ttl < 0 {
		return ErrInvalidTTL
	}
	return nil
}

// MatchPattern checks if a key matches a pattern.
// Supports * wildcard at the end (e.g., "config.*" matches "config.foo").
func MatchPattern(pattern, key string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(key, prefix)
	}
	return pattern == key
}
