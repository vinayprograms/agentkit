// Package transport provides pluggable transports for JSON-RPC 2.0 communication.
//
// The Transport interface enables bidirectional message passing over various
// backends (stdio, WebSocket, SSE) while maintaining JSON-RPC 2.0 as the protocol.
package transport

import (
	"context"
	"encoding/json"
	"errors"
)

// Common errors.
var (
	ErrClosed      = errors.New("transport closed")
	ErrSendTimeout = errors.New("send timeout")
)

// Transport provides bidirectional JSON-RPC message passing.
type Transport interface {
	// Recv returns channel for incoming messages.
	// Channel is closed when transport shuts down.
	Recv() <-chan *InboundMessage

	// Send queues a message for delivery.
	// Returns ErrClosed if transport is closed.
	Send(msg *OutboundMessage) error

	// Run starts the transport, blocks until ctx cancelled or error.
	// Returns nil on graceful shutdown, error otherwise.
	Run(ctx context.Context) error

	// Close initiates graceful shutdown.
	// Drains pending sends before returning.
	Close() error
}

// InboundMessage wraps an incoming JSON-RPC message.
type InboundMessage struct {
	// Request is set if this is a JSON-RPC request (has ID).
	Request *Request

	// Notification is set if this is a notification (no ID).
	Notification *Notification

	// Raw contains the original bytes for passthrough scenarios.
	Raw json.RawMessage
}

// OutboundMessage wraps an outgoing JSON-RPC message.
type OutboundMessage struct {
	// Response is set when replying to a request.
	Response *Response

	// Notification is set when sending an unsolicited notification.
	Notification *Notification
}

// ParseInbound parses raw JSON into an InboundMessage.
func ParseInbound(data []byte) (*InboundMessage, error) {
	// First, parse to check structure
	var raw struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &Error{Code: ParseError, Message: "Parse error", Data: err.Error()}
	}

	if raw.JSONRPC != "2.0" {
		return nil, &Error{Code: InvalidRequest, Message: "Invalid Request", Data: "jsonrpc must be 2.0"}
	}

	msg := &InboundMessage{Raw: data}

	// If ID is present and not null, it's a request
	if len(raw.ID) > 0 && string(raw.ID) != "null" {
		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, &Error{Code: ParseError, Message: "Parse error", Data: err.Error()}
		}
		msg.Request = &req
	} else {
		// It's a notification
		var notif Notification
		if err := json.Unmarshal(data, &notif); err != nil {
			return nil, &Error{Code: ParseError, Message: "Parse error", Data: err.Error()}
		}
		msg.Notification = &notif
	}

	return msg, nil
}

// MarshalOutbound serializes an OutboundMessage to JSON.
func MarshalOutbound(msg *OutboundMessage) ([]byte, error) {
	if msg.Response != nil {
		return json.Marshal(msg.Response)
	}
	if msg.Notification != nil {
		return json.Marshal(msg.Notification)
	}
	return nil, errors.New("empty outbound message")
}

// Config holds common transport configuration.
type Config struct {
	// RecvBufferSize is the size of the receive channel buffer.
	// Default: 100
	RecvBufferSize int

	// SendBufferSize is the size of the internal send buffer.
	// Default: 100
	SendBufferSize int
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		RecvBufferSize: 100,
		SendBufferSize: 100,
	}
}
