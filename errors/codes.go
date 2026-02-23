package errors

// ErrorCategory classifies errors by their nature and retry semantics.
type ErrorCategory string

// Error categories define how errors should be handled.
const (
	// CategoryTransient indicates temporary failures where retry may succeed.
	// Examples: network timeouts, temporary service unavailability.
	CategoryTransient ErrorCategory = "transient"

	// CategoryPermanent indicates failures where retry will not help.
	// Examples: invalid input, resource not found, permission denied.
	CategoryPermanent ErrorCategory = "permanent"

	// CategoryResource indicates resource exhaustion or quota issues.
	// Examples: rate limiting, storage quota exceeded, connection pool exhausted.
	CategoryResource ErrorCategory = "resource"

	// CategoryInternal indicates unexpected errors, bugs, or system failures.
	// Examples: nil pointer, assertion failures, corrupted state.
	CategoryInternal ErrorCategory = "internal"
)

// String returns the string representation of the category.
func (c ErrorCategory) String() string {
	return string(c)
}

// IsRetryable returns true if errors in this category may succeed on retry.
func (c ErrorCategory) IsRetryable() bool {
	switch c {
	case CategoryTransient, CategoryResource:
		return true
	default:
		return false
	}
}

// ErrorCode identifies specific error types within categories.
type ErrorCode string

// Error codes for common failure scenarios.
const (
	// Transient errors
	ErrCodeTimeout     ErrorCode = "TIMEOUT"      // Operation timed out
	ErrCodeUnavailable ErrorCode = "UNAVAILABLE"  // Service temporarily unavailable
	ErrCodeNetworkErr  ErrorCode = "NETWORK_ERR"  // Network connectivity issue
	ErrCodeRetryLater  ErrorCode = "RETRY_LATER"  // Server requested retry

	// Permanent errors
	ErrCodeNotFound       ErrorCode = "NOT_FOUND"       // Resource does not exist
	ErrCodeConflict       ErrorCode = "CONFLICT"        // Conflicting operation or state
	ErrCodeInvalidInput   ErrorCode = "INVALID_INPUT"   // Malformed or invalid input
	ErrCodeUnauthorized   ErrorCode = "UNAUTHORIZED"    // Authentication failed
	ErrCodeForbidden      ErrorCode = "FORBIDDEN"       // Authorization denied
	ErrCodeAlreadyExists  ErrorCode = "ALREADY_EXISTS"  // Resource already exists
	ErrCodePrecondition   ErrorCode = "PRECONDITION"    // Precondition not met
	ErrCodeUnsupported    ErrorCode = "UNSUPPORTED"     // Operation not supported
	ErrCodeCanceled       ErrorCode = "CANCELED"        // Operation was canceled

	// Resource errors
	ErrCodeRateLimit     ErrorCode = "RATE_LIMITED"     // Rate limit exceeded
	ErrCodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED"   // Resource quota exhausted
	ErrCodeResourceBusy  ErrorCode = "RESOURCE_BUSY"    // Resource is busy/locked
	ErrCodeCapacity      ErrorCode = "CAPACITY"         // System at capacity

	// Internal errors
	ErrCodeInternal   ErrorCode = "INTERNAL"    // Unexpected internal error
	ErrCodeCorruption ErrorCode = "CORRUPTION"  // Data corruption detected
	ErrCodeAssertion  ErrorCode = "ASSERTION"   // Assertion/invariant violation
	ErrCodePanic      ErrorCode = "PANIC"       // Recovered from panic

	// Agent-specific errors
	ErrCodeAgentOffline     ErrorCode = "AGENT_OFFLINE"      // Target agent is offline
	ErrCodeAgentBusy        ErrorCode = "AGENT_BUSY"         // Agent is processing another task
	ErrCodeTaskFailed       ErrorCode = "TASK_FAILED"        // Task execution failed
	ErrCodeCoordination     ErrorCode = "COORDINATION"       // Swarm coordination failure
	ErrCodeHandoffFailed    ErrorCode = "HANDOFF_FAILED"     // Agent handoff failed
	ErrCodeCapabilityMissing ErrorCode = "CAPABILITY_MISSING" // Required capability not available
)

// String returns the string representation of the error code.
func (c ErrorCode) String() string {
	return string(c)
}

// DefaultCategory returns the default category for an error code.
func (c ErrorCode) DefaultCategory() ErrorCategory {
	switch c {
	// Transient
	case ErrCodeTimeout, ErrCodeUnavailable, ErrCodeNetworkErr, ErrCodeRetryLater:
		return CategoryTransient

	// Permanent
	case ErrCodeNotFound, ErrCodeConflict, ErrCodeInvalidInput, ErrCodeUnauthorized,
		ErrCodeForbidden, ErrCodeAlreadyExists, ErrCodePrecondition, ErrCodeUnsupported,
		ErrCodeCanceled:
		return CategoryPermanent

	// Resource
	case ErrCodeRateLimit, ErrCodeQuotaExceeded, ErrCodeResourceBusy, ErrCodeCapacity:
		return CategoryResource

	// Internal
	case ErrCodeInternal, ErrCodeCorruption, ErrCodeAssertion, ErrCodePanic:
		return CategoryInternal

	// Agent-specific (varies)
	case ErrCodeAgentOffline, ErrCodeAgentBusy, ErrCodeCoordination, ErrCodeHandoffFailed:
		return CategoryTransient
	case ErrCodeTaskFailed, ErrCodeCapabilityMissing:
		return CategoryPermanent

	default:
		return CategoryInternal
	}
}

// DefaultRetryable returns whether this error code is typically retryable.
func (c ErrorCode) DefaultRetryable() bool {
	return c.DefaultCategory().IsRetryable()
}

// codeDescriptions provides human-readable descriptions for error codes.
var codeDescriptions = map[ErrorCode]string{
	ErrCodeTimeout:          "operation timed out",
	ErrCodeUnavailable:      "service temporarily unavailable",
	ErrCodeNetworkErr:       "network connectivity error",
	ErrCodeRetryLater:       "server requested retry later",
	ErrCodeNotFound:         "resource not found",
	ErrCodeConflict:         "conflicting operation",
	ErrCodeInvalidInput:     "invalid input provided",
	ErrCodeUnauthorized:     "authentication required",
	ErrCodeForbidden:        "access denied",
	ErrCodeAlreadyExists:    "resource already exists",
	ErrCodePrecondition:     "precondition failed",
	ErrCodeUnsupported:      "operation not supported",
	ErrCodeCanceled:         "operation canceled",
	ErrCodeRateLimit:        "rate limit exceeded",
	ErrCodeQuotaExceeded:    "quota exceeded",
	ErrCodeResourceBusy:     "resource is busy",
	ErrCodeCapacity:         "system at capacity",
	ErrCodeInternal:         "internal error",
	ErrCodeCorruption:       "data corruption detected",
	ErrCodeAssertion:        "assertion failed",
	ErrCodePanic:            "recovered from panic",
	ErrCodeAgentOffline:     "agent is offline",
	ErrCodeAgentBusy:        "agent is busy",
	ErrCodeTaskFailed:       "task execution failed",
	ErrCodeCoordination:     "coordination failure",
	ErrCodeHandoffFailed:    "handoff failed",
	ErrCodeCapabilityMissing: "required capability missing",
}

// Description returns a human-readable description for the error code.
func (c ErrorCode) Description() string {
	if desc, ok := codeDescriptions[c]; ok {
		return desc
	}
	return "unknown error"
}
