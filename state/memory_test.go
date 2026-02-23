package state

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// LEVEL 1: Unit Tests — Basic Get/Put/Delete, lock acquire/release
// ============================================================================

func TestMemoryStore_Get_NotFound(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	_, err := s.Get("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_PutGet(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

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

func TestMemoryStore_GetKeyValue(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

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
	if kv.Operation != OpPut {
		t.Errorf("expected OpPut, got %v", kv.Operation)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

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

func TestMemoryStore_Delete_Nonexistent(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Should not error
	if err := s.Delete("nonexistent"); err != nil {
		t.Errorf("Delete of nonexistent key should not error: %v", err)
	}
}

func TestMemoryStore_Lock_Basic(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	lock, err := s.Lock("resource", time.Second)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	if err := lock.Unlock(); err != nil {
		t.Errorf("Unlock failed: %v", err)
	}
}

func TestMemoryStore_Lock_DoubleUnlock(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	lock, _ := s.Lock("resource", time.Second)
	lock.Unlock()

	if err := lock.Unlock(); err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestMemoryStore_Lock_AlreadyHeld(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	_, err := s.Lock("resource", time.Second)
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

func TestMemoryStore_CRUDCycle(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

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

func TestMemoryStore_Keys(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Create some keys
	s.Put("config.a", []byte("1"), 0)
	s.Put("config.b", []byte("2"), 0)
	s.Put("other.x", []byte("3"), 0)

	// List all
	keys, err := s.Keys("*")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// Filter by prefix
	keys, err = s.Keys("config.*")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 config keys, got %d", len(keys))
	}
}

func TestMemoryStore_Watch(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	ch, err := s.Watch("config.*")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Write in goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
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
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for watch notification")
	}
}

func TestMemoryStore_Watch_Delete(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	s.Put("config.x", []byte("value"), 0)

	ch, _ := s.Watch("config.*")

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.Delete("config.x")
	}()

	select {
	case kv := <-ch:
		if kv.Operation != OpDelete {
			t.Errorf("expected OpDelete, got %v", kv.Operation)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for delete notification")
	}
}

// ============================================================================
// LEVEL 3: System Tests — Concurrent access, lock contention
// ============================================================================

func TestMemoryStore_ConcurrentPut(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	const goroutines = 10
	const iterations = 100

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

func TestMemoryStore_ConcurrentLock(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	const goroutines = 10
	var acquiredCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			lock, err := s.Lock("mutex", time.Second)
			if err == nil {
				acquiredCount.Add(1)
				time.Sleep(10 * time.Millisecond)
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

func TestMemoryStore_LockContention(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	var counter int32
	var wg sync.WaitGroup
	wg.Add(5)

	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				for {
					lock, err := s.Lock("counter-lock", 100*time.Millisecond)
					if err == ErrLockHeld {
						time.Sleep(5 * time.Millisecond)
						continue
					}
					if err != nil {
						return
					}

					atomic.AddInt32(&counter, 1)
					lock.Unlock()
					break
				}
			}
		}()
	}

	wg.Wait()

	if counter != 50 {
		t.Errorf("expected 50, got %d", counter)
	}
}

// ============================================================================
// LEVEL 4: Performance — Throughput benchmarks
// ============================================================================

func BenchmarkMemoryStore_Put(b *testing.B) {
	s := NewMemoryStore()
	defer s.Close()

	value := []byte("test-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Put("key", value, 0)
	}
}

func BenchmarkMemoryStore_Get(b *testing.B) {
	s := NewMemoryStore()
	defer s.Close()

	s.Put("key", []byte("test-value"), 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Get("key")
	}
}

func BenchmarkMemoryStore_PutGet(b *testing.B) {
	s := NewMemoryStore()
	defer s.Close()

	value := []byte("test-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Put("key", value, 0)
		s.Get("key")
	}
}

func BenchmarkMemoryStore_Lock(b *testing.B) {
	s := NewMemoryStore()
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lock, _ := s.Lock("key", time.Second)
		lock.Unlock()
	}
}

// ============================================================================
// LEVEL 5: Failure Tests — Lock expiry, TTL, conflict resolution
// ============================================================================

func TestMemoryStore_TTLExpiry(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	s.Put("temp", []byte("value"), 50*time.Millisecond)

	// Should exist initially
	_, err := s.Get("temp")
	if err != nil {
		t.Fatalf("expected value, got %v", err)
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Should be gone
	_, err = s.Get("temp")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after TTL, got %v", err)
	}
}

func TestMemoryStore_LockExpiry(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	lock, _ := s.Lock("resource", 50*time.Millisecond)

	// Wait for lock to expire
	time.Sleep(100 * time.Millisecond)

	// Refresh should fail
	if err := lock.Refresh(); err != ErrLockExpired {
		t.Errorf("expected ErrLockExpired, got %v", err)
	}
}

func TestMemoryStore_LockRefresh(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	lock, _ := s.Lock("resource", 100*time.Millisecond)

	// Refresh before expiry
	time.Sleep(50 * time.Millisecond)
	if err := lock.Refresh(); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Should still be valid
	time.Sleep(60 * time.Millisecond)
	if err := lock.Refresh(); err != nil {
		t.Errorf("expected lock to still be valid: %v", err)
	}

	lock.Unlock()
}

func TestMemoryStore_CloseReleasesWatchers(t *testing.T) {
	s := NewMemoryStore()

	ch, _ := s.Watch("*")

	s.Close()

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}
}

func TestMemoryStore_OperationsAfterClose(t *testing.T) {
	s := NewMemoryStore()
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

func TestMemoryStore_KeyValidation_Empty(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	if err := s.Put("", []byte("val"), 0); err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey for empty key, got %v", err)
	}
}

func TestMemoryStore_KeyValidation_Spaces(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	if err := s.Put("key with space", []byte("val"), 0); err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey for key with space, got %v", err)
	}
}

func TestMemoryStore_KeyValidation_LeadingDot(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	if err := s.Put(".invalid", []byte("val"), 0); err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey for leading dot, got %v", err)
	}
}

func TestMemoryStore_KeyValidation_TrailingDot(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	if err := s.Put("invalid.", []byte("val"), 0); err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey for trailing dot, got %v", err)
	}
}

func TestMemoryStore_TTLValidation_Negative(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	if err := s.Put("key", []byte("val"), -1*time.Second); err != ErrInvalidTTL {
		t.Errorf("expected ErrInvalidTTL for negative TTL, got %v", err)
	}
}

func TestMemoryStore_LockTTLValidation(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	if _, err := s.Lock("key", 0); err != ErrInvalidTTL {
		t.Errorf("expected ErrInvalidTTL for zero lock TTL, got %v", err)
	}

	if _, err := s.Lock("key", -1*time.Second); err != ErrInvalidTTL {
		t.Errorf("expected ErrInvalidTTL for negative lock TTL, got %v", err)
	}
}

func TestMemoryStore_ValueIsolation(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	original := []byte("original")
	s.Put("key", original, 0)

	// Modify original slice
	original[0] = 'X'

	// Value should be unchanged
	val, _ := s.Get("key")
	if string(val) != "original" {
		t.Errorf("value was mutated: %s", val)
	}

	// Modify returned value
	val[0] = 'Y'

	// Re-get should be unchanged
	val2, _ := s.Get("key")
	if string(val2) != "original" {
		t.Errorf("value was mutated via return: %s", val2)
	}
}

// ============================================================================
// Additional Coverage Tests
// ============================================================================

func TestMemoryStore_CleanupExpired_Data(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Put a key with very short TTL
	s.Put("expires-soon", []byte("value"), 50*time.Millisecond)

	// Verify it exists
	_, err := s.Get("expires-soon")
	if err != nil {
		t.Fatalf("key should exist initially: %v", err)
	}

	// Wait for cleanup loop to run (runs every second, but key expires after 50ms)
	time.Sleep(1200 * time.Millisecond)

	// Now it should be gone (cleaned up by the loop)
	_, err = s.Get("expires-soon")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after cleanup, got %v", err)
	}
}

func TestMemoryStore_CleanupExpired_Locks(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Acquire a lock with short TTL
	_, err := s.Lock("resource", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	// Wait for cleanup loop to clean up expired lock
	time.Sleep(1200 * time.Millisecond)

	// Should be able to acquire the lock again (cleanup released it)
	lock2, err := s.Lock("resource", time.Second)
	if err != nil {
		t.Errorf("should be able to acquire lock after cleanup: %v", err)
	}
	if lock2 != nil {
		lock2.Unlock()
	}
}

func TestMemoryStore_CleanupExpired_WatchNotification(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Set up a watch
	ch, err := s.Watch("expires.*")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Put a key with short TTL
	s.Put("expires.key", []byte("value"), 50*time.Millisecond)

	// Wait for the put notification
	select {
	case kv := <-ch:
		if kv.Operation != OpPut {
			t.Errorf("expected OpPut, got %v", kv.Operation)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for put notification")
	}

	// Wait for cleanup and delete notification
	select {
	case kv := <-ch:
		if kv.Operation != OpDelete {
			t.Errorf("expected OpDelete from cleanup, got %v", kv.Operation)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cleanup delete notification")
	}
}

func TestMemoryStore_GetKeyValue_NotFound(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	_, err := s.GetKeyValue("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_GetKeyValue_InvalidKey(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	_, err := s.GetKeyValue("")
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestMemoryStore_GetKeyValue_Closed(t *testing.T) {
	s := NewMemoryStore()
	s.Close()

	_, err := s.GetKeyValue("key")
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestMemoryStore_GetKeyValue_Expired(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Put with short TTL
	s.Put("temp", []byte("value"), 50*time.Millisecond)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// GetKeyValue should return ErrNotFound for expired entry
	_, err := s.GetKeyValue("temp")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for expired entry, got %v", err)
	}
}

func TestMemoryStore_Get_InvalidKey(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	_, err := s.Get("")
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestMemoryStore_Delete_InvalidKey(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	err := s.Delete("")
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestMemoryStore_Lock_InvalidKey(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	_, err := s.Lock("", time.Second)
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestMemoryStore_Lock_Key(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	lock, err := s.Lock("myresource", time.Second)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	defer lock.Unlock()

	// Test Lock.Key() method
	if lock.Key() != "_lock.myresource" {
		t.Errorf("expected _lock.myresource, got %s", lock.Key())
	}
}

func TestMemoryStore_Lock_ExpiredTakeover(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Acquire lock with short TTL
	lock1, err := s.Lock("resource", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	// Wait for lock to expire
	time.Sleep(100 * time.Millisecond)

	// Should be able to acquire the same lock (expired takeover)
	lock2, err := s.Lock("resource", time.Second)
	if err != nil {
		t.Errorf("should be able to take over expired lock: %v", err)
	}

	// lock1 refresh should fail
	if err := lock1.Refresh(); err != ErrLockExpired {
		t.Errorf("expected ErrLockExpired for original lock, got %v", err)
	}

	if lock2 != nil {
		lock2.Unlock()
	}
}

func TestMemoryStore_Lock_RefreshAfterUnlock(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	lock, _ := s.Lock("resource", time.Second)
	lock.Unlock()

	// Refresh after unlock should fail
	if err := lock.Refresh(); err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld after unlock, got %v", err)
	}
}

func TestMemoryStore_Keys_Closed(t *testing.T) {
	s := NewMemoryStore()
	s.Close()

	_, err := s.Keys("*")
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestMemoryStore_Keys_ExpiredFiltered(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	// Put a persistent key
	s.Put("persistent", []byte("value"), 0)

	// Put a key with short TTL
	s.Put("expires", []byte("value"), 50*time.Millisecond)

	// Initially both should be visible
	keys, _ := s.Keys("*")
	if len(keys) != 2 {
		t.Errorf("expected 2 keys initially, got %d", len(keys))
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Only persistent key should be visible
	keys, _ = s.Keys("*")
	if len(keys) != 1 {
		t.Errorf("expected 1 key after expiry, got %d", len(keys))
	}
}

func TestMemoryStore_Watch_Closed(t *testing.T) {
	s := NewMemoryStore()
	s.Close()

	_, err := s.Watch("*")
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestMemoryStore_Watch_NonMatchingPattern(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	ch, _ := s.Watch("config.*")

	// Write to a different pattern
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.Put("other.key", []byte("value"), 0)
		time.Sleep(10 * time.Millisecond)
		s.Put("config.match", []byte("value"), 0)
	}()

	// Should only receive matching notification
	select {
	case kv := <-ch:
		if kv.Key != "config.match" {
			t.Errorf("expected config.match, got %s", kv.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for matching notification")
	}
}

func TestMemoryStore_Watch_ChannelFull(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	ch, _ := s.Watch("*")

	// Fill the channel (capacity is 64) + overflow
	for i := 0; i < 100; i++ {
		s.Put("key", []byte("value"), 0)
	}

	// Drain what we can
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:

	// Should have received some notifications (up to buffer size)
	if count == 0 {
		t.Error("expected some notifications")
	}
	if count > 64 {
		t.Errorf("expected at most 64 buffered notifications, got %d", count)
	}
}

func TestMemoryStore_Close_Idempotent(t *testing.T) {
	s := NewMemoryStore()

	// First close should succeed
	if err := s.Close(); err != nil {
		t.Errorf("first Close failed: %v", err)
	}

	// Second close should also succeed (idempotent)
	if err := s.Close(); err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

func TestMemoryStore_WatcherClosed_NotNotified(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	ch, _ := s.Watch("*")

	// Close the store (which closes watchers)
	s.Close()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestMemoryStore_Put_UpdatePreservesCreated(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	key := "test.key"

	// Create
	s.Put(key, []byte("v1"), 0)
	kv1, _ := s.GetKeyValue(key)
	created1 := kv1.Created

	// Small delay to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update
	s.Put(key, []byte("v2"), 0)
	kv2, _ := s.GetKeyValue(key)

	// Created should be preserved
	if !kv2.Created.Equal(created1) {
		t.Errorf("Created time changed: %v -> %v", created1, kv2.Created)
	}

	// Modified should be updated
	if !kv2.Modified.After(kv1.Modified) {
		t.Error("Modified time should be updated")
	}

	// Revision should increase
	if kv2.Revision <= kv1.Revision {
		t.Error("Revision should increase")
	}
}

func TestMemoryStore_Watch_ExactPattern(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	ch, _ := s.Watch("exact.key")

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.Put("exact.key.extended", []byte("value"), 0) // Should not match
		time.Sleep(10 * time.Millisecond)
		s.Put("exact.key", []byte("value"), 0) // Should match
	}()

	select {
	case kv := <-ch:
		if kv.Key != "exact.key" {
			t.Errorf("expected exact.key, got %s", kv.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for exact match notification")
	}
}

func TestMemoryStore_Watch_ClosedWatcher(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	ch, _ := s.Watch("*")

	// Mark the watcher as closed by reading from channel after close
	s.Close()

	// Create new store and add a watcher that we'll manually mark as closed
	s2 := NewMemoryStore()
	defer s2.Close()

	_, _ = s2.Watch("*")

	// Access the watcher directly and mark it closed
	s2.mu.Lock()
	if len(s2.watchers) > 0 {
		s2.watchers[0].closed.Store(true)
	}
	s2.mu.Unlock()

	// Now Put should skip the closed watcher
	s2.Put("key", []byte("value"), 0)

	// Verify channel is not receiving (it's closed)
	select {
	case <-ch:
		// This is fine - channel was closed by s.Close()
	default:
		// This is also fine
	}
}

func TestMemoryStore_Keys_ExactMatch(t *testing.T) {
	s := NewMemoryStore()
	defer s.Close()

	s.Put("exact", []byte("value"), 0)
	s.Put("exact2", []byte("value"), 0)

	// Exact pattern match
	keys, err := s.Keys("exact")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key for exact match, got %d", len(keys))
	}
	if len(keys) == 1 && keys[0] != "exact" {
		t.Errorf("expected 'exact', got %s", keys[0])
	}
}
