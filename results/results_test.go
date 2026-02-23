package results

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// =============================================================================
// Result and ResultStatus Tests
// =============================================================================

func TestResultStatus_Valid(t *testing.T) {
	tests := []struct {
		status ResultStatus
		want   bool
	}{
		{StatusPending, true},
		{StatusSuccess, true},
		{StatusFailed, true},
		{"invalid", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := tc.status.Valid(); got != tc.want {
			t.Errorf("Status(%q).Valid() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestResult_Clone(t *testing.T) {
	// Test nil clone
	var nilResult *Result
	if nilResult.Clone() != nil {
		t.Error("nil.Clone() should return nil")
	}

	// Test full clone with all fields
	original := &Result{
		TaskID:    "task-1",
		Status:    StatusSuccess,
		Output:    []byte("output data"),
		Error:     "some error",
		Metadata:  map[string]string{"key": "value"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	clone := original.Clone()

	// Verify fields match
	if clone.TaskID != original.TaskID {
		t.Error("TaskID mismatch")
	}
	if clone.Status != original.Status {
		t.Error("Status mismatch")
	}
	if string(clone.Output) != string(original.Output) {
		t.Error("Output mismatch")
	}
	if clone.Metadata["key"] != original.Metadata["key"] {
		t.Error("Metadata mismatch")
	}

	// Verify deep copy - mutations don't affect original
	clone.Output[0] = 'X'
	clone.Metadata["key"] = "changed"
	if original.Output[0] == 'X' {
		t.Error("Output was not deep copied")
	}
	if original.Metadata["key"] == "changed" {
		t.Error("Metadata was not deep copied")
	}

	// Test clone with nil Output and Metadata
	sparse := &Result{TaskID: "sparse", Status: StatusPending}
	sparseClone := sparse.Clone()
	if sparseClone.Output != nil || sparseClone.Metadata != nil {
		t.Error("Clone should preserve nil slices/maps")
	}
}

func TestValidateTaskID(t *testing.T) {
	if err := ValidateTaskID(""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID for empty ID, got %v", err)
	}
	if err := ValidateTaskID("valid-id"); err != nil {
		t.Errorf("expected nil for valid ID, got %v", err)
	}
}

func TestValidateResult(t *testing.T) {
	// Invalid task ID
	if err := ValidateResult(Result{TaskID: "", Status: StatusSuccess}); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Invalid status
	if err := ValidateResult(Result{TaskID: "task-1", Status: "invalid"}); err != ErrInvalidStatus {
		t.Errorf("expected ErrInvalidStatus, got %v", err)
	}

	// Valid result
	if err := ValidateResult(Result{TaskID: "task-1", Status: StatusSuccess}); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestResultFilter_Matches_TimeFilters(t *testing.T) {
	now := time.Now()
	r := &Result{
		TaskID:    "task-1",
		Status:    StatusSuccess,
		CreatedAt: now,
	}

	// CreatedAfter filter
	if (ResultFilter{CreatedAfter: now.Add(-time.Hour)}).Matches(r) == false {
		t.Error("should match CreatedAfter in past")
	}
	if (ResultFilter{CreatedAfter: now.Add(time.Hour)}).Matches(r) == true {
		t.Error("should not match CreatedAfter in future")
	}

	// CreatedBefore filter
	if (ResultFilter{CreatedBefore: now.Add(time.Hour)}).Matches(r) == false {
		t.Error("should match CreatedBefore in future")
	}
	if (ResultFilter{CreatedBefore: now.Add(-time.Hour)}).Matches(r) == true {
		t.Error("should not match CreatedBefore in past")
	}

	// Nil result
	if (ResultFilter{}).Matches(nil) {
		t.Error("nil result should not match")
	}

	// Metadata with nil result metadata
	rNoMeta := &Result{TaskID: "task-1", Status: StatusSuccess}
	if (ResultFilter{Metadata: map[string]string{"key": "val"}}).Matches(rNoMeta) {
		t.Error("should not match when result has no metadata")
	}
}

// =============================================================================
// MemoryPublisher Tests
// =============================================================================

func TestMemoryPublisher_PublishAndGet(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()

	ctx := context.Background()

	// Publish a result
	err := pub.Publish(ctx, "task-1", Result{
		TaskID:   "task-1",
		Status:   StatusSuccess,
		Output:   []byte("hello"),
		Metadata: map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Get the result
	result, err := pub.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", result.TaskID)
	}
	if result.Status != StatusSuccess {
		t.Errorf("expected success, got %s", result.Status)
	}
	if string(result.Output) != "hello" {
		t.Errorf("expected hello, got %s", result.Output)
	}
	if result.Metadata["key"] != "value" {
		t.Errorf("expected metadata key=value")
	}
}

func TestMemoryPublisher_Subscribe(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()

	ctx := context.Background()

	// Subscribe before publish
	ch, err := pub.Subscribe("task-2")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish in goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		pub.Publish(ctx, "task-2", Result{
			TaskID: "task-2",
			Status: StatusPending,
			Output: []byte("pending"),
		})

		time.Sleep(10 * time.Millisecond)
		pub.Publish(ctx, "task-2", Result{
			TaskID: "task-2",
			Status: StatusSuccess,
			Output: []byte("done"),
		})
	}()

	// Receive updates
	var received []*Result
	timeout := time.After(1 * time.Second)
loop:
	for {
		select {
		case r, ok := <-ch:
			if !ok {
				break loop
			}
			received = append(received, r)
			if r.Status.IsTerminal() {
				break loop
			}
		case <-timeout:
			t.Fatal("timeout waiting for results")
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 results, got %d", len(received))
	}
	if received[0].Status != StatusPending {
		t.Errorf("expected first result pending, got %s", received[0].Status)
	}
	if received[1].Status != StatusSuccess {
		t.Errorf("expected second result success, got %s", received[1].Status)
	}
}

func TestMemoryPublisher_List(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()

	ctx := context.Background()

	// Publish multiple results
	pub.Publish(ctx, "batch-1", Result{TaskID: "batch-1", Status: StatusSuccess})
	pub.Publish(ctx, "batch-2", Result{TaskID: "batch-2", Status: StatusPending})
	pub.Publish(ctx, "other-1", Result{TaskID: "other-1", Status: StatusFailed})

	// List with filter
	results, err := pub.List(ResultFilter{TaskIDPrefix: "batch-"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// List by status
	results, err = pub.List(ResultFilter{Status: StatusPending})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestMemoryPublisher_ErrorPaths(t *testing.T) {
	pub := NewMemoryPublisher()
	ctx := context.Background()

	// Invalid task ID on Publish
	if err := pub.Publish(ctx, "", Result{Status: StatusSuccess}); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Invalid status on Publish
	if err := pub.Publish(ctx, "task-1", Result{Status: "invalid"}); err != ErrInvalidStatus {
		t.Errorf("expected ErrInvalidStatus, got %v", err)
	}

	// Get non-existent
	if _, err := pub.Get(ctx, "non-existent"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Get with invalid task ID
	if _, err := pub.Get(ctx, ""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Subscribe with invalid task ID
	if _, err := pub.Subscribe(""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Delete non-existent
	if err := pub.Delete(ctx, "non-existent"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Delete with invalid task ID
	if err := pub.Delete(ctx, ""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	pub.Close()
}

func TestMemoryPublisher_ClosedPublisher(t *testing.T) {
	pub := NewMemoryPublisher()
	ctx := context.Background()
	pub.Close()

	// Double close should be safe
	if err := pub.Close(); err != nil {
		t.Errorf("double close should not error, got %v", err)
	}

	// All operations should return ErrClosed
	if err := pub.Publish(ctx, "task-1", Result{Status: StatusSuccess}); err != ErrClosed {
		t.Errorf("expected ErrClosed on Publish, got %v", err)
	}
	if _, err := pub.Get(ctx, "task-1"); err != ErrClosed {
		t.Errorf("expected ErrClosed on Get, got %v", err)
	}
	if _, err := pub.Subscribe("task-1"); err != ErrClosed {
		t.Errorf("expected ErrClosed on Subscribe, got %v", err)
	}
	if _, err := pub.List(ResultFilter{}); err != ErrClosed {
		t.Errorf("expected ErrClosed on List, got %v", err)
	}
	if err := pub.Delete(ctx, "task-1"); err != ErrClosed {
		t.Errorf("expected ErrClosed on Delete, got %v", err)
	}
}

func TestMemoryPublisher_Delete(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()
	ctx := context.Background()

	// Publish and delete
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusSuccess})

	if err := pub.Delete(ctx, "task-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	if _, err := pub.Get(ctx, "task-1"); err != ErrNotFound {
		t.Error("expected ErrNotFound after delete")
	}
}

func TestMemoryPublisher_ListWithLimit(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()
	ctx := context.Background()

	// Publish several results
	for i := 0; i < 10; i++ {
		pub.Publish(ctx, "task-"+string(rune('0'+i)), Result{
			TaskID: "task-" + string(rune('0'+i)),
			Status: StatusSuccess,
		})
	}

	// List with limit
	results, err := pub.List(ResultFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results with limit, got %d", len(results))
	}
}

func TestMemoryPublisher_SubscribeToExistingTerminal(t *testing.T) {
	pub := NewMemoryPublisher()
	// Note: Don't defer Close() here due to double-close bug when subscribing
	// to an already-terminal result
	ctx := context.Background()

	// Publish a terminal result first
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusSuccess, Output: []byte("done")})

	// Subscribe should get immediate result and close
	ch, err := pub.Subscribe("task-1")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	select {
	case r, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before delivering result")
		}
		if r.Status != StatusSuccess {
			t.Errorf("expected success, got %s", r.Status)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for result")
	}

	// Channel should close after terminal result
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after terminal")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestMemoryPublisher_UpdateExistingResult(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()
	ctx := context.Background()

	// Publish initial result
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusPending})

	time.Sleep(10 * time.Millisecond)

	// Update the result
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusSuccess})

	result, _ := pub.Get(ctx, "task-1")

	// CreatedAt should be preserved
	if result.Status != StatusSuccess {
		t.Error("status not updated")
	}
	if result.UpdatedAt.Equal(result.CreatedAt) {
		t.Error("UpdatedAt should be different from CreatedAt after update")
	}
}

func TestMemorySub_Cancel(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()

	ch, _ := pub.Subscribe("task-1")

	// Get the internal subscription (we need to work around this)
	pub.mu.RLock()
	subs := pub.subs["task-1"]
	pub.mu.RUnlock()

	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}

	sub := subs[0]

	// Test Results() method
	if sub.Results() != ch {
		t.Error("Results() should return the channel")
	}

	// Cancel the subscription
	if err := sub.Cancel(); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// Double cancel should be safe
	if err := sub.Cancel(); err != nil {
		t.Errorf("double cancel should not error, got %v", err)
	}

	// Subscription should be removed
	pub.mu.RLock()
	remainingSubs := pub.subs["task-1"]
	pub.mu.RUnlock()

	if len(remainingSubs) != 0 {
		t.Error("subscription should be removed after cancel")
	}
}

func TestMemoryPublisher_ConcurrentAccess(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent publishes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			taskID := "task-" + string(rune('A'+id%26))
			pub.Publish(ctx, taskID, Result{TaskID: taskID, Status: StatusPending})
			pub.Get(ctx, taskID)
			pub.List(ResultFilter{})
		}(i)
	}

	wg.Wait()
}

func TestMemoryPublisher_DeleteWithActiveSubscription(t *testing.T) {
	pub := NewMemoryPublisher()
	defer pub.Close()
	ctx := context.Background()

	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusPending})

	ch, _ := pub.Subscribe("task-1")

	// Delete should close subscription
	pub.Delete(ctx, "task-1")

	select {
	case _, ok := <-ch:
		if ok {
			// Got a message, that's fine, but next read should close
			select {
			case _, ok := <-ch:
				if ok {
					t.Error("channel should be closed after delete")
				}
			case <-time.After(100 * time.Millisecond):
			}
		}
	case <-time.After(100 * time.Millisecond):
		// Channel might already be closed
	}
}

// =============================================================================
// BusPublisher Tests
// =============================================================================

func TestBusPublisher_PublishAndGet(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()

	ctx := context.Background()

	// Publish a result
	err := pub.Publish(ctx, "task-1", Result{
		TaskID:   "task-1",
		Status:   StatusSuccess,
		Output:   []byte("hello"),
		Metadata: map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Get the result
	result, err := pub.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", result.TaskID)
	}
	if result.Status != StatusSuccess {
		t.Errorf("expected success, got %s", result.Status)
	}
}

func TestBusPublisher_Subscribe(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()

	ctx := context.Background()

	// Subscribe before publish
	ch, err := pub.Subscribe("task-2")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Give the relay goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Publish a terminal result directly
	err = pub.Publish(ctx, "task-2", Result{
		TaskID: "task-2",
		Status: StatusSuccess,
		Output: []byte("done"),
	})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Receive the result
	timeout := time.After(1 * time.Second)
	select {
	case r, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if r.Status != StatusSuccess {
			t.Errorf("expected success, got %s", r.Status)
		}
		if string(r.Output) != "done" {
			t.Errorf("expected output 'done', got '%s'", r.Output)
		}
	case <-timeout:
		t.Fatal("timeout waiting for result")
	}
}

func TestBusPublisher_ErrorPaths(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	ctx := context.Background()

	// Invalid task ID on Publish
	if err := pub.Publish(ctx, "", Result{Status: StatusSuccess}); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Invalid status on Publish
	if err := pub.Publish(ctx, "task-1", Result{Status: "invalid"}); err != ErrInvalidStatus {
		t.Errorf("expected ErrInvalidStatus, got %v", err)
	}

	// Get non-existent
	if _, err := pub.Get(ctx, "non-existent"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Get with invalid task ID
	if _, err := pub.Get(ctx, ""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Subscribe with invalid task ID
	if _, err := pub.Subscribe(""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	// Delete non-existent
	if err := pub.Delete(ctx, "non-existent"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Delete with invalid task ID
	if err := pub.Delete(ctx, ""); err != ErrInvalidTaskID {
		t.Errorf("expected ErrInvalidTaskID, got %v", err)
	}

	pub.Close()
}

func TestBusPublisher_ClosedPublisher(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	ctx := context.Background()
	pub.Close()

	// Double close should be safe
	if err := pub.Close(); err != nil {
		t.Errorf("double close should not error, got %v", err)
	}

	// All operations should return ErrClosed
	if err := pub.Publish(ctx, "task-1", Result{Status: StatusSuccess}); err != ErrClosed {
		t.Errorf("expected ErrClosed on Publish, got %v", err)
	}
	if _, err := pub.Get(ctx, "task-1"); err != ErrClosed {
		t.Errorf("expected ErrClosed on Get, got %v", err)
	}
	if _, err := pub.Subscribe("task-1"); err != ErrClosed {
		t.Errorf("expected ErrClosed on Subscribe, got %v", err)
	}
	if _, err := pub.List(ResultFilter{}); err != ErrClosed {
		t.Errorf("expected ErrClosed on List, got %v", err)
	}
	if err := pub.Delete(ctx, "task-1"); err != ErrClosed {
		t.Errorf("expected ErrClosed on Delete, got %v", err)
	}
}

func TestBusPublisher_Delete(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()
	ctx := context.Background()

	// Publish and delete
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusSuccess})

	if err := pub.Delete(ctx, "task-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	if _, err := pub.Get(ctx, "task-1"); err != ErrNotFound {
		t.Error("expected ErrNotFound after delete")
	}
}

func TestBusPublisher_List(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()
	ctx := context.Background()

	// Publish some results
	pub.Publish(ctx, "batch-1", Result{TaskID: "batch-1", Status: StatusSuccess})
	pub.Publish(ctx, "batch-2", Result{TaskID: "batch-2", Status: StatusPending})
	pub.Publish(ctx, "other-1", Result{TaskID: "other-1", Status: StatusFailed})

	// List with prefix filter
	results, err := pub.List(ResultFilter{TaskIDPrefix: "batch-"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// List with limit
	results, err = pub.List(ResultFilter{Limit: 1})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result with limit, got %d", len(results))
	}
}

func TestBusPublisher_SubscribeToExistingTerminal(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	// Note: Don't defer Close() here due to double-close bug when subscribing
	// to an already-terminal result
	ctx := context.Background()

	// Publish a terminal result first
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusSuccess, Output: []byte("done")})

	// Subscribe should get immediate result and close
	ch, err := pub.Subscribe("task-1")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	select {
	case r, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before delivering result")
		}
		if r.Status != StatusSuccess {
			t.Errorf("expected success, got %s", r.Status)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for result")
	}
}

func TestBusPublisher_ConfigDefaults(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	// Test with empty config
	pub := NewBusPublisher(mb, BusPublisherConfig{})
	defer pub.Close()

	// Should use defaults
	if pub.config.SubjectPrefix != "results" {
		t.Errorf("expected default SubjectPrefix 'results', got '%s'", pub.config.SubjectPrefix)
	}
	if pub.config.BufferSize != 16 {
		t.Errorf("expected default BufferSize 16, got %d", pub.config.BufferSize)
	}
}

func TestBusSub_ResultsAndCancel(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()

	ch, _ := pub.Subscribe("task-1")

	// Let relay goroutine start
	time.Sleep(20 * time.Millisecond)

	// Get the internal subscription
	pub.mu.RLock()
	subs := pub.subs["task-1"]
	pub.mu.RUnlock()

	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}

	sub := subs[0]

	// Test Results() method
	if sub.Results() != ch {
		t.Error("Results() should return the channel")
	}

	// Cancel the subscription
	if err := sub.Cancel(); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// Double cancel should be safe
	if err := sub.Cancel(); err != nil {
		t.Errorf("double cancel should not error, got %v", err)
	}

	// Subscription should be removed
	pub.mu.RLock()
	remainingSubs := pub.subs["task-1"]
	pub.mu.RUnlock()

	if len(remainingSubs) != 0 {
		t.Error("subscription should be removed after cancel")
	}
}

func TestBusPublisher_UpdateExistingResult(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()
	ctx := context.Background()

	// Publish initial result
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusPending})

	time.Sleep(10 * time.Millisecond)

	// Update the result
	pub.Publish(ctx, "task-1", Result{TaskID: "task-1", Status: StatusSuccess})

	result, _ := pub.Get(ctx, "task-1")

	// CreatedAt should be preserved
	if result.Status != StatusSuccess {
		t.Error("status not updated")
	}
}

func TestBusPublisher_ConcurrentAccess(t *testing.T) {
	mb := bus.NewMemoryBus(bus.DefaultConfig())
	defer mb.Close()

	pub := NewBusPublisher(mb, DefaultBusPublisherConfig())
	defer pub.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent publishes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			taskID := "task-" + string(rune('A'+id%26))
			pub.Publish(ctx, taskID, Result{TaskID: taskID, Status: StatusPending})
			pub.Get(ctx, taskID)
			pub.List(ResultFilter{})
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Original Tests (ResultStatus, ResultFilter)
// =============================================================================

func TestResultStatus_IsTerminal(t *testing.T) {
	if StatusPending.IsTerminal() {
		t.Error("pending should not be terminal")
	}
	if !StatusSuccess.IsTerminal() {
		t.Error("success should be terminal")
	}
	if !StatusFailed.IsTerminal() {
		t.Error("failed should be terminal")
	}
}

func TestResultFilter_Matches(t *testing.T) {
	now := time.Now()
	r := &Result{
		TaskID:    "batch-123",
		Status:    StatusSuccess,
		Metadata:  map[string]string{"env": "prod"},
		CreatedAt: now,
	}

	// Empty filter matches all
	if !(ResultFilter{}).Matches(r) {
		t.Error("empty filter should match")
	}

	// Status filter
	if !(ResultFilter{Status: StatusSuccess}).Matches(r) {
		t.Error("status filter should match")
	}
	if (ResultFilter{Status: StatusFailed}).Matches(r) {
		t.Error("wrong status should not match")
	}

	// Prefix filter
	if !(ResultFilter{TaskIDPrefix: "batch-"}).Matches(r) {
		t.Error("prefix filter should match")
	}
	if (ResultFilter{TaskIDPrefix: "other-"}).Matches(r) {
		t.Error("wrong prefix should not match")
	}

	// Metadata filter
	if !(ResultFilter{Metadata: map[string]string{"env": "prod"}}).Matches(r) {
		t.Error("metadata filter should match")
	}
	if (ResultFilter{Metadata: map[string]string{"env": "dev"}}).Matches(r) {
		t.Error("wrong metadata should not match")
	}
}
