package errors

import (
	"context"
	stderrors "errors"
)

// Wrap wraps an error with additional context.
// If err is nil, Wrap returns nil.
// If err is an *Error, it preserves the code, category, and metadata.
// Context errors (DeadlineExceeded, Canceled) are detected automatically.
// All other errors are wrapped as Internal.
func Wrap(err error, message string, opts ...Option) *Error {
	if err == nil {
		return nil
	}

	var agentErr *Error
	if stderrors.As(err, &agentErr) {
		wrapped := &Error{
			code:      agentErr.code,
			category:  agentErr.category,
			message:   message,
			cause:     err,
			metadata:  agentErr.Metadata(),
			retryable: agentErr.retryable,
		}
		for _, opt := range opts {
			opt(wrapped)
		}
		return wrapped
	}

	if stderrors.Is(err, context.DeadlineExceeded) {
		return New(Timeout, message, append(opts, WithCause(err))...)
	}
	if stderrors.Is(err, context.Canceled) {
		return New(Canceled, message, append(opts, WithCause(err))...)
	}

	return New(Internal, message, append(opts, WithCause(err))...)
}

// Has reports whether any error in the chain has the given code.
func Has(err error, code Code) bool {
	var agentErr *Error
	if stderrors.As(err, &agentErr) {
		return agentErr.code == code
	}
	return false
}

// IsRetryable reports whether the error is retryable.
func IsRetryable(err error) bool {
	var agentErr *Error
	if stderrors.As(err, &agentErr) {
		return agentErr.Retryable()
	}
	return false
}

// CodeOf extracts the error code from an error chain.
// Returns empty string if err does not contain an *Error.
func CodeOf(err error) Code {
	var agentErr *Error
	if stderrors.As(err, &agentErr) {
		return agentErr.code
	}
	return ""
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

// Collect gathers non-nil errors from the arguments.
func Collect(errs ...error) []error {
	var result []error
	for _, err := range errs {
		if err != nil {
			result = append(result, err)
		}
	}
	return result
}

// --- stdlib re-exports ---

// As finds the first error in err's chain that matches target,
// and if so, sets target to that error value and returns true.
func As(err error, target any) bool { return stderrors.As(err, target) }

// Is reports whether any error in err's chain matches target.
func Is(err, target error) bool { return stderrors.Is(err, target) }

// Unwrap returns the result of calling the Unwrap method on err,
// if err's type contains an Unwrap method returning error.
func Unwrap(err error) error { return stderrors.Unwrap(err) }

// Join returns an error that wraps the given errors.
// Any nil error values are discarded.
func Join(errs ...error) error { return stderrors.Join(errs...) }
