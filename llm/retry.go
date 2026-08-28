package llm

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// Retry configuration defaults
const (
	defaultMaxRetries  = 5
	defaultInitBackoff = 1 * time.Second
	defaultMaxBackoff  = 60 * time.Second
	backoffFactor      = 2.0
)

// resolveRetryConfig returns effective retry settings with defaults applied.
func resolveRetryConfig(cfg RetryConfig) (maxRetries int, initBackoff, maxBackoff time.Duration) {
	maxRetries = cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	initBackoff = cfg.InitBackoff
	if initBackoff <= 0 {
		initBackoff = defaultInitBackoff
	}
	maxBackoff = cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = defaultMaxBackoff
	}
	return
}

// withRetry executes fn with exponential backoff retry on transient errors.
func withRetry[T any](ctx context.Context, cfg RetryConfig, providerName string, fn func() (T, error)) (T, error) {
	maxRetries, initBackoff, maxBackoff := resolveRetryConfig(cfg)
	var zero T
	var lastErr error
	backoff := initBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err

		if isBillingError(err) {
			return zero, fmt.Errorf("billing/payment error (fatal): %w", err)
		}
		if !isRetryableError(err) {
			return zero, fmt.Errorf("%s request failed: %w", providerName, err)
		}
		if attempt == maxRetries {
			return zero, fmt.Errorf("%s request failed after %d retries: %w", providerName, maxRetries, err)
		}

		// Full jitter: sleep is uniform in [0, backoff) to break up
		// thundering herds when many callers retry in lockstep.
		sleep := time.Duration(rand.Float64() * float64(backoff))
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(sleep):
		}

		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return zero, lastErr
}

// withStreamRetry is like withRetry, but stops retrying as soon as
// *delivered is true. Once a streaming callback has received any delta, a
// retry would re-run the request and deliver that data a second time, so a
// subsequent failure must surface as an error instead.
func withStreamRetry[T any](ctx context.Context, cfg RetryConfig, providerName string, delivered *bool, fn func() (T, error)) (T, error) {
	maxRetries, initBackoff, maxBackoff := resolveRetryConfig(cfg)
	var zero T
	var lastErr error
	backoff := initBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err

		if *delivered {
			return zero, fmt.Errorf("%s stream failed after delivering data: %w", providerName, err)
		}
		if isBillingError(err) {
			return zero, fmt.Errorf("billing/payment error (fatal): %w", err)
		}
		if !isRetryableError(err) {
			return zero, fmt.Errorf("%s request failed: %w", providerName, err)
		}
		if attempt == maxRetries {
			return zero, fmt.Errorf("%s request failed after %d retries: %w", providerName, maxRetries, err)
		}

		sleep := time.Duration(rand.Float64() * float64(backoff))
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(sleep):
		}

		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return zero, lastErr
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "overloaded") ||
		strings.Contains(errStr, "capacity")
}

func isServerError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "internal server error") ||
		strings.Contains(errStr, "bad gateway") ||
		strings.Contains(errStr, "service unavailable") ||
		strings.Contains(errStr, "gateway timeout") ||
		strings.Contains(errStr, "temporarily unavailable")
}

func isRetryableError(err error) bool {
	return isRateLimitError(err) || isServerError(err)
}

func isBillingError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "billing") ||
		strings.Contains(errStr, "payment") ||
		strings.Contains(errStr, "credits") ||
		strings.Contains(errStr, "quota exceeded") ||
		strings.Contains(errStr, "insufficient") ||
		strings.Contains(errStr, "402") ||
		strings.Contains(errStr, "subscription") ||
		strings.Contains(errStr, "expired")
}
