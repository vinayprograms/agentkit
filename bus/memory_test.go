package bus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Subject validation and matching
// ============================================================================

func TestValidateSubject(t *testing.T) {
	tests := []struct {
		subject string
		wantErr bool
	}{
		{"foo", false},
		{"foo.bar", false},
		{"foo.bar.baz", false},
		{"", true},
	}

	for _, tt := range tests {
		err := validateSubject(tt.subject)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateSubject(%q) = %v, wantErr %v", tt.subject, err, tt.wantErr)
		}
	}
}

func TestSubjectMatch(t *testing.T) {
	tests := []struct {
		pattern string
		subject string
		want    bool
	}{
		// Exact match
		{"foo", "foo", true},
		{"foo.bar", "foo.bar", true},
		{"foo", "bar", false},
		{"foo.bar", "foo.baz", false},

		// Wildcard (*)
		{"foo.*", "foo.bar", true},
		{"foo.*", "foo.baz", true},
		{"foo.*", "foo", false},
		{"foo.*", "foo.bar.baz", false},
		{"*.bar", "foo.bar", true},
		{"*.bar", "baz.bar", true},
		{"foo.*.baz", "foo.bar.baz", true},
		{"foo.*.baz", "foo.bar.qux", false},

		// WildcardAll (>)
		{"foo.>", "foo.bar", true},
		{"foo.>", "foo.bar.baz", true},
		{"foo.>", "foo", false}, // > must match at least one
		{">", "foo", true},
		{">", "foo.bar", true},

		// Length mismatches
		{"foo.bar", "foo", false},
		{"foo", "foo.bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.subject, func(t *testing.T) {
			if got := subjectMatch(tt.pattern, tt.subject); got != tt.want {
				t.Errorf("subjectMatch(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Publish
// ============================================================================

func TestPublish(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	if err := b.Publish("test", []byte("hello")); err != nil {
		t.Errorf("Publish error: %v", err)
	}
}

func TestPublishInvalidSubject(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	if err := b.Publish("", []byte("hello")); err != ErrInvalidSubject {
		t.Errorf("expected ErrInvalidSubject, got %v", err)
	}
}

func TestPublishAfterClose(t *testing.T) {
	b := Memory(Config{})
	b.Close()

	if err := b.Publish("test", []byte("hello")); err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

// ============================================================================
// Subscribe
// ============================================================================

func TestSubscribe(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, err := b.Subscribe("test")
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	defer sub.Unsubscribe()

	b.Publish("test", []byte("hello"))

	select {
	case msg := <-sub.Messages():
		if string(msg.Data) != "hello" {
			t.Errorf("data = %q, want %q", msg.Data, "hello")
		}
		if msg.Subject != "test" {
			t.Errorf("subject = %q, want %q", msg.Subject, "test")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestSubscribeMultiple(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub1, _ := b.Subscribe("test")
	sub2, _ := b.Subscribe("test")
	defer sub1.Unsubscribe()
	defer sub2.Unsubscribe()

	b.Publish("test", []byte("hello"))

	for i, sub := range []Subscription{sub1, sub2} {
		select {
		case msg := <-sub.Messages():
			if string(msg.Data) != "hello" {
				t.Errorf("sub%d: data = %q, want %q", i+1, msg.Data, "hello")
			}
		case <-time.After(time.Second):
			t.Errorf("sub%d: timeout", i+1)
		}
	}
}

func TestSubscribeAfterClose(t *testing.T) {
	b := Memory(Config{})
	b.Close()

	_, err := b.Subscribe("test")
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestSubscribeWildcard(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.Subscribe("events.*")
	defer sub.Unsubscribe()

	b.Publish("events.user", []byte("user event"))
	b.Publish("events.order", []byte("order event"))
	b.Publish("other.topic", []byte("should not match"))

	for _, want := range []string{"user event", "order event"} {
		select {
		case msg := <-sub.Messages():
			if string(msg.Data) != want {
				t.Errorf("data = %q, want %q", msg.Data, want)
			}
		case <-time.After(time.Second):
			t.Errorf("timeout waiting for %q", want)
		}
	}

	select {
	case msg := <-sub.Messages():
		t.Errorf("unexpected message: %q", msg.Data)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscribeWildcardAll(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.Subscribe("events.>")
	defer sub.Unsubscribe()

	b.Publish("events.user.created", []byte("deep"))
	b.Publish("events.order", []byte("shallow"))
	b.Publish("other", []byte("no match"))

	for _, want := range []string{"deep", "shallow"} {
		select {
		case msg := <-sub.Messages():
			if string(msg.Data) != want {
				t.Errorf("data = %q, want %q", msg.Data, want)
			}
		case <-time.After(time.Second):
			t.Errorf("timeout waiting for %q", want)
		}
	}
}

// ============================================================================
// Queue subscribe
// ============================================================================

func TestQueueSubscribe(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	var subs []Subscription
	for i := 0; i < 3; i++ {
		sub, _ := b.QueueSubscribe("test", "workers")
		subs = append(subs, sub)
	}
	defer func() {
		for _, sub := range subs {
			sub.Unsubscribe()
		}
	}()

	for i := 0; i < 10; i++ {
		b.Publish("test", []byte("msg"))
	}

	var received [3]int32
	var wg sync.WaitGroup
	for i, sub := range subs {
		wg.Add(1)
		go func(idx int, s Subscription) {
			defer wg.Done()
			timeout := time.After(100 * time.Millisecond)
			for {
				select {
				case <-s.Messages():
					atomic.AddInt32(&received[idx], 1)
				case <-timeout:
					return
				}
			}
		}(i, sub)
	}
	wg.Wait()

	total := received[0] + received[1] + received[2]
	if total != 10 {
		t.Errorf("total received = %d, want 10 (distribution: %v)", total, received)
	}
}

func TestQueueSubscribeEmptyQueue(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	_, err := b.QueueSubscribe("test", "")
	if err != ErrInvalidSubject {
		t.Errorf("expected ErrInvalidSubject, got %v", err)
	}
}

func TestQueueSubscribeAfterClose(t *testing.T) {
	b := Memory(Config{})
	b.Close()

	_, err := b.QueueSubscribe("test", "workers")
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

// ============================================================================
// Request/Reply
// ============================================================================

func TestRequest(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.Subscribe("service")
	go func() {
		for msg := range sub.Messages() {
			if msg.Reply != "" {
				b.Publish(msg.Reply, []byte("pong"))
			}
		}
	}()
	defer sub.Unsubscribe()

	reply, err := b.Request("service", []byte("ping"), time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if string(reply.Data) != "pong" {
		t.Errorf("reply = %q, want %q", reply.Data, "pong")
	}
}

func TestRequestTimeout(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	_, err := b.Request("service", []byte("ping"), 50*time.Millisecond)
	if err != ErrTimeout {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestRequestInvalidSubject(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	_, err := b.Request("", []byte("ping"), time.Second)
	if err != ErrInvalidSubject {
		t.Errorf("expected ErrInvalidSubject, got %v", err)
	}
}

func TestRequestAfterClose(t *testing.T) {
	b := Memory(Config{})
	b.Close()

	_, err := b.Request("service", []byte("ping"), time.Second)
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

// ============================================================================
// Unsubscribe and Close
// ============================================================================

func TestUnsubscribe(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.Subscribe("test")
	if err := sub.Unsubscribe(); err != nil {
		t.Errorf("Unsubscribe error: %v", err)
	}

	_, ok := <-sub.Messages()
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestUnsubscribeIdempotent(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.Subscribe("test")
	sub.Unsubscribe()
	if err := sub.Unsubscribe(); err != nil {
		t.Errorf("second Unsubscribe should be no-op, got %v", err)
	}
}

func TestCloseClosesSubscriptions(t *testing.T) {
	b := Memory(Config{})
	sub, _ := b.Subscribe("test")

	b.Close()

	_, ok := <-sub.Messages()
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestCloseIdempotent(t *testing.T) {
	b := Memory(Config{})
	b.Close()
	if err := b.Close(); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

// ============================================================================
// Buffer behavior
// ============================================================================

func TestBufferFull(t *testing.T) {
	b := Memory(Config{BufferSize: 1})
	defer b.Close()

	sub, _ := b.Subscribe("test")

	b.Publish("test", []byte("1"))
	b.Publish("test", []byte("2")) // dropped

	select {
	case msg := <-sub.Messages():
		if string(msg.Data) != "1" {
			t.Errorf("expected first message, got %q", msg.Data)
		}
	default:
		t.Error("expected at least one message")
	}

	select {
	case <-sub.Messages():
		t.Error("unexpected second message")
	default:
	}
}

func TestDefaultBufferSize(t *testing.T) {
	b := Memory(Config{}) // zero value
	defer b.Close()

	sub, _ := b.Subscribe("test")
	defer sub.Unsubscribe()

	// Should use default 256 buffer
	b.Publish("test", []byte("hello"))

	select {
	case msg := <-sub.Messages():
		if string(msg.Data) != "hello" {
			t.Errorf("data = %q, want %q", msg.Data, "hello")
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

// ============================================================================
// Queue unsubscribe
// ============================================================================

func TestQueueUnsubscribe(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.QueueSubscribe("test", "workers")
	if err := sub.Unsubscribe(); err != nil {
		t.Errorf("Unsubscribe error: %v", err)
	}

	_, ok := <-sub.Messages()
	if ok {
		t.Error("expected channel to be closed")
	}
}

// ============================================================================
// Edge cases
// ============================================================================

func TestSubscribeInvalidSubject(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	_, err := b.Subscribe("")
	if err != ErrInvalidSubject {
		t.Errorf("expected ErrInvalidSubject, got %v", err)
	}
}

func TestQueueSubscribeInvalidSubject(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	_, err := b.QueueSubscribe("", "workers")
	if err != ErrInvalidSubject {
		t.Errorf("expected ErrInvalidSubject, got %v", err)
	}
}

func TestDispatchNonMatchingSubject(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	sub, _ := b.QueueSubscribe("foo", "workers")
	defer sub.Unsubscribe()

	// Publish to different subject — should not match
	b.Publish("bar", []byte("no match"))

	select {
	case <-sub.Messages():
		t.Error("should not receive message for non-matching subject")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCloseWithQueueSubscriptions(t *testing.T) {
	b := Memory(Config{})

	sub1, _ := b.QueueSubscribe("test", "workers")
	sub2, _ := b.QueueSubscribe("test", "workers")

	b.Close()

	_, ok1 := <-sub1.Messages()
	_, ok2 := <-sub2.Messages()
	if ok1 || ok2 {
		t.Error("expected all queue channels to be closed")
	}
}

func TestRemoveQueueSubMissing(t *testing.T) {
	b := Memory(Config{})
	defer b.Close()

	// Subscribe and unsubscribe from a queue — exercises removeQueueSub
	sub, _ := b.QueueSubscribe("test", "group1")
	sub.Unsubscribe()

	// Second unsubscribe — idempotent
	sub.Unsubscribe()
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkPublish(b *testing.B) {
	mb := Memory(Config{})
	defer mb.Close()

	sub, _ := mb.Subscribe("bench")
	go func() {
		for range sub.Messages() {
		}
	}()

	data := []byte("benchmark message")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.Publish("bench", data)
	}
}

func BenchmarkRequest(b *testing.B) {
	mb := Memory(Config{})
	defer mb.Close()

	sub, _ := mb.Subscribe("service")
	go func() {
		for msg := range sub.Messages() {
			if msg.Reply != "" {
				mb.Publish(msg.Reply, []byte("pong"))
			}
		}
	}()

	data := []byte("ping")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.Request("service", data, time.Second)
	}
}
