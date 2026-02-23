package bus

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSBus implements MessageBus using NATS.
type NATSBus struct {
	conn   *nats.Conn
	config NATSConfig
}

// NATSConfig holds NATS connection configuration.
type NATSConfig struct {
	Config // Embed base config

	// URL is the NATS server URL (e.g., "nats://localhost:4222").
	URL string

	// Name is the client name for identification.
	Name string

	// Token for token-based auth.
	Token string

	// User and Password for basic auth.
	User     string
	Password string

	// TLSConfig for secure connections (nil = no TLS).
	// Use nats.Secure() or nats.RootCAs() options instead.

	// ReconnectWait is the time to wait between reconnection attempts.
	ReconnectWait time.Duration

	// MaxReconnects is the maximum number of reconnection attempts.
	// -1 = unlimited
	MaxReconnects int

	// ConnectTimeout for initial connection.
	ConnectTimeout time.Duration
}

// DefaultNATSConfig returns configuration with sensible defaults.
func DefaultNATSConfig() NATSConfig {
	return NATSConfig{
		Config:         DefaultConfig(),
		URL:            nats.DefaultURL,
		ReconnectWait:  2 * time.Second,
		MaxReconnects:  -1, // Unlimited
		ConnectTimeout: 5 * time.Second,
	}
}

// NewNATSBus creates a new NATS message bus.
func NewNATSBus(cfg NATSConfig) (*NATSBus, error) {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = DefaultConfig().BufferSize
	}
	if cfg.URL == "" {
		cfg.URL = nats.DefaultURL
	}

	opts := buildNATSOptions(cfg)

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &NATSBus{
		conn:   conn,
		config: cfg,
	}, nil
}

// NewNATSBusFromConn creates a NATSBus from an existing connection.
func NewNATSBusFromConn(conn *nats.Conn, cfg NATSConfig) *NATSBus {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = DefaultConfig().BufferSize
	}

	return &NATSBus{
		conn:   conn,
		config: cfg,
	}
}

// buildNATSOptions constructs NATS connection options from config.
func buildNATSOptions(cfg NATSConfig) []nats.Option {
	opts := []nats.Option{
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.Timeout(cfg.ConnectTimeout),
	}

	if cfg.Name != "" {
		opts = append(opts, nats.Name(cfg.Name))
	}

	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	}

	if cfg.User != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}

	return opts
}

// Publish sends a message to a subject.
func (b *NATSBus) Publish(subject string, data []byte) error {
	if err := ValidateSubject(subject); err != nil {
		return err
	}
	if b.conn.IsClosed() {
		return ErrClosed
	}

	if err := b.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}

	return nil
}

// Subscribe creates a subscription to a subject.
func (b *NATSBus) Subscribe(subject string) (Subscription, error) {
	if err := ValidateSubject(subject); err != nil {
		return nil, err
	}
	if b.conn.IsClosed() {
		return nil, ErrClosed
	}

	ch := make(chan *Message, b.config.BufferSize)
	
	natsSub, err := b.conn.Subscribe(subject, func(m *nats.Msg) {
		msg := &Message{
			Subject: m.Subject,
			Data:    m.Data,
			Reply:   m.Reply,
		}
		select {
		case ch <- msg:
		default:
			// Buffer full
		}
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats subscribe: %w", err)
	}

	return &natsSub_{
		sub: natsSub,
		ch:  ch,
	}, nil
}

// QueueSubscribe creates a queue subscription.
func (b *NATSBus) QueueSubscribe(subject, queue string) (Subscription, error) {
	if err := ValidateSubject(subject); err != nil {
		return nil, err
	}
	if queue == "" {
		return nil, ErrInvalidSubject
	}
	if b.conn.IsClosed() {
		return nil, ErrClosed
	}

	ch := make(chan *Message, b.config.BufferSize)

	natsSub, err := b.conn.QueueSubscribe(subject, queue, func(m *nats.Msg) {
		msg := &Message{
			Subject: m.Subject,
			Data:    m.Data,
			Reply:   m.Reply,
		}
		select {
		case ch <- msg:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats queue subscribe: %w", err)
	}

	return &natsSub_{
		sub: natsSub,
		ch:  ch,
	}, nil
}

// Request sends a request and waits for reply.
func (b *NATSBus) Request(subject string, data []byte, timeout time.Duration) (*Message, error) {
	if err := ValidateSubject(subject); err != nil {
		return nil, err
	}
	if b.conn.IsClosed() {
		return nil, ErrClosed
	}

	reply, err := b.conn.Request(subject, data, timeout)
	if err != nil {
		if err == nats.ErrTimeout {
			return nil, ErrTimeout
		}
		if err == nats.ErrNoResponders {
			return nil, ErrNoResponders
		}
		return nil, fmt.Errorf("nats request: %w", err)
	}

	return &Message{
		Subject: reply.Subject,
		Data:    reply.Data,
		Reply:   reply.Reply,
	}, nil
}

// Close shuts down the NATS connection.
func (b *NATSBus) Close() error {
	b.conn.Close()
	return nil
}

// Conn returns the underlying NATS connection for advanced use.
func (b *NATSBus) Conn() *nats.Conn {
	return b.conn
}

// natsSub_ wraps a NATS subscription.
type natsSub_ struct {
	sub *nats.Subscription
	ch  chan *Message
}

// Messages returns the message channel.
func (s *natsSub_) Messages() <-chan *Message {
	return s.ch
}

// Unsubscribe cancels the subscription.
func (s *natsSub_) Unsubscribe() error {
	err := s.sub.Unsubscribe()
	close(s.ch)
	return err
}
