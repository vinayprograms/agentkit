package results

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// BusPublisherConfig configures the bus-backed result publisher.
type BusPublisherConfig struct {
	// SubjectPrefix is the prefix for result subjects.
	// Default: "results"
	SubjectPrefix string

	// BufferSize for subscription channels.
	// Default: 16
	BufferSize int
}

// DefaultBusPublisherConfig returns configuration with sensible defaults.
func DefaultBusPublisherConfig() BusPublisherConfig {
	return BusPublisherConfig{
		SubjectPrefix: "results",
		BufferSize:    16,
	}
}

// BusPublisher implements ResultPublisher using a message bus for notifications.
// Results are stored in memory, but updates are broadcast over the bus,
// enabling distributed subscribers to receive notifications.
type BusPublisher struct {
	bus    bus.MessageBus
	config BusPublisherConfig

	mu      sync.RWMutex
	results map[string]*Result
	subs    map[string][]*busResultSub
	closed  atomic.Bool
}

type busResultSub struct {
	taskID  string
	ch      chan *Result
	busSub  bus.Subscription
	closed  atomic.Bool
	pub     *BusPublisher
	stopCh  chan struct{}
}

// NewBusPublisher creates a new bus-backed result publisher.
func NewBusPublisher(mb bus.MessageBus, cfg BusPublisherConfig) *BusPublisher {
	if cfg.SubjectPrefix == "" {
		cfg.SubjectPrefix = DefaultBusPublisherConfig().SubjectPrefix
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = DefaultBusPublisherConfig().BufferSize
	}

	return &BusPublisher{
		bus:     mb,
		config:  cfg,
		results: make(map[string]*Result),
		subs:    make(map[string][]*busResultSub),
	}
}

// resultSubject returns the bus subject for a task.
func (p *BusPublisher) resultSubject(taskID string) string {
	return p.config.SubjectPrefix + "." + taskID
}

// Publish stores or updates a task result and broadcasts to subscribers.
func (p *BusPublisher) Publish(ctx context.Context, taskID string, result Result) error {
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
	p.mu.Unlock()

	// Broadcast over bus
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}

	if err := p.bus.Publish(p.resultSubject(taskID), data); err != nil {
		// Log but don't fail - local storage succeeded
		// In production, you might want proper error handling here
	}

	// Note: Don't close local subscriptions here on terminal results.
	// The relay goroutines will close themselves when they receive
	// the terminal result via the bus, ensuring the message is delivered.

	return nil
}

// Get retrieves a result by task ID.
func (p *BusPublisher) Get(ctx context.Context, taskID string) (*Result, error) {
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
// Updates come from the message bus, enabling distributed notification.
func (p *BusPublisher) Subscribe(taskID string) (<-chan *Result, error) {
	if p.closed.Load() {
		return nil, ErrClosed
	}

	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}

	// Subscribe to bus subject
	subject := p.resultSubject(taskID)
	busSub, err := p.bus.Subscribe(subject)
	if err != nil {
		return nil, err
	}

	sub := &busResultSub{
		taskID: taskID,
		ch:     make(chan *Result, p.config.BufferSize),
		busSub: busSub,
		pub:    p,
		stopCh: make(chan struct{}),
	}

	p.mu.Lock()
	p.subs[taskID] = append(p.subs[taskID], sub)

	// If result already exists, send it immediately
	var existingResult *Result
	if existing, exists := p.results[taskID]; exists {
		existingResult = existing.Clone()
	}
	p.mu.Unlock()

	if existingResult != nil {
		select {
		case sub.ch <- existingResult:
		default:
		}

		// If already terminal, close subscription immediately
		if existingResult.Status.IsTerminal() {
			busSub.Unsubscribe()
			close(sub.ch)
			return sub.ch, nil
		}
	}

	// Start goroutine to relay bus messages to subscription channel
	go sub.relay()

	return sub.ch, nil
}

// relay forwards bus messages to the subscription channel.
func (s *busResultSub) relay() {
	defer func() {
		if !s.closed.Load() {
			s.closed.Store(true)
			close(s.ch)
		}
	}()

	for {
		select {
		case <-s.stopCh:
			return
		case msg, ok := <-s.busSub.Messages():
			if !ok {
				return
			}

			var result Result
			if err := json.Unmarshal(msg.Data, &result); err != nil {
				continue // Skip malformed messages
			}

			// Update local cache
			s.pub.mu.Lock()
			s.pub.results[result.TaskID] = result.Clone()
			s.pub.mu.Unlock()

			// Forward to subscriber
			if !s.closed.Load() {
				select {
				case s.ch <- result.Clone():
				default:
					// Buffer full, drop update
				}
			}

			// If terminal, stop relaying
			if result.Status.IsTerminal() {
				return
			}
		}
	}
}

// List returns results matching the filter criteria.
func (p *BusPublisher) List(filter ResultFilter) ([]*Result, error) {
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
func (p *BusPublisher) Delete(ctx context.Context, taskID string) error {
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
	p.closeLocalSubscriptionsLocked(taskID)

	return nil
}

// Close shuts down the publisher.
func (p *BusPublisher) Close() error {
	if p.closed.Swap(true) {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all subscriptions
	for taskID := range p.subs {
		p.closeLocalSubscriptionsLocked(taskID)
	}

	p.results = nil
	p.subs = nil

	return nil
}

// closeLocalSubscriptions closes all local subscriptions for a task.
func (p *BusPublisher) closeLocalSubscriptions(taskID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocalSubscriptionsLocked(taskID)
}

// closeLocalSubscriptionsLocked closes subscriptions while holding the lock.
func (p *BusPublisher) closeLocalSubscriptionsLocked(taskID string) {
	subs := p.subs[taskID]
	for _, sub := range subs {
		if !sub.closed.Swap(true) {
			close(sub.stopCh)
			sub.busSub.Unsubscribe()
			close(sub.ch)
		}
	}
	delete(p.subs, taskID)
}

// Results returns the subscription channel.
func (s *busResultSub) Results() <-chan *Result {
	return s.ch
}

// Cancel cancels the subscription.
func (s *busResultSub) Cancel() error {
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

	close(s.stopCh)
	s.busSub.Unsubscribe()
	close(s.ch)
	return nil
}
