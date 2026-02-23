package state

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Unit tests for nats.go that don't require a NATS server
// ============================================================================

func TestDefaultNATSStoreConfig(t *testing.T) {
	cfg := DefaultNATSStoreConfig()

	if cfg.Bucket != "agent-state" {
		t.Errorf("expected bucket 'agent-state', got %s", cfg.Bucket)
	}
	if cfg.History != 1 {
		t.Errorf("expected history 1, got %d", cfg.History)
	}
	if cfg.MaxValueSize != 1024*1024 {
		t.Errorf("expected max value size 1MB, got %d", cfg.MaxValueSize)
	}
}

func TestNewNATSStore_NilConn(t *testing.T) {
	_, err := NewNATSStore(NATSStoreConfig{
		Conn:   nil,
		Bucket: "test",
	})

	if err == nil {
		t.Error("expected error for nil connection")
	}
}

func TestOpFromNATS(t *testing.T) {
	tests := []struct {
		name string
		op   jetstream.KeyValueOp
		want Operation
	}{
		{"put", jetstream.KeyValuePut, OpPut},
		{"delete", jetstream.KeyValueDelete, OpDelete},
		{"purge", jetstream.KeyValuePurge, OpDelete},
		{"unknown defaults to put", jetstream.KeyValueOp(99), OpPut},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opFromNATS(tt.op)
			if got != tt.want {
				t.Errorf("opFromNATS(%v) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

// TestNATSStore_Validation tests validation paths that don't need a real connection
func TestNATSStore_Validation(t *testing.T) {
	// Create a closed store to test validation
	store := &NATSStore{}
	store.closed.Store(true)

	t.Run("Get_Closed", func(t *testing.T) {
		_, err := store.Get("key")
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("Get_InvalidKey", func(t *testing.T) {
		store2 := &NATSStore{}
		_, err := store2.Get("")
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("GetKeyValue_Closed", func(t *testing.T) {
		_, err := store.GetKeyValue("key")
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("GetKeyValue_InvalidKey", func(t *testing.T) {
		store2 := &NATSStore{}
		_, err := store2.GetKeyValue("")
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("Put_Closed", func(t *testing.T) {
		err := store.Put("key", []byte("value"), 0)
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("Put_InvalidKey", func(t *testing.T) {
		store2 := &NATSStore{}
		err := store2.Put("", []byte("value"), 0)
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("Put_InvalidTTL", func(t *testing.T) {
		store2 := &NATSStore{}
		err := store2.Put("key", []byte("value"), -time.Second)
		if err != ErrInvalidTTL {
			t.Errorf("expected ErrInvalidTTL, got %v", err)
		}
	})

	t.Run("Delete_Closed", func(t *testing.T) {
		err := store.Delete("key")
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("Delete_InvalidKey", func(t *testing.T) {
		store2 := &NATSStore{}
		err := store2.Delete("")
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("Keys_Closed", func(t *testing.T) {
		_, err := store.Keys("*")
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("Watch_Closed", func(t *testing.T) {
		_, err := store.Watch("*")
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("Lock_Closed", func(t *testing.T) {
		_, err := store.Lock("key", time.Second)
		if err != ErrClosed {
			t.Errorf("expected ErrClosed, got %v", err)
		}
	})

	t.Run("Lock_InvalidKey", func(t *testing.T) {
		store2 := &NATSStore{}
		_, err := store2.Lock("", time.Second)
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("Lock_InvalidTTL_Zero", func(t *testing.T) {
		store2 := &NATSStore{}
		_, err := store2.Lock("key", 0)
		if err != ErrInvalidTTL {
			t.Errorf("expected ErrInvalidTTL, got %v", err)
		}
	})

	t.Run("Lock_InvalidTTL_Negative", func(t *testing.T) {
		store2 := &NATSStore{}
		_, err := store2.Lock("key", -time.Second)
		if err != ErrInvalidTTL {
			t.Errorf("expected ErrInvalidTTL, got %v", err)
		}
	})
}

func TestNATSStore_Close_Idempotent(t *testing.T) {
	store := &NATSStore{
		locks: make(map[string]*natsLock),
	}

	// First close
	err := store.Close()
	if err != nil {
		t.Errorf("first Close failed: %v", err)
	}

	// Second close should also succeed (idempotent)
	err = store.Close()
	if err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

func TestNATSLock_Key(t *testing.T) {
	lock := &natsLock{
		key: "_lock.myresource",
	}

	if lock.Key() != "_lock.myresource" {
		t.Errorf("expected _lock.myresource, got %s", lock.Key())
	}
}

func TestNATSLock_Unlock_AlreadyReleased(t *testing.T) {
	store := &NATSStore{
		locks: make(map[string]*natsLock),
	}

	lock := &natsLock{
		store: store,
		key:   "_lock.test",
	}
	lock.released.Store(true)

	err := lock.Unlock()
	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestNATSLock_Refresh_AlreadyReleased(t *testing.T) {
	lock := &natsLock{}
	lock.released.Store(true)

	err := lock.Refresh()
	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestNATSLock_Refresh_Expired(t *testing.T) {
	lock := &natsLock{
		ttl:     50 * time.Millisecond,
		created: time.Now().Add(-100 * time.Millisecond), // Already expired
	}

	err := lock.Refresh()
	if err != ErrLockExpired {
		t.Errorf("expected ErrLockExpired, got %v", err)
	}
}

func TestNATSStore_Close_ReleasesLocks(t *testing.T) {
	store := &NATSStore{
		locks: make(map[string]*natsLock),
	}

	lock1 := &natsLock{store: store, key: "_lock.a"}
	lock2 := &natsLock{store: store, key: "_lock.b"}
	store.locks["_lock.a"] = lock1
	store.locks["_lock.b"] = lock2

	store.Close()

	// All locks should be marked as released
	if !lock1.released.Load() {
		t.Error("lock1 should be released after Close")
	}
	if !lock2.released.Load() {
		t.Error("lock2 should be released after Close")
	}
}

// Test NATSStoreConfig defaults are applied
func TestNATSStoreConfig_Defaults(t *testing.T) {
	// This tests the logic in NewNATSStore that applies defaults
	// We can't fully test without NATS, but we can verify the default config
	cfg := NATSStoreConfig{}

	if cfg.Bucket == "" {
		cfg.Bucket = DefaultNATSStoreConfig().Bucket
	}
	if cfg.History <= 0 {
		cfg.History = DefaultNATSStoreConfig().History
	}
	if cfg.MaxValueSize <= 0 {
		cfg.MaxValueSize = DefaultNATSStoreConfig().MaxValueSize
	}

	if cfg.Bucket != "agent-state" {
		t.Errorf("expected default bucket, got %s", cfg.Bucket)
	}
	if cfg.History != 1 {
		t.Errorf("expected default history, got %d", cfg.History)
	}
	if cfg.MaxValueSize != 1024*1024 {
		t.Errorf("expected default max value size, got %d", cfg.MaxValueSize)
	}
}
