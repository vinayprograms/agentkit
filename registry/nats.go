package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSRegistry implements Registry using NATS JetStream KV store.
// Suitable for distributed deployments across multiple nodes.
type NATSRegistry struct {
	conn   *nats.Conn
	kv     jetstream.KeyValue
	config NATSRegistryConfig

	mu       sync.RWMutex
	watchers []chan Event
	closed   bool
	cancel   context.CancelFunc
}

// NATSRegistryConfig configures the NATS registry.
type NATSRegistryConfig struct {
	// BucketName is the KV bucket name. Default: "agent-registry"
	BucketName string

	// TTL for agent entries. Zero means no expiry.
	// Note: NATS KV has its own TTL handling.
	TTL time.Duration

	// Replicas for the KV store (1-5). Default: 1
	Replicas int
}

// DefaultNATSRegistryConfig returns configuration with sensible defaults.
func DefaultNATSRegistryConfig() NATSRegistryConfig {
	return NATSRegistryConfig{
		BucketName: "agent-registry",
		TTL:        30 * time.Second,
		Replicas:   1,
	}
}

// NewNATSRegistry creates a new NATS registry from an existing connection.
func NewNATSRegistry(conn *nats.Conn, cfg NATSRegistryConfig) (*NATSRegistry, error) {
	if conn == nil {
		return nil, fmt.Errorf("nil connection")
	}

	if cfg.BucketName == "" {
		cfg.BucketName = "agent-registry"
	}
	if cfg.Replicas < 1 {
		cfg.Replicas = 1
	}

	js, err := jetstream.New(conn)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	ctx := context.Background()

	// Create or get KV bucket
	kvCfg := jetstream.KeyValueConfig{
		Bucket:   cfg.BucketName,
		Replicas: cfg.Replicas,
	}
	if cfg.TTL > 0 {
		kvCfg.TTL = cfg.TTL
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, kvCfg)
	if err != nil {
		return nil, fmt.Errorf("create kv bucket: %w", err)
	}

	watchCtx, cancel := context.WithCancel(context.Background())

	r := &NATSRegistry{
		conn:     conn,
		kv:       kv,
		config:   cfg,
		watchers: make([]chan Event, 0),
		cancel:   cancel,
	}

	// Start KV watcher
	go r.watchKV(watchCtx)

	return r, nil
}

// Register adds or updates an agent in the registry.
func (r *NATSRegistry) Register(info AgentInfo) error {
	if err := ValidateAgentInfo(info); err != nil {
		return err
	}

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return ErrClosed
	}
	r.mu.RUnlock()

	// Set LastSeen to now
	info.LastSeen = time.Now()

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal agent info: %w", err)
	}

	ctx := context.Background()
	_, err = r.kv.Put(ctx, info.ID, data)
	if err != nil {
		return fmt.Errorf("put to kv: %w", err)
	}

	return nil
}

// Deregister removes an agent from the registry.
func (r *NATSRegistry) Deregister(id string) error {
	if id == "" {
		return ErrInvalidID
	}

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return ErrClosed
	}
	r.mu.RUnlock()

	ctx := context.Background()

	// Check if exists first
	_, err := r.kv.Get(ctx, id)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return ErrNotFound
		}
		return fmt.Errorf("get from kv: %w", err)
	}

	err = r.kv.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("delete from kv: %w", err)
	}

	return nil
}

// Get retrieves a specific agent by ID.
func (r *NATSRegistry) Get(id string) (*AgentInfo, error) {
	if id == "" {
		return nil, ErrInvalidID
	}

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, ErrClosed
	}
	r.mu.RUnlock()

	ctx := context.Background()
	entry, err := r.kv.Get(ctx, id)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get from kv: %w", err)
	}

	var info AgentInfo
	if err := json.Unmarshal(entry.Value(), &info); err != nil {
		return nil, fmt.Errorf("unmarshal agent info: %w", err)
	}

	return &info, nil
}

// List returns all agents matching the filter.
func (r *NATSRegistry) List(filter *Filter) ([]AgentInfo, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, ErrClosed
	}
	r.mu.RUnlock()

	ctx := context.Background()
	keys, err := r.kv.Keys(ctx)
	if err != nil {
		if err == jetstream.ErrNoKeysFound {
			return []AgentInfo{}, nil
		}
		return nil, fmt.Errorf("list keys: %w", err)
	}

	var result []AgentInfo
	for _, key := range keys {
		entry, err := r.kv.Get(ctx, key)
		if err != nil {
			continue // Key might have been deleted
		}

		var info AgentInfo
		if err := json.Unmarshal(entry.Value(), &info); err != nil {
			continue
		}

		if MatchesFilter(info, filter) {
			result = append(result, info)
		}
	}

	// Sort by ID for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// FindByCapability returns agents with a specific capability.
func (r *NATSRegistry) FindByCapability(capability string) ([]AgentInfo, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, ErrClosed
	}
	r.mu.RUnlock()

	ctx := context.Background()
	keys, err := r.kv.Keys(ctx)
	if err != nil {
		if err == jetstream.ErrNoKeysFound {
			return []AgentInfo{}, nil
		}
		return nil, fmt.Errorf("list keys: %w", err)
	}

	var result []AgentInfo
	for _, key := range keys {
		entry, err := r.kv.Get(ctx, key)
		if err != nil {
			continue
		}

		var info AgentInfo
		if err := json.Unmarshal(entry.Value(), &info); err != nil {
			continue
		}

		if HasCapability(info, capability) {
			result = append(result, info)
		}
	}

	// Sort by load (lowest first) for load balancing
	sort.Slice(result, func(i, j int) bool {
		return result[i].Load < result[j].Load
	})

	return result, nil
}

// Watch returns a channel of registry events.
func (r *NATSRegistry) Watch() (<-chan Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrClosed
	}

	ch := make(chan Event, 64)
	r.watchers = append(r.watchers, ch)

	return ch, nil
}

// Close shuts down the registry.
func (r *NATSRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	r.cancel()

	// Close all watcher channels
	for _, ch := range r.watchers {
		close(ch)
	}
	r.watchers = nil

	return nil
}

// watchKV monitors the KV store for changes and notifies watchers.
func (r *NATSRegistry) watchKV(ctx context.Context) {
	watcher, err := r.kv.WatchAll(ctx)
	if err != nil {
		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-watcher.Updates():
			if entry == nil {
				continue
			}

			r.mu.RLock()
			if r.closed {
				r.mu.RUnlock()
				return
			}

			var event Event
			switch entry.Operation() {
			case jetstream.KeyValuePut:
				var info AgentInfo
				if err := json.Unmarshal(entry.Value(), &info); err != nil {
					r.mu.RUnlock()
					continue
				}
				// Check revision to determine if added or updated
				if entry.Revision() == 1 {
					event = Event{Type: EventAdded, Agent: info}
				} else {
					event = Event{Type: EventUpdated, Agent: info}
				}
			case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
				event = Event{
					Type:  EventRemoved,
					Agent: AgentInfo{ID: entry.Key()},
				}
			default:
				r.mu.RUnlock()
				continue
			}

			// Notify watchers
			for _, ch := range r.watchers {
				select {
				case ch <- event:
				default:
				}
			}
			r.mu.RUnlock()
		}
	}
}

// Conn returns the underlying NATS connection.
func (r *NATSRegistry) Conn() *nats.Conn {
	return r.conn
}
