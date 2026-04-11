package errors

import (
	"encoding/json"
	"fmt"
)

// Error is a structured error carrying a code, category, and optional metadata.
type Error struct {
	code      Code
	category  Category
	message   string
	cause     error
	metadata  map[string]string
	retryable *bool // nil means use default based on category
}

var (
	_ json.Marshaler   = (*Error)(nil)
	_ json.Unmarshaler = (*Error)(nil)
)

// Error returns the error message, including the cause if present.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Code returns the error code.
func (e *Error) Code() Code { return e.code }

// Category returns the error category.
func (e *Error) Category() Category { return e.category }

// Retryable reports whether this error may succeed on retry.
func (e *Error) Retryable() bool {
	if e.retryable != nil {
		return *e.retryable
	}
	return e.category.IsRetryable()
}

// Metadata returns a copy of the error metadata.
func (e *Error) Metadata() map[string]string {
	if e.metadata == nil {
		return make(map[string]string)
	}
	result := make(map[string]string, len(e.metadata))
	for k, v := range e.metadata {
		result[k] = v
	}
	return result
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error { return e.cause }

// errorJSON is the JSON representation of an Error.
type errorJSON struct {
	Code      Code              `json:"code"`
	Category  Category          `json:"category"`
	Message   string            `json:"message"`
	Cause     string            `json:"cause,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Retryable bool              `json:"retryable"`
}

// MarshalJSON implements json.Marshaler.
func (e *Error) MarshalJSON() ([]byte, error) {
	j := errorJSON{
		Code:      e.code,
		Category:  e.category,
		Message:   e.message,
		Metadata:  e.metadata,
		Retryable: e.Retryable(),
	}
	if e.cause != nil {
		j.Cause = e.cause.Error()
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
	r := j.Retryable
	e.retryable = &r
	if j.Cause != "" {
		e.cause = fmt.Errorf("%s", j.Cause)
	}
	return nil
}

// Option configures an Error during construction.
type Option func(*Error)

// WithCategory overrides the default category.
func WithCategory(cat Category) Option {
	return func(e *Error) { e.category = cat }
}

// WithRetryable explicitly sets whether the error is retryable.
func WithRetryable(retryable bool) Option {
	return func(e *Error) { e.retryable = &retryable }
}

// WithMetadata adds a metadata key-value pair.
func WithMetadata(key, value string) Option {
	return func(e *Error) {
		if e.metadata == nil {
			e.metadata = make(map[string]string)
		}
		e.metadata[key] = value
	}
}

// WithCause sets the underlying cause.
func WithCause(cause error) Option {
	return func(e *Error) { e.cause = cause }
}

// New creates a new Error with the given code and message.
func New(code Code, message string, opts ...Option) *Error {
	e := &Error{
		code:     code,
		category: code.DefaultCategory(),
		message:  message,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Newf creates a new Error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return New(code, fmt.Sprintf(format, args...))
}

// From creates an error using the code's default description.
func From(code Code, opts ...Option) *Error {
	return New(code, code.Description(), opts...)
}

// RecoverPanic converts a recovered panic value into an Error.
func RecoverPanic(recovered any) *Error {
	if recovered == nil {
		return nil
	}
	var message string
	switch v := recovered.(type) {
	case error:
		message = v.Error()
	case string:
		message = v
	default:
		message = fmt.Sprintf("%v", v)
	}
	return New(Panic, message, WithMetadata("panic_value", fmt.Sprintf("%T", recovered)))
}
