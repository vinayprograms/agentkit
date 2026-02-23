package shutdown

import (
	"context"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Coordinator implements ShutdownCoordinator.
type Coordinator struct {
	config Config

	mu            sync.Mutex
	handlers      []registration
	shutdownOnce  sync.Once
	shutdownErr   error
	done          chan struct{}
	result        *ShutdownResult
	signalChan    chan os.Signal
	shutdownStart time.Time
}

// NewCoordinator creates a new shutdown coordinator.
func NewCoordinator(config Config) *Coordinator {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = DefaultConfig().DefaultTimeout
	}
	if config.DefaultPhase == 0 {
		config.DefaultPhase = DefaultConfig().DefaultPhase
	}

	return &Coordinator{
		config:     config,
		handlers:   make([]registration, 0),
		done:       make(chan struct{}),
		signalChan: make(chan os.Signal, 1),
	}
}

// Register adds a handler to be called during shutdown.
func (c *Coordinator) Register(name string, handler ShutdownHandler) {
	c.RegisterWithPhase(name, handler, c.config.DefaultPhase)
}

// RegisterWithPhase adds a handler with a specific phase.
func (c *Coordinator) RegisterWithPhase(name string, handler ShutdownHandler, phase int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.handlers = append(c.handlers, registration{
		name:    name,
		handler: handler,
		phase:   phase,
	})
}

// RegisterFunc is a convenience method for registering a function as a handler.
func (c *Coordinator) RegisterFunc(name string, fn func(ctx context.Context) error) {
	c.Register(name, ShutdownFunc(fn))
}

// RegisterFuncWithPhase is a convenience method for registering a function with a phase.
func (c *Coordinator) RegisterFuncWithPhase(name string, fn func(ctx context.Context) error, phase int) {
	c.RegisterWithPhase(name, ShutdownFunc(fn), phase)
}

// Shutdown initiates graceful shutdown.
func (c *Coordinator) Shutdown(ctx context.Context) error {
	var err error
	c.shutdownOnce.Do(func() {
		c.shutdownStart = time.Now()
		err = c.doShutdown(ctx)
		c.shutdownErr = err
		close(c.done)
	})

	// If called again, return the appropriate error
	select {
	case <-c.done:
		return c.shutdownErr
	default:
		return ErrAlreadyShutdown
	}
}

// ShutdownWithTimeout initiates shutdown with a timeout.
func (c *Coordinator) ShutdownWithTimeout(timeout time.Duration) error {
	if timeout == 0 {
		timeout = c.config.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.Shutdown(ctx)
}

// HandleSignals registers handlers for SIGTERM and SIGINT.
func (c *Coordinator) HandleSignals() {
	signal.Notify(c.signalChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-c.signalChan
		_ = sig // signal received
		_ = c.ShutdownWithTimeout(c.config.DefaultTimeout)
	}()
}

// Done returns a channel that is closed when shutdown is complete.
func (c *Coordinator) Done() <-chan struct{} {
	return c.done
}

// Err returns any error that occurred during shutdown.
func (c *Coordinator) Err() error {
	select {
	case <-c.done:
		return c.shutdownErr
	default:
		return nil
	}
}

// Result returns the detailed shutdown result.
// Only valid after Done() is closed.
func (c *Coordinator) Result() *ShutdownResult {
	select {
	case <-c.done:
		return c.result
	default:
		return nil
	}
}

// doShutdown performs the actual shutdown sequence.
func (c *Coordinator) doShutdown(ctx context.Context) error {
	c.mu.Lock()
	handlers := make([]registration, len(c.handlers))
	copy(handlers, c.handlers)
	c.mu.Unlock()

	// Sort handlers by phase
	sort.Slice(handlers, func(i, j int) bool {
		return handlers[i].phase < handlers[j].phase
	})

	// Group handlers by phase
	phaseGroups := groupByPhase(handlers)

	result := &ShutdownResult{
		Results: make([]HandlerResult, 0, len(handlers)),
	}

	var overallErr error

	// Execute each phase in order
	for _, group := range phaseGroups {
		// Check if context is already cancelled
		select {
		case <-ctx.Done():
			result.Err = ErrTimeout
			result.TotalDuration = time.Since(c.shutdownStart)
			c.result = result
			return ErrTimeout
		default:
		}

		// Execute all handlers in this phase concurrently
		phaseResults := c.executePhase(ctx, group)
		result.Results = append(result.Results, phaseResults...)

		// Check for errors
		for _, hr := range phaseResults {
			if hr.Err != nil && overallErr == nil {
				overallErr = ErrHandlerFailed
			}
			if !c.config.ContinueOnError && hr.Err != nil {
				result.Err = overallErr
				result.TotalDuration = time.Since(c.shutdownStart)
				c.result = result
				return overallErr
			}
		}
	}

	result.Err = overallErr
	result.TotalDuration = time.Since(c.shutdownStart)
	c.result = result
	return overallErr
}

// executePhase runs all handlers in a phase concurrently.
func (c *Coordinator) executePhase(ctx context.Context, handlers []registration) []HandlerResult {
	results := make([]HandlerResult, len(handlers))
	var wg sync.WaitGroup

	for i, reg := range handlers {
		wg.Add(1)
		go func(idx int, r registration) {
			defer wg.Done()

			start := time.Now()
			err := r.handler.OnShutdown(ctx)
			duration := time.Since(start)

			hr := HandlerResult{
				Name:     r.name,
				Phase:    r.phase,
				Duration: duration,
				Err:      err,
			}
			results[idx] = hr

			if c.config.OnProgress != nil {
				c.config.OnProgress(hr)
			}
		}(i, reg)
	}

	wg.Wait()
	return results
}

// groupByPhase groups handlers by their phase number.
func groupByPhase(handlers []registration) [][]registration {
	if len(handlers) == 0 {
		return nil
	}

	// Handlers are already sorted by phase
	var groups [][]registration
	var currentGroup []registration
	currentPhase := handlers[0].phase

	for _, h := range handlers {
		if h.phase != currentPhase {
			groups = append(groups, currentGroup)
			currentGroup = nil
			currentPhase = h.phase
		}
		currentGroup = append(currentGroup, h)
	}

	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	return groups
}

// Trigger manually triggers shutdown (useful for testing).
func (c *Coordinator) Trigger() {
	select {
	case c.signalChan <- syscall.SIGTERM:
	default:
	}
}

// Reset resets the coordinator for reuse (mainly for testing).
// This is not safe to call during an active shutdown.
func (c *Coordinator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.handlers = make([]registration, 0)
	c.shutdownOnce = sync.Once{}
	c.shutdownErr = nil
	c.done = make(chan struct{})
	c.result = nil
}
