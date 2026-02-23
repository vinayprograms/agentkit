package results

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	ErrNotFound       = errors.New("result not found")
	ErrAlreadyExists  = errors.New("result already exists")
	ErrClosed         = errors.New("publisher closed")
	ErrInvalidTaskID  = errors.New("invalid task ID")
	ErrInvalidStatus  = errors.New("invalid result status")
)

// ResultStatus represents the state of a task result.
type ResultStatus string

const (
	// StatusPending indicates the task is still in progress.
	StatusPending ResultStatus = "pending"

	// StatusSuccess indicates the task completed successfully.
	StatusSuccess ResultStatus = "success"

	// StatusFailed indicates the task failed.
	StatusFailed ResultStatus = "failed"
)

// Valid returns true if the status is a known value.
func (s ResultStatus) Valid() bool {
	switch s {
	case StatusPending, StatusSuccess, StatusFailed:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if the status represents a final state.
func (s ResultStatus) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed
}

// Result represents a task's output.
type Result struct {
	// TaskID uniquely identifies the task.
	TaskID string

	// Status indicates the current state of the result.
	Status ResultStatus

	// Output contains the task's output data.
	// Empty for pending or failed tasks.
	Output []byte

	// Error contains the error message if Status is StatusFailed.
	Error string

	// Metadata contains additional key-value data about the result.
	Metadata map[string]string

	// CreatedAt is when the result was first created.
	CreatedAt time.Time

	// UpdatedAt is when the result was last updated.
	UpdatedAt time.Time
}

// Clone returns a deep copy of the result.
func (r *Result) Clone() *Result {
	if r == nil {
		return nil
	}

	clone := &Result{
		TaskID:    r.TaskID,
		Status:    r.Status,
		Error:     r.Error,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}

	if r.Output != nil {
		clone.Output = make([]byte, len(r.Output))
		copy(clone.Output, r.Output)
	}

	if r.Metadata != nil {
		clone.Metadata = make(map[string]string, len(r.Metadata))
		for k, v := range r.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// ResultFilter specifies criteria for listing results.
type ResultFilter struct {
	// Status filters by result status. Empty means all statuses.
	Status ResultStatus

	// TaskIDPrefix filters by task ID prefix.
	TaskIDPrefix string

	// CreatedAfter filters results created after this time.
	CreatedAfter time.Time

	// CreatedBefore filters results created before this time.
	CreatedBefore time.Time

	// Limit caps the number of results returned. 0 means no limit.
	Limit int

	// Metadata filters by metadata key-value pairs (all must match).
	Metadata map[string]string
}

// Matches returns true if the result matches the filter criteria.
func (f ResultFilter) Matches(r *Result) bool {
	if r == nil {
		return false
	}

	// Status filter
	if f.Status != "" && r.Status != f.Status {
		return false
	}

	// Task ID prefix filter
	if f.TaskIDPrefix != "" && !hasPrefix(r.TaskID, f.TaskIDPrefix) {
		return false
	}

	// Time filters
	if !f.CreatedAfter.IsZero() && !r.CreatedAt.After(f.CreatedAfter) {
		return false
	}
	if !f.CreatedBefore.IsZero() && !r.CreatedAt.Before(f.CreatedBefore) {
		return false
	}

	// Metadata filter (all must match)
	if f.Metadata != nil {
		for k, v := range f.Metadata {
			if r.Metadata == nil || r.Metadata[k] != v {
				return false
			}
		}
	}

	return true
}

// hasPrefix checks if s starts with prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// ResultPublisher provides result storage, retrieval, and subscription.
type ResultPublisher interface {
	// Publish stores or updates a task result.
	// If the result already exists, it is updated.
	Publish(ctx context.Context, taskID string, result Result) error

	// Get retrieves a result by task ID.
	// Returns ErrNotFound if the result doesn't exist.
	Get(ctx context.Context, taskID string) (*Result, error)

	// Subscribe returns a channel that receives updates for a task.
	// The channel is closed when the task reaches a terminal state
	// or when the subscription is cancelled.
	// If the result already exists, it is sent immediately.
	Subscribe(taskID string) (<-chan *Result, error)

	// List returns results matching the filter criteria.
	List(filter ResultFilter) ([]*Result, error)

	// Delete removes a result by task ID.
	// Returns ErrNotFound if the result doesn't exist.
	Delete(ctx context.Context, taskID string) error

	// Close shuts down the publisher and releases resources.
	Close() error
}

// Subscription represents an active result subscription.
type Subscription interface {
	// Results returns the channel for incoming result updates.
	Results() <-chan *Result

	// Cancel cancels the subscription.
	Cancel() error
}

// ValidateTaskID checks if a task ID is valid.
func ValidateTaskID(taskID string) error {
	if taskID == "" {
		return ErrInvalidTaskID
	}
	return nil
}

// ValidateResult checks if a result is valid for publishing.
func ValidateResult(r Result) error {
	if err := ValidateTaskID(r.TaskID); err != nil {
		return err
	}
	if !r.Status.Valid() {
		return ErrInvalidStatus
	}
	return nil
}
