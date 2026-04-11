package bus

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSConfig holds NATS connection configuration.
type NATSConfig struct {
	Config // embed base config

	URL            string        // NATS server URL (default: nats://localhost:4222)
	Name           string        // client name for identification
	Token          string        // token-based auth
	User           string        // basic auth user
	Password       string        // basic auth password
	ReconnectWait  time.Duration // time between reconnection attempts
	MaxReconnects  int           // max reconnection attempts (-1 = unlimited)
	ConnectTimeout time.Duration // initial connection timeout
}

// NATSDefaults returns NATSConfig with sensible defaults.
func NATSDefaults() NATSConfig {
	return NATSConfig{
		URL:            nats.DefaultURL,
		ReconnectWait:  2 * time.Second,
		MaxReconnects:  -1,
		ConnectTimeout: 5 * time.Second,
	}
}

// natsBus implements Bus using NATS.
type natsBus struct {
	conn   *nats.Conn
	config NATSConfig
}

// NATS creates a Bus backed by a NATS server.
func NATS(cfg NATSConfig) (Bus, error) {
	if cfg.URL == "" {
		cfg.URL = nats.DefaultURL
	}

	conn, err := nats.Connect(cfg.URL, natsOptions(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &natsBus{conn: conn, config: cfg}, nil
}

// FromConn creates a Bus from an existing NATS connection.
func FromConn(conn *nats.Conn, cfg NATSConfig) Bus {
	return &natsBus{conn: conn, config: cfg}
}

// natsOptions constructs NATS connection options from config.
func natsOptions(cfg NATSConfig) []nats.Option {
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
func (b *natsBus) Publish(subject string, data []byte) error {
	if err := validateSubject(subject); err != nil {
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
func (b *natsBus) Subscribe(subject string) (Subscription, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	if b.conn.IsClosed() {
		return nil, ErrClosed
	}

	ch := make(chan *Message, b.config.bufferSize())

	ns, err := b.conn.Subscribe(subject, func(m *nats.Msg) {
		select {
		case ch <- &Message{Subject: m.Subject, Data: m.Data, Reply: m.Reply}:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats subscribe: %w", err)
	}

	return &natsSub{sub: ns, ch: ch}, nil
}

// QueueSubscribe creates a queue subscription.
func (b *natsBus) QueueSubscribe(subject, queue string) (Subscription, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	if queue == "" {
		return nil, ErrInvalidSubject
	}
	if b.conn.IsClosed() {
		return nil, ErrClosed
	}

	ch := make(chan *Message, b.config.bufferSize())

	ns, err := b.conn.QueueSubscribe(subject, queue, func(m *nats.Msg) {
		select {
		case ch <- &Message{Subject: m.Subject, Data: m.Data, Reply: m.Reply}:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats queue subscribe: %w", err)
	}

	return &natsSub{sub: ns, ch: ch}, nil
}

// Request sends a request and waits for a reply.
func (b *natsBus) Request(subject string, data []byte, timeout time.Duration) (*Message, error) {
	if err := validateSubject(subject); err != nil {
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

	return &Message{Subject: reply.Subject, Data: reply.Data, Reply: reply.Reply}, nil
}

// Close shuts down the NATS connection.
func (b *natsBus) Close() error {
	b.conn.Close()
	return nil
}

// natsSub wraps a NATS subscription.
type natsSub struct {
	sub       *nats.Subscription
	ch        chan *Message
	closeOnce sync.Once
}

// Messages returns the message channel.
func (s *natsSub) Messages() <-chan *Message { return s.ch }

// Unsubscribe cancels the subscription.
func (s *natsSub) Unsubscribe() error {
	err := s.sub.Unsubscribe()
	s.closeOnce.Do(func() { close(s.ch) })
	return err
}
