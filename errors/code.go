package errors

// Code identifies a specific error type.
type Code string

// Codes for common failure scenarios.
const (
	// Transient — retry may succeed.
	Timeout     Code = "TIMEOUT"
	Unavailable Code = "UNAVAILABLE"
	NetworkErr  Code = "NETWORK_ERR"
	RetryLater  Code = "RETRY_LATER"

	// Permanent — retry will not help.
	NotFound          Code = "NOT_FOUND"
	Conflict          Code = "CONFLICT"
	InvalidInput      Code = "INVALID_INPUT"
	Unauthorized      Code = "UNAUTHORIZED"
	Forbidden         Code = "FORBIDDEN"
	AlreadyExists     Code = "ALREADY_EXISTS"
	Precondition      Code = "PRECONDITION"
	Unsupported       Code = "UNSUPPORTED"
	Canceled          Code = "CANCELED"
	TaskFailed        Code = "TASK_FAILED"
	CapabilityMissing Code = "CAPABILITY_MISSING"

	// Resource — exhaustion or quota issues.
	RateLimit     Code = "RATE_LIMITED"
	QuotaExceeded Code = "QUOTA_EXCEEDED"
	ResourceBusy  Code = "RESOURCE_BUSY"
	Capacity      Code = "CAPACITY"

	// Internal — bugs or system failures.
	Internal   Code = "INTERNAL"
	Corruption Code = "CORRUPTION"
	Assertion  Code = "ASSERTION"
	Panic      Code = "PANIC"

	// Agent-specific.
	AgentOffline  Code = "AGENT_OFFLINE"
	AgentBusy     Code = "AGENT_BUSY"
	Coordination  Code = "COORDINATION"
	HandoffFailed Code = "HANDOFF_FAILED"
)

// DefaultCategory returns the default category for this code.
func (c Code) DefaultCategory() Category {
	switch c {
	case Timeout, Unavailable, NetworkErr, RetryLater,
		AgentOffline, AgentBusy, Coordination, HandoffFailed:
		return CategoryTransient

	case NotFound, Conflict, InvalidInput, Unauthorized,
		Forbidden, AlreadyExists, Precondition, Unsupported,
		Canceled, TaskFailed, CapabilityMissing:
		return CategoryPermanent

	case RateLimit, QuotaExceeded, ResourceBusy, Capacity:
		return CategoryResource

	case Internal, Corruption, Assertion, Panic:
		return CategoryInternal

	default:
		return CategoryInternal
	}
}

// descriptions maps codes to human-readable messages.
var descriptions = map[Code]string{
	Timeout:           "operation timed out",
	Unavailable:       "service temporarily unavailable",
	NetworkErr:        "network connectivity error",
	RetryLater:        "server requested retry later",
	NotFound:          "resource not found",
	Conflict:          "conflicting operation",
	InvalidInput:      "invalid input provided",
	Unauthorized:      "authentication required",
	Forbidden:         "access denied",
	AlreadyExists:     "resource already exists",
	Precondition:      "precondition failed",
	Unsupported:       "operation not supported",
	Canceled:          "operation canceled",
	RateLimit:         "rate limit exceeded",
	QuotaExceeded:     "quota exceeded",
	ResourceBusy:      "resource is busy",
	Capacity:          "system at capacity",
	Internal:          "internal error",
	Corruption:        "data corruption detected",
	Assertion:         "assertion failed",
	Panic:             "recovered from panic",
	AgentOffline:      "agent is offline",
	AgentBusy:         "agent is busy",
	TaskFailed:        "task execution failed",
	Coordination:      "coordination failure",
	HandoffFailed:     "handoff failed",
	CapabilityMissing: "required capability missing",
}

// Description returns a human-readable description for the code.
func (c Code) Description() string {
	if desc, ok := descriptions[c]; ok {
		return desc
	}
	return "unknown error"
}

// Category classifies errors by their nature and retry semantics.
type Category string

const (
	// CategoryTransient indicates temporary failures where retry may succeed.
	CategoryTransient Category = "transient"

	// CategoryPermanent indicates failures where retry will not help.
	CategoryPermanent Category = "permanent"

	// CategoryResource indicates resource exhaustion or quota issues.
	CategoryResource Category = "resource"

	// CategoryInternal indicates unexpected errors, bugs, or system failures.
	CategoryInternal Category = "internal"
)

// IsRetryable reports whether errors in this category may succeed on retry.
func (c Category) IsRetryable() bool {
	return c == CategoryTransient || c == CategoryResource
}
