package errors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// ============================================================================
// 1. Code and Category types
// ============================================================================

func TestCodeDefaultCategory(t *testing.T) {
	tests := []struct {
		code Code
		want Category
	}{
		// Transient
		{Timeout, CategoryTransient},
		{Unavailable, CategoryTransient},
		{NetworkErr, CategoryTransient},
		{RetryLater, CategoryTransient},
		{AgentOffline, CategoryTransient},
		{AgentBusy, CategoryTransient},
		{Coordination, CategoryTransient},
		{HandoffFailed, CategoryTransient},
		// Permanent
		{NotFound, CategoryPermanent},
		{Conflict, CategoryPermanent},
		{InvalidInput, CategoryPermanent},
		{Unauthorized, CategoryPermanent},
		{Forbidden, CategoryPermanent},
		{AlreadyExists, CategoryPermanent},
		{Precondition, CategoryPermanent},
		{Unsupported, CategoryPermanent},
		{Canceled, CategoryPermanent},
		{TaskFailed, CategoryPermanent},
		{CapabilityMissing, CategoryPermanent},
		// Resource
		{RateLimit, CategoryResource},
		{QuotaExceeded, CategoryResource},
		{ResourceBusy, CategoryResource},
		{Capacity, CategoryResource},
		// Internal
		{Internal, CategoryInternal},
		{Corruption, CategoryInternal},
		{Assertion, CategoryInternal},
		{Panic, CategoryInternal},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := tt.code.DefaultCategory(); got != tt.want {
				t.Errorf("DefaultCategory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodeDefaultCategoryUnknown(t *testing.T) {
	unknown := Code("UNKNOWN_CODE")
	if got := unknown.DefaultCategory(); got != CategoryInternal {
		t.Errorf("DefaultCategory() = %v, want CategoryInternal", got)
	}
}

func TestCodeDescription(t *testing.T) {
	if got := Timeout.Description(); got != "operation timed out" {
		t.Errorf("Description() = %v", got)
	}
}

func TestCodeDescriptionUnknown(t *testing.T) {
	unknown := Code("UNKNOWN_CODE")
	if got := unknown.Description(); got != "unknown error" {
		t.Errorf("Description() = %v, want 'unknown error'", got)
	}
}

func TestCategoryIsRetryable(t *testing.T) {
	tests := []struct {
		category  Category
		retryable bool
	}{
		{CategoryTransient, true},
		{CategoryResource, true},
		{CategoryPermanent, false},
		{CategoryInternal, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := tt.category.IsRetryable(); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

// ============================================================================
// 2. Error creation
// ============================================================================

func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		code         Code
		message      string
		wantCategory Category
	}{
		{"timeout", Timeout, "operation timed out", CategoryTransient},
		{"not_found", NotFound, "resource not found", CategoryPermanent},
		{"rate_limit", RateLimit, "too many requests", CategoryResource},
		{"internal", Internal, "internal error", CategoryInternal},
		{"agent_offline", AgentOffline, "agent down", CategoryTransient},
		{"task_failed", TaskFailed, "task failed", CategoryPermanent},
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
		})
	}
}

func TestNewf(t *testing.T) {
	err := Newf(NotFound, "user %s not found", "alice")
	if got := err.Error(); got != "user alice not found" {
		t.Errorf("Error() = %v, want 'user alice not found'", got)
	}
}

func TestFrom(t *testing.T) {
	err := From(Timeout)
	if err.Code() != Timeout {
		t.Errorf("Code() = %v, want %v", err.Code(), Timeout)
	}
	if err.Error() != "operation timed out" {
		t.Errorf("Error() = %v, want 'operation timed out'", err.Error())
	}
}

func TestFromWithOptions(t *testing.T) {
	err := From(NotFound, WithMetadata("resource", "user"))
	if err.Metadata()["resource"] != "user" {
		t.Error("expected metadata 'resource' to be 'user'")
	}
}

// ============================================================================
// 3. Retryable behavior
// ============================================================================

func TestRetryable(t *testing.T) {
	tests := []struct {
		name      string
		code      Code
		wantRetry bool
	}{
		{"timeout is retryable", Timeout, true},
		{"unavailable is retryable", Unavailable, true},
		{"network_err is retryable", NetworkErr, true},
		{"rate_limit is retryable", RateLimit, true},
		{"not_found is not retryable", NotFound, false},
		{"invalid_input is not retryable", InvalidInput, false},
		{"internal is not retryable", Internal, false},
		{"forbidden is not retryable", Forbidden, false},
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
	err := New(Timeout, "permanent timeout", WithRetryable(false))
	if err.Retryable() {
		t.Error("expected error to be non-retryable after override")
	}

	// Override a normally non-retryable error to be retryable
	err2 := New(NotFound, "maybe retry", WithRetryable(true))
	if !err2.Retryable() {
		t.Error("expected error to be retryable after override")
	}
}

// ============================================================================
// 4. Metadata
// ============================================================================

func TestMetadata(t *testing.T) {
	err := New(Internal, "test",
		WithMetadata("key1", "value1"),
		WithMetadata("key2", "value2"),
	)

	meta := err.Metadata()
	if meta["key1"] != "value1" || meta["key2"] != "value2" {
		t.Errorf("Metadata() = %v, want key1=value1, key2=value2", meta)
	}
}

func TestMetadataImmutability(t *testing.T) {
	err := New(Internal, "test", WithMetadata("original", "value"))

	meta := err.Metadata()
	meta["injected"] = "evil"

	if err.Metadata()["injected"] != "" {
		t.Error("Metadata() should return a copy, not the original map")
	}
}

func TestNilMetadata(t *testing.T) {
	err := New(Internal, "test")
	meta := err.Metadata()
	if meta == nil {
		t.Error("Metadata() should return empty map, not nil")
	}
	if len(meta) != 0 {
		t.Errorf("Metadata() should be empty, got %v", meta)
	}
}

// ============================================================================
// 5. Options
// ============================================================================

func TestWithCategory(t *testing.T) {
	err := New(Timeout, "timeout", WithCategory(CategoryPermanent))
	if err.Category() != CategoryPermanent {
		t.Errorf("Category() = %v, want %v", err.Category(), CategoryPermanent)
	}
}

func TestWithCause(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := New(Internal, "wrapper", WithCause(cause))

	if err.Unwrap() != cause {
		t.Error("Unwrap() should return cause set via WithCause")
	}
}

func TestErrorMessageWithCause(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := New(Internal, "wrapper", WithCause(cause))

	if err.Error() != "wrapper: root cause" {
		t.Errorf("Error() = %v, want 'wrapper: root cause'", err.Error())
	}
}

func TestErrorMessageWithoutCause(t *testing.T) {
	err := New(Internal, "test message")
	if err.Error() != "test message" {
		t.Errorf("Error() = %v, want 'test message'", err.Error())
	}
}

// ============================================================================
// 6. Wrapping
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
	// Default to internal for unknown errors
	if err.Code() != Internal {
		t.Errorf("Code() = %v, want %v", err.Code(), Internal)
	}
}

func TestWrapNil(t *testing.T) {
	err := Wrap(nil, "message")
	if err != nil {
		t.Error("Wrap(nil, ...) should return nil")
	}
}

func TestWrapAgentError(t *testing.T) {
	original := New(NotFound, "resource missing",
		WithMetadata("id", "123"),
	)
	wrapped := Wrap(original, "operation failed")

	if wrapped.Code() != NotFound {
		t.Errorf("wrapped.Code() = %v, want %v", wrapped.Code(), NotFound)
	}
	if wrapped.Metadata()["id"] != "123" {
		t.Error("wrapped error should preserve metadata")
	}
	if !errors.Is(wrapped, original) {
		t.Error("wrapped error should be 'Is' original")
	}
}

func TestWrapContextDeadlineExceeded(t *testing.T) {
	err := Wrap(context.DeadlineExceeded, "operation timed out")

	if err.Code() != Timeout {
		t.Errorf("Code() = %v, want %v", err.Code(), Timeout)
	}
	if !errors.Is(err.Unwrap(), context.DeadlineExceeded) {
		t.Error("should preserve original context error")
	}
}

func TestWrapContextCanceled(t *testing.T) {
	err := Wrap(context.Canceled, "operation canceled")

	if err.Code() != Canceled {
		t.Errorf("Code() = %v, want %v", err.Code(), Canceled)
	}
	if !errors.Is(err.Unwrap(), context.Canceled) {
		t.Error("should preserve original context error")
	}
}

func TestWrapWrappedContextError(t *testing.T) {
	wrapped := fmt.Errorf("inner: %w", context.DeadlineExceeded)
	err := Wrap(wrapped, "outer context")

	if err.Code() != Timeout {
		t.Errorf("Code() = %v, want %v for wrapped context.DeadlineExceeded", err.Code(), Timeout)
	}
}

func TestWrapWithOptions(t *testing.T) {
	cause := fmt.Errorf("db error")
	err := Wrap(cause, "query failed", WithMetadata("table", "users"))

	if err.Metadata()["table"] != "users" {
		t.Error("Wrap should accept options")
	}
}

func TestWrapAgentErrorWithOptions(t *testing.T) {
	original := New(NotFound, "missing")
	wrapped := Wrap(original, "lookup failed", WithMetadata("extra", "info"))

	if wrapped.Metadata()["extra"] != "info" {
		t.Error("Wrap should apply options when wrapping an *Error")
	}
	if wrapped.Code() != NotFound {
		t.Errorf("Code() = %v, want %v", wrapped.Code(), NotFound)
	}
}

// ============================================================================
// 7. Inspection free functions
// ============================================================================

func TestHas(t *testing.T) {
	err := New(NotFound, "not found")

	if !Has(err, NotFound) {
		t.Error("Has() should return true for matching code")
	}
	if Has(err, Timeout) {
		t.Error("Has() should return false for non-matching code")
	}
}

func TestHasWithWrappedError(t *testing.T) {
	original := New(NotFound, "not found")
	wrapped := fmt.Errorf("context: %w", original)

	if !Has(wrapped, NotFound) {
		t.Error("Has() should find code in wrapped error")
	}
}

func TestHasWithNonError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if Has(err, Internal) {
		t.Error("Has() should return false for non-Error")
	}
}

func TestIsRetryable(t *testing.T) {
	if !IsRetryable(New(Timeout, "timeout")) {
		t.Error("IsRetryable() should return true for retryable error")
	}
	if IsRetryable(New(NotFound, "not found")) {
		t.Error("IsRetryable() should return false for non-retryable error")
	}
}

func TestIsRetryableNonError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if IsRetryable(err) {
		t.Error("IsRetryable() should return false for non-Error")
	}
}

func TestCodeOf(t *testing.T) {
	err := New(Timeout, "timeout")
	if CodeOf(err) != Timeout {
		t.Errorf("CodeOf() = %v, want %v", CodeOf(err), Timeout)
	}
}

func TestCodeOfNonError(t *testing.T) {
	err := fmt.Errorf("regular error")
	if CodeOf(err) != "" {
		t.Error("CodeOf() should return empty string for non-Error")
	}
}

func TestCodeOfWrapped(t *testing.T) {
	original := New(NotFound, "not found")
	wrapped := fmt.Errorf("context: %w", original)

	if CodeOf(wrapped) != NotFound {
		t.Error("CodeOf() should find code in wrapped error")
	}
}

// ============================================================================
// 8. Stdlib re-exports
// ============================================================================

func TestStdlibAs(t *testing.T) {
	agentErr := New(Timeout, "timeout")
	wrapped := fmt.Errorf("wrapped: %w", agentErr)

	var target *Error
	if !As(wrapped, &target) {
		t.Error("As() should extract Error from wrapped")
	}
	if target.Code() != Timeout {
		t.Errorf("extracted.Code() = %v, want %v", target.Code(), Timeout)
	}
}

func TestStdlibIs(t *testing.T) {
	sentinel := fmt.Errorf("sentinel")
	wrapped := fmt.Errorf("wrapped: %w", sentinel)

	if !Is(wrapped, sentinel) {
		t.Error("Is() should find sentinel in chain")
	}
}

func TestStdlibUnwrap(t *testing.T) {
	inner := fmt.Errorf("inner")
	outer := fmt.Errorf("outer: %w", inner)

	if Unwrap(outer) != inner {
		t.Error("Unwrap() should return inner error")
	}
}

func TestStdlibJoin(t *testing.T) {
	err1 := New(Timeout, "timeout 1")
	err2 := New(NotFound, "not found")

	joined := Join(err1, err2)
	if joined == nil {
		t.Fatal("Join() should return error")
	}
	if !errors.Is(joined, err1) || !errors.Is(joined, err2) {
		t.Error("joined error should contain both errors")
	}
}

func TestStdlibJoinAllNil(t *testing.T) {
	joined := Join(nil, nil, nil)
	if joined != nil {
		t.Error("Join() with all nils should return nil")
	}
}

// ============================================================================
// 9. Chain utilities
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
	if Cause(err) != err {
		t.Error("Cause() should return same error if no chain")
	}
}

func TestCauseWithAgentError(t *testing.T) {
	root := fmt.Errorf("database error")
	agentErr := New(Internal, "operation failed", WithCause(root))

	if Cause(agentErr) != root {
		t.Error("Cause() should find root through Error")
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

// ============================================================================
// 10. RecoverPanic
// ============================================================================

func TestRecoverPanicWithError(t *testing.T) {
	err := RecoverPanic(fmt.Errorf("panic error"))
	if err == nil {
		t.Fatal("RecoverPanic() should return error")
	}
	if err.Code() != Panic {
		t.Errorf("Code() = %v, want %v", err.Code(), Panic)
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
}

func TestRecoverPanicWithNil(t *testing.T) {
	if RecoverPanic(nil) != nil {
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
	if recovered.Code() != Panic {
		t.Errorf("Code() = %v, want %v", recovered.Code(), Panic)
	}
}

// ============================================================================
// 11. JSON serialization
// ============================================================================

func TestJSONRoundtrip(t *testing.T) {
	original := New(NotFound, "user not found",
		WithMetadata("user_id", "123"),
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
	if restored.Error() != original.message {
		t.Errorf("Message mismatch: %v vs %v", restored.Error(), original.message)
	}
	if restored.Retryable() != original.Retryable() {
		t.Errorf("Retryable mismatch: %v vs %v", restored.Retryable(), original.Retryable())
	}
	if restored.Metadata()["user_id"] != "123" {
		t.Error("Metadata not preserved")
	}
}

func TestJSONWithCause(t *testing.T) {
	cause := fmt.Errorf("underlying issue")
	err := New(Internal, "wrapper", WithCause(cause))

	data, _ := json.Marshal(err)

	var j map[string]any
	json.Unmarshal(data, &j)

	if j["cause"] != "underlying issue" {
		t.Errorf("cause should be serialized: %v", j["cause"])
	}
}

func TestJSONUnmarshalWithCause(t *testing.T) {
	jsonStr := `{"code":"INTERNAL","category":"internal","message":"test","cause":"original error"}`

	var e Error
	if err := json.Unmarshal([]byte(jsonStr), &e); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if e.Unwrap() == nil {
		t.Error("Unwrap() should return reconstructed cause")
	}
	if e.Unwrap().Error() != "original error" {
		t.Errorf("Unwrap().Error() = %v, want 'original error'", e.Unwrap().Error())
	}
}

func TestJSONNoTimestampOrAgentFields(t *testing.T) {
	err := New(Internal, "test", WithMetadata("agent_id", "a1"))

	data, _ := json.Marshal(err)
	var j map[string]any
	json.Unmarshal(data, &j)

	// These fields should not exist in JSON
	if _, ok := j["timestamp"]; ok {
		t.Error("JSON should not contain 'timestamp' field")
	}
	if _, ok := j["agent_id"]; ok {
		t.Error("JSON should not contain 'agent_id' field — agent_id goes in metadata")
	}
	if _, ok := j["task_id"]; ok {
		t.Error("JSON should not contain 'task_id' field")
	}
}

func TestJSONUnmarshalError(t *testing.T) {
	var e Error
	if err := json.Unmarshal([]byte(`{invalid}`), &e); err == nil {
		t.Error("should fail on structurally invalid JSON")
	}
	// Structurally valid JSON but wrong field types — exercises the
	// inner json.Unmarshal failure path inside UnmarshalJSON.
	if err := e.UnmarshalJSON([]byte(`{"retryable":"not-a-bool"}`)); err == nil {
		t.Error("should fail when field types don't match errorJSON")
	}
}

// ============================================================================
// 12. All codes have descriptions and categories
// ============================================================================

func TestAllCodesHaveDescriptionsAndCategories(t *testing.T) {
	codes := []Code{
		Timeout, Unavailable, NetworkErr, RetryLater,
		NotFound, Conflict, InvalidInput, Unauthorized,
		Forbidden, AlreadyExists, Precondition, Unsupported,
		Canceled, RateLimit, QuotaExceeded, ResourceBusy,
		Capacity, Internal, Corruption, Assertion,
		Panic, AgentOffline, AgentBusy, TaskFailed,
		Coordination, HandoffFailed, CapabilityMissing,
	}

	for _, code := range codes {
		cat := code.DefaultCategory()
		if cat == "" {
			t.Errorf("code %s has empty default category", code)
		}
		desc := code.Description()
		if desc == "" || desc == "unknown error" {
			t.Errorf("code %s missing description", code)
		}
	}
}

// ============================================================================
// 13. Edge cases for coverage
// ============================================================================

func TestJSONRoundtripWithoutCause(t *testing.T) {
	original := New(NotFound, "not found")
	data, _ := json.Marshal(original)

	var restored Error
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if restored.Unwrap() != nil {
		t.Error("Unwrap() should be nil when no cause in JSON")
	}
}

// nilUnwrapper implements Unwrap() but returns nil.
type nilUnwrapper struct{}

func (n nilUnwrapper) Error() string { return "nil unwrapper" }
func (n nilUnwrapper) Unwrap() error { return nil }

func TestCauseWithNilUnwrap(t *testing.T) {
	err := nilUnwrapper{}
	cause := Cause(err)
	if cause != err {
		t.Error("Cause() should return the error itself when Unwrap() returns nil")
	}
}

// ============================================================================
// 14. Metadata merge across multiple WithMetadata calls
// ============================================================================

func TestMetadataMerge(t *testing.T) {
	err := New(Internal, "test",
		WithMetadata("a", "1"),
		WithMetadata("b", "2"),
		WithMetadata("c", "3"),
	)

	meta := err.Metadata()
	expected := map[string]string{"a": "1", "b": "2", "c": "3"}
	for k, v := range expected {
		if meta[k] != v {
			t.Errorf("Metadata[%s] = %v, want %v", k, meta[k], v)
		}
	}
}
