package state

import (
	"sync"
	"sync/atomic"
	"time"
)

// MemoryStore implements StateStore using in-memory storage.
// Useful for testing and single-process scenarios.
type MemoryStore struct {
	mu       sync.RWMutex
	data     map[string]*entry
	locks    map[string]*memoryLock
	watchers []*watcher
	revision uint64
	closed   atomic.Bool

	// For TTL cleanup
	cleanupTicker *time.Ticker
	done          chan struct{}
}

type entry struct {
	value    []byte
	revision uint64
	created  time.Time
	modified time.Time
	expires  time.Time // Zero means no expiry
}

type watcher struct {
	pattern string
	ch      chan *KeyValue
	closed  atomic.Bool
}

// NewMemoryStore creates a new in-memory state store.
func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		data:          make(map[string]*entry),
		locks:         make(map[string]*memoryLock),
		cleanupTicker: time.NewTicker(time.Second),
		done:          make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// cleanupLoop removes expired entries periodically.
func (s *MemoryStore) cleanupLoop() {
	for {
		select {
		case <-s.cleanupTicker.C:
			s.cleanupExpired()
		case <-s.done:
			return
		}
	}
}

// cleanupExpired removes entries that have expired.
func (s *MemoryStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, e := range s.data {
		if !e.expires.IsZero() && now.After(e.expires) {
			delete(s.data, key)
			s.notifyWatchers(key, nil, OpDelete)
		}
	}

	// Clean up expired locks
	for key, lock := range s.locks {
		if now.After(lock.expires) {
			lock.released.Store(true)
			delete(s.locks, key)
		}
	}
}

// Get retrieves a value by key.
func (s *MemoryStore) Get(key string) ([]byte, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if s.closed.Load() {
		return nil, ErrClosed
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}

	// Check expiry
	if !e.expires.IsZero() && time.Now().After(e.expires) {
		return nil, ErrNotFound
	}

	// Return a copy to prevent mutation
	val := make([]byte, len(e.value))
	copy(val, e.value)
	return val, nil
}

// GetKeyValue retrieves the full KeyValue entry.
func (s *MemoryStore) GetKeyValue(key string) (*KeyValue, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if s.closed.Load() {
		return nil, ErrClosed
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}

	if !e.expires.IsZero() && time.Now().After(e.expires) {
		return nil, ErrNotFound
	}

	val := make([]byte, len(e.value))
	copy(val, e.value)

	return &KeyValue{
		Key:       key,
		Value:     val,
		Revision:  e.revision,
		Operation: OpPut,
		Created:   e.created,
		Modified:  e.modified,
	}, nil
}

// Put stores a value with optional TTL.
func (s *MemoryStore) Put(key string, value []byte, ttl time.Duration) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := ValidateTTL(ttl); err != nil {
		return err
	}
	if s.closed.Load() {
		return ErrClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.revision++
	rev := s.revision

	// Copy value to prevent external mutation
	val := make([]byte, len(value))
	copy(val, value)

	var expires time.Time
	if ttl > 0 {
		expires = now.Add(ttl)
	}

	existing, exists := s.data[key]
	created := now
	if exists {
		created = existing.created
	}

	s.data[key] = &entry{
		value:    val,
		revision: rev,
		created:  created,
		modified: now,
		expires:  expires,
	}

	s.notifyWatchers(key, val, OpPut)
	return nil
}

// Delete removes a key.
func (s *MemoryStore) Delete(key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if s.closed.Load() {
		return ErrClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[key]; ok {
		delete(s.data, key)
		s.notifyWatchers(key, nil, OpDelete)
	}

	return nil
}

// Keys returns all keys matching a pattern.
func (s *MemoryStore) Keys(pattern string) ([]string, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var keys []string
	for key, e := range s.data {
		if !e.expires.IsZero() && now.After(e.expires) {
			continue
		}
		if MatchPattern(pattern, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Watch watches for changes to keys matching a pattern.
func (s *MemoryStore) Watch(pattern string) (<-chan *KeyValue, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}

	ch := make(chan *KeyValue, 64)
	w := &watcher{
		pattern: pattern,
		ch:      ch,
	}

	s.mu.Lock()
	s.watchers = append(s.watchers, w)
	s.mu.Unlock()

	return ch, nil
}

// notifyWatchers sends notifications to matching watchers.
// Must be called with lock held.
func (s *MemoryStore) notifyWatchers(key string, value []byte, op Operation) {
	s.revision++
	kv := &KeyValue{
		Key:       key,
		Value:     value,
		Revision:  s.revision,
		Operation: op,
		Modified:  time.Now(),
	}

	for _, w := range s.watchers {
		if w.closed.Load() {
			continue
		}
		if MatchPattern(w.pattern, key) {
			select {
			case w.ch <- kv:
			default:
				// Channel full, drop notification
			}
		}
	}
}

// Lock acquires a distributed lock.
func (s *MemoryStore) Lock(key string, ttl time.Duration) (Lock, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}
	if s.closed.Load() {
		return nil, ErrClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	lockKey := "_lock." + key

	// Check if lock exists and is still valid
	if existing, ok := s.locks[lockKey]; ok {
		if !existing.released.Load() && time.Now().Before(existing.expires) {
			return nil, ErrLockHeld
		}
	}

	lock := &memoryLock{
		store:   s,
		key:     lockKey,
		ttl:     ttl,
		expires: time.Now().Add(ttl),
	}
	s.locks[lockKey] = lock

	return lock, nil
}

// Close shuts down the store.
func (s *MemoryStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	close(s.done)
	s.cleanupTicker.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Close all watchers
	for _, w := range s.watchers {
		if !w.closed.Swap(true) {
			close(w.ch)
		}
	}
	s.watchers = nil
	s.data = nil
	s.locks = nil

	return nil
}

// memoryLock implements the Lock interface for MemoryStore.
type memoryLock struct {
	store    *MemoryStore
	key      string
	ttl      time.Duration
	expires  time.Time
	released atomic.Bool
}

// Unlock releases the lock.
func (l *memoryLock) Unlock() error {
	if l.released.Swap(true) {
		return ErrLockNotHeld
	}

	l.store.mu.Lock()
	defer l.store.mu.Unlock()

	delete(l.store.locks, l.key)
	return nil
}

// Refresh extends the lock TTL.
func (l *memoryLock) Refresh() error {
	if l.released.Load() {
		return ErrLockNotHeld
	}

	l.store.mu.Lock()
	defer l.store.mu.Unlock()

	if time.Now().After(l.expires) {
		l.released.Store(true)
		delete(l.store.locks, l.key)
		return ErrLockExpired
	}

	l.expires = time.Now().Add(l.ttl)
	return nil
}

// Key returns the lock key.
func (l *memoryLock) Key() string {
	return l.key
}
