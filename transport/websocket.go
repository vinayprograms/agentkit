package transport

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketTransport implements Transport over WebSocket.
type WebSocketTransport struct {
	conn     *websocket.Conn
	config   WebSocketConfig
	upgrader *websocket.Upgrader

	recv     chan *InboundMessage
	send     chan *OutboundMessage
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
}

// WebSocketConfig holds WebSocket transport configuration.
type WebSocketConfig struct {
	Config // Embed base config

	// WriteTimeout for write operations.
	WriteTimeout time.Duration

	// ReadTimeout for read operations (0 = no timeout).
	ReadTimeout time.Duration

	// MaxMessageSize limits incoming message size.
	MaxMessageSize int64

	// PingInterval for keepalive pings (0 = disabled).
	PingInterval time.Duration
}

// DefaultWebSocketConfig returns configuration with sensible defaults.
func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		Config:         DefaultConfig(),
		WriteTimeout:   10 * time.Second,
		ReadTimeout:    0,
		MaxMessageSize: 1024 * 1024, // 1MB
		PingInterval:   30 * time.Second,
	}
}

// NewWebSocketTransport creates a transport from an existing connection.
func NewWebSocketTransport(conn *websocket.Conn, cfg WebSocketConfig) *WebSocketTransport {
	if cfg.RecvBufferSize <= 0 {
		cfg.RecvBufferSize = DefaultConfig().RecvBufferSize
	}
	if cfg.SendBufferSize <= 0 {
		cfg.SendBufferSize = DefaultConfig().SendBufferSize
	}

	conn.SetReadLimit(cfg.MaxMessageSize)

	return &WebSocketTransport{
		conn:   conn,
		config: cfg,
		recv:   make(chan *InboundMessage, cfg.RecvBufferSize),
		send:   make(chan *OutboundMessage, cfg.SendBufferSize),
		done:   make(chan struct{}),
	}
}

// NewWebSocketUpgrader creates an upgrader for accepting WebSocket connections.
func NewWebSocketUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true }, // Override in production
	}
}

// Recv returns the channel for incoming messages.
func (t *WebSocketTransport) Recv() <-chan *InboundMessage {
	return t.recv
}

// Send queues a message for delivery.
func (t *WebSocketTransport) Send(msg *OutboundMessage) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrClosed
	}
	t.mu.Unlock()

	select {
	case t.send <- msg:
		return nil
	case <-t.done:
		return ErrClosed
	}
}

// Run starts the transport, blocking until shutdown.
func (t *WebSocketTransport) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(2)

	// Reader goroutine
	go func() {
		defer wg.Done()
		t.readLoop(ctx)
	}()

	// Writer goroutine
	go func() {
		defer wg.Done()
		t.writeLoop(ctx)
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Signal shutdown
	t.Close()
	wg.Wait()

	return ctx.Err()
}

// Close initiates graceful shutdown.
func (t *WebSocketTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.done)
	t.mu.Unlock()

	// Send close message
	t.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)

	return t.conn.Close()
}

// readLoop reads WebSocket messages and sends to recv channel.
func (t *WebSocketTransport) readLoop(ctx context.Context) {
	defer close(t.recv)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		default:
		}

		_, data, err := t.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			// Connection error, exit loop
			return
		}

		msg, parseErr := ParseInbound(data)
		if parseErr != nil {
			t.sendParseError(data, parseErr)
			continue
		}

		select {
		case t.recv <- msg:
		case <-ctx.Done():
			return
		case <-t.done:
			return
		}
	}
}

// writeLoop reads from send channel and writes to WebSocket.
func (t *WebSocketTransport) writeLoop(ctx context.Context) {
	ticker := t.createPingTicker()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.drainSendQueue()
			return
		case <-t.done:
			t.drainSendQueue()
			return
		case <-ticker.C:
			t.writePing()
		case msg, ok := <-t.send:
			if !ok {
				return
			}
			t.writeMessage(msg)
		}
	}
}

// createPingTicker creates a ticker for keepalive pings.
func (t *WebSocketTransport) createPingTicker() *time.Ticker {
	if t.config.PingInterval > 0 {
		return time.NewTicker(t.config.PingInterval)
	}
	// Return a ticker that never fires
	ticker := time.NewTicker(time.Hour)
	ticker.Stop()
	return ticker
}

// writePing sends a WebSocket ping frame.
func (t *WebSocketTransport) writePing() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	t.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
}

// drainSendQueue writes remaining messages before shutdown.
func (t *WebSocketTransport) drainSendQueue() {
	for {
		select {
		case msg, ok := <-t.send:
			if !ok {
				return
			}
			t.writeMessage(msg)
		default:
			return
		}
	}
}

// writeMessage serializes and writes a single message.
func (t *WebSocketTransport) writeMessage(msg *OutboundMessage) {
	data, err := MarshalOutbound(msg)
	if err != nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	if t.config.WriteTimeout > 0 {
		t.conn.SetWriteDeadline(time.Now().Add(t.config.WriteTimeout))
	}

	t.conn.WriteMessage(websocket.TextMessage, data)
}

// sendParseError sends an error response for parse failures.
func (t *WebSocketTransport) sendParseError(raw []byte, parseErr error) {
	rpcErr, ok := parseErr.(*Error)
	if !ok {
		rpcErr = &Error{Code: ParseError, Message: "Parse error", Data: parseErr.Error()}
	}

	t.Send(&OutboundMessage{
		Response: &Response{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   rpcErr,
		},
	})
}
