package shutdown

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	// ErrAlreadyShutdown indicates shutdown was already initiated.
	ErrAlreadyShutdown = errors.New("shutdown already initiated")

	// ErrTimeout indicates shutdown did not complete within the timeout.
	ErrTimeout = errors.New("shutdown timeout exceeded")

	// ErrHandlerFailed indicates one or more handlers failed during shutdown.
	ErrHandlerFailed = errors.New("one or more handlers failed")

	// ErrInvalidConfig indicates invalid configuration.
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Handler is implemented by components that need graceful shutdown.
type Handler interface {
	Shutdown(ctx context.Context) error
}

// Func is a convenience type for simple shutdown functions.
// It implements Handler, similar to http.HandlerFunc.
//
//	seq.Register("cache", shutdown.Func(func(ctx context.Context) error {
//	    return cache.Flush(ctx)
//	}))
type Func func(ctx context.Context) error

// Shutdown implements Handler.
func (f Func) Shutdown(ctx context.Context) error {
	return f(ctx)
}

// Result contains the outcome of a single handler's shutdown.
type Result struct {
	Name     string
	Phase    int
	Duration time.Duration
	Err      error
}

// Summary contains the complete shutdown outcome.
type Summary struct {
	TotalDuration time.Duration
	Results       []Result
	Err           error
}

// Failed reports whether any handler failed.
func (s *Summary) Failed() bool {
	return s.Err != nil
}

// FailedHandlers returns the names of handlers that failed.
func (s *Summary) FailedHandlers() []string {
	var failed []string
	for _, r := range s.Results {
		if r.Err != nil {
			failed = append(failed, r.Name)
		}
	}
	return failed
}

// Config configures the shutdown sequence.
type Config struct {
	// DefaultTimeout is used when HandleSignals triggers shutdown.
	// Default: 30 seconds.
	DefaultTimeout time.Duration

	// DefaultPhase is assigned to handlers registered without a phase.
	// Default: 100.
	DefaultPhase int

	// ContinueOnError determines whether shutdown continues if a handler fails.
	// Default: true.
	ContinueOnError bool

	// Progress is called when each handler completes. Optional.
	Progress func(Result)
}

// Validate reports whether the configuration is valid.
func (c *Config) Validate() error {
	if c.DefaultTimeout < 0 {
		return ErrInvalidConfig
	}
	return nil
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		DefaultTimeout:  30 * time.Second,
		DefaultPhase:    100,
		ContinueOnError: true,
	}
}

// entry holds a registered handler with its metadata.
type entry struct {
	name    string
	handler Handler
	phase   int
}
