package state

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSStore implements StateStore using NATS JetStream KV.
type NATSStore struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	kv     jetstream.KeyValue
	config NATSStoreConfig
	closed atomic.Bool

	// Lock management
	lockMu sync.Mutex
	locks  map[string]*natsLock
}

// NATSStoreConfig holds NATS KV store configuration.
type NATSStoreConfig struct {
	// Conn is the NATS connection to use.
	Conn *nats.Conn

	// Bucket is the KV bucket name.
	Bucket string

	// TTL is the default TTL for entries (0 = no default).
	TTL time.Duration

	// History is the number of revisions to keep per key.
	// Default: 1
	History int

	// MaxValueSize is the maximum value size in bytes.
	// Default: 1MB
	MaxValueSize int32
}

// DefaultNATSStoreConfig returns configuration with sensible defaults.
func DefaultNATSStoreConfig() NATSStoreConfig {
	return NATSStoreConfig{
		Bucket:       "agent-state",
		History:      1,
		MaxValueSize: 1024 * 1024, // 1MB
	}
}

// NewNATSStore creates a new NATS JetStream KV store.
func NewNATSStore(cfg NATSStoreConfig) (*NATSStore, error) {
	if cfg.Conn == nil {
		return nil, fmt.Errorf("nats connection required")
	}
	if cfg.Bucket == "" {
		cfg.Bucket = DefaultNATSStoreConfig().Bucket
	}
	if cfg.History <= 0 {
		cfg.History = DefaultNATSStoreConfig().History
	}
	if cfg.MaxValueSize <= 0 {
		cfg.MaxValueSize = DefaultNATSStoreConfig().MaxValueSize
	}

	js, err := jetstream.New(cfg.Conn)
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:       cfg.Bucket,
		TTL:          cfg.TTL,
		History:      uint8(cfg.History),
		MaxValueSize: cfg.MaxValueSize,
	})
	if err != nil {
		return nil, fmt.Errorf("create kv bucket: %w", err)
	}

	return &NATSStore{
		conn:   cfg.Conn,
		js:     js,
		kv:     kv,
		config: cfg,
		locks:  make(map[string]*natsLock),
	}, nil
}

// Get retrieves a value by key.
func (s *NATSStore) Get(key string) ([]byte, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if s.closed.Load() {
		return nil, ErrClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("kv get: %w", err)
	}

	return entry.Value(), nil
}

// GetKeyValue retrieves the full KeyValue entry.
func (s *NATSStore) GetKeyValue(key string) (*KeyValue, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if s.closed.Load() {
		return nil, ErrClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("kv get: %w", err)
	}

	return &KeyValue{
		Key:       entry.Key(),
		Value:     entry.Value(),
		Revision:  entry.Revision(),
		Operation: opFromNATS(entry.Operation()),
		Created:   entry.Created(),
		Modified:  entry.Created(), // NATS KV uses Created for last modified
	}, nil
}

// opFromNATS converts NATS operation to our Operation type.
func opFromNATS(op jetstream.KeyValueOp) Operation {
	switch op {
	case jetstream.KeyValuePut:
		return OpPut
	case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
		return OpDelete
	default:
		return OpPut
	}
}

// Put stores a value with optional TTL.
func (s *NATSStore) Put(key string, value []byte, ttl time.Duration) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := ValidateTTL(ttl); err != nil {
		return err
	}
	if s.closed.Load() {
		return ErrClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: NATS KV TTL is bucket-level, not per-key.
	// For per-key TTL, we store expiry in the value or use a separate cleanup mechanism.
	_, err := s.kv.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("kv put: %w", err)
	}

	return nil
}

// Delete removes a key.
func (s *NATSStore) Delete(key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if s.closed.Load() {
		return ErrClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.kv.Delete(ctx, key)
	if err != nil && err != jetstream.ErrKeyNotFound {
		return fmt.Errorf("kv delete: %w", err)
	}

	return nil
}

// Keys returns all keys matching a pattern.
func (s *NATSStore) Keys(pattern string) ([]string, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Convert our pattern to NATS pattern
	natsPattern := pattern
	if pattern == "*" {
		natsPattern = ">"
	} else if strings.HasSuffix(pattern, "*") {
		natsPattern = strings.TrimSuffix(pattern, "*") + ">"
	}

	lister, err := s.kv.ListKeys(ctx, jetstream.MetaOnly())
	if err != nil {
		return nil, fmt.Errorf("kv list keys: %w", err)
	}

	var keys []string
	for key := range lister.Keys() {
		if MatchPattern(pattern, key) || natsPattern == ">" {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// Watch watches for changes to keys matching a pattern.
func (s *NATSStore) Watch(pattern string) (<-chan *KeyValue, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}

	ctx := context.Background()

	// Convert our pattern to NATS pattern
	natsPattern := pattern
	if pattern == "*" {
		natsPattern = ">"
	} else if strings.HasSuffix(pattern, "*") {
		natsPattern = strings.TrimSuffix(pattern, "*") + ">"
	}

	var watcher jetstream.KeyWatcher
	var err error

	if natsPattern == ">" {
		watcher, err = s.kv.WatchAll(ctx, jetstream.IgnoreDeletes())
	} else {
		watcher, err = s.kv.Watch(ctx, natsPattern, jetstream.IgnoreDeletes())
	}
	if err != nil {
		return nil, fmt.Errorf("kv watch: %w", err)
	}

	ch := make(chan *KeyValue, 64)

	go s.watchLoop(watcher, ch, pattern)

	return ch, nil
}

// watchLoop processes watch updates.
func (s *NATSStore) watchLoop(watcher jetstream.KeyWatcher, ch chan *KeyValue, pattern string) {
	defer close(ch)
	defer watcher.Stop()

	for {
		select {
		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete marker
			}

			// Filter by our pattern if NATS pattern was broader
			if !MatchPattern(pattern, entry.Key()) {
				continue
			}

			kv := &KeyValue{
				Key:       entry.Key(),
				Value:     entry.Value(),
				Revision:  entry.Revision(),
				Operation: opFromNATS(entry.Operation()),
				Created:   entry.Created(),
				Modified:  entry.Created(),
			}

			select {
			case ch <- kv:
			default:
				// Channel full
			}
		}

		if s.closed.Load() {
			return
		}
	}
}

// Lock acquires a distributed lock.
func (s *NATSStore) Lock(key string, ttl time.Duration) (Lock, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}
	if s.closed.Load() {
		return nil, ErrClosed
	}

	lockKey := "_lock." + key

	s.lockMu.Lock()
	defer s.lockMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to create lock entry (atomic via revision check)
	entry, err := s.kv.Get(ctx, lockKey)
	if err == nil {
		// Lock exists, check if it's expired
		lockAge := time.Since(entry.Created())
		// Parse TTL from value (stored as duration string)
		storedTTL, _ := time.ParseDuration(string(entry.Value()))
		if lockAge < storedTTL {
			return nil, ErrLockHeld
		}
		// Lock expired, we can take it
	} else if err != jetstream.ErrKeyNotFound {
		return nil, fmt.Errorf("check lock: %w", err)
	}

	// Create or update lock
	_, err = s.kv.Put(ctx, lockKey, []byte(ttl.String()))
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}

	lock := &natsLock{
		store:   s,
		key:     lockKey,
		ttl:     ttl,
		created: time.Now(),
	}
	s.locks[lockKey] = lock

	return lock, nil
}

// Close shuts down the store.
func (s *NATSStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.lockMu.Lock()
	defer s.lockMu.Unlock()

	// Release all locks
	for _, lock := range s.locks {
		lock.released.Store(true)
	}
	s.locks = nil

	return nil
}

// natsLock implements the Lock interface for NATSStore.
type natsLock struct {
	store    *NATSStore
	key      string
	ttl      time.Duration
	created  time.Time
	released atomic.Bool
}

// Unlock releases the lock.
func (l *natsLock) Unlock() error {
	if l.released.Swap(true) {
		return ErrLockNotHeld
	}

	l.store.lockMu.Lock()
	delete(l.store.locks, l.key)
	l.store.lockMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := l.store.kv.Delete(ctx, l.key)
	if err != nil && err != jetstream.ErrKeyNotFound {
		return fmt.Errorf("release lock: %w", err)
	}

	return nil
}

// Refresh extends the lock TTL.
func (l *natsLock) Refresh() error {
	if l.released.Load() {
		return ErrLockNotHeld
	}

	// Check if lock has expired locally
	if time.Since(l.created) > l.ttl {
		l.released.Store(true)
		return ErrLockExpired
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update lock with new TTL
	_, err := l.store.kv.Put(ctx, l.key, []byte(l.ttl.String()))
	if err != nil {
		return fmt.Errorf("refresh lock: %w", err)
	}

	l.created = time.Now()
	return nil
}

// Key returns the lock key.
func (l *natsLock) Key() string {
	return l.key
}
