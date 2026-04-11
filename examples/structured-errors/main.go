// Package main demonstrates the agentkit/errors package for structured error handling.
//
// This example shows how to create, wrap, check, and handle structured errors
// in agent-based systems, including retry logic based on error categories.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/vinayprograms/agentkit/errors"
)

func main() {
	fmt.Println("=== AgentKit Structured Errors Example ===")

	// 1. Creating errors with different categories
	demonstrateErrorCreation()

	// 2. Wrapping errors with context
	demonstrateErrorWrapping()

	// 3. Checking error categories
	demonstrateErrorChecking()

	// 4. Retry decisions based on error type
	demonstrateRetryLogic()

	// 5. Error formatting and JSON output
	demonstrateErrorFormatting()
}

// demonstrateErrorCreation shows how to create errors with different categories.
func demonstrateErrorCreation() {
	fmt.Println("--- 1. Creating Errors with Different Categories ---")

	// Transient errors (may succeed on retry)
	timeout := errors.New(errors.Timeout, "database connection timed out")
	fmt.Printf("Timeout error:\n  Message: %s\n  Code: %s\n  Category: %s\n  Retryable: %v\n\n",
		timeout.Error(), timeout.Code(), timeout.Category(), timeout.Retryable())

	// Permanent errors (won't succeed on retry)
	notFound := errors.New(errors.NotFound, "user ID 12345 does not exist")
	fmt.Printf("NotFound error:\n  Message: %s\n  Code: %s\n  Category: %s\n  Retryable: %v\n\n",
		notFound.Error(), notFound.Code(), notFound.Category(), notFound.Retryable())

	// Resource errors (rate limits, quotas)
	rateLimited := errors.New(errors.RateLimit, "API quota exceeded",
		errors.WithMetadata("limit", "100/min"),
		errors.WithMetadata("reset_at", time.Now().Add(time.Minute).Format(time.RFC3339)),
	)
	fmt.Printf("RateLimit error:\n  Message: %s\n  Code: %s\n  Category: %s\n  Retryable: %v\n  Metadata: %v\n\n",
		rateLimited.Error(), rateLimited.Code(), rateLimited.Category(), rateLimited.Retryable(), rateLimited.Metadata())

	// Agent-specific errors
	agentOffline := errors.New(errors.AgentOffline, "worker-agent-3 is offline",
		errors.WithMetadata("agent_id", "worker-agent-3"),
	)
	fmt.Printf("AgentOffline error:\n  Message: %s\n  Code: %s\n  Category: %s\n  Retryable: %v\n\n",
		agentOffline.Error(), agentOffline.Code(), agentOffline.Category(), agentOffline.Retryable())

	// Custom error with explicit category override
	customErr := errors.New(errors.Internal, "unexpected state detected",
		errors.WithCategory(errors.CategoryTransient), // Override: make it transient
		errors.WithRetryable(true),
		errors.WithMetadata("agent_id", "coordinator"),
		errors.WithMetadata("task_id", "task-abc-123"),
	)
	fmt.Printf("Custom error with overrides:\n  Message: %s\n  Code: %s\n  Category: %s\n  Retryable: %v\n\n",
		customErr.Error(), customErr.Code(), customErr.Category(), customErr.Retryable())
}

// demonstrateErrorWrapping shows how to wrap errors with additional context.
func demonstrateErrorWrapping() {
	fmt.Println("--- 2. Wrapping Errors with Context ---")

	// Simulate a chain of operations that fail
	baseErr := errors.New(errors.Timeout, "connection reset by peer")

	// Wrap with context at each layer
	serviceErr := errors.Wrap(baseErr, "failed to fetch user profile")
	handlerErr := errors.Wrap(serviceErr, "user lookup handler failed",
		errors.WithMetadata("agent_id", "api-gateway"),
	)

	fmt.Printf("Error chain:\n")
	fmt.Printf("  Handler: %s\n", handlerErr.Error())
	fmt.Printf("  Code preserved: %s\n", handlerErr.Code())
	fmt.Printf("  Category preserved: %s\n", handlerErr.Category())
	fmt.Printf("  Still retryable: %v\n\n", handlerErr.Retryable())

	// Wrap a standard library error
	_, stdErr := http.Get("http://invalid-host-that-does-not-exist.local")
	if stdErr != nil {
		wrapped := errors.Wrap(stdErr, "health check failed")
		fmt.Printf("Wrapped stdlib error:\n  Message: %s\n  Code: %s (defaults to INTERNAL)\n  Retryable: %v\n\n",
			wrapped.Error(), wrapped.Code(), wrapped.Retryable())
	}

	// Wrap with specific code using New + WithCause
	specificErr := errors.New(errors.Unavailable, "service unavailable", errors.WithCause(stdErr))
	fmt.Printf("Wrapped with specific code:\n  Code: %s\n  Category: %s\n  Retryable: %v\n\n",
		specificErr.Code(), specificErr.Category(), specificErr.Retryable())
}

// demonstrateErrorChecking shows how to check error categories with Has/As.
func demonstrateErrorChecking() {
	fmt.Println("--- 3. Checking Error Categories with Has/As ---")

	// Create a wrapped error chain
	inner := errors.New(errors.RateLimit, "too many requests")
	middle := errors.Wrap(inner, "agent request failed")
	outer := errors.Wrap(middle, "task coordination error")

	// Check specific code
	fmt.Printf("Has RATE_LIMITED: %v\n", errors.Has(outer, errors.RateLimit))
	fmt.Printf("Has TIMEOUT: %v\n", errors.Has(outer, errors.Timeout))

	// Check retryable
	fmt.Printf("Is Retryable: %v\n", errors.IsRetryable(outer))

	// Extract as *Error using stdlib As
	var agentErr *errors.Error
	if errors.As(outer, &agentErr) {
		fmt.Printf("\nExtracted *Error:\n")
		fmt.Printf("  Code: %s\n", agentErr.Code())
		fmt.Printf("  Category: %s\n", agentErr.Category())
		fmt.Printf("  Metadata: %v\n", agentErr.Metadata())
	}

	// Get code directly
	fmt.Printf("\nDirect extraction:\n")
	fmt.Printf("  Code: %s\n", errors.CodeOf(outer))
	fmt.Printf("  Retryable: %v\n\n", errors.IsRetryable(outer))
}

// demonstrateRetryLogic shows retry decisions based on error type.
func demonstrateRetryLogic() {
	fmt.Println("--- 4. Retry Decisions Based on Error Type ---")

	// Simulate different error scenarios
	testCases := []struct {
		name string
		err  error
	}{
		{"Network timeout", errors.New(errors.Timeout, "connection timeout")},
		{"Not found", errors.New(errors.NotFound, "resource missing")},
		{"Rate limited", errors.New(errors.RateLimit, "quota exceeded")},
		{"Auth failed", errors.New(errors.Unauthorized, "invalid token")},
		{"Agent offline", errors.New(errors.AgentOffline, "worker-1 is offline")},
		{"Coordination failure", errors.New(errors.Coordination, "consensus not reached")},
	}

	for _, tc := range testCases {
		decision := makeRetryDecision(tc.err)
		fmt.Printf("%-25s → %s\n", tc.name, decision)
	}

	fmt.Println()

	// Demonstrate retry loop pattern
	fmt.Println("Retry loop example:")
	simulateRetryLoop()
}

// makeRetryDecision returns a human-readable retry decision.
func makeRetryDecision(err error) string {
	if !errors.IsRetryable(err) {
		return "Don't retry (permanent)"
	}

	var e *errors.Error
	if errors.As(err, &e) {
		switch e.Category() {
		case errors.CategoryTransient:
			return "Retry immediately (transient)"
		case errors.CategoryResource:
			if resetAt, ok := e.Metadata()["reset_at"]; ok {
				return fmt.Sprintf("Retry after %s (rate limited)", resetAt)
			}
			return "Retry with backoff (resource)"
		}
	}

	return "Retry with caution"
}

// simulateRetryLoop demonstrates a typical retry pattern.
func simulateRetryLoop() {
	maxRetries := 3
	attempt := 0

	for {
		attempt++
		err := simulateUnreliableOperation(attempt)

		if err == nil {
			fmt.Printf("  Attempt %d: Success!\n", attempt)
			break
		}

		fmt.Printf("  Attempt %d: %s (code=%s)\n", attempt, err.Error(), errors.CodeOf(err))

		if !errors.IsRetryable(err) {
			fmt.Printf("  → Giving up: error is not retryable\n")
			break
		}

		if attempt >= maxRetries {
			fmt.Printf("  → Giving up: max retries reached\n")
			break
		}

		fmt.Printf("  → Will retry\n")
	}
	fmt.Println()
}

// simulateUnreliableOperation simulates an operation that fails transiently.
func simulateUnreliableOperation(attempt int) error {
	switch attempt {
	case 1:
		return errors.New(errors.Timeout, "operation timed out")
	case 2:
		return errors.New(errors.Unavailable, "service temporarily unavailable")
	default:
		return nil // Success on third attempt
	}
}

// demonstrateErrorFormatting shows error formatting and JSON serialization.
func demonstrateErrorFormatting() {
	fmt.Println("--- 5. Error Formatting and JSON Output ---")

	// Create a rich error with full metadata
	err := errors.New(errors.TaskFailed, "task xyz-789 failed: execution timeout",
		errors.WithMetadata("task_id", "task-xyz-789"),
		errors.WithMetadata("agent_id", "executor-2"),
		errors.WithMetadata("duration_ms", "30000"),
		errors.WithMetadata("stage", "data_processing"),
	)

	// Standard error output
	fmt.Printf("String format:\n  %s\n\n", err.Error())

	// Detailed output
	fmt.Printf("Detailed:\n")
	fmt.Printf("  Code: %s\n", err.Code())
	fmt.Printf("  Category: %s\n", err.Category())
	fmt.Printf("  Retryable: %v\n", err.Retryable())
	fmt.Printf("  Metadata: %v\n\n", err.Metadata())

	// JSON serialization (useful for cross-agent communication)
	jsonData, _ := json.MarshalIndent(err, "", "  ")
	fmt.Printf("JSON format:\n%s\n\n", string(jsonData))

	// Demonstrate deserialization
	var restored errors.Error
	if e := json.Unmarshal(jsonData, &restored); e != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal: %v\n", e)
		return
	}
	fmt.Printf("Restored from JSON:\n")
	fmt.Printf("  Code: %s\n", restored.Code())
	fmt.Printf("  Message: %s\n", restored.Error())
	fmt.Printf("  Retryable: %v\n", restored.Retryable())
}
