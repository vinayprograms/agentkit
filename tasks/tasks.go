package tasks

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	// ErrTaskNotFound indicates the requested task does not exist.
	ErrTaskNotFound = errors.New("task not found")

	// ErrTaskAlreadyClaimed indicates the task is already claimed by a worker.
	ErrTaskAlreadyClaimed = errors.New("task already claimed")

	// ErrTaskNotClaimed indicates the task must be claimed before this operation.
	ErrTaskNotClaimed = errors.New("task not claimed")

	// ErrTaskCompleted indicates the task has already been completed.
	ErrTaskCompleted = errors.New("task already completed")

	// ErrTaskFailed indicates the task has permanently failed.
	ErrTaskFailed = errors.New("task has failed permanently")

	// ErrMaxAttemptsReached indicates no more retries are allowed.
	ErrMaxAttemptsReached = errors.New("maximum attempts reached")

	// ErrInvalidTask indicates the task is invalid (missing required fields).
	ErrInvalidTask = errors.New("invalid task")

	// ErrInvalidWorkerID indicates the worker ID is invalid.
	ErrInvalidWorkerID = errors.New("invalid worker ID")

	// ErrWrongWorker indicates the operation was attempted by the wrong worker.
	ErrWrongWorker = errors.New("task claimed by different worker")

	// ErrStoreClosed indicates the underlying store has been closed.
	ErrStoreClosed = errors.New("store closed")
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	// StatusPending indicates the task is waiting to be claimed.
	StatusPending TaskStatus = "pending"

	// StatusClaimed indicates the task has been claimed by a worker.
	StatusClaimed TaskStatus = "claimed"

	// StatusCompleted indicates the task has been successfully completed.
	StatusCompleted TaskStatus = "completed"

	// StatusFailed indicates the task has permanently failed.
	StatusFailed TaskStatus = "failed"
)

// String returns the string representation of the status.
func (s TaskStatus) String() string {
	return string(s)
}

// IsTerminal returns true if the status is a terminal state.
func (s TaskStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

// Task represents a unit of work to be processed.
type Task struct {
	// ID is the unique identifier for the task.
	// Generated automatically on submission if empty.
	ID string

	// IdempotencyKey is used for deduplication.
	// Tasks with the same IdempotencyKey will not be duplicated.
	IdempotencyKey string

	// Payload is the task data to be processed.
	Payload []byte

	// Status is the current state of the task.
	Status TaskStatus

	// Attempts is the number of times this task has been attempted.
	Attempts int

	// MaxAttempts is the maximum number of attempts allowed.
	// Zero means unlimited attempts.
	MaxAttempts int

	// CreatedAt is when the task was created.
	CreatedAt time.Time

	// ClaimedAt is when the task was last claimed.
	ClaimedAt *time.Time

	// ClaimedBy is the worker ID that currently holds the task.
	ClaimedBy string

	// CompletedAt is when the task was completed or failed.
	CompletedAt *time.Time

	// Result is the output from task completion.
	Result []byte

	// Error is the error message if the task failed.
	Error string
}

// Clone creates a deep copy of the task.
func (t *Task) Clone() *Task {
	clone := &Task{
		ID:             t.ID,
		IdempotencyKey: t.IdempotencyKey,
		Status:         t.Status,
		Attempts:       t.Attempts,
		MaxAttempts:    t.MaxAttempts,
		CreatedAt:      t.CreatedAt,
		ClaimedBy:      t.ClaimedBy,
		Error:          t.Error,
	}

	if t.Payload != nil {
		clone.Payload = make([]byte, len(t.Payload))
		copy(clone.Payload, t.Payload)
	}

	if t.ClaimedAt != nil {
		claimed := *t.ClaimedAt
		clone.ClaimedAt = &claimed
	}

	if t.CompletedAt != nil {
		completed := *t.CompletedAt
		clone.CompletedAt = &completed
	}

	if t.Result != nil {
		clone.Result = make([]byte, len(t.Result))
		copy(clone.Result, t.Result)
	}

	return clone
}

// TaskManager provides idempotent task management.
type TaskManager interface {
	// Submit creates a new task or returns the existing task ID if
	// a task with the same IdempotencyKey already exists.
	// Returns the task ID.
	Submit(ctx context.Context, task Task) (string, error)

	// Claim marks a task as being worked on by a specific worker.
	// Returns ErrTaskAlreadyClaimed if the task is already claimed.
	// Returns ErrTaskCompleted if the task is already completed.
	Claim(ctx context.Context, taskID string, workerID string) error

	// Complete marks a task as successfully completed with the given result.
	// The task must be claimed by a worker first.
	// Only the claiming worker can complete the task.
	Complete(ctx context.Context, taskID string, result []byte) error

	// Fail marks a task as failed with the given error.
	// If retries are available, the task moves back to pending.
	// If max attempts reached, the task is permanently failed.
	Fail(ctx context.Context, taskID string, err error) error

	// Retry moves a failed or claimed task back to pending for retry.
	// Returns ErrMaxAttemptsReached if no more attempts are allowed.
	Retry(ctx context.Context, taskID string) error

	// Get retrieves a task by ID.
	// Returns ErrTaskNotFound if the task does not exist.
	Get(ctx context.Context, taskID string) (*Task, error)

	// GetByIdempotencyKey retrieves a task by its idempotency key.
	// Returns ErrTaskNotFound if no task exists with that key.
	GetByIdempotencyKey(ctx context.Context, key string) (*Task, error)

	// List returns all tasks matching the given status filter.
	// If status is empty, returns all tasks.
	List(ctx context.Context, status TaskStatus) ([]*Task, error)

	// Delete removes a task by ID.
	// Only terminal tasks (completed/failed) can be deleted.
	Delete(ctx context.Context, taskID string) error

	// Close releases resources held by the manager.
	Close() error
}
