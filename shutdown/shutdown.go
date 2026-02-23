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

// ShutdownHandler is implemented by components that need graceful shutdown.
type ShutdownHandler interface {
	// OnShutdown is called when shutdown is initiated.
	// The context will be cancelled when the timeout is reached.
	// Implementations should:
	// - Stop accepting new work
	// - Finish in-progress tasks (if time permits)
	// - Re-queue pending work
	// - Release resources
	OnShutdown(ctx context.Context) error
}

// ShutdownFunc is a convenience type for simple shutdown functions.
type ShutdownFunc func(ctx context.Context) error

// OnShutdown implements ShutdownHandler.
func (f ShutdownFunc) OnShutdown(ctx context.Context) error {
	return f(ctx)
}

// ShutdownCoordinator manages graceful shutdown for multiple components.
type ShutdownCoordinator interface {
	// Register adds a handler to be called during shutdown.
	// Handlers are called in registration order when no phase is specified.
	Register(name string, handler ShutdownHandler)

	// RegisterWithPhase adds a handler with a specific phase.
	// Lower phase numbers are shut down first.
	// Handlers in the same phase are shut down concurrently.
	RegisterWithPhase(name string, handler ShutdownHandler, phase int)

	// Shutdown initiates graceful shutdown.
	// It calls all registered handlers in order.
	// Returns ErrAlreadyShutdown if called more than once.
	Shutdown(ctx context.Context) error

	// ShutdownWithTimeout initiates shutdown with a timeout.
	// This is a convenience wrapper around Shutdown.
	ShutdownWithTimeout(timeout time.Duration) error

	// HandleSignals registers handlers for SIGTERM and SIGINT.
	// When a signal is received, Shutdown is called with the configured timeout.
	// Must be called before the signals are expected.
	HandleSignals()

	// Done returns a channel that is closed when shutdown is complete.
	Done() <-chan struct{}

	// Err returns any error that occurred during shutdown.
	// Only valid after Done() is closed.
	Err() error
}

// HandlerResult contains the result of a single handler's shutdown.
type HandlerResult struct {
	// Name of the handler.
	Name string

	// Phase the handler was registered with.
	Phase int

	// Duration how long the handler took to shut down.
	Duration time.Duration

	// Err is any error returned by the handler.
	Err error
}

// ShutdownResult contains the complete shutdown result.
type ShutdownResult struct {
	// TotalDuration of the entire shutdown process.
	TotalDuration time.Duration

	// Results for each handler.
	Results []HandlerResult

	// Err is the overall error (nil if all handlers succeeded).
	Err error
}

// Failed returns true if any handler failed.
func (r *ShutdownResult) Failed() bool {
	return r.Err != nil
}

// FailedHandlers returns the names of handlers that failed.
func (r *ShutdownResult) FailedHandlers() []string {
	var failed []string
	for _, hr := range r.Results {
		if hr.Err != nil {
			failed = append(failed, hr.Name)
		}
	}
	return failed
}

// Config configures the shutdown coordinator.
type Config struct {
	// DefaultTimeout is used when ShutdownWithTimeout is called without a timeout.
	// Default: 30 seconds
	DefaultTimeout time.Duration

	// DefaultPhase is assigned to handlers registered without a phase.
	// Default: 100
	DefaultPhase int

	// ContinueOnError determines whether shutdown continues if a handler fails.
	// If true, subsequent handlers are still called.
	// Default: true
	ContinueOnError bool

	// OnProgress is called when each handler completes.
	// Can be used for logging.
	OnProgress func(result HandlerResult)
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.DefaultTimeout < 0 {
		return ErrInvalidConfig
	}
	return nil
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultTimeout:  30 * time.Second,
		DefaultPhase:    100,
		ContinueOnError: true,
	}
}

// registration holds a registered handler with its metadata.
type registration struct {
	name    string
	handler ShutdownHandler
	phase   int
}
