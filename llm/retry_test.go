package llm

import (
	"testing"
	"time"
)

func TestResolveRetryConfig_Custom(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  3,
		InitBackoff: 2 * time.Second,
		MaxBackoff:  30 * time.Second,
	}

	maxRetries, initBackoff, maxBackoff := resolveRetryConfig(cfg)
	if maxRetries != 3 {
		t.Errorf("expected maxRetries 3, got %d", maxRetries)
	}
	if initBackoff != 2*time.Second {
		t.Errorf("expected initBackoff 2s, got %v", initBackoff)
	}
	if maxBackoff != 30*time.Second {
		t.Errorf("expected maxBackoff 30s, got %v", maxBackoff)
	}
}

func TestResolveRetryConfig_Defaults(t *testing.T) {
	cfg := RetryConfig{} // No values set

	maxRetries, initBackoff, maxBackoff := resolveRetryConfig(cfg)
	if maxRetries != defaultMaxRetries {
		t.Errorf("expected default maxRetries %d, got %d", defaultMaxRetries, maxRetries)
	}
	if initBackoff != defaultInitBackoff {
		t.Errorf("expected default initBackoff, got %v", initBackoff)
	}
	if maxBackoff != defaultMaxBackoff {
		t.Errorf("expected default maxBackoff, got %v", maxBackoff)
	}
}

// =============================================================================
// Error Classification Tests
// =============================================================================

// Helper type for testing error classification
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"rate limit exceeded", true},
		{"too many requests", true},
		{"error: 429", true},
		{"server overloaded", true},
		{"at capacity", true},
		{"internal server error", false},
		{"invalid api key", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = &testError{msg: tt.errMsg}
			}
			got := isRateLimitError(err)
			if got != tt.want {
				t.Errorf("isRateLimitError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"internal server error", true},
		{"bad gateway", true},
		{"service unavailable", true},
		{"gateway timeout", true},
		{"error: 500", true},
		{"error: 502", true},
		{"error: 503", true},
		{"error: 504", true},
		{"temporarily unavailable", true},
		{"rate limit exceeded", false},
		{"invalid api key", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			got := isServerError(&testError{msg: tt.errMsg})
			if got != tt.want {
				t.Errorf("isServerError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}

func TestIsBillingError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"billing issue", true},
		{"payment required", true},
		{"insufficient credits", true},
		{"quota exceeded", true},
		{"subscription expired", true},
		{"error: 402", true},
		{"rate limit exceeded", false},
		{"internal server error", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			got := isBillingError(&testError{msg: tt.errMsg})
			if got != tt.want {
				t.Errorf("isBillingError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		// Rate limit errors - retryable
		{"rate limit exceeded", true},
		{"429", true},
		// Server errors - retryable
		{"500 internal server error", true},
		{"503 service unavailable", true},
		// Non-retryable
		{"invalid api key", false},
		{"billing issue", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			got := isRetryableError(&testError{msg: tt.errMsg})
			if got != tt.want {
				t.Errorf("isRetryableError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}
