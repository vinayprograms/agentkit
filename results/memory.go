package results

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryPublisher implements ResultPublisher using in-memory storage.
// Useful for testing and single-process scenarios.
type MemoryPublisher struct {
	mu      sync.RWMutex
	results map[string]*Result
	subs    map[string][]*memorySub
	closed  atomic.Bool
}

type memorySub struct {
	taskID string
	ch     chan *Result
	closed atomic.Bool
	pub    *MemoryPublisher
}

// NewMemoryPublisher creates a new in-memory result publisher.
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{
		results: make(map[string]*Result),
		subs:    make(map[string][]*memorySub),
	}
}

// Publish stores or updates a task result.
func (p *MemoryPublisher) Publish(ctx context.Context, taskID string, result Result) error {
	if p.closed.Load() {
		return ErrClosed
	}

	if err := ValidateTaskID(taskID); err != nil {
		return err
	}

	// Ensure taskID in result matches
	result.TaskID = taskID

	if !result.Status.Valid() {
		return ErrInvalidStatus
	}

	now := time.Now()

	p.mu.Lock()
	existing, exists := p.results[taskID]
	if exists {
		result.CreatedAt = existing.CreatedAt
	} else {
		result.CreatedAt = now
	}
	result.UpdatedAt = now

	// Store a clone to prevent external mutation
	stored := result.Clone()
	p.results[taskID] = stored

	// Notify subscribers
	subs := p.subs[taskID]
	p.mu.Unlock()

	// Send to subscribers outside of lock
	for _, sub := range subs {
		if !sub.closed.Load() {
			select {
			case sub.ch <- stored.Clone():
			default:
				// Buffer full, drop update
			}
		}
	}

	// If terminal, close subscriptions
	if result.Status.IsTerminal() {
		p.closeSubscriptions(taskID)
	}

	return nil
}

// Get retrieves a result by task ID.
func (p *MemoryPublisher) Get(ctx context.Context, taskID string) (*Result, error) {
	if p.closed.Load() {
		return nil, ErrClosed
	}

	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}

	p.mu.RLock()
	result, exists := p.results[taskID]
	p.mu.RUnlock()

	if !exists {
		return nil, ErrNotFound
	}

	return result.Clone(), nil
}

// Subscribe returns a channel that receives updates for a task.
func (p *MemoryPublisher) Subscribe(taskID string) (<-chan *Result, error) {
	if p.closed.Load() {
		return nil, ErrClosed
	}

	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}

	sub := &memorySub{
		taskID: taskID,
		ch:     make(chan *Result, 16),
		pub:    p,
	}

	p.mu.Lock()
	p.subs[taskID] = append(p.subs[taskID], sub)

	// If result already exists, send it immediately
	if existing, exists := p.results[taskID]; exists {
		select {
		case sub.ch <- existing.Clone():
		default:
		}

		// If already terminal, close subscription
		if existing.Status.IsTerminal() {
			p.mu.Unlock()
			close(sub.ch)
			return sub.ch, nil
		}
	}
	p.mu.Unlock()

	return sub.ch, nil
}

// List returns results matching the filter criteria.
func (p *MemoryPublisher) List(filter ResultFilter) ([]*Result, error) {
	if p.closed.Load() {
		return nil, ErrClosed
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []*Result
	for _, r := range p.results {
		if filter.Matches(r) {
			results = append(results, r.Clone())
			if filter.Limit > 0 && len(results) >= filter.Limit {
				break
			}
		}
	}

	return results, nil
}

// Delete removes a result by task ID.
func (p *MemoryPublisher) Delete(ctx context.Context, taskID string) error {
	if p.closed.Load() {
		return ErrClosed
	}

	if err := ValidateTaskID(taskID); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.results[taskID]; !exists {
		return ErrNotFound
	}

	delete(p.results, taskID)
	p.closeSubscriptionsLocked(taskID)

	return nil
}

// Close shuts down the publisher.
func (p *MemoryPublisher) Close() error {
	if p.closed.Swap(true) {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all subscriptions
	for taskID := range p.subs {
		p.closeSubscriptionsLocked(taskID)
	}

	p.results = nil
	p.subs = nil

	return nil
}

// closeSubscriptions closes all subscriptions for a task.
func (p *MemoryPublisher) closeSubscriptions(taskID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeSubscriptionsLocked(taskID)
}

// closeSubscriptionsLocked closes subscriptions while holding the lock.
func (p *MemoryPublisher) closeSubscriptionsLocked(taskID string) {
	subs := p.subs[taskID]
	for _, sub := range subs {
		if !sub.closed.Swap(true) {
			close(sub.ch)
		}
	}
	delete(p.subs, taskID)
}

// Results returns the subscription channel.
func (s *memorySub) Results() <-chan *Result {
	return s.ch
}

// Cancel cancels the subscription.
func (s *memorySub) Cancel() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.pub.mu.Lock()
	defer s.pub.mu.Unlock()

	// Remove from publisher's subscription list
	subs := s.pub.subs[s.taskID]
	for i, sub := range subs {
		if sub == s {
			s.pub.subs[s.taskID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}

	close(s.ch)
	return nil
}
