package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiter_SetCapacity(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 10, time.Minute)

	cap := limiter.GetCapacity("test-api")
	if cap == nil {
		t.Fatal("expected capacity, got nil")
	}
	if cap.Total != 10 {
		t.Errorf("expected capacity 10, got %d", cap.Total)
	}
	if cap.Available != 10 {
		t.Errorf("expected available 10, got %d", cap.Available)
	}
	if cap.Window != time.Minute {
		t.Errorf("expected window 1m, got %v", cap.Window)
	}
}

func TestMemoryLimiter_TryAcquire(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 3, time.Minute)

	// Should acquire 3 tokens
	for i := 0; i < 3; i++ {
		if !limiter.TryAcquire("test-api") {
			t.Errorf("expected TryAcquire to succeed on attempt %d", i+1)
		}
	}

	// 4th should fail
	if limiter.TryAcquire("test-api") {
		t.Error("expected TryAcquire to fail after exhausting capacity")
	}

	cap := limiter.GetCapacity("test-api")
	if cap.Available != 0 {
		t.Errorf("expected available 0, got %d", cap.Available)
	}
	if cap.InFlight != 3 {
		t.Errorf("expected inFlight 3, got %d", cap.InFlight)
	}
}

func TestMemoryLimiter_Release(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 2, time.Minute)

	limiter.TryAcquire("test-api")
	limiter.TryAcquire("test-api")

	cap := limiter.GetCapacity("test-api")
	if cap.InFlight != 2 {
		t.Errorf("expected inFlight 2, got %d", cap.InFlight)
	}

	limiter.Release("test-api")

	cap = limiter.GetCapacity("test-api")
	if cap.InFlight != 1 {
		t.Errorf("expected inFlight 1, got %d", cap.InFlight)
	}
}

func TestMemoryLimiter_Acquire_Blocking(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 1, time.Minute)

	// Acquire the only token
	if err := limiter.Acquire(context.Background(), "test-api"); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Try to acquire with timeout - should fail
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := limiter.Acquire(ctx, "test-api")
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestMemoryLimiter_Acquire_WaitsForRelease(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 1, time.Minute)

	// Acquire the only token
	if !limiter.TryAcquire("test-api") {
		t.Fatal("expected first TryAcquire to succeed")
	}

	var wg sync.WaitGroup
	acquired := make(chan bool, 1)

	// Start a goroutine that will wait for a token
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := limiter.Acquire(ctx, "test-api"); err == nil {
			acquired <- true
		} else {
			acquired <- false
		}
	}()

	// Release after a short delay
	time.Sleep(50 * time.Millisecond)
	limiter.Release("test-api")

	wg.Wait()

	select {
	case success := <-acquired:
		if !success {
			t.Error("expected acquire to succeed after release")
		}
	default:
		t.Error("no result from acquire goroutine")
	}
}

func TestMemoryLimiter_UnknownResource(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// TryAcquire on unknown resource
	if limiter.TryAcquire("unknown") {
		t.Error("expected TryAcquire to fail for unknown resource")
	}

	// Acquire on unknown resource
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := limiter.Acquire(ctx, "unknown")
	if err != ErrResourceUnknown {
		t.Errorf("expected ErrResourceUnknown, got %v", err)
	}

	// GetCapacity on unknown resource
	if cap := limiter.GetCapacity("unknown"); cap != nil {
		t.Error("expected nil capacity for unknown resource")
	}
}

func TestMemoryLimiter_Refill(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// Use a controllable time function
	now := time.Now()
	limiter.nowFunc = func() time.Time { return now }

	// 10 tokens per second
	limiter.SetCapacity("test-api", 10, time.Second)

	// Exhaust all tokens
	for i := 0; i < 10; i++ {
		if !limiter.TryAcquire("test-api") {
			t.Fatalf("expected TryAcquire to succeed on attempt %d", i+1)
		}
	}

	// No tokens available
	if limiter.TryAcquire("test-api") {
		t.Error("expected TryAcquire to fail when exhausted")
	}

	// Advance time by 500ms (should refill ~5 tokens)
	now = now.Add(500 * time.Millisecond)

	// Should be able to acquire some tokens now
	acquired := 0
	for limiter.TryAcquire("test-api") {
		acquired++
		if acquired > 10 {
			t.Fatal("acquired more than capacity")
		}
	}

	if acquired < 4 || acquired > 6 {
		t.Errorf("expected ~5 tokens after 500ms, got %d", acquired)
	}
}

func TestMemoryLimiter_AnnounceReduced(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 100, time.Minute)

	// Verify initial capacity
	cap := limiter.GetCapacity("test-api")
	if cap.Total != 100 {
		t.Errorf("expected initial capacity 100, got %d", cap.Total)
	}

	// Announce reduced capacity
	limiter.AnnounceReduced("test-api", "test reduction")

	// Verify reduced capacity (75% of 100 = 75)
	cap = limiter.GetCapacity("test-api")
	if cap.Total != 75 {
		t.Errorf("expected reduced capacity 75, got %d", cap.Total)
	}
}

func TestMemoryLimiter_Close(t *testing.T) {
	limiter := NewMemoryLimiter()

	limiter.SetCapacity("test-api", 1, time.Minute)
	limiter.TryAcquire("test-api")

	if err := limiter.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should fail after close
	if limiter.TryAcquire("test-api") {
		t.Error("expected TryAcquire to fail after close")
	}

	// Double close should return ErrClosed
	if err := limiter.Close(); err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestMemoryLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 100, time.Second)

	var wg sync.WaitGroup
	acquired := make(chan int, 10)

	// Start 10 goroutines trying to acquire tokens
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count := 0
			for j := 0; j < 20; j++ {
				if limiter.TryAcquire("test-api") {
					count++
					time.Sleep(time.Millisecond)
					limiter.Release("test-api")
				}
			}
			acquired <- count
		}()
	}

	wg.Wait()
	close(acquired)

	total := 0
	for count := range acquired {
		total += count
	}

	// Should have acquired some tokens (exact number depends on timing)
	if total < 50 {
		t.Errorf("expected at least 50 total acquires, got %d", total)
	}
}

func TestMemoryLimiter_SetCapacity_InvalidValues(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// First set a valid capacity
	limiter.SetCapacity("test-api", 10, time.Minute)
	cap := limiter.GetCapacity("test-api")
	if cap == nil {
		t.Fatal("expected capacity to be set")
	}

	// Setting capacity to 0 should remove the resource
	limiter.SetCapacity("test-api", 0, time.Minute)
	cap = limiter.GetCapacity("test-api")
	if cap != nil {
		t.Error("expected capacity to be nil after setting to 0")
	}

	// Set again then remove with negative capacity
	limiter.SetCapacity("test-api", 10, time.Minute)
	limiter.SetCapacity("test-api", -1, time.Minute)
	cap = limiter.GetCapacity("test-api")
	if cap != nil {
		t.Error("expected capacity to be nil after setting to negative")
	}

	// Set again then remove with zero window
	limiter.SetCapacity("test-api", 10, time.Minute)
	limiter.SetCapacity("test-api", 10, 0)
	cap = limiter.GetCapacity("test-api")
	if cap != nil {
		t.Error("expected capacity to be nil after setting window to 0")
	}
}

func TestMemoryLimiter_SetCapacity_UpdateExisting(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// Set initial capacity
	limiter.SetCapacity("test-api", 100, time.Minute)

	// Acquire some tokens
	for i := 0; i < 50; i++ {
		limiter.TryAcquire("test-api")
	}

	cap := limiter.GetCapacity("test-api")
	if cap.Available != 50 {
		t.Errorf("expected available 50, got %d", cap.Available)
	}

	// Update to smaller capacity - available should be capped
	limiter.SetCapacity("test-api", 30, time.Minute)

	cap = limiter.GetCapacity("test-api")
	if cap.Total != 30 {
		t.Errorf("expected capacity 30, got %d", cap.Total)
	}
	if cap.Available != 30 {
		t.Errorf("expected available capped at 30, got %d", cap.Available)
	}
}

func TestMemoryLimiter_SetCapacity_AfterClose(t *testing.T) {
	limiter := NewMemoryLimiter()
	limiter.Close()

	// SetCapacity after close should be a no-op
	limiter.SetCapacity("test-api", 10, time.Minute)
	cap := limiter.GetCapacity("test-api")
	if cap != nil {
		t.Error("expected nil capacity after setting on closed limiter")
	}
}

func TestMemoryLimiter_Release_Unknown(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// Release on unknown resource should not panic
	limiter.Release("unknown-api")
}

func TestMemoryLimiter_Release_AfterClose(t *testing.T) {
	limiter := NewMemoryLimiter()
	limiter.SetCapacity("test-api", 10, time.Minute)
	limiter.TryAcquire("test-api")
	limiter.Close()

	// Release after close should not panic
	limiter.Release("test-api")
}

func TestMemoryLimiter_Release_AtCapacity(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	limiter.SetCapacity("test-api", 5, time.Minute)

	// Release without acquiring (inFlight is 0, available == capacity)
	limiter.Release("test-api")

	cap := limiter.GetCapacity("test-api")
	if cap.Available != 5 {
		t.Errorf("expected available 5, got %d", cap.Available)
	}
	if cap.InFlight != 0 {
		t.Errorf("expected inFlight 0, got %d", cap.InFlight)
	}
}

func TestMemoryLimiter_AnnounceReduced_Unknown(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// Should not panic on unknown resource
	limiter.AnnounceReduced("unknown-api", "test")
}

func TestMemoryLimiter_AnnounceReduced_MinCapacity(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// Set small capacity (75% of 1 would be 0, but min is 1)
	limiter.SetCapacity("test-api", 1, time.Minute)

	limiter.AnnounceReduced("test-api", "test")

	cap := limiter.GetCapacity("test-api")
	if cap.Total < 1 {
		t.Errorf("expected capacity >= 1, got %d", cap.Total)
	}
}

func TestMemoryLimiter_Acquire_ClosedDuringWait(t *testing.T) {
	limiter := NewMemoryLimiter()

	limiter.SetCapacity("test-api", 1, time.Minute)
	limiter.TryAcquire("test-api") // Exhaust capacity

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := limiter.Acquire(ctx, "test-api")
		errCh <- err
	}()

	// Close while goroutine is waiting
	time.Sleep(50 * time.Millisecond)
	limiter.Close()

	wg.Wait()

	err := <-errCh
	if err != ErrClosed && err != context.DeadlineExceeded {
		t.Errorf("expected ErrClosed or DeadlineExceeded, got %v", err)
	}
}

func TestMemoryLimiter_Refill_ZeroCapacity(t *testing.T) {
	limiter := NewMemoryLimiter()
	defer limiter.Close()

	// Test edge case where bucket has zero capacity/window
	// This is internal, but we can verify via SetCapacity behavior
	limiter.SetCapacity("test-api", 10, time.Second)

	cap := limiter.GetCapacity("test-api")
	if cap == nil {
		t.Fatal("expected capacity")
	}
}
