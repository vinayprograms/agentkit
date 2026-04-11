package bus

import (
	"os"
	"testing"
	"time"
)

// natsURL returns the NATS URL for testing, or skips the test.
func natsURL(t *testing.T) string {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}

	if testing.Short() {
		t.Skip("skipping NATS test in short mode")
	}

	cfg := NATSDefaults()
	cfg.URL = url
	cfg.ConnectTimeout = 2 * time.Second
	cfg.MaxReconnects = 0

	b, err := NATS(cfg)
	if err != nil {
		t.Skipf("skipping: NATS not available at %s: %v", url, err)
	}
	b.Close()

	return url
}

func TestNATS_PubSub(t *testing.T) {
	url := natsURL(t)

	cfg := NATSDefaults()
	cfg.URL = url
	b, err := NATS(cfg)
	if err != nil {
		t.Fatalf("NATS error: %v", err)
	}
	defer b.Close()

	sub, err := b.Subscribe("test.nats")
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}
	defer sub.Unsubscribe()

	b.Publish("test.nats", []byte("hello nats"))

	select {
	case msg := <-sub.Messages():
		if string(msg.Data) != "hello nats" {
			t.Errorf("data = %q, want %q", msg.Data, "hello nats")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestNATS_QueueSubscribe(t *testing.T) {
	url := natsURL(t)

	cfg := NATSDefaults()
	cfg.URL = url
	b, err := NATS(cfg)
	if err != nil {
		t.Fatalf("NATS error: %v", err)
	}
	defer b.Close()

	sub1, _ := b.QueueSubscribe("test.queue", "workers")
	sub2, _ := b.QueueSubscribe("test.queue", "workers")
	defer sub1.Unsubscribe()
	defer sub2.Unsubscribe()

	b.Publish("test.queue", []byte("queued"))

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

func TestNATS_Request(t *testing.T) {
	url := natsURL(t)

	cfg := NATSDefaults()
	cfg.URL = url
	b, err := NATS(cfg)
	if err != nil {
		t.Fatalf("NATS error: %v", err)
	}
	defer b.Close()

	sub, _ := b.Subscribe("test.service")
	go func() {
		for msg := range sub.Messages() {
			if msg.Reply != "" {
				b.Publish(msg.Reply, []byte("nats-pong"))
			}
		}
	}()
	defer sub.Unsubscribe()

	reply, err := b.Request("test.service", []byte("ping"), 2*time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if string(reply.Data) != "nats-pong" {
		t.Errorf("reply = %q, want %q", reply.Data, "nats-pong")
	}
}

func TestNATS_RequestTimeout(t *testing.T) {
	url := natsURL(t)

	cfg := NATSDefaults()
	cfg.URL = url
	b, err := NATS(cfg)
	if err != nil {
		t.Fatalf("NATS error: %v", err)
	}
	defer b.Close()

	_, err = b.Request("test.noresponder", []byte("ping"), 100*time.Millisecond)
	if err != ErrTimeout && err != ErrNoResponders {
		t.Errorf("expected timeout/no responders error, got %v", err)
	}
}

func TestNATS_InvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	cfg := NATSDefaults()
	cfg.URL = "nats://invalid-host-that-does-not-exist:4222"
	cfg.ConnectTimeout = 500 * time.Millisecond
	cfg.MaxReconnects = 0

	_, err := NATS(cfg)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestNATS_PublishAfterClose(t *testing.T) {
	url := natsURL(t)

	cfg := NATSDefaults()
	cfg.URL = url
	b, err := NATS(cfg)
	if err != nil {
		t.Fatalf("NATS error: %v", err)
	}

	b.Close()

	err = b.Publish("test", []byte("hello"))
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func BenchmarkNATS_Publish(b *testing.B) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		b.Skip("NATS_URL not set")
	}

	cfg := NATSDefaults()
	cfg.URL = url
	mb, err := NATS(cfg)
	if err != nil {
		b.Fatalf("NATS error: %v", err)
	}
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
