package bus

import (
	"os"
	"testing"
	"time"
)

// getNATSURL returns the NATS URL for testing, or skips the test.
func getNATSURL(t *testing.T) string {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}

	// Skip if short mode or NATS not available
	if testing.Short() {
		t.Skip("skipping NATS test in short mode")
	}

	// Try to connect
	cfg := DefaultNATSConfig()
	cfg.URL = url
	cfg.ConnectTimeout = 2 * time.Second
	cfg.MaxReconnects = 0

	bus, err := NewNATSBus(cfg)
	if err != nil {
		t.Skipf("skipping: NATS not available at %s: %v", url, err)
	}
	bus.Close()

	return url
}

// --- Integration Tests ---

func TestNATSBus_PubSub(t *testing.T) {
	url := getNATSURL(t)

	cfg := DefaultNATSConfig()
	cfg.URL = url
	bus, err := NewNATSBus(cfg)
	if err != nil {
		t.Fatalf("NewNATSBus error: %v", err)
	}
	defer bus.Close()

	sub, err := bus.Subscribe("test.nats")
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	defer sub.Unsubscribe()

	// Publish
	err = bus.Publish("test.nats", []byte("hello nats"))
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}

	// Receive
	select {
	case msg := <-sub.Messages():
		if string(msg.Data) != "hello nats" {
			t.Errorf("data = %q, want %q", msg.Data, "hello nats")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestNATSBus_QueueSubscribe(t *testing.T) {
	url := getNATSURL(t)

	cfg := DefaultNATSConfig()
	cfg.URL = url
	bus, err := NewNATSBus(cfg)
	if err != nil {
		t.Fatalf("NewNATSBus error: %v", err)
	}
	defer bus.Close()

	// Create queue subscribers
	sub1, _ := bus.QueueSubscribe("test.queue", "workers")
	sub2, _ := bus.QueueSubscribe("test.queue", "workers")
	defer sub1.Unsubscribe()
	defer sub2.Unsubscribe()

	// Publish
	bus.Publish("test.queue", []byte("queued"))

	// Only one should receive
	received := 0
	timeout := time.After(time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-sub1.Messages():
			received++
		case <-sub2.Messages():
			received++
		case <-timeout:
			break
		}
	}

	if received != 1 {
		t.Errorf("received = %d, want 1 (load balanced)", received)
	}
}

func TestNATSBus_Request(t *testing.T) {
	url := getNATSURL(t)

	cfg := DefaultNATSConfig()
	cfg.URL = url
	bus, err := NewNATSBus(cfg)
	if err != nil {
		t.Fatalf("NewNATSBus error: %v", err)
	}
	defer bus.Close()

	// Start responder
	sub, _ := bus.Subscribe("test.service")
	go func() {
		for msg := range sub.Messages() {
			if msg.Reply != "" {
				bus.Publish(msg.Reply, []byte("nats-pong"))
			}
		}
	}()
	defer sub.Unsubscribe()

	// Request
	reply, err := bus.Request("test.service", []byte("ping"), 2*time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if string(reply.Data) != "nats-pong" {
		t.Errorf("reply = %q, want %q", reply.Data, "nats-pong")
	}
}

func TestNATSBus_RequestTimeout(t *testing.T) {
	url := getNATSURL(t)

	cfg := DefaultNATSConfig()
	cfg.URL = url
	bus, err := NewNATSBus(cfg)
	if err != nil {
		t.Fatalf("NewNATSBus error: %v", err)
	}
	defer bus.Close()

	// No responder - should timeout
	_, err = bus.Request("test.noresponder", []byte("ping"), 100*time.Millisecond)
	if err != ErrTimeout && err != ErrNoResponders {
		t.Errorf("expected timeout/no responders error, got %v", err)
	}
}

// --- Failure Tests ---

func TestNATSBus_InvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	cfg := DefaultNATSConfig()
	cfg.URL = "nats://invalid-host-that-does-not-exist:4222"
	cfg.ConnectTimeout = 500 * time.Millisecond
	cfg.MaxReconnects = 0

	_, err := NewNATSBus(cfg)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestNATSBus_PublishAfterClose(t *testing.T) {
	url := getNATSURL(t)

	cfg := DefaultNATSConfig()
	cfg.URL = url
	bus, err := NewNATSBus(cfg)
	if err != nil {
		t.Fatalf("NewNATSBus error: %v", err)
	}

	bus.Close()

	err = bus.Publish("test", []byte("hello"))
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

// --- Performance Tests ---

func BenchmarkNATSBus_Publish(b *testing.B) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		b.Skip("NATS_URL not set")
	}

	cfg := DefaultNATSConfig()
	cfg.URL = url
	bus, err := NewNATSBus(cfg)
	if err != nil {
		b.Fatalf("NewNATSBus error: %v", err)
	}
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
