package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinayprograms/agentkit/state"
)

const (
	// Key prefixes for state store.
	taskPrefix       = "tasks.task."
	idempotencyPrefix = "tasks.idem."
)

// Manager implements TaskManager using a state store backend.
type Manager struct {
	store  state.StateStore
	mu     sync.RWMutex
	closed atomic.Bool
	idGen  func() string
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithIDGenerator sets a custom ID generator function.
func WithIDGenerator(gen func() string) ManagerOption {
	return func(m *Manager) {
		m.idGen = gen
	}
}

// NewManager creates a new task manager backed by the given state store.
func NewManager(store state.StateStore, opts ...ManagerOption) *Manager {
	m := &Manager{
		store: store,
		idGen: generateID,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Submit creates a new task or returns the existing task ID if
// a task with the same IdempotencyKey already exists.
func (m *Manager) Submit(ctx context.Context, task Task) (string, error) {
	if m.closed.Load() {
		return "", ErrStoreClosed
	}

	// Validate payload is present
	if task.Payload == nil {
		return "", ErrInvalidTask
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check idempotency key if provided
	if task.IdempotencyKey != "" {
		existingID, err := m.getIdempotencyKey(task.IdempotencyKey)
		if err == nil && existingID != "" {
			return existingID, nil
		}
	}

	// Generate ID if not provided
	if task.ID == "" {
		task.ID = m.idGen()
	}

	// Initialize task state
	task.Status = StatusPending
	task.Attempts = 0
	task.CreatedAt = time.Now()
	task.ClaimedAt = nil
	task.ClaimedBy = ""
	task.CompletedAt = nil
	task.Result = nil
	task.Error = ""

	// Store the task
	if err := m.saveTask(&task); err != nil {
		return "", err
	}

	// Store idempotency mapping if key provided
	if task.IdempotencyKey != "" {
		if err := m.setIdempotencyKey(task.IdempotencyKey, task.ID); err != nil {
			// Best effort - task is already saved
			_ = m.deleteTask(task.ID)
			return "", err
		}
	}

	return task.ID, nil
}

// Claim marks a task as being worked on by a specific worker.
func (m *Manager) Claim(ctx context.Context, taskID string, workerID string) error {
	if m.closed.Load() {
		return ErrStoreClosed
	}

	if workerID == "" {
		return ErrInvalidWorkerID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.loadTask(taskID)
	if err != nil {
		return err
	}

	switch task.Status {
	case StatusClaimed:
		if task.ClaimedBy == workerID {
			// Re-claim by same worker is allowed (idempotent)
			return nil
		}
		return ErrTaskAlreadyClaimed
	case StatusCompleted:
		return ErrTaskCompleted
	case StatusFailed:
		return ErrTaskFailed
	}

	// Claim the task
	now := time.Now()
	task.Status = StatusClaimed
	task.ClaimedAt = &now
	task.ClaimedBy = workerID
	task.Attempts++

	return m.saveTask(task)
}

// Complete marks a task as successfully completed with the given result.
func (m *Manager) Complete(ctx context.Context, taskID string, result []byte) error {
	if m.closed.Load() {
		return ErrStoreClosed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.loadTask(taskID)
	if err != nil {
		return err
	}

	switch task.Status {
	case StatusPending:
		return ErrTaskNotClaimed
	case StatusCompleted:
		// Idempotent - already completed
		return nil
	case StatusFailed:
		return ErrTaskFailed
	}

	// Complete the task
	now := time.Now()
	task.Status = StatusCompleted
	task.CompletedAt = &now
	if result != nil {
		task.Result = make([]byte, len(result))
		copy(task.Result, result)
	}

	return m.saveTask(task)
}

// Fail marks a task as failed with the given error.
func (m *Manager) Fail(ctx context.Context, taskID string, taskErr error) error {
	if m.closed.Load() {
		return ErrStoreClosed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.loadTask(taskID)
	if err != nil {
		return err
	}

	switch task.Status {
	case StatusPending:
		return ErrTaskNotClaimed
	case StatusCompleted:
		return ErrTaskCompleted
	case StatusFailed:
		// Idempotent - already failed
		return nil
	}

	// Record the error
	if taskErr != nil {
		task.Error = taskErr.Error()
	}

	// Check if we can retry
	if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
		// Permanently failed
		now := time.Now()
		task.Status = StatusFailed
		task.CompletedAt = &now
		task.ClaimedBy = ""
		task.ClaimedAt = nil
	} else {
		// Move back to pending for retry
		task.Status = StatusPending
		task.ClaimedBy = ""
		task.ClaimedAt = nil
	}

	return m.saveTask(task)
}

// Retry moves a failed or claimed task back to pending for retry.
func (m *Manager) Retry(ctx context.Context, taskID string) error {
	if m.closed.Load() {
		return ErrStoreClosed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.loadTask(taskID)
	if err != nil {
		return err
	}

	switch task.Status {
	case StatusPending:
		// Already pending - idempotent
		return nil
	case StatusCompleted:
		return ErrTaskCompleted
	}

	// Check attempt limit
	if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
		return ErrMaxAttemptsReached
	}

	// Move back to pending
	task.Status = StatusPending
	task.ClaimedBy = ""
	task.ClaimedAt = nil
	task.Error = ""

	return m.saveTask(task)
}

// Get retrieves a task by ID.
func (m *Manager) Get(ctx context.Context, taskID string) (*Task, error) {
	if m.closed.Load() {
		return nil, ErrStoreClosed
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	task, err := m.loadTask(taskID)
	if err != nil {
		return nil, err
	}

	return task.Clone(), nil
}

// GetByIdempotencyKey retrieves a task by its idempotency key.
func (m *Manager) GetByIdempotencyKey(ctx context.Context, key string) (*Task, error) {
	if m.closed.Load() {
		return nil, ErrStoreClosed
	}

	if key == "" {
		return nil, ErrTaskNotFound
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	taskID, err := m.getIdempotencyKey(key)
	if err != nil || taskID == "" {
		return nil, ErrTaskNotFound
	}

	task, err := m.loadTask(taskID)
	if err != nil {
		return nil, err
	}

	return task.Clone(), nil
}

// List returns all tasks matching the given status filter.
func (m *Manager) List(ctx context.Context, status TaskStatus) ([]*Task, error) {
	if m.closed.Load() {
		return nil, ErrStoreClosed
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	keys, err := m.store.Keys(taskPrefix + "*")
	if err != nil {
		return nil, err
	}

	var tasks []*Task
	for _, key := range keys {
		taskID := strings.TrimPrefix(key, taskPrefix)
		task, err := m.loadTask(taskID)
		if err != nil {
			continue
		}

		if status == "" || task.Status == status {
			tasks = append(tasks, task.Clone())
		}
	}

	return tasks, nil
}

// Delete removes a task by ID.
func (m *Manager) Delete(ctx context.Context, taskID string) error {
	if m.closed.Load() {
		return ErrStoreClosed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.loadTask(taskID)
	if err != nil {
		return err
	}

	// Only allow deleting terminal tasks
	if !task.Status.IsTerminal() {
		return ErrTaskNotClaimed // Using this error to indicate task is still active
	}

	// Delete idempotency mapping if exists
	if task.IdempotencyKey != "" {
		_ = m.deleteIdempotencyKey(task.IdempotencyKey)
	}

	return m.deleteTask(taskID)
}

// Close releases resources held by the manager.
func (m *Manager) Close() error {
	if m.closed.Swap(true) {
		return nil
	}
	return nil
}

// Internal methods

func (m *Manager) loadTask(taskID string) (*Task, error) {
	data, err := m.store.Get(taskPrefix + taskID)
	if err != nil {
		if err == state.ErrNotFound {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}

	return &task, nil
}

func (m *Manager) saveTask(task *Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return m.store.Put(taskPrefix+task.ID, data, 0)
}

func (m *Manager) deleteTask(taskID string) error {
	return m.store.Delete(taskPrefix + taskID)
}

func (m *Manager) getIdempotencyKey(key string) (string, error) {
	data, err := m.store.Get(idempotencyPrefix + key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Manager) setIdempotencyKey(key, taskID string) error {
	return m.store.Put(idempotencyPrefix+key, []byte(taskID), 0)
}

func (m *Manager) deleteIdempotencyKey(key string) error {
	return m.store.Delete(idempotencyPrefix + key)
}

// generateID creates a unique task ID.
func generateID() string {
	// Use timestamp + random suffix for uniqueness
	return time.Now().Format("20060102150405.000000000")
}
