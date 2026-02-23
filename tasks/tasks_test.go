package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/state"
)

func TestManagerSubmit(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit a task
	task := Task{
		IdempotencyKey: "test-key-1",
		Payload:        []byte("test payload"),
		MaxAttempts:    3,
	}

	id, err := mgr.Submit(ctx, task)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("Expected non-empty task ID")
	}

	// Verify task was created
	got, err := mgr.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("Expected status pending, got %s", got.Status)
	}
	if got.IdempotencyKey != "test-key-1" {
		t.Errorf("Expected idempotency key test-key-1, got %s", got.IdempotencyKey)
	}
}

func TestManagerIdempotency(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit first task
	task := Task{
		IdempotencyKey: "idempotent-key",
		Payload:        []byte("payload"),
	}

	id1, err := mgr.Submit(ctx, task)
	if err != nil {
		t.Fatalf("First submit failed: %v", err)
	}

	// Submit duplicate with same idempotency key
	id2, err := mgr.Submit(ctx, Task{
		IdempotencyKey: "idempotent-key",
		Payload:        []byte("different payload"),
	})
	if err != nil {
		t.Fatalf("Second submit failed: %v", err)
	}

	// Should return same ID
	if id1 != id2 {
		t.Errorf("Expected same ID for idempotent submissions, got %s and %s", id1, id2)
	}

	// Only one task should exist
	tasks, err := mgr.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}
}

func TestManagerClaimAndComplete(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit task
	id, err := mgr.Submit(ctx, Task{
		Payload: []byte("work"),
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Claim task
	err = mgr.Claim(ctx, id, "worker-1")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	// Verify claimed
	task, _ := mgr.Get(ctx, id)
	if task.Status != StatusClaimed {
		t.Errorf("Expected status claimed, got %s", task.Status)
	}
	if task.ClaimedBy != "worker-1" {
		t.Errorf("Expected claimed by worker-1, got %s", task.ClaimedBy)
	}
	if task.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", task.Attempts)
	}

	// Complete task
	err = mgr.Complete(ctx, id, []byte("result"))
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Verify completed
	task, _ = mgr.Get(ctx, id)
	if task.Status != StatusCompleted {
		t.Errorf("Expected status completed, got %s", task.Status)
	}
	if string(task.Result) != "result" {
		t.Errorf("Expected result 'result', got %s", task.Result)
	}
}

func TestManagerClaimConflict(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit and claim
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	mgr.Claim(ctx, id, "worker-1")

	// Try to claim with different worker
	err := mgr.Claim(ctx, id, "worker-2")
	if err != ErrTaskAlreadyClaimed {
		t.Errorf("Expected ErrTaskAlreadyClaimed, got %v", err)
	}

	// Same worker re-claim should succeed (idempotent)
	err = mgr.Claim(ctx, id, "worker-1")
	if err != nil {
		t.Errorf("Expected idempotent re-claim to succeed, got %v", err)
	}
}

func TestManagerFailAndRetry(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit with max attempts
	id, _ := mgr.Submit(ctx, Task{
		Payload:     []byte("work"),
		MaxAttempts: 2,
	})

	// First attempt fails
	mgr.Claim(ctx, id, "worker-1")
	err := mgr.Fail(ctx, id, errors.New("temp error"))
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	// Should be pending again
	task, _ := mgr.Get(ctx, id)
	if task.Status != StatusPending {
		t.Errorf("Expected status pending after first fail, got %s", task.Status)
	}

	// Second attempt fails
	mgr.Claim(ctx, id, "worker-1")
	err = mgr.Fail(ctx, id, errors.New("another error"))
	if err != nil {
		t.Fatalf("Second fail failed: %v", err)
	}

	// Should be permanently failed now (2 attempts = max)
	task, _ = mgr.Get(ctx, id)
	if task.Status != StatusFailed {
		t.Errorf("Expected status failed after max attempts, got %s", task.Status)
	}
}

func TestManagerRetryManual(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit and fail
	id, _ := mgr.Submit(ctx, Task{
		Payload:     []byte("work"),
		MaxAttempts: 1,
	})
	mgr.Claim(ctx, id, "worker-1")
	mgr.Fail(ctx, id, errors.New("error"))

	// Should be permanently failed
	task, _ := mgr.Get(ctx, id)
	if task.Status != StatusFailed {
		t.Fatalf("Expected failed status")
	}

	// Manual retry should fail (max attempts reached)
	err := mgr.Retry(ctx, id)
	if err != ErrMaxAttemptsReached {
		t.Errorf("Expected ErrMaxAttemptsReached, got %v", err)
	}
}

func TestManagerGetByIdempotencyKey(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit task
	id, _ := mgr.Submit(ctx, Task{
		IdempotencyKey: "lookup-key",
		Payload:        []byte("data"),
	})

	// Get by idempotency key
	task, err := mgr.GetByIdempotencyKey(ctx, "lookup-key")
	if err != nil {
		t.Fatalf("GetByIdempotencyKey failed: %v", err)
	}
	if task.ID != id {
		t.Errorf("Expected ID %s, got %s", id, task.ID)
	}

	// Non-existent key
	_, err = mgr.GetByIdempotencyKey(ctx, "no-such-key")
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound, got %v", err)
	}
}

func TestManagerDelete(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit and complete
	id, _ := mgr.Submit(ctx, Task{
		IdempotencyKey: "delete-test",
		Payload:        []byte("data"),
	})
	mgr.Claim(ctx, id, "worker")
	mgr.Complete(ctx, id, nil)

	// Delete completed task
	err := mgr.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not exist
	_, err = mgr.Get(ctx, id)
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound after delete, got %v", err)
	}
}

func TestManagerListByStatus(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Create tasks in different states
	id1, _ := mgr.Submit(ctx, Task{Payload: []byte("1")})
	id2, _ := mgr.Submit(ctx, Task{Payload: []byte("2")})
	id3, _ := mgr.Submit(ctx, Task{Payload: []byte("3")})

	mgr.Claim(ctx, id2, "worker")
	mgr.Claim(ctx, id3, "worker")
	mgr.Complete(ctx, id3, nil)

	// List pending
	pending, _ := mgr.List(ctx, StatusPending)
	if len(pending) != 1 || pending[0].ID != id1 {
		t.Errorf("Expected 1 pending task (id1)")
	}

	// List claimed
	claimed, _ := mgr.List(ctx, StatusClaimed)
	if len(claimed) != 1 || claimed[0].ID != id2 {
		t.Errorf("Expected 1 claimed task (id2)")
	}

	// List completed
	completed, _ := mgr.List(ctx, StatusCompleted)
	if len(completed) != 1 || completed[0].ID != id3 {
		t.Errorf("Expected 1 completed task (id3)")
	}

	// List all
	all, _ := mgr.List(ctx, "")
	if len(all) != 3 {
		t.Errorf("Expected 3 total tasks, got %d", len(all))
	}
}

func TestTaskClone(t *testing.T) {
	now := time.Now()
	original := &Task{
		ID:             "test-id",
		IdempotencyKey: "test-key",
		Payload:        []byte("payload"),
		Status:         StatusClaimed,
		Attempts:       1,
		MaxAttempts:    3,
		CreatedAt:      now,
		ClaimedAt:      &now,
		ClaimedBy:      "worker",
		Result:         []byte("result"),
		Error:          "some error",
	}

	clone := original.Clone()

	// Modify original
	original.Payload[0] = 'X'
	original.Result[0] = 'Y'

	// Clone should be unaffected
	if clone.Payload[0] == 'X' {
		t.Error("Clone payload should not be affected by original modification")
	}
	if clone.Result[0] == 'Y' {
		t.Error("Clone result should not be affected by original modification")
	}
}

func TestTaskStatusIsTerminal(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		terminal bool
	}{
		{StatusPending, false},
		{StatusClaimed, false},
		{StatusCompleted, true},
		{StatusFailed, true},
	}

	for _, tt := range tests {
		if tt.status.IsTerminal() != tt.terminal {
			t.Errorf("Status %s: expected terminal=%v, got %v", tt.status, tt.terminal, !tt.terminal)
		}
	}
}

func TestManagerInvalidOperations(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit without payload
	_, err := mgr.Submit(ctx, Task{})
	if err != ErrInvalidTask {
		t.Errorf("Expected ErrInvalidTask for empty payload, got %v", err)
	}

	// Claim with empty worker ID
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	err = mgr.Claim(ctx, id, "")
	if err != ErrInvalidWorkerID {
		t.Errorf("Expected ErrInvalidWorkerID, got %v", err)
	}

	// Complete without claim
	err = mgr.Complete(ctx, id, nil)
	if err != ErrTaskNotClaimed {
		t.Errorf("Expected ErrTaskNotClaimed, got %v", err)
	}

	// Fail without claim
	err = mgr.Fail(ctx, id, errors.New("err"))
	if err != ErrTaskNotClaimed {
		t.Errorf("Expected ErrTaskNotClaimed, got %v", err)
	}

	// Get non-existent
	_, err = mgr.Get(ctx, "no-such-id")
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound, got %v", err)
	}
}

func TestWithIDGenerator(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	customID := "custom-task-id-123"
	mgr := NewManager(store, WithIDGenerator(func() string {
		return customID
	}))
	defer mgr.Close()

	ctx := context.Background()

	id, err := mgr.Submit(ctx, Task{Payload: []byte("work")})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id != customID {
		t.Errorf("Expected custom ID %s, got %s", customID, id)
	}
}

func TestTaskStatusString(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusClaimed, "claimed"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("Status %v.String() = %s, want %s", tt.status, got, tt.expected)
		}
	}
}

func TestManagerRetryFromClaimed(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit and claim
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	mgr.Claim(ctx, id, "worker-1")

	// Retry from claimed state should work
	err := mgr.Retry(ctx, id)
	if err != nil {
		t.Fatalf("Retry from claimed failed: %v", err)
	}

	// Should be pending again
	task, _ := mgr.Get(ctx, id)
	if task.Status != StatusPending {
		t.Errorf("Expected status pending after retry, got %s", task.Status)
	}
	if task.ClaimedBy != "" {
		t.Errorf("Expected no claimant after retry, got %s", task.ClaimedBy)
	}
}

func TestManagerRetryIdempotent(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit task (already pending)
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})

	// Retry on pending should be idempotent
	err := mgr.Retry(ctx, id)
	if err != nil {
		t.Errorf("Retry on pending task should succeed (idempotent), got %v", err)
	}
}

func TestManagerCompleteIdempotent(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit, claim, complete
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	mgr.Claim(ctx, id, "worker")
	mgr.Complete(ctx, id, []byte("result"))

	// Complete again should be idempotent (no error)
	err := mgr.Complete(ctx, id, []byte("different result"))
	if err != nil {
		t.Errorf("Complete on already-completed task should succeed (idempotent), got %v", err)
	}
}

func TestManagerFailIdempotent(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit with max 1 attempt
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work"), MaxAttempts: 1})
	mgr.Claim(ctx, id, "worker")
	mgr.Fail(ctx, id, errors.New("error"))

	// Fail again should be idempotent
	err := mgr.Fail(ctx, id, errors.New("another error"))
	if err != nil {
		t.Errorf("Fail on already-failed task should succeed (idempotent), got %v", err)
	}
}

func TestManagerClaimOnCompletedAndFailed(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Test claim on completed task
	id1, _ := mgr.Submit(ctx, Task{Payload: []byte("work1")})
	mgr.Claim(ctx, id1, "worker")
	mgr.Complete(ctx, id1, nil)

	err := mgr.Claim(ctx, id1, "worker2")
	if err != ErrTaskCompleted {
		t.Errorf("Expected ErrTaskCompleted, got %v", err)
	}

	// Test claim on failed task
	id2, _ := mgr.Submit(ctx, Task{Payload: []byte("work2"), MaxAttempts: 1})
	mgr.Claim(ctx, id2, "worker")
	mgr.Fail(ctx, id2, errors.New("error"))

	err = mgr.Claim(ctx, id2, "worker2")
	if err != ErrTaskFailed {
		t.Errorf("Expected ErrTaskFailed, got %v", err)
	}
}

func TestManagerCompleteOnFailed(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work"), MaxAttempts: 1})
	mgr.Claim(ctx, id, "worker")
	mgr.Fail(ctx, id, errors.New("error"))

	err := mgr.Complete(ctx, id, nil)
	if err != ErrTaskFailed {
		t.Errorf("Expected ErrTaskFailed, got %v", err)
	}
}

func TestManagerFailOnCompleted(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	mgr.Claim(ctx, id, "worker")
	mgr.Complete(ctx, id, nil)

	err := mgr.Fail(ctx, id, errors.New("error"))
	if err != ErrTaskCompleted {
		t.Errorf("Expected ErrTaskCompleted, got %v", err)
	}
}

func TestManagerRetryOnCompleted(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	mgr.Claim(ctx, id, "worker")
	mgr.Complete(ctx, id, nil)

	err := mgr.Retry(ctx, id)
	if err != ErrTaskCompleted {
		t.Errorf("Expected ErrTaskCompleted, got %v", err)
	}
}

func TestManagerDeleteOnNonTerminal(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Try to delete pending task
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})

	err := mgr.Delete(ctx, id)
	if err != ErrTaskNotClaimed {
		t.Errorf("Expected error deleting pending task, got %v", err)
	}

	// Try to delete claimed task
	mgr.Claim(ctx, id, "worker")

	err = mgr.Delete(ctx, id)
	if err != ErrTaskNotClaimed {
		t.Errorf("Expected error deleting claimed task, got %v", err)
	}
}

func TestManagerDeleteNonExistent(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	err := mgr.Delete(ctx, "non-existent-id")
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound, got %v", err)
	}
}

func TestManagerDoubleClose(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)

	// First close
	err := mgr.Close()
	if err != nil {
		t.Errorf("First close failed: %v", err)
	}

	// Second close should be safe
	err = mgr.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

func TestManagerOperationsAfterClose(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	ctx := context.Background()

	// Submit a task before closing
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})

	// Close the manager
	mgr.Close()

	// All operations should return ErrStoreClosed
	_, err := mgr.Submit(ctx, Task{Payload: []byte("new")})
	if err != ErrStoreClosed {
		t.Errorf("Submit after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Claim(ctx, id, "worker")
	if err != ErrStoreClosed {
		t.Errorf("Claim after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Complete(ctx, id, nil)
	if err != ErrStoreClosed {
		t.Errorf("Complete after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Fail(ctx, id, errors.New("err"))
	if err != ErrStoreClosed {
		t.Errorf("Fail after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Retry(ctx, id)
	if err != ErrStoreClosed {
		t.Errorf("Retry after close: expected ErrStoreClosed, got %v", err)
	}

	_, err = mgr.Get(ctx, id)
	if err != ErrStoreClosed {
		t.Errorf("Get after close: expected ErrStoreClosed, got %v", err)
	}

	_, err = mgr.GetByIdempotencyKey(ctx, "key")
	if err != ErrStoreClosed {
		t.Errorf("GetByIdempotencyKey after close: expected ErrStoreClosed, got %v", err)
	}

	_, err = mgr.List(ctx, "")
	if err != ErrStoreClosed {
		t.Errorf("List after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Delete(ctx, id)
	if err != ErrStoreClosed {
		t.Errorf("Delete after close: expected ErrStoreClosed, got %v", err)
	}
}

func TestManagerGetByEmptyIdempotencyKey(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	_, err := mgr.GetByIdempotencyKey(ctx, "")
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound for empty key, got %v", err)
	}
}

func TestTaskCloneWithNilFields(t *testing.T) {
	// Test clone with nil optional fields
	task := &Task{
		ID:      "test",
		Status:  StatusPending,
		Payload: nil,
		Result:  nil,
	}

	clone := task.Clone()
	if clone.ID != "test" {
		t.Errorf("Expected ID test, got %s", clone.ID)
	}
	if clone.Payload != nil {
		t.Error("Expected nil payload in clone")
	}
	if clone.Result != nil {
		t.Error("Expected nil result in clone")
	}
}

func TestManagerFailWithUnlimitedRetries(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Submit with unlimited attempts (MaxAttempts = 0)
	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work"), MaxAttempts: 0})

	// Fail multiple times, should always go back to pending
	for i := 0; i < 5; i++ {
		mgr.Claim(ctx, id, "worker")
		err := mgr.Fail(ctx, id, errors.New("error"))
		if err != nil {
			t.Fatalf("Fail %d failed: %v", i+1, err)
		}

		task, _ := mgr.Get(ctx, id)
		if task.Status != StatusPending {
			t.Errorf("Attempt %d: expected pending status, got %s", i+1, task.Status)
		}
	}
}

func TestManagerFailWithNilError(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	id, _ := mgr.Submit(ctx, Task{Payload: []byte("work")})
	mgr.Claim(ctx, id, "worker")

	// Fail with nil error
	err := mgr.Fail(ctx, id, nil)
	if err != nil {
		t.Fatalf("Fail with nil error failed: %v", err)
	}

	task, _ := mgr.Get(ctx, id)
	if task.Error != "" {
		t.Errorf("Expected empty error string, got %s", task.Error)
	}
}

func TestManagerDeleteFailedTask(t *testing.T) {
	store := state.NewMemoryStore()
	defer store.Close()

	mgr := NewManager(store)
	defer mgr.Close()

	ctx := context.Background()

	// Create and fail a task
	id, _ := mgr.Submit(ctx, Task{
		IdempotencyKey: "delete-failed",
		Payload:        []byte("work"),
		MaxAttempts:    1,
	})
	mgr.Claim(ctx, id, "worker")
	mgr.Fail(ctx, id, errors.New("error"))

	// Delete failed task
	err := mgr.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete failed task failed: %v", err)
	}

	// Should not exist
	_, err = mgr.Get(ctx, id)
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound after delete, got %v", err)
	}
}
