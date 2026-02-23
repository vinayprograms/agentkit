package bus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Unit Tests ---

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
		err := ValidateSubject(tt.subject)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSubject(%q) = %v, wantErr %v", tt.subject, err, tt.wantErr)
		}
	}
}

func TestMemoryBus_Publish(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	// Publish without subscribers should not error
	err := bus.Publish("test", []byte("hello"))
	if err != nil {
		t.Errorf("Publish error: %v", err)
	}
}

func TestMemoryBus_PublishInvalidSubject(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	err := bus.Publish("", []byte("hello"))
	if err != ErrInvalidSubject {
		t.Errorf("expected ErrInvalidSubject, got %v", err)
	}
}

// --- Integration Tests ---

func TestMemoryBus_Subscribe(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	sub, err := bus.Subscribe("test")
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	defer sub.Unsubscribe()

	// Publish
	bus.Publish("test", []byte("hello"))

	// Receive
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

func TestMemoryBus_MultipleSubscribers(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	sub1, _ := bus.Subscribe("test")
	sub2, _ := bus.Subscribe("test")
	defer sub1.Unsubscribe()
	defer sub2.Unsubscribe()

	bus.Publish("test", []byte("hello"))

	// Both should receive
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

func TestMemoryBus_QueueSubscribe(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	// Create 3 queue subscribers
	var subs []Subscription
	for i := 0; i < 3; i++ {
		sub, _ := bus.QueueSubscribe("test", "workers")
		subs = append(subs, sub)
	}
	defer func() {
		for _, sub := range subs {
			sub.Unsubscribe()
		}
	}()

	// Publish 10 messages
	for i := 0; i < 10; i++ {
		bus.Publish("test", []byte("msg"))
	}

	// Count received per subscriber
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

	// All 10 messages should be received (distributed)
	total := received[0] + received[1] + received[2]
	if total != 10 {
		t.Errorf("total received = %d, want 10 (distribution: %v)", total, received)
	}
}

func TestMemoryBus_Request(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	// Start responder
	sub, _ := bus.Subscribe("service")
	go func() {
		for msg := range sub.Messages() {
			if msg.Reply != "" {
				bus.Publish(msg.Reply, []byte("pong"))
			}
		}
	}()
	defer sub.Unsubscribe()

	// Send request
	reply, err := bus.Request("service", []byte("ping"), time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if string(reply.Data) != "pong" {
		t.Errorf("reply = %q, want %q", reply.Data, "pong")
	}
}

func TestMemoryBus_RequestTimeout(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	// No responder
	_, err := bus.Request("service", []byte("ping"), 50*time.Millisecond)
	if err != ErrTimeout {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

// --- Failure Tests ---

func TestMemoryBus_PublishAfterClose(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	bus.Close()

	err := bus.Publish("test", []byte("hello"))
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestMemoryBus_SubscribeAfterClose(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	bus.Close()

	_, err := bus.Subscribe("test")
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestMemoryBus_Unsubscribe(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	sub, _ := bus.Subscribe("test")

	// Unsubscribe before any publish
	err := sub.Unsubscribe()
	if err != nil {
		t.Errorf("Unsubscribe error: %v", err)
	}

	// Channel should be closed after unsubscribe
	_, ok := <-sub.Messages()
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}
}

func TestMemoryBus_CloseClosesSubscriptions(t *testing.T) {
	bus := NewMemoryBus(DefaultConfig())
	sub, _ := bus.Subscribe("test")

	bus.Close()

	// Channel should be closed
	_, ok := <-sub.Messages()
	if ok {
		t.Error("expected channel to be closed")
	}
}

// --- Performance Tests ---

func BenchmarkMemoryBus_Publish(b *testing.B) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	sub, _ := bus.Subscribe("bench")
	go func() {
		for range sub.Messages() {
		}
	}()

	data := []byte("benchmark message")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bus.Publish("bench", data)
	}
}

func BenchmarkMemoryBus_Request(b *testing.B) {
	bus := NewMemoryBus(DefaultConfig())
	defer bus.Close()

	sub, _ := bus.Subscribe("service")
	go func() {
		for msg := range sub.Messages() {
			if msg.Reply != "" {
				bus.Publish(msg.Reply, []byte("pong"))
			}
		}
	}()

	data := []byte("ping")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bus.Request("service", data, time.Second)
	}
}

// --- Security Tests ---

func TestMemoryBus_BufferFull(t *testing.T) {
	bus := NewMemoryBus(Config{BufferSize: 1})
	defer bus.Close()

	sub, _ := bus.Subscribe("test")

	// Fill buffer
	bus.Publish("test", []byte("1"))
	bus.Publish("test", []byte("2")) // Should be dropped

	select {
	case msg := <-sub.Messages():
		if string(msg.Data) != "1" {
			t.Errorf("expected first message, got %q", msg.Data)
		}
	default:
		t.Error("expected at least one message")
	}

	// Should not block
	select {
	case <-sub.Messages():
		t.Error("unexpected second message")
	default:
		// Expected - second was dropped
	}
}
