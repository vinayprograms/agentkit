//go:build integration

package state

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// getNATSURL returns the NATS URL from environment or default.
func getNATSURL() string {
	if url := os.Getenv("NATS_URL"); url != "" {
		return url
	}
	return nats.DefaultURL
}

// newTestNATSStore creates a NATSStore for testing.
func newTestNATSStore(t *testing.T, bucket string) *NATSStore {
	conn, err := nats.Connect(getNATSURL())
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}

	store, err := NewNATSStore(NATSStoreConfig{
		Conn:   conn,
		Bucket: bucket,
	})
	if err != nil {
		conn.Close()
		t.Fatalf("NewNATSStore failed: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
		conn.Close()
	})

	return store
}

// ============================================================================
// LEVEL 1: Unit Tests — Basic Get/Put/Delete, lock acquire/release
// ============================================================================

func TestNATSStore_Get_NotFound(t *testing.T) {
	s := newTestNATSStore(t, "test-get-notfound")

	_, err := s.Get("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNATSStore_PutGet(t *testing.T) {
	s := newTestNATSStore(t, "test-put-get")

	key := "test.key"
	value := []byte("test-value")

	if err := s.Put(key, value, 0); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := s.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got) != string(value) {
		t.Errorf("expected %s, got %s", value, got)
	}
}

func TestNATSStore_GetKeyValue(t *testing.T) {
	s := newTestNATSStore(t, "test-get-kv")

	key := "test.key"
	value := []byte("test-value")

	if err := s.Put(key, value, 0); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	kv, err := s.GetKeyValue(key)
	if err != nil {
		t.Fatalf("GetKeyValue failed: %v", err)
	}

	if kv.Key != key {
		t.Errorf("expected key %s, got %s", key, kv.Key)
	}
	if string(kv.Value) != string(value) {
		t.Errorf("expected value %s, got %s", value, kv.Value)
	}
	if kv.Revision == 0 {
		t.Error("expected non-zero revision")
	}
}

func TestNATSStore_Delete(t *testing.T) {
	s := newTestNATSStore(t, "test-delete")

	key := "test.key"
	if err := s.Put(key, []byte("value"), 0); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := s.Get(key)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestNATSStore_Lock_Basic(t *testing.T) {
	s := newTestNATSStore(t, "test-lock-basic")

	lock, err := s.Lock("resource", time.Second)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	if err := lock.Unlock(); err != nil {
		t.Errorf("Unlock failed: %v", err)
	}
}

func TestNATSStore_Lock_AlreadyHeld(t *testing.T) {
	s := newTestNATSStore(t, "test-lock-held")

	_, err := s.Lock("resource", 5*time.Second)
	if err != nil {
		t.Fatalf("First lock failed: %v", err)
	}

	_, err = s.Lock("resource", time.Second)
	if err != ErrLockHeld {
		t.Errorf("expected ErrLockHeld, got %v", err)
	}
}

// ============================================================================
// LEVEL 2: Integration Tests — Full CRUD cycle, watch notifications
// ============================================================================

func TestNATSStore_CRUDCycle(t *testing.T) {
	s := newTestNATSStore(t, "test-crud")

	key := "config.timeout"

	// Create
	if err := s.Put(key, []byte("30s"), 0); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read
	val, err := s.Get(key)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(val) != "30s" {
		t.Errorf("expected 30s, got %s", val)
	}

	// Update
	if err := s.Put(key, []byte("60s"), 0); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	val, _ = s.Get(key)
	if string(val) != "60s" {
		t.Errorf("expected 60s, got %s", val)
	}

	// Delete
	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = s.Get(key)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNATSStore_Keys(t *testing.T) {
	s := newTestNATSStore(t, "test-keys")

	// Create some keys
	s.Put("config.a", []byte("1"), 0)
	s.Put("config.b", []byte("2"), 0)
	s.Put("other.x", []byte("3"), 0)

	// List all
	keys, err := s.Keys("*")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) < 3 {
		t.Errorf("expected at least 3 keys, got %d", len(keys))
	}

	// Filter by prefix
	keys, err = s.Keys("config.*")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) < 2 {
		t.Errorf("expected at least 2 config keys, got %d", len(keys))
	}
}

func TestNATSStore_Watch(t *testing.T) {
	s := newTestNATSStore(t, "test-watch")

	ch, err := s.Watch("config.*")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Write in goroutine
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Put("config.timeout", []byte("30s"), 0)
	}()

	select {
	case kv := <-ch:
		if kv.Key != "config.timeout" {
			t.Errorf("expected config.timeout, got %s", kv.Key)
		}
		if string(kv.Value) != "30s" {
			t.Errorf("expected 30s, got %s", kv.Value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for watch notification")
	}
}

// ============================================================================
// LEVEL 3: System Tests — Concurrent access, lock contention
// ============================================================================

func TestNATSStore_ConcurrentPut(t *testing.T) {
	s := newTestNATSStore(t, "test-concurrent-put")

	const goroutines = 5
	const iterations = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := "counter"
				s.Put(key, []byte("value"), 0)
			}
		}(i)
	}

	wg.Wait()

	// Should have a value
	_, err := s.Get("counter")
	if err != nil {
		t.Errorf("expected value, got %v", err)
	}
}

func TestNATSStore_ConcurrentLock(t *testing.T) {
	s := newTestNATSStore(t, "test-concurrent-lock")

	const goroutines = 5
	var acquiredCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			lock, err := s.Lock("mutex", 2*time.Second)
			if err == nil {
				acquiredCount.Add(1)
				time.Sleep(100 * time.Millisecond)
				lock.Unlock()
			}
		}()
	}

	wg.Wait()

	// At least one should have acquired
	if acquiredCount.Load() == 0 {
		t.Error("expected at least one goroutine to acquire lock")
	}
}

// ============================================================================
// LEVEL 4: Performance — Throughput benchmarks
// ============================================================================

func BenchmarkNATSStore_Put(b *testing.B) {
	conn, err := nats.Connect(getNATSURL())
	if err != nil {
		b.Skipf("NATS not available: %v", err)
	}
	defer conn.Close()

	s, err := NewNATSStore(NATSStoreConfig{Conn: conn, Bucket: "bench-put"})
	if err != nil {
		b.Fatalf("NewNATSStore failed: %v", err)
	}
	defer s.Close()

	value := []byte("test-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Put("key", value, 0)
	}
}

func BenchmarkNATSStore_Get(b *testing.B) {
	conn, err := nats.Connect(getNATSURL())
	if err != nil {
		b.Skipf("NATS not available: %v", err)
	}
	defer conn.Close()

	s, err := NewNATSStore(NATSStoreConfig{Conn: conn, Bucket: "bench-get"})
	if err != nil {
		b.Fatalf("NewNATSStore failed: %v", err)
	}
	defer s.Close()

	s.Put("key", []byte("test-value"), 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Get("key")
	}
}

func BenchmarkNATSStore_Lock(b *testing.B) {
	conn, err := nats.Connect(getNATSURL())
	if err != nil {
		b.Skipf("NATS not available: %v", err)
	}
	defer conn.Close()

	s, err := NewNATSStore(NATSStoreConfig{Conn: conn, Bucket: "bench-lock"})
	if err != nil {
		b.Fatalf("NewNATSStore failed: %v", err)
	}
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lock, _ := s.Lock("key", time.Second)
		if lock != nil {
			lock.Unlock()
		}
	}
}

// ============================================================================
// LEVEL 5: Failure Tests — Lock expiry, connection handling
// ============================================================================

func TestNATSStore_LockExpiry(t *testing.T) {
	s := newTestNATSStore(t, "test-lock-expiry")

	lock, _ := s.Lock("resource", 100*time.Millisecond)

	// Wait for lock to expire
	time.Sleep(200 * time.Millisecond)

	// Should be able to acquire again
	lock2, err := s.Lock("resource", time.Second)
	if err != nil {
		t.Errorf("should be able to acquire expired lock: %v", err)
	}
	if lock2 != nil {
		lock2.Unlock()
	}

	// Original lock refresh should fail
	if err := lock.Refresh(); err != ErrLockExpired {
		t.Errorf("expected ErrLockExpired, got %v", err)
	}
}

func TestNATSStore_LockRefresh(t *testing.T) {
	s := newTestNATSStore(t, "test-lock-refresh")

	lock, _ := s.Lock("resource", 200*time.Millisecond)

	// Refresh before expiry
	time.Sleep(100 * time.Millisecond)
	if err := lock.Refresh(); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Should still be valid
	time.Sleep(150 * time.Millisecond)
	if err := lock.Refresh(); err != nil {
		t.Errorf("expected lock to still be valid: %v", err)
	}

	lock.Unlock()
}

func TestNATSStore_OperationsAfterClose(t *testing.T) {
	s := newTestNATSStore(t, "test-after-close")
	s.Close()

	if _, err := s.Get("key"); err != ErrClosed {
		t.Errorf("Get: expected ErrClosed, got %v", err)
	}
	if err := s.Put("key", []byte("val"), 0); err != ErrClosed {
		t.Errorf("Put: expected ErrClosed, got %v", err)
	}
	if err := s.Delete("key"); err != ErrClosed {
		t.Errorf("Delete: expected ErrClosed, got %v", err)
	}
	if _, err := s.Lock("key", time.Second); err != ErrClosed {
		t.Errorf("Lock: expected ErrClosed, got %v", err)
	}
}

// ============================================================================
// LEVEL 6: Security Tests — Key validation, TTL enforcement
// ============================================================================

func TestNATSStore_KeyValidation_Empty(t *testing.T) {
	s := newTestNATSStore(t, "test-key-empty")

	if err := s.Put("", []byte("val"), 0); err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey for empty key, got %v", err)
	}
}

func TestNATSStore_KeyValidation_Spaces(t *testing.T) {
	s := newTestNATSStore(t, "test-key-spaces")

	if err := s.Put("key with space", []byte("val"), 0); err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey for key with space, got %v", err)
	}
}

func TestNATSStore_TTLValidation_Negative(t *testing.T) {
	s := newTestNATSStore(t, "test-ttl-negative")

	if err := s.Put("key", []byte("val"), -1*time.Second); err != ErrInvalidTTL {
		t.Errorf("expected ErrInvalidTTL for negative TTL, got %v", err)
	}
}

func TestNATSStore_LockTTLValidation(t *testing.T) {
	s := newTestNATSStore(t, "test-lock-ttl")

	if _, err := s.Lock("key", 0); err != ErrInvalidTTL {
		t.Errorf("expected ErrInvalidTTL for zero lock TTL, got %v", err)
	}

	if _, err := s.Lock("key", -1*time.Second); err != ErrInvalidTTL {
		t.Errorf("expected ErrInvalidTTL for negative lock TTL, got %v", err)
	}
}
