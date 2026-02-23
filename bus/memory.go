package bus

import (
	"sync"
	"sync/atomic"
	"time"
)

// MemoryBus implements MessageBus using in-memory channels.
// Useful for testing and single-process scenarios.
type MemoryBus struct {
	config Config

	mu          sync.RWMutex
	subs        map[string][]*memorySub
	queueGroups map[string]map[string][]*memorySub // subject -> queue -> subs
	closed      atomic.Bool

	// For request/reply
	replyMu   sync.Mutex
	replySubs map[string]chan *Message
	replySeq  uint64
}

type memorySub struct {
	subject string
	queue   string
	ch      chan *Message
	closed  atomic.Bool
	bus     *MemoryBus
}

// NewMemoryBus creates a new in-memory message bus.
func NewMemoryBus(cfg Config) *MemoryBus {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = DefaultConfig().BufferSize
	}

	return &MemoryBus{
		config:      cfg,
		subs:        make(map[string][]*memorySub),
		queueGroups: make(map[string]map[string][]*memorySub),
		replySubs:   make(map[string]chan *Message),
	}
}

// Publish sends a message to all subscribers.
func (b *MemoryBus) Publish(subject string, data []byte) error {
	if err := ValidateSubject(subject); err != nil {
		return err
	}
	if b.closed.Load() {
		return ErrClosed
	}

	msg := &Message{
		Subject: subject,
		Data:    data,
	}

	b.deliverToSubscribers(subject, msg)
	b.deliverToQueueGroups(subject, msg)
	b.deliverToReply(subject, msg)

	return nil
}

// deliverToSubscribers sends to all regular subscribers.
func (b *MemoryBus) deliverToSubscribers(subject string, msg *Message) {
	b.mu.RLock()
	subs := b.subs[subject]
	b.mu.RUnlock()

	for _, sub := range subs {
		if !sub.closed.Load() {
			select {
			case sub.ch <- msg:
			default:
				// Buffer full, drop message
			}
		}
	}
}

// deliverToQueueGroups sends to one subscriber per queue group.
func (b *MemoryBus) deliverToQueueGroups(subject string, msg *Message) {
	b.mu.RLock()
	queues := b.queueGroups[subject]
	b.mu.RUnlock()

	for _, qsubs := range queues {
		b.deliverToOneInQueue(qsubs, msg)
	}
}

// deliverToOneInQueue picks one subscriber from the queue (round-robin).
func (b *MemoryBus) deliverToOneInQueue(subs []*memorySub, msg *Message) {
	// Simple round-robin: try each until one accepts
	for _, sub := range subs {
		if !sub.closed.Load() {
			select {
			case sub.ch <- msg:
				return
			default:
				continue
			}
		}
	}
}

// deliverToReply handles reply subjects for request/reply.
func (b *MemoryBus) deliverToReply(subject string, msg *Message) {
	b.replyMu.Lock()
	ch, ok := b.replySubs[subject]
	if ok {
		delete(b.replySubs, subject)
	}
	b.replyMu.Unlock()

	if ok {
		select {
		case ch <- msg:
		default:
		}
		close(ch)
	}
}

// Subscribe creates a subscription to a subject.
func (b *MemoryBus) Subscribe(subject string) (Subscription, error) {
	if err := ValidateSubject(subject); err != nil {
		return nil, err
	}
	if b.closed.Load() {
		return nil, ErrClosed
	}

	sub := &memorySub{
		subject: subject,
		ch:      make(chan *Message, b.config.BufferSize),
		bus:     b,
	}

	b.mu.Lock()
	b.subs[subject] = append(b.subs[subject], sub)
	b.mu.Unlock()

	return sub, nil
}

// QueueSubscribe creates a queue subscription.
func (b *MemoryBus) QueueSubscribe(subject, queue string) (Subscription, error) {
	if err := ValidateSubject(subject); err != nil {
		return nil, err
	}
	if queue == "" {
		return nil, ErrInvalidSubject
	}
	if b.closed.Load() {
		return nil, ErrClosed
	}

	sub := &memorySub{
		subject: subject,
		queue:   queue,
		ch:      make(chan *Message, b.config.BufferSize),
		bus:     b,
	}

	b.mu.Lock()
	if b.queueGroups[subject] == nil {
		b.queueGroups[subject] = make(map[string][]*memorySub)
	}
	b.queueGroups[subject][queue] = append(b.queueGroups[subject][queue], sub)
	b.mu.Unlock()

	return sub, nil
}

// Request sends a request and waits for reply.
func (b *MemoryBus) Request(subject string, data []byte, timeout time.Duration) (*Message, error) {
	if err := ValidateSubject(subject); err != nil {
		return nil, err
	}
	if b.closed.Load() {
		return nil, ErrClosed
	}

	// Create unique reply subject
	replySubject := b.createReplySubject()
	replyCh := make(chan *Message, 1)

	b.replyMu.Lock()
	b.replySubs[replySubject] = replyCh
	b.replyMu.Unlock()

	// Publish request with reply subject
	msg := &Message{
		Subject: subject,
		Data:    data,
		Reply:   replySubject,
	}

	b.deliverToSubscribers(subject, msg)
	b.deliverToQueueGroups(subject, msg)

	// Wait for reply
	select {
	case reply := <-replyCh:
		return reply, nil
	case <-time.After(timeout):
		b.replyMu.Lock()
		delete(b.replySubs, replySubject)
		b.replyMu.Unlock()
		return nil, ErrTimeout
	}
}

// createReplySubject generates a unique reply subject.
func (b *MemoryBus) createReplySubject() string {
	seq := atomic.AddUint64(&b.replySeq, 1)
	return "_INBOX." + string(rune(seq))
}

// Close shuts down the bus.
func (b *MemoryBus) Close() error {
	if b.closed.Swap(true) {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Close all subscriptions
	for _, subs := range b.subs {
		for _, sub := range subs {
			sub.closed.Store(true)
			close(sub.ch)
		}
	}

	for _, queues := range b.queueGroups {
		for _, subs := range queues {
			for _, sub := range subs {
				sub.closed.Store(true)
				close(sub.ch)
			}
		}
	}

	b.subs = nil
	b.queueGroups = nil

	return nil
}

// Messages returns the message channel.
func (s *memorySub) Messages() <-chan *Message {
	return s.ch
}

// Unsubscribe cancels the subscription.
func (s *memorySub) Unsubscribe() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()

	if s.queue == "" {
		s.bus.removeSub(s.subject, s)
	} else {
		s.bus.removeQueueSub(s.subject, s.queue, s)
	}

	close(s.ch)
	return nil
}

// removeSub removes a regular subscription.
func (b *MemoryBus) removeSub(subject string, target *memorySub) {
	subs := b.subs[subject]
	for i, sub := range subs {
		if sub == target {
			b.subs[subject] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// removeQueueSub removes a queue subscription.
func (b *MemoryBus) removeQueueSub(subject, queue string, target *memorySub) {
	if b.queueGroups[subject] == nil {
		return
	}
	subs := b.queueGroups[subject][queue]
	for i, sub := range subs {
		if sub == target {
			b.queueGroups[subject][queue] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}
