package errors

import (
	"context"
	"errors"
	"fmt"
)

// Wrap wraps an error with additional context while preserving the error chain.
// If err is nil, Wrap returns nil.
// If err is already an AgentError, it wraps it with the new message.
// Otherwise, it creates a new Internal error wrapping the original.
func Wrap(err error, message string, opts ...Option) *Error {
	if err == nil {
		return nil
	}

	// If it's already an AgentError, preserve its properties
	var agentErr *Error
	if errors.As(err, &agentErr) {
		wrapped := &Error{
			code:      agentErr.code,
			category:  agentErr.category,
			message:   message,
			cause:     err,
			metadata:  agentErr.Metadata(),
			retryable: agentErr.retryable,
			agentID:   agentErr.agentID,
			taskID:    agentErr.taskID,
		}
		for _, opt := range opts {
			opt(wrapped)
		}
		return wrapped
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return New(ErrCodeTimeout, message, append(opts, WithCause(err))...)
	}
	if errors.Is(err, context.Canceled) {
		return New(ErrCodeCanceled, message, append(opts, WithCause(err))...)
	}

	// Default to internal error for unknown errors
	return New(ErrCodeInternal, message, append(opts, WithCause(err))...)
}

// Wrapf wraps an error with a formatted message.
func Wrapf(err error, format string, args ...interface{}) *Error {
	return Wrap(err, fmt.Sprintf(format, args...))
}

// WrapWithCode wraps an error with a specific error code.
func WrapWithCode(err error, code ErrorCode, message string, opts ...Option) *Error {
	if err == nil {
		return nil
	}
	opts = append(opts, WithCause(err))
	return New(code, message, opts...)
}

// As attempts to extract an AgentError from an error chain.
// Returns nil if no AgentError is found.
func AsAgentError(err error) AgentError {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr
	}
	return nil
}

// Is checks if any error in the chain has the given error code.
func Is(err error, code ErrorCode) bool {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr.code == code
	}
	return false
}

// IsCategory checks if any error in the chain has the given category.
func IsCategory(err error, category ErrorCategory) bool {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr.category == category
	}
	return false
}

// IsRetryable checks if the error is retryable.
func IsRetryable(err error) bool {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr.Retryable()
	}
	// Default to not retryable for non-AgentErrors
	return false
}

// IsTransient checks if the error is transient.
func IsTransient(err error) bool {
	return IsCategory(err, CategoryTransient)
}

// IsPermanent checks if the error is permanent.
func IsPermanent(err error) bool {
	return IsCategory(err, CategoryPermanent)
}

// IsResource checks if the error is resource-related.
func IsResource(err error) bool {
	return IsCategory(err, CategoryResource)
}

// IsInternal checks if the error is an internal error.
func IsInternal(err error) bool {
	return IsCategory(err, CategoryInternal)
}

// Code extracts the error code from an error, if available.
// Returns empty string if err is not an AgentError.
func Code(err error) ErrorCode {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr.code
	}
	return ""
}

// Category extracts the error category from an error, if available.
// Returns empty string if err is not an AgentError.
func Category(err error) ErrorCategory {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr.category
	}
	return ""
}

// GetMetadata extracts metadata from an error.
// Returns nil if err is not an AgentError.
func GetMetadata(err error) map[string]string {
	var agentErr *Error
	if errors.As(err, &agentErr) {
		return agentErr.Metadata()
	}
	return nil
}

// Cause returns the root cause of the error chain.
func Cause(err error) error {
	for {
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		inner := unwrapper.Unwrap()
		if inner == nil {
			return err
		}
		err = inner
	}
}

// Join combines multiple errors into a single error.
// If all errors are nil, returns nil.
// Uses errors.Join from the standard library.
func Join(errs ...error) error {
	return errors.Join(errs...)
}

// Collect gathers multiple errors into a slice, filtering nils.
func Collect(errs ...error) []error {
	var result []error
	for _, err := range errs {
		if err != nil {
			result = append(result, err)
		}
	}
	return result
}

// FirstRetryable returns the first retryable error from a slice.
// Returns nil if no retryable error is found.
func FirstRetryable(errs []error) error {
	for _, err := range errs {
		if IsRetryable(err) {
			return err
		}
	}
	return nil
}

// AllRetryable checks if all errors in the slice are retryable.
// Returns true for empty slice.
func AllRetryable(errs []error) bool {
	for _, err := range errs {
		if !IsRetryable(err) {
			return false
		}
	}
	return true
}

// AnyRetryable checks if any error in the slice is retryable.
// Returns false for empty slice.
func AnyRetryable(errs []error) bool {
	for _, err := range errs {
		if IsRetryable(err) {
			return true
		}
	}
	return false
}

// RecoverPanic converts a recovered panic value into an Error.
func RecoverPanic(recovered interface{}) *Error {
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
	return New(ErrCodePanic, message, WithMetadata("panic_value", fmt.Sprintf("%T", recovered)))
}
