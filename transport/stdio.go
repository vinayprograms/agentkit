package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
)

// StdioTransport implements Transport over stdin/stdout.
type StdioTransport struct {
	reader io.Reader
	writer io.Writer
	config Config

	recv     chan *InboundMessage
	send     chan *OutboundMessage
	done     chan struct{}
	closeErr error
	mu       sync.Mutex
	closed   bool
}

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(r io.Reader, w io.Writer, cfg Config) *StdioTransport {
	if cfg.RecvBufferSize <= 0 {
		cfg.RecvBufferSize = DefaultConfig().RecvBufferSize
	}
	if cfg.SendBufferSize <= 0 {
		cfg.SendBufferSize = DefaultConfig().SendBufferSize
	}

	return &StdioTransport{
		reader: r,
		writer: w,
		config: cfg,
		recv:   make(chan *InboundMessage, cfg.RecvBufferSize),
		send:   make(chan *OutboundMessage, cfg.SendBufferSize),
		done:   make(chan struct{}),
	}
}

// Recv returns the channel for incoming messages.
func (t *StdioTransport) Recv() <-chan *InboundMessage {
	return t.recv
}

// Send queues a message for delivery.
func (t *StdioTransport) Send(msg *OutboundMessage) error {
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
func (t *StdioTransport) Run(ctx context.Context) error {
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

	// Signal shutdown and wait
	t.Close()
	wg.Wait()

	return ctx.Err()
}

// Close initiates graceful shutdown.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.done)
	t.mu.Unlock()

	return nil
}

// readLoop reads from input and sends to recv channel.
func (t *StdioTransport) readLoop(ctx context.Context) {
	defer close(t.recv)

	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		msg, err := ParseInbound(line)
		if err != nil {
			// Send error response if we can determine the ID
			t.sendParseError(line, err)
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

// writeLoop reads from send channel and writes to output.
func (t *StdioTransport) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			t.drainSendQueue()
			return
		case <-t.done:
			t.drainSendQueue()
			return
		case msg, ok := <-t.send:
			if !ok {
				return
			}
			t.writeMessage(msg)
		}
	}
}

// drainSendQueue writes any remaining messages in the send queue.
func (t *StdioTransport) drainSendQueue() {
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
func (t *StdioTransport) writeMessage(msg *OutboundMessage) {
	data, err := MarshalOutbound(msg)
	if err != nil {
		return // Log error in production
	}

	t.mu.Lock()
	t.writer.Write(append(data, '\n'))
	t.mu.Unlock()
}

// sendParseError sends an error response for parse failures.
func (t *StdioTransport) sendParseError(raw []byte, parseErr error) {
	// Try to extract ID from malformed message
	var partial struct {
		ID interface{} `json:"id"`
	}
	json.Unmarshal(raw, &partial)

	rpcErr, ok := parseErr.(*Error)
	if !ok {
		rpcErr = &Error{Code: ParseError, Message: "Parse error", Data: parseErr.Error()}
	}

	t.Send(&OutboundMessage{
		Response: &Response{
			JSONRPC: "2.0",
			ID:      partial.ID,
			Error:   rpcErr,
		},
	})
}
