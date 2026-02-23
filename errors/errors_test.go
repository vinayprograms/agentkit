package errors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 1. Error creation with different codes/categories
// ============================================================================

func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		code         ErrorCode
		message      string
		wantCategory ErrorCategory
	}{
		{"timeout", ErrCodeTimeout, "operation timed out", CategoryTransient},
		{"not_found", ErrCodeNotFound, "resource not found", CategoryPermanent},
		{"rate_limit", ErrCodeRateLimit, "too many requests", CategoryResource},
		{"internal", ErrCodeInternal, "internal error", CategoryInternal},
		{"agent_offline", ErrCodeAgentOffline, "agent down", CategoryTransient},
		{"task_failed", ErrCodeTaskFailed, "task failed", CategoryPermanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, tt.message)
			if err.Code() != tt.code {
				t.Errorf("Code() = %v, want %v", err.Code(), tt.code)
			}
			if err.Category() != tt.wantCategory {
				t.Errorf("Category() = %v, want %v", err.Category(), tt.wantCategory)
			}
			if err.Error() != tt.message {
				t.Errorf("Error() = %v, want %v", err.Error(), tt.message)
			}
			if err.Timestamp().IsZero() {
				t.Error("Timestamp() should not be zero")
			}
		})
	}
}

func TestNewf(t *testing.T) {
	err := Newf(ErrCodeNotFound, "user %s not found", "alice")
	want := "user alice not found"
	if err.Error() != want {
		t.Errorf("Error() = %v, want %v", err.Error(), want)
	}
}

func TestFromCode(t *testing.T) {
	err := FromCode(ErrCodeTimeout)
	if err.Code() != ErrCodeTimeout {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeTimeout)
	}
	// Should use the default description
	if err.Error() != "operation timed out" {
		t.Errorf("Error() = %v, want %v", err.Error(), "operation timed out")
	}
}

func TestFromCodeWithOptions(t *testing.T) {
	err := FromCode(ErrCodeNotFound, WithMetadata("resource", "user"))
	if err.Metadata()["resource"] != "user" {
		t.Error("expected metadata 'resource' to be 'user'")
	}
}

// ============================================================================
// 2. Retryable vs non-retryable errors
// ============================================================================

func TestRetryable(t *testing.T) {
	tests := []struct {
		name      string
		code      ErrorCode
		wantRetry bool
	}{
		{"timeout is retryable", ErrCodeTimeout, true},
		{"unavailable is retryable", ErrCodeUnavailable, true},
		{"network_err is retryable", ErrCodeNetworkErr, true},
		{"rate_limit is retryable", ErrCodeRateLimit, true},
		{"not_found is not retryable", ErrCodeNotFound, false},
		{"invalid_input is not retryable", ErrCodeInvalidInput, false},
		{"internal is not retryable", ErrCodeInternal, false},
		{"forbidden is not retryable", ErrCodeForbidden, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "test")
			if err.Retryable() != tt.wantRetry {
				t.Errorf("Retryable() = %v, want %v", err.Retryable(), tt.wantRetry)
			}
		})
	}
}

func TestWithRetryableOverride(t *testing.T) {
	// Override a normally retryable error to be non-retryable
	err := New(ErrCodeTimeout, "permanent timeout", WithRetryable(false))
	if err.Retryable() {
		t.Error("expected error to be non-retryable after override")
	}

	// Override a normally non-retryable error to be retryable
	err2 := New(ErrCodeNotFound, "maybe retry", WithRetryable(true))
	if !err2.Retryable() {
		t.Error("expected error to be retryable after override")
	}
}

func TestErrorCategoryIsRetryable(t *testing.T) {
	tests := []struct {
		category  ErrorCategory
		retryable bool
	}{
		{CategoryTransient, true},
		{CategoryResource, true},
		{CategoryPermanent, false},
		{CategoryInternal, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if tt.category.IsRetryable() != tt.retryable {
				t.Errorf("%s.IsRetryable() = %v, want %v", tt.category, tt.category.IsRetryable(), tt.retryable)
			}
		})
	}
}

// ============================================================================
// 3. Metadata handling
// ============================================================================

func TestMetadata(t *testing.T) {
	err := New(ErrCodeInternal, "test",
		WithMetadata("key1", "value1"),
		WithMetadata("key2", "value2"),
	)

	meta := err.Metadata()
	if meta["key1"] != "value1" || meta["key2"] != "value2" {
		t.Errorf("Metadata() = %v, want key1=value1, key2=value2", meta)
	}
}

func TestWithMetadataMap(t *testing.T) {
	m := map[string]string{"a": "1", "b": "2"}
	err := New(ErrCodeInternal, "test", WithMetadataMap(m))

	meta := err.Metadata()
	if meta["a"] != "1" || meta["b"] != "2" {
		t.Errorf("Metadata() = %v, want a=1, b=2", meta)
	}
}

func TestMetadataImmutability(t *testing.T) {
	err := New(ErrCodeInternal, "test", WithMetadata("original", "value"))

	meta := err.Metadata()
	meta["injected"] = "evil"

	// Original should not be modified
	if err.Metadata()["injected"] != "" {
		t.Error("Metadata() should return a copy, not the original map")
	}
}

func TestNilMetadata(t *testing.T) {
	err := New(ErrCodeInternal, "test")
	meta := err.Metadata()
	if meta == nil {
		t.Error("Metadata() should return empty map, not nil")
	}
	if len(meta) != 0 {
		t.Errorf("Metadata() should be empty, got %v", meta)
	}
}

// ============================================================================
// 4. Error wrapping and unwrapping
// ============================================================================

func TestWrap(t *testing.T) {
	cause := fmt.Errorf("original error")
	err := Wrap(cause, "wrapped message")

	if err.Error() != "wrapped message: original error" {
		t.Errorf("Error() = %v, want 'wrapped message: original error'", err.Error())
	}
	if err.Unwrap() != cause {
		t.Error("Unwrap() should return original error")
	}
	// Should default to internal for unknown errors
	if err.Code() != ErrCodeInternal {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeInternal)
	}
}

func TestWrapNil(t *testing.T) {
	err := Wrap(nil, "message")
	if err != nil {
		t.Error("Wrap(nil, ...) should return nil")
	}
}

func TestWrapAgentError(t *testing.T) {
	original := New(ErrCodeNotFound, "resource missing",
		WithMetadata("id", "123"),
		WithAgentID("agent-1"),
		WithTaskID("task-1"),
	)
	wrapped := Wrap(original, "operation failed")

	// Should preserve properties
	if wrapped.Code() != ErrCodeNotFound {
		t.Errorf("wrapped.Code() = %v, want %v", wrapped.Code(), ErrCodeNotFound)
	}
	if wrapped.Metadata()["id"] != "123" {
		t.Error("wrapped error should preserve metadata")
	}
	if wrapped.AgentID() != "agent-1" {
		t.Error("wrapped error should preserve agent ID")
	}
	if wrapped.TaskID() != "task-1" {
		t.Error("wrapped error should preserve task ID")
	}
	if !errors.Is(wrapped, original) {
		t.Error("wrapped error should be 'Is' original")
	}
}

func TestWrapf(t *testing.T) {
	cause := fmt.Errorf("db error")
	err := Wrapf(cause, "failed to fetch user %s", "bob")

	if err.Error() != "failed to fetch user bob: db error" {
		t.Errorf("Error() = %v", err.Error())
	}
}

func TestWrapWithCode(t *testing.T) {
	cause := fmt.Errorf("network issue")
	err := WrapWithCode(cause, ErrCodeUnavailable, "service down")

	if err.Code() != ErrCodeUnavailable {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeUnavailable)
	}
}

func TestWrapWithCodeNil(t *testing.T) {
	err := WrapWithCode(nil, ErrCodeInternal, "message")
	if err != nil {
		t.Error("WrapWithCode(nil, ...) should return nil")
	}
}

func TestWithCause(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := New(ErrCodeInternal, "wrapper", WithCause(cause))

	if err.Unwrap() != cause {
		t.Error("Unwrap() should return cause set via WithCause")
	}
}

// ============================================================================
// 5. JSON serialization/deserialization roundtrip
// ============================================================================

func TestJSONRoundtrip(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	original := New(ErrCodeNotFound, "user not found",
		WithMetadata("user_id", "123"),
		WithAgentID("agent-1"),
		WithTaskID("task-42"),
		WithTimestamp(ts),
		WithRetryable(false),
	)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored Error
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Code() != original.Code() {
		t.Errorf("Code mismatch: %v vs %v", restored.Code(), original.Code())
	}
	if restored.Category() != original.Category() {
		t.Errorf("Category mismatch: %v vs %v", restored.Category(), original.Category())
	}
	if restored.Error() != original.message { // note: no cause after roundtrip
		t.Errorf("Message mismatch: %v vs %v", restored.Error(), original.message)
	}
	if restored.AgentID() != original.AgentID() {
		t.Errorf("AgentID mismatch: %v vs %v", restored.AgentID(), original.AgentID())
	}
	if restored.TaskID() != original.TaskID() {
		t.Errorf("TaskID mismatch: %v vs %v", restored.TaskID(), original.TaskID())
	}
	if restored.Retryable() != original.Retryable() {
		t.Errorf("Retryable mismatch: %v vs %v", restored.Retryable(), original.Retryable())
	}
	if restored.Metadata()["user_id"] != "123" {
		t.Error("Metadata not preserved")
	}
	if !restored.Timestamp().Equal(ts) {
		t.Errorf("Timestamp mismatch: %v vs %v", restored.Timestamp(), ts)
	}
}

func TestJSONWithCause(t *testing.T) {
	cause := fmt.Errorf("underlying issue")
	err := New(ErrCodeInternal, "wrapper", WithCause(cause))

	data, _ := json.Marshal(err)

	var j map[string]interface{}
	json.Unmarshal(data, &j)

	if j["cause"] != "underlying issue" {
		t.Errorf("cause should be serialized: %v", j["cause"])
	}
}

func TestJSONUnmarshalWithCause(t *testing.T) {
	jsonStr := `{"code":"INTERNAL","category":"internal","message":"test","cause":"original error"}`

	var err Error
	if e := json.Unmarshal([]byte(jsonStr), &err); e != nil {
		t.Fatalf("Unmarshal failed: %v", e)
	}

	if err.Unwrap() == nil {
		t.Error("Unwrap() should return reconstructed cause")
	}
	if err.Unwrap().Error() != "original error" {
		t.Errorf("Unwrap().Error() = %v, want 'original error'", err.Unwrap().Error())
	}
}

func TestJSONWithoutTimestamp(t *testing.T) {
	jsonStr := `{"code":"NOT_FOUND","category":"permanent","message":"test"}`

	var err Error
	if e := json.Unmarshal([]byte(jsonStr), &err); e != nil {
		t.Fatalf("Unmarshal failed: %v", e)
	}

	if !err.Timestamp().IsZero() {
		t.Error("Timestamp should be zero when not in JSON")
	}
}

// ============================================================================
// 6. Inspection helpers (Is, IsCategory, IsRetryable, etc.)
// ============================================================================

func TestIs(t *testing.T) {
	err := New(ErrCodeNotFound, "not found")

	if !Is(err, ErrCodeNotFound) {
		t.Error("Is() should return true for matching code")
	}
	if Is(err, ErrCodeTimeout) {
		t.Error("Is() should return false for non-matching code")
	}
}

func TestIsWithWrappedError(t *testing.T) {
	original := New(ErrCodeNotFound, "not found")
	wrapped := fmt.Errorf("context: %w", original)

	if !Is(wrapped, ErrCodeNotFound) {
		t.Error("Is() should find code in wrapped error")
	}
}

func TestIsWithNonAgentError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if Is(err, ErrCodeInternal) {
		t.Error("Is() should return false for non-AgentError")
	}
}

func TestIsCategory(t *testing.T) {
	err := New(ErrCodeTimeout, "timeout")

	if !IsCategory(err, CategoryTransient) {
		t.Error("IsCategory() should match")
	}
	if IsCategory(err, CategoryPermanent) {
		t.Error("IsCategory() should not match wrong category")
	}
}

func TestIsCategoryNonAgentError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if IsCategory(err, CategoryInternal) {
		t.Error("IsCategory() should return false for non-AgentError")
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := New(ErrCodeTimeout, "timeout")
	nonRetryable := New(ErrCodeNotFound, "not found")

	if !IsRetryable(retryable) {
		t.Error("IsRetryable() should return true for retryable error")
	}
	if IsRetryable(nonRetryable) {
		t.Error("IsRetryable() should return false for non-retryable error")
	}
}

func TestIsRetryableNonAgentError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if IsRetryable(err) {
		t.Error("IsRetryable() should return false for non-AgentError")
	}
}

func TestIsTransient(t *testing.T) {
	if !IsTransient(New(ErrCodeTimeout, "timeout")) {
		t.Error("IsTransient() should return true")
	}
	if IsTransient(New(ErrCodeNotFound, "not found")) {
		t.Error("IsTransient() should return false")
	}
}

func TestIsPermanent(t *testing.T) {
	if !IsPermanent(New(ErrCodeNotFound, "not found")) {
		t.Error("IsPermanent() should return true")
	}
	if IsPermanent(New(ErrCodeTimeout, "timeout")) {
		t.Error("IsPermanent() should return false")
	}
}

func TestIsResource(t *testing.T) {
	if !IsResource(New(ErrCodeRateLimit, "rate limited")) {
		t.Error("IsResource() should return true")
	}
	if IsResource(New(ErrCodeNotFound, "not found")) {
		t.Error("IsResource() should return false")
	}
}

func TestIsInternal(t *testing.T) {
	if !IsInternal(New(ErrCodeInternal, "internal")) {
		t.Error("IsInternal() should return true")
	}
	if IsInternal(New(ErrCodeNotFound, "not found")) {
		t.Error("IsInternal() should return false")
	}
}

func TestCode(t *testing.T) {
	err := New(ErrCodeTimeout, "timeout")
	if Code(err) != ErrCodeTimeout {
		t.Errorf("Code() = %v, want %v", Code(err), ErrCodeTimeout)
	}
}

func TestCodeNonAgentError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if Code(err) != "" {
		t.Errorf("Code() should return empty string for non-AgentError")
	}
}

func TestCategoryExtract(t *testing.T) {
	err := New(ErrCodeTimeout, "timeout")
	if Category(err) != CategoryTransient {
		t.Errorf("Category() = %v, want %v", Category(err), CategoryTransient)
	}
}

func TestCategoryExtractNonAgentError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if Category(err) != "" {
		t.Errorf("Category() should return empty string for non-AgentError")
	}
}

func TestGetMetadata(t *testing.T) {
	err := New(ErrCodeInternal, "test", WithMetadata("key", "value"))
	meta := GetMetadata(err)
	if meta["key"] != "value" {
		t.Error("GetMetadata() should return metadata")
	}
}

func TestGetMetadataNonAgentError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if GetMetadata(err) != nil {
		t.Error("GetMetadata() should return nil for non-AgentError")
	}
}

func TestAsAgentError(t *testing.T) {
	agentErr := New(ErrCodeTimeout, "timeout")
	wrapped := fmt.Errorf("wrapped: %w", agentErr)

	extracted := AsAgentError(wrapped)
	if extracted == nil {
		t.Error("AsAgentError() should extract AgentError from wrapped")
	}
	if extracted.Code() != ErrCodeTimeout {
		t.Errorf("extracted.Code() = %v, want %v", extracted.Code(), ErrCodeTimeout)
	}
}

func TestAsAgentErrorNonAgent(t *testing.T) {
	err := fmt.Errorf("regular error")
	if AsAgentError(err) != nil {
		t.Error("AsAgentError() should return nil for non-AgentError")
	}
}

// ============================================================================
// 7. Convenience constructors (Timeout, NotFound, RateLimited, etc.)
// ============================================================================

func TestTimeout(t *testing.T) {
	err := Timeout("operation timed out")
	if err.Code() != ErrCodeTimeout {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeTimeout)
	}
	if err.Category() != CategoryTransient {
		t.Errorf("Category() = %v, want %v", err.Category(), CategoryTransient)
	}
}

func TestNotFound(t *testing.T) {
	err := NotFound("user not found")
	if err.Code() != ErrCodeNotFound {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeNotFound)
	}
	if err.Category() != CategoryPermanent {
		t.Errorf("Category() = %v, want %v", err.Category(), CategoryPermanent)
	}
}

func TestRateLimited(t *testing.T) {
	err := RateLimited("too many requests")
	if err.Code() != ErrCodeRateLimit {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeRateLimit)
	}
	if !err.Retryable() {
		t.Error("rate limit error should be retryable")
	}
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("invalid token")
	if err.Code() != ErrCodeUnauthorized {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeUnauthorized)
	}
}

func TestForbidden(t *testing.T) {
	err := Forbidden("access denied")
	if err.Code() != ErrCodeForbidden {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeForbidden)
	}
}

func TestInvalidInput(t *testing.T) {
	err := InvalidInput("missing field")
	if err.Code() != ErrCodeInvalidInput {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeInvalidInput)
	}
}

func TestConflict(t *testing.T) {
	err := Conflict("version mismatch")
	if err.Code() != ErrCodeConflict {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeConflict)
	}
}

func TestInternal(t *testing.T) {
	err := Internal("unexpected error")
	if err.Code() != ErrCodeInternal {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeInternal)
	}
}

func TestAgentOffline(t *testing.T) {
	err := AgentOffline("agent-123")
	if err.Code() != ErrCodeAgentOffline {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeAgentOffline)
	}
	if err.AgentID() != "agent-123" {
		t.Errorf("AgentID() = %v, want 'agent-123'", err.AgentID())
	}
	if !err.Retryable() {
		t.Error("agent offline should be retryable")
	}
}

func TestTaskFailed(t *testing.T) {
	err := TaskFailed("task-456", "execution timeout")
	if err.Code() != ErrCodeTaskFailed {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeTaskFailed)
	}
	if err.TaskID() != "task-456" {
		t.Errorf("TaskID() = %v, want 'task-456'", err.TaskID())
	}
}

func TestCoordinationFailure(t *testing.T) {
	err := CoordinationFailure("consensus failed")
	if err.Code() != ErrCodeCoordination {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeCoordination)
	}
}

func TestConvenienceWithOptions(t *testing.T) {
	err := Timeout("timeout", WithMetadata("attempt", "3"), WithAgentID("agent-1"))
	if err.Metadata()["attempt"] != "3" {
		t.Error("convenience constructor should accept options")
	}
	if err.AgentID() != "agent-1" {
		t.Error("convenience constructor should apply agent ID option")
	}
}

// ============================================================================
// 8. Panic recovery
// ============================================================================

func TestRecoverPanicWithError(t *testing.T) {
	err := RecoverPanic(fmt.Errorf("panic error"))
	if err == nil {
		t.Fatal("RecoverPanic() should return error")
	}
	if err.Code() != ErrCodePanic {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodePanic)
	}
	if err.Error() != "panic error" {
		t.Errorf("Error() = %v", err.Error())
	}
	if err.Metadata()["panic_value"] != "*errors.errorString" {
		t.Errorf("panic_value metadata = %v", err.Metadata()["panic_value"])
	}
}

func TestRecoverPanicWithString(t *testing.T) {
	err := RecoverPanic("something went wrong")
	if err.Error() != "something went wrong" {
		t.Errorf("Error() = %v", err.Error())
	}
	if err.Metadata()["panic_value"] != "string" {
		t.Errorf("panic_value metadata = %v", err.Metadata()["panic_value"])
	}
}

func TestRecoverPanicWithOtherType(t *testing.T) {
	err := RecoverPanic(42)
	if err.Error() != "42" {
		t.Errorf("Error() = %v", err.Error())
	}
	if err.Metadata()["panic_value"] != "int" {
		t.Errorf("panic_value metadata = %v", err.Metadata()["panic_value"])
	}
}

func TestRecoverPanicWithNil(t *testing.T) {
	err := RecoverPanic(nil)
	if err != nil {
		t.Error("RecoverPanic(nil) should return nil")
	}
}

func TestRecoverPanicIntegration(t *testing.T) {
	var recovered *Error

	func() {
		defer func() {
			if r := recover(); r != nil {
				recovered = RecoverPanic(r)
			}
		}()
		panic("test panic")
	}()

	if recovered == nil {
		t.Fatal("should have recovered panic")
	}
	if recovered.Code() != ErrCodePanic {
		t.Errorf("Code() = %v, want %v", recovered.Code(), ErrCodePanic)
	}
}

// ============================================================================
// 9. Context error detection (deadline exceeded, canceled)
// ============================================================================

func TestWrapContextDeadlineExceeded(t *testing.T) {
	err := Wrap(context.DeadlineExceeded, "operation timed out")

	if err.Code() != ErrCodeTimeout {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeTimeout)
	}
	if !errors.Is(err.Unwrap(), context.DeadlineExceeded) {
		t.Error("should preserve original context error")
	}
}

func TestWrapContextCanceled(t *testing.T) {
	err := Wrap(context.Canceled, "operation canceled")

	if err.Code() != ErrCodeCanceled {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrCodeCanceled)
	}
	if !errors.Is(err.Unwrap(), context.Canceled) {
		t.Error("should preserve original context error")
	}
}

func TestWrapWrappedContextError(t *testing.T) {
	wrapped := fmt.Errorf("inner: %w", context.DeadlineExceeded)
	err := Wrap(wrapped, "outer context")

	if err.Code() != ErrCodeTimeout {
		t.Errorf("Code() = %v, want %v for wrapped context.DeadlineExceeded", err.Code(), ErrCodeTimeout)
	}
}

// ============================================================================
// 10. Error chain inspection
// ============================================================================

func TestCause(t *testing.T) {
	root := fmt.Errorf("root cause")
	middle := fmt.Errorf("middle: %w", root)
	outer := fmt.Errorf("outer: %w", middle)

	cause := Cause(outer)
	if cause != root {
		t.Errorf("Cause() = %v, want root cause", cause)
	}
}

func TestCauseNoChain(t *testing.T) {
	err := fmt.Errorf("single error")
	cause := Cause(err)
	if cause != err {
		t.Error("Cause() should return same error if no chain")
	}
}

func TestCauseWithAgentError(t *testing.T) {
	root := fmt.Errorf("database error")
	agentErr := New(ErrCodeInternal, "operation failed", WithCause(root))

	cause := Cause(agentErr)
	if cause != root {
		t.Error("Cause() should find root through AgentError")
	}
}

func TestJoin(t *testing.T) {
	err1 := New(ErrCodeTimeout, "timeout 1")
	err2 := New(ErrCodeNotFound, "not found")

	joined := Join(err1, err2)
	if joined == nil {
		t.Fatal("Join() should return error")
	}
	if !errors.Is(joined, err1) || !errors.Is(joined, err2) {
		t.Error("joined error should contain both errors")
	}
}

func TestJoinAllNil(t *testing.T) {
	joined := Join(nil, nil, nil)
	if joined != nil {
		t.Error("Join() with all nils should return nil")
	}
}

func TestCollect(t *testing.T) {
	err1 := fmt.Errorf("error 1")
	err2 := fmt.Errorf("error 2")

	collected := Collect(nil, err1, nil, err2, nil)
	if len(collected) != 2 {
		t.Errorf("Collect() returned %d errors, want 2", len(collected))
	}
}

func TestCollectAllNil(t *testing.T) {
	collected := Collect(nil, nil)
	if len(collected) != 0 {
		t.Error("Collect() with all nils should return empty slice")
	}
}

func TestFirstRetryable(t *testing.T) {
	errs := []error{
		New(ErrCodeNotFound, "not found"),
		New(ErrCodeTimeout, "timeout"),
		New(ErrCodeRateLimit, "rate limited"),
	}

	first := FirstRetryable(errs)
	if first == nil {
		t.Fatal("FirstRetryable() should find retryable error")
	}
	if Code(first) != ErrCodeTimeout {
		t.Errorf("FirstRetryable() = %v, want first retryable (timeout)", Code(first))
	}
}

func TestFirstRetryableNone(t *testing.T) {
	errs := []error{
		New(ErrCodeNotFound, "not found"),
		New(ErrCodeForbidden, "forbidden"),
	}

	first := FirstRetryable(errs)
	if first != nil {
		t.Error("FirstRetryable() should return nil when none retryable")
	}
}

func TestFirstRetryableEmpty(t *testing.T) {
	first := FirstRetryable(nil)
	if first != nil {
		t.Error("FirstRetryable(nil) should return nil")
	}
}

func TestAllRetryable(t *testing.T) {
	errs := []error{
		New(ErrCodeTimeout, "timeout"),
		New(ErrCodeRateLimit, "rate limited"),
	}

	if !AllRetryable(errs) {
		t.Error("AllRetryable() should return true when all are retryable")
	}
}

func TestAllRetryableFalse(t *testing.T) {
	errs := []error{
		New(ErrCodeTimeout, "timeout"),
		New(ErrCodeNotFound, "not found"),
	}

	if AllRetryable(errs) {
		t.Error("AllRetryable() should return false when one is not retryable")
	}
}

func TestAllRetryableEmpty(t *testing.T) {
	if !AllRetryable(nil) {
		t.Error("AllRetryable(nil) should return true")
	}
}

func TestAnyRetryable(t *testing.T) {
	errs := []error{
		New(ErrCodeNotFound, "not found"),
		New(ErrCodeTimeout, "timeout"),
	}

	if !AnyRetryable(errs) {
		t.Error("AnyRetryable() should return true when one is retryable")
	}
}

func TestAnyRetryableFalse(t *testing.T) {
	errs := []error{
		New(ErrCodeNotFound, "not found"),
		New(ErrCodeForbidden, "forbidden"),
	}

	if AnyRetryable(errs) {
		t.Error("AnyRetryable() should return false when none are retryable")
	}
}

func TestAnyRetryableEmpty(t *testing.T) {
	if AnyRetryable(nil) {
		t.Error("AnyRetryable(nil) should return false")
	}
}

// ============================================================================
// Additional edge cases and coverage
// ============================================================================

func TestErrorCodeString(t *testing.T) {
	code := ErrCodeTimeout
	if code.String() != "TIMEOUT" {
		t.Errorf("String() = %v, want TIMEOUT", code.String())
	}
}

func TestErrorCategoryString(t *testing.T) {
	cat := CategoryTransient
	if cat.String() != "transient" {
		t.Errorf("String() = %v, want transient", cat.String())
	}
}

func TestErrorCodeDefaultRetryable(t *testing.T) {
	if !ErrCodeTimeout.DefaultRetryable() {
		t.Error("Timeout should be default retryable")
	}
	if ErrCodeNotFound.DefaultRetryable() {
		t.Error("NotFound should not be default retryable")
	}
}

func TestErrorCodeDescription(t *testing.T) {
	if ErrCodeTimeout.Description() != "operation timed out" {
		t.Errorf("Description() = %v", ErrCodeTimeout.Description())
	}
}

func TestErrorCodeDescriptionUnknown(t *testing.T) {
	unknown := ErrorCode("UNKNOWN_CODE")
	if unknown.Description() != "unknown error" {
		t.Errorf("Description() = %v, want 'unknown error'", unknown.Description())
	}
}

func TestErrorCodeDefaultCategoryUnknown(t *testing.T) {
	unknown := ErrorCode("UNKNOWN_CODE")
	if unknown.DefaultCategory() != CategoryInternal {
		t.Errorf("DefaultCategory() = %v, want CategoryInternal", unknown.DefaultCategory())
	}
}

func TestWithCategory(t *testing.T) {
	// Override default category
	err := New(ErrCodeTimeout, "timeout", WithCategory(CategoryPermanent))
	if err.Category() != CategoryPermanent {
		t.Errorf("Category() = %v, want %v", err.Category(), CategoryPermanent)
	}
}

func TestAgentErrorInterface(t *testing.T) {
	// Ensure *Error implements AgentError
	var _ AgentError = New(ErrCodeInternal, "test")
}

func TestErrorWithEmptyCause(t *testing.T) {
	err := New(ErrCodeInternal, "test message")
	if err.Error() != "test message" {
		t.Errorf("Error() without cause = %v, want 'test message'", err.Error())
	}
}

func TestAllErrorCodes(t *testing.T) {
	// Test that all error codes have valid default categories
	codes := []ErrorCode{
		ErrCodeTimeout, ErrCodeUnavailable, ErrCodeNetworkErr, ErrCodeRetryLater,
		ErrCodeNotFound, ErrCodeConflict, ErrCodeInvalidInput, ErrCodeUnauthorized,
		ErrCodeForbidden, ErrCodeAlreadyExists, ErrCodePrecondition, ErrCodeUnsupported,
		ErrCodeCanceled, ErrCodeRateLimit, ErrCodeQuotaExceeded, ErrCodeResourceBusy,
		ErrCodeCapacity, ErrCodeInternal, ErrCodeCorruption, ErrCodeAssertion,
		ErrCodePanic, ErrCodeAgentOffline, ErrCodeAgentBusy, ErrCodeTaskFailed,
		ErrCodeCoordination, ErrCodeHandoffFailed, ErrCodeCapabilityMissing,
	}

	for _, code := range codes {
		cat := code.DefaultCategory()
		if cat == "" {
			t.Errorf("code %s has empty default category", code)
		}
		// All codes should have descriptions
		desc := code.Description()
		if desc == "" || desc == "unknown error" {
			t.Errorf("code %s missing description", code)
		}
	}
}

func TestMetadataMapMerge(t *testing.T) {
	// Test that multiple metadata calls merge
	err := New(ErrCodeInternal, "test",
		WithMetadata("a", "1"),
		WithMetadataMap(map[string]string{"b": "2", "c": "3"}),
		WithMetadata("d", "4"),
	)

	meta := err.Metadata()
	expected := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}
	for k, v := range expected {
		if meta[k] != v {
			t.Errorf("Metadata[%s] = %v, want %v", k, meta[k], v)
		}
	}
}

func TestJSONInvalidTimestamp(t *testing.T) {
	// Invalid timestamp should be silently ignored
	jsonStr := `{"code":"INTERNAL","category":"internal","message":"test","timestamp":"invalid"}`

	var err Error
	if e := json.Unmarshal([]byte(jsonStr), &err); e != nil {
		t.Fatalf("Unmarshal failed: %v", e)
	}

	if !err.Timestamp().IsZero() {
		t.Error("invalid timestamp should result in zero time")
	}
}

func TestJSONUnmarshalError(t *testing.T) {
	invalidJSON := `{invalid json}`

	var err Error
	if e := json.Unmarshal([]byte(invalidJSON), &err); e == nil {
		t.Error("should fail on invalid JSON")
	}
}
