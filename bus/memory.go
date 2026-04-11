package bus

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// memoryBus implements Bus using in-memory channels.
// All subscriptions live in a single registry and delivery
// is driven by subject matching, mirroring NATS semantics.
type memoryBus struct {
	config Config

	mu       sync.RWMutex
	subs     []*memorySub
	closed   atomic.Bool
	replySeq uint64
	queueIdx sync.Map // queue name -> *uint64 (round-robin counter)
}

type memorySub struct {
	pattern string
	queue   string // empty = regular, non-empty = queue group member
	ch      chan *Message
	once    bool // auto-unsubscribe after one delivery (reply)
	closed  atomic.Bool
	bus     *memoryBus
}

// Memory creates an in-memory Bus. Safe for concurrent use.
func Memory(cfg Config) Bus {
	return &memoryBus{config: cfg}
}

// Publish sends a message to all matching subscribers.
// Subject matching drives all delivery — regular subs, queue groups,
// and reply channels are all handled through the same path.
func (b *memoryBus) Publish(subject string, data []byte) error {
	if err := validateSubject(subject); err != nil {
		return err
	}
	if b.closed.Load() {
		return ErrClosed
	}

	b.deliver(subject, &Message{Subject: subject, Data: data})
	return nil
}

// deliver walks all subscriptions, matches patterns, and sends.
// Queue group members are grouped — one per group receives the message.
// Once-subscriptions (reply) are removed after delivery.
func (b *memoryBus) deliver(subject string, msg *Message) {
	b.mu.RLock()
	subs := b.subs
	b.mu.RUnlock()

	// First pass: deliver to regular subs, collect queue groups.
	queues := map[string][]*memorySub{} // queue name -> matching subs
	var delivered []*memorySub          // once-subs to remove after delivery

	for _, sub := range subs {
		if sub.closed.Load() || !subjectMatch(sub.pattern, subject) {
			continue
		}
		if sub.queue != "" {
			queues[sub.queue] = append(queues[sub.queue], sub)
			continue
		}
		select {
		case sub.ch <- msg:
			if sub.once {
				delivered = append(delivered, sub)
			}
		default:
		}
	}

	// Second pass: pick one per queue group (round-robin).
	for name, group := range queues {
		n := len(group)
		start := int(b.nextQueueIdx(name)) % n
		for i := range n {
			sub := group[(start+i)%n]
			select {
			case sub.ch <- msg:
				goto picked
			default:
				continue
			}
		}
	picked:
	}

	// Clean up once-subs (reply channels).
	for _, sub := range delivered {
		sub.closed.Store(true)
		close(sub.ch)
		b.remove(sub)
	}
}

// Subscribe creates a subscription to a subject pattern.
func (b *memoryBus) Subscribe(subject string) (Subscription, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	if b.closed.Load() {
		return nil, ErrClosed
	}

	sub := &memorySub{
		pattern: subject,
		ch:      make(chan *Message, b.config.bufferSize()),
		bus:     b,
	}

	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	return sub, nil
}

// QueueSubscribe creates a queue subscription.
func (b *memoryBus) QueueSubscribe(subject, queue string) (Subscription, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	if queue == "" {
		return nil, ErrInvalidSubject
	}
	if b.closed.Load() {
		return nil, ErrClosed
	}

	sub := &memorySub{
		pattern: subject,
		queue:   queue,
		ch:      make(chan *Message, b.config.bufferSize()),
		bus:     b,
	}

	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	return sub, nil
}

// Request sends a request and waits for a reply.
// Creates a temporary once-subscription on a reply subject.
func (b *memoryBus) Request(subject string, data []byte, timeout time.Duration) (*Message, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	if b.closed.Load() {
		return nil, ErrClosed
	}

	replySubject := fmt.Sprintf("_INBOX.%d", atomic.AddUint64(&b.replySeq, 1))

	replySub := &memorySub{
		pattern: replySubject,
		ch:      make(chan *Message, 1),
		once:    true,
		bus:     b,
	}

	b.mu.Lock()
	b.subs = append(b.subs, replySub)
	b.mu.Unlock()

	msg := &Message{Subject: subject, Data: data, Reply: replySubject}

	b.mu.RLock()
	subs := b.subs
	b.mu.RUnlock()

	for _, sub := range subs {
		if sub == replySub || sub.closed.Load() || !subjectMatch(sub.pattern, subject) {
			continue
		}
		select {
		case sub.ch <- msg:
		default:
		}
	}

	select {
	case reply := <-replySub.ch:
		return reply, nil
	case <-time.After(timeout):
		replySub.closed.Store(true)
		b.remove(replySub)
		return nil, ErrTimeout
	}
}

// Close shuts down the bus.
func (b *memoryBus) Close() error {
	if b.closed.Swap(true) {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, sub := range b.subs {
		if !sub.closed.Swap(true) {
			close(sub.ch)
		}
	}
	b.subs = nil
	return nil
}

// Messages returns the message channel.
func (s *memorySub) Messages() <-chan *Message { return s.ch }

// Unsubscribe cancels the subscription.
func (s *memorySub) Unsubscribe() error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.ch)
	s.bus.remove(s)
	return nil
}

// nextQueueIdx returns the next round-robin index for a queue group.
func (b *memoryBus) nextQueueIdx(queue string) uint64 {
	var zero uint64
	actual, _ := b.queueIdx.LoadOrStore(queue, &zero)
	return atomic.AddUint64(actual.(*uint64), 1)
}

// remove removes a subscription from the registry.
func (b *memoryBus) remove(target *memorySub) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, sub := range b.subs {
		if sub == target {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}
