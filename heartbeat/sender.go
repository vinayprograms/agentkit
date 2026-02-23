package heartbeat

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vinayprograms/agentkit/bus"
)

// BusSender sends heartbeats over a message bus.
type BusSender struct {
	bus     bus.MessageBus
	agentID string
	interval time.Duration

	mu       sync.RWMutex
	status   string
	load     float64
	metadata map[string]string

	running atomic.Bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewBusSender creates a new heartbeat sender.
func NewBusSender(cfg SenderConfig) (*BusSender, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultSenderConfig().Interval
	}

	status := cfg.InitialStatus
	if status == "" {
		status = DefaultSenderConfig().InitialStatus
	}

	return &BusSender{
		bus:      cfg.Bus,
		agentID:  cfg.AgentID,
		interval: interval,
		status:   status,
		metadata: make(map[string]string),
	}, nil
}

// Start begins sending heartbeats at the configured interval.
func (s *BusSender) Start(ctx context.Context) error {
	if s.running.Swap(true) {
		return ErrAlreadyStarted
	}

	if ctx == nil {
		ctx = context.Background()
	}

	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})

	go s.run(ctx)
	return nil
}

// run is the main heartbeat loop.
func (s *BusSender) run(ctx context.Context) {
	defer close(s.doneCh)

	// Send initial heartbeat immediately
	s.sendHeartbeat()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.running.Store(false)
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.sendHeartbeat()
		}
	}
}

// sendHeartbeat publishes a heartbeat message.
func (s *BusSender) sendHeartbeat() error {
	hb := s.buildHeartbeat()
	data, err := hb.Marshal()
	if err != nil {
		return err
	}
	return s.bus.Publish(hb.Subject(), data)
}

// buildHeartbeat creates a heartbeat with current state.
func (s *BusSender) buildHeartbeat() *Heartbeat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hb := &Heartbeat{
		AgentID:   s.agentID,
		Timestamp: time.Now(),
		Status:    s.status,
		Load:      s.load,
	}

	if len(s.metadata) > 0 {
		hb.Metadata = make(map[string]string, len(s.metadata))
		for k, v := range s.metadata {
			hb.Metadata[k] = v
		}
	}

	return hb
}

// SetStatus updates the status included in heartbeats.
func (s *BusSender) SetStatus(status string) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

// SetLoad updates the load metric.
func (s *BusSender) SetLoad(load float64) {
	s.mu.Lock()
	if load < 0 {
		load = 0
	}
	if load > 1 {
		load = 1
	}
	s.load = load
	s.mu.Unlock()
}

// SetMetadata updates a metadata field.
func (s *BusSender) SetMetadata(key, value string) {
	s.mu.Lock()
	s.metadata[key] = value
	s.mu.Unlock()
}

// Stop stops sending heartbeats.
func (s *BusSender) Stop() error {
	if !s.running.Swap(false) {
		return ErrNotStarted
	}
	close(s.stopCh)
	<-s.doneCh
	return nil
}

// AgentID returns the sender's agent ID.
func (s *BusSender) AgentID() string {
	return s.agentID
}

// MemorySender is a test implementation that records sent heartbeats.
type MemorySender struct {
	agentID string
	interval time.Duration

	mu       sync.RWMutex
	status   string
	load     float64
	metadata map[string]string
	sent     []*Heartbeat

	running atomic.Bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewMemorySender creates a sender for testing.
func NewMemorySender(agentID string, interval time.Duration) *MemorySender {
	if interval <= 0 {
		interval = DefaultSenderConfig().Interval
	}
	return &MemorySender{
		agentID:  agentID,
		interval: interval,
		status:   "idle",
		metadata: make(map[string]string),
	}
}

// Start begins recording heartbeats.
func (s *MemorySender) Start(ctx context.Context) error {
	if s.running.Swap(true) {
		return ErrAlreadyStarted
	}

	if ctx == nil {
		ctx = context.Background()
	}

	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})

	go s.run(ctx)
	return nil
}

func (s *MemorySender) run(ctx context.Context) {
	defer close(s.doneCh)

	s.record()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.running.Store(false)
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.record()
		}
	}
}

func (s *MemorySender) record() {
	s.mu.Lock()
	defer s.mu.Unlock()

	hb := &Heartbeat{
		AgentID:   s.agentID,
		Timestamp: time.Now(),
		Status:    s.status,
		Load:      s.load,
	}

	if len(s.metadata) > 0 {
		hb.Metadata = make(map[string]string, len(s.metadata))
		for k, v := range s.metadata {
			hb.Metadata[k] = v
		}
	}

	s.sent = append(s.sent, hb)
}

func (s *MemorySender) SetStatus(status string) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func (s *MemorySender) SetLoad(load float64) {
	s.mu.Lock()
	if load < 0 {
		load = 0
	}
	if load > 1 {
		load = 1
	}
	s.load = load
	s.mu.Unlock()
}

func (s *MemorySender) SetMetadata(key, value string) {
	s.mu.Lock()
	s.metadata[key] = value
	s.mu.Unlock()
}

func (s *MemorySender) Stop() error {
	if !s.running.Swap(false) {
		return ErrNotStarted
	}
	close(s.stopCh)
	<-s.doneCh
	return nil
}

// Sent returns all recorded heartbeats.
func (s *MemorySender) Sent() []*Heartbeat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Heartbeat, len(s.sent))
	copy(result, s.sent)
	return result
}

// Clear clears recorded heartbeats.
func (s *MemorySender) Clear() {
	s.mu.Lock()
	s.sent = nil
	s.mu.Unlock()
}
