package errors

import (
	"encoding/json"
	"fmt"
	"time"
)

// AgentError is the interface for all structured errors in agentkit.
// It extends the standard error interface with additional context for
// swarm coordination and retry logic.
type AgentError interface {
	error

	// Code returns the specific error code identifying the failure type.
	Code() ErrorCode

	// Category returns the error category for retry/handling decisions.
	Category() ErrorCategory

	// Retryable returns true if the operation may succeed on retry.
	Retryable() bool

	// Metadata returns additional context as key-value pairs.
	Metadata() map[string]string

	// Unwrap returns the underlying error, if any.
	Unwrap() error
}

// Error is the concrete implementation of AgentError.
type Error struct {
	code      ErrorCode
	category  ErrorCategory
	message   string
	cause     error
	metadata  map[string]string
	retryable *bool // nil means use default based on category
	timestamp time.Time
	agentID   string // source agent, if applicable
	taskID    string // related task, if applicable
}

// Ensure Error implements AgentError and json.Marshaler/Unmarshaler.
var (
	_ AgentError       = (*Error)(nil)
	_ json.Marshaler   = (*Error)(nil)
	_ json.Unmarshaler = (*Error)(nil)
)

// Error returns the error message.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Code returns the error code.
func (e *Error) Code() ErrorCode {
	return e.code
}

// Category returns the error category.
func (e *Error) Category() ErrorCategory {
	return e.category
}

// Retryable returns whether this error is retryable.
func (e *Error) Retryable() bool {
	if e.retryable != nil {
		return *e.retryable
	}
	return e.category.IsRetryable()
}

// Metadata returns the error metadata.
func (e *Error) Metadata() map[string]string {
	if e.metadata == nil {
		return make(map[string]string)
	}
	// Return a copy to prevent modification
	result := make(map[string]string, len(e.metadata))
	for k, v := range e.metadata {
		result[k] = v
	}
	return result
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error {
	return e.cause
}

// Timestamp returns when the error occurred.
func (e *Error) Timestamp() time.Time {
	return e.timestamp
}

// AgentID returns the source agent ID, if set.
func (e *Error) AgentID() string {
	return e.agentID
}

// TaskID returns the related task ID, if set.
func (e *Error) TaskID() string {
	return e.taskID
}

// errorJSON is the JSON representation of an Error.
type errorJSON struct {
	Code      ErrorCode         `json:"code"`
	Category  ErrorCategory     `json:"category"`
	Message   string            `json:"message"`
	Cause     string            `json:"cause,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Retryable bool              `json:"retryable"`
	Timestamp string            `json:"timestamp,omitempty"`
	AgentID   string            `json:"agent_id,omitempty"`
	TaskID    string            `json:"task_id,omitempty"`
}

// MarshalJSON implements json.Marshaler.
func (e *Error) MarshalJSON() ([]byte, error) {
	j := errorJSON{
		Code:      e.code,
		Category:  e.category,
		Message:   e.message,
		Metadata:  e.metadata,
		Retryable: e.Retryable(),
		AgentID:   e.agentID,
		TaskID:    e.taskID,
	}
	if e.cause != nil {
		j.Cause = e.cause.Error()
	}
	if !e.timestamp.IsZero() {
		j.Timestamp = e.timestamp.Format(time.RFC3339Nano)
	}
	return json.Marshal(j)
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *Error) UnmarshalJSON(data []byte) error {
	var j errorJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	e.code = j.Code
	e.category = j.Category
	e.message = j.Message
	e.metadata = j.Metadata
	e.agentID = j.AgentID
	e.taskID = j.TaskID
	r := j.Retryable
	e.retryable = &r
	if j.Cause != "" {
		e.cause = fmt.Errorf("%s", j.Cause)
	}
	if j.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, j.Timestamp); err == nil {
			e.timestamp = t
		}
	}
	return nil
}

// Option is a functional option for configuring an Error.
type Option func(*Error)

// WithCategory overrides the default category.
func WithCategory(cat ErrorCategory) Option {
	return func(e *Error) {
		e.category = cat
	}
}

// WithRetryable explicitly sets whether the error is retryable.
func WithRetryable(retryable bool) Option {
	return func(e *Error) {
		e.retryable = &retryable
	}
}

// WithMetadata adds metadata key-value pairs.
func WithMetadata(key, value string) Option {
	return func(e *Error) {
		if e.metadata == nil {
			e.metadata = make(map[string]string)
		}
		e.metadata[key] = value
	}
}

// WithMetadataMap adds multiple metadata key-value pairs.
func WithMetadataMap(m map[string]string) Option {
	return func(e *Error) {
		if e.metadata == nil {
			e.metadata = make(map[string]string)
		}
		for k, v := range m {
			e.metadata[k] = v
		}
	}
}

// WithAgentID sets the source agent ID.
func WithAgentID(id string) Option {
	return func(e *Error) {
		e.agentID = id
	}
}

// WithTaskID sets the related task ID.
func WithTaskID(id string) Option {
	return func(e *Error) {
		e.taskID = id
	}
}

// WithTimestamp sets a custom timestamp.
func WithTimestamp(t time.Time) Option {
	return func(e *Error) {
		e.timestamp = t
	}
}

// WithCause sets the underlying cause.
func WithCause(cause error) Option {
	return func(e *Error) {
		e.cause = cause
	}
}

// New creates a new Error with the given code and message.
func New(code ErrorCode, message string, opts ...Option) *Error {
	e := &Error{
		code:      code,
		category:  code.DefaultCategory(),
		message:   message,
		timestamp: time.Now(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Newf creates a new Error with a formatted message.
func Newf(code ErrorCode, format string, args ...interface{}) *Error {
	return New(code, fmt.Sprintf(format, args...))
}

// FromCode creates an error with the default description for the code.
func FromCode(code ErrorCode, opts ...Option) *Error {
	return New(code, code.Description(), opts...)
}

// Timeout creates a timeout error.
func Timeout(message string, opts ...Option) *Error {
	return New(ErrCodeTimeout, message, opts...)
}

// NotFound creates a not found error.
func NotFound(message string, opts ...Option) *Error {
	return New(ErrCodeNotFound, message, opts...)
}

// RateLimited creates a rate limit error.
func RateLimited(message string, opts ...Option) *Error {
	return New(ErrCodeRateLimit, message, opts...)
}

// Unauthorized creates an unauthorized error.
func Unauthorized(message string, opts ...Option) *Error {
	return New(ErrCodeUnauthorized, message, opts...)
}

// Forbidden creates a forbidden error.
func Forbidden(message string, opts ...Option) *Error {
	return New(ErrCodeForbidden, message, opts...)
}

// InvalidInput creates an invalid input error.
func InvalidInput(message string, opts ...Option) *Error {
	return New(ErrCodeInvalidInput, message, opts...)
}

// Conflict creates a conflict error.
func Conflict(message string, opts ...Option) *Error {
	return New(ErrCodeConflict, message, opts...)
}

// Internal creates an internal error.
func Internal(message string, opts ...Option) *Error {
	return New(ErrCodeInternal, message, opts...)
}

// AgentOffline creates an agent offline error.
func AgentOffline(agentID string, opts ...Option) *Error {
	opts = append([]Option{WithAgentID(agentID)}, opts...)
	return New(ErrCodeAgentOffline, fmt.Sprintf("agent %s is offline", agentID), opts...)
}

// TaskFailed creates a task failed error.
func TaskFailed(taskID, reason string, opts ...Option) *Error {
	opts = append([]Option{WithTaskID(taskID)}, opts...)
	return New(ErrCodeTaskFailed, fmt.Sprintf("task %s failed: %s", taskID, reason), opts...)
}

// CoordinationFailure creates a coordination failure error.
func CoordinationFailure(message string, opts ...Option) *Error {
	return New(ErrCodeCoordination, message, opts...)
}
