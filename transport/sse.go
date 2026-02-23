package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// SSETransport implements Transport using Server-Sent Events for server→client
// and HTTP POST for client→server communication.
type SSETransport struct {
	config SSEConfig

	recv   chan *InboundMessage
	send   chan *OutboundMessage
	done   chan struct{}
	mu     sync.Mutex
	closed bool

	// Server-side: track connected SSE clients
	clients   map[string]chan []byte
	clientsMu sync.RWMutex
}

// SSEConfig holds SSE transport configuration.
type SSEConfig struct {
	Config // Embed base config

	// FlushInterval for SSE writes.
	FlushInterval time.Duration

	// HeartbeatInterval sends SSE comments as keepalive (0 = disabled).
	HeartbeatInterval time.Duration
}

// DefaultSSEConfig returns configuration with sensible defaults.
func DefaultSSEConfig() SSEConfig {
	return SSEConfig{
		Config:            DefaultConfig(),
		FlushInterval:     100 * time.Millisecond,
		HeartbeatInterval: 30 * time.Second,
	}
}

// NewSSETransport creates a new SSE transport.
func NewSSETransport(cfg SSEConfig) *SSETransport {
	if cfg.RecvBufferSize <= 0 {
		cfg.RecvBufferSize = DefaultConfig().RecvBufferSize
	}
	if cfg.SendBufferSize <= 0 {
		cfg.SendBufferSize = DefaultConfig().SendBufferSize
	}

	return &SSETransport{
		config:  cfg,
		recv:    make(chan *InboundMessage, cfg.RecvBufferSize),
		send:    make(chan *OutboundMessage, cfg.SendBufferSize),
		done:    make(chan struct{}),
		clients: make(map[string]chan []byte),
	}
}

// Recv returns the channel for incoming messages.
func (t *SSETransport) Recv() <-chan *InboundMessage {
	return t.recv
}

// Send queues a message for delivery to all connected SSE clients.
func (t *SSETransport) Send(msg *OutboundMessage) error {
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
func (t *SSETransport) Run(ctx context.Context) error {
	// Broadcast loop
	go t.broadcastLoop(ctx)

	<-ctx.Done()
	t.Close()
	return ctx.Err()
}

// Close initiates graceful shutdown.
func (t *SSETransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.done)
	t.mu.Unlock()

	// Close all client channels
	t.clientsMu.Lock()
	for id, ch := range t.clients {
		close(ch)
		delete(t.clients, id)
	}
	t.clientsMu.Unlock()

	return nil
}

// HandleSSE is an HTTP handler for SSE connections.
// Mount this at your SSE endpoint (e.g., /events).
func (t *SSETransport) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Flush headers immediately to establish connection
	flusher.Flush()

	// Create client channel
	clientID := fmt.Sprintf("%p", r)
	clientCh := make(chan []byte, 100)

	t.clientsMu.Lock()
	t.clients[clientID] = clientCh
	t.clientsMu.Unlock()

	defer func() {
		t.clientsMu.Lock()
		delete(t.clients, clientID)
		t.clientsMu.Unlock()
	}()

	// Heartbeat ticker
	var heartbeat <-chan time.Time
	if t.config.HeartbeatInterval > 0 {
		ticker := time.NewTicker(t.config.HeartbeatInterval)
		defer ticker.Stop()
		heartbeat = ticker.C
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-t.done:
			return
		case <-heartbeat:
			// Send SSE comment as heartbeat
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case data, ok := <-clientCh:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// HandlePost is an HTTP handler for receiving JSON-RPC requests.
// Mount this at your request endpoint (e.g., /rpc).
func (t *SSETransport) HandlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024)) // 1MB limit
	if err != nil {
		http.Error(w, "Read error", http.StatusBadRequest)
		return
	}

	msg, parseErr := ParseInbound(body)
	if parseErr != nil {
		t.writeHTTPError(w, parseErr)
		return
	}

	select {
	case t.recv <- msg:
		// For requests, response goes via SSE
		// For immediate response, could use synchronous mode
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"accepted"}`))
	case <-t.done:
		http.Error(w, "Transport closed", http.StatusServiceUnavailable)
	}
}

// broadcastLoop sends outbound messages to all connected clients.
func (t *SSETransport) broadcastLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		case msg, ok := <-t.send:
			if !ok {
				return
			}
			t.broadcast(msg)
		}
	}
}

// broadcast sends a message to all connected SSE clients.
func (t *SSETransport) broadcast(msg *OutboundMessage) {
	data, err := MarshalOutbound(msg)
	if err != nil {
		return
	}

	t.clientsMu.RLock()
	defer t.clientsMu.RUnlock()

	for _, ch := range t.clients {
		select {
		case ch <- data:
		default:
			// Client buffer full, skip
		}
	}
}

// writeHTTPError writes a JSON-RPC error as HTTP response.
func (t *SSETransport) writeHTTPError(w http.ResponseWriter, err error) {
	rpcErr, ok := err.(*Error)
	if !ok {
		rpcErr = &Error{Code: InternalError, Message: err.Error()}
	}

	resp := Response{
		JSONRPC: "2.0",
		ID:      nil,
		Error:   rpcErr,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(resp)
}

// --- Client-side SSE support ---

// SSEClient connects to an SSE endpoint and receives messages.
type SSEClient struct {
	url    string
	recv   chan *InboundMessage
	done   chan struct{}
	mu     sync.Mutex
	closed bool
}

// NewSSEClient creates a client for connecting to an SSE endpoint.
func NewSSEClient(url string, bufferSize int) *SSEClient {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &SSEClient{
		url:  url,
		recv: make(chan *InboundMessage, bufferSize),
		done: make(chan struct{}),
	}
}

// Recv returns the channel for incoming messages.
func (c *SSEClient) Recv() <-chan *InboundMessage {
	return c.recv
}

// Connect establishes the SSE connection and starts receiving.
func (c *SSEClient) Connect(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	go c.readLoop(ctx, resp.Body)
	return nil
}

// Close closes the SSE client.
func (c *SSEClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.done)
	c.mu.Unlock()
	return nil
}

// readLoop reads SSE events and parses JSON-RPC messages.
func (c *SSEClient) readLoop(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	defer close(c.recv)

	scanner := bufio.NewScanner(body)
	var dataBuffer bytes.Buffer

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
		}

		line := scanner.Text()

		if line == "" {
			// End of event, process accumulated data
			if dataBuffer.Len() > 0 {
				c.processEvent(dataBuffer.Bytes())
				dataBuffer.Reset()
			}
			continue
		}

		if len(line) > 5 && line[:5] == "data:" {
			data := line[5:]
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			dataBuffer.WriteString(data)
		}
	}
}

// processEvent parses an SSE data payload as JSON-RPC.
func (c *SSEClient) processEvent(data []byte) {
	// Try parsing as Response first
	var resp Response
	if err := json.Unmarshal(data, &resp); err == nil && resp.JSONRPC == "2.0" {
		// It's a response or notification from server
		// For SSE, server sends responses and notifications
		msg := &InboundMessage{Raw: data}
		if resp.ID != nil {
			// Has ID, but from server perspective it's a response
			// Client sees it as inbound
		}
		// Parse as notification for client
		var notif Notification
		if json.Unmarshal(data, &notif) == nil {
			msg.Notification = &notif
		}

		select {
		case c.recv <- msg:
		default:
		}
	}
}
