package state

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Operation.String() tests
// ============================================================================

func TestOperation_String(t *testing.T) {
	tests := []struct {
		op   Operation
		want string
	}{
		{OpPut, "put"},
		{OpDelete, "delete"},
		{Operation(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.op.String()
		if got != tt.want {
			t.Errorf("Operation(%d).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

// ============================================================================
// ValidateKey tests
// ============================================================================

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr error
	}{
		{"valid simple key", "mykey", nil},
		{"valid dotted key", "config.timeout", nil},
		{"valid with numbers", "key123", nil},
		{"valid long key", strings.Repeat("a", 1024), nil},
		{"empty key", "", ErrInvalidKey},
		{"key with space", "key with space", ErrInvalidKey},
		{"leading dot", ".key", ErrInvalidKey},
		{"trailing dot", "key.", ErrInvalidKey},
		{"too long key", strings.Repeat("a", 1025), ErrInvalidKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if err != tt.wantErr {
				t.Errorf("ValidateKey(%q) = %v, want %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// ValidateTTL tests
// ============================================================================

func TestValidateTTL(t *testing.T) {
	tests := []struct {
		name    string
		ttl     time.Duration
		wantErr error
	}{
		{"zero TTL", 0, nil},
		{"positive TTL", time.Second, nil},
		{"negative TTL", -time.Second, ErrInvalidTTL},
		{"very long TTL", 24 * time.Hour, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTTL(tt.ttl)
			if err != tt.wantErr {
				t.Errorf("ValidateTTL(%v) = %v, want %v", tt.ttl, err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// MatchPattern tests
// ============================================================================

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		key     string
		want    bool
	}{
		// Wildcard * matches all
		{"*", "anything", true},
		{"*", "config.foo", true},
		{"*", "", true},

		// Prefix wildcard
		{"config.*", "config.foo", true},
		{"config.*", "config.bar.baz", true},
		{"config.*", "config.", true},
		{"config.*", "other.foo", false},
		{"config.*", "configfoo", false},

		// Exact match
		{"config.foo", "config.foo", true},
		{"config.foo", "config.bar", false},
		{"config.foo", "config.foobar", false},
		{"key", "key", true},
		{"key", "key2", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.key, func(t *testing.T) {
			got := MatchPattern(tt.pattern, tt.key)
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.key, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Error variable tests
// ============================================================================

func TestErrorStrings(t *testing.T) {
	// Ensure errors have meaningful strings
	errors := []error{
		ErrNotFound,
		ErrClosed,
		ErrLockHeld,
		ErrLockNotHeld,
		ErrLockExpired,
		ErrInvalidKey,
		ErrInvalidTTL,
		ErrWatchClosed,
	}

	for _, err := range errors {
		if err.Error() == "" {
			t.Errorf("error %v has empty string", err)
		}
	}
}
