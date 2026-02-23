// Package errors provides a structured error taxonomy for swarm coordination
// in agentkit. It defines a comprehensive set of error types, codes, and
// categories that enable consistent error handling across distributed agent
// systems.
//
// # Error Categories
//
// Errors are classified into four categories:
//
//   - Transient: Temporary failures where retry may succeed (network issues, etc.)
//   - Permanent: Failures where retry will not help (invalid input, not found, etc.)
//   - Resource: Resource exhaustion issues (rate limits, quotas, etc.)
//   - Internal: Unexpected errors indicating bugs or system failures
//
// # Error Codes
//
// Each error has a specific code that identifies the type of failure:
//
//   - TIMEOUT: Operation timed out
//   - RATE_LIMITED: Rate limit exceeded
//   - NOT_FOUND: Resource not found
//   - CONFLICT: Conflicting operation
//   - UNAUTHORIZED: Authentication/authorization failure
//   - And more...
//
// # Usage
//
// Create a new error:
//
//	err := errors.New(errors.ErrCodeTimeout, "operation timed out")
//
// Wrap an existing error with context:
//
//	wrapped := errors.Wrap(err, "fetching agent state")
//
// Check if an error is retryable:
//
//	if agentErr, ok := errors.AsAgentError(err); ok && agentErr.Retryable() {
//	    // retry logic
//	}
//
// # JSON Serialization
//
// All errors support JSON serialization for cross-agent communication:
//
//	data, err := json.Marshal(agentErr)
//
// Errors can be deserialized back:
//
//	var agentErr errors.Error
//	json.Unmarshal(data, &agentErr)
package errors
