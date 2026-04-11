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

// Sequence manages graceful shutdown for multiple components.
// Handlers are called in phase order; lower phase numbers run first.
// Handlers within the same phase run concurrently.
//
// Use New to create a Sequence. Pass handlers via Register or RegisterWithPhase.
// Use shutdown.Func to adapt plain functions.
type Sequence interface {
	// Register adds a handler at the default phase.
	Register(name string, h Handler)

	// RegisterWithPhase adds a handler at a specific phase.
	// Lower phase numbers are stopped first.
	RegisterWithPhase(name string, h Handler, phase int)

	// Shutdown initiates graceful shutdown, calling all handlers in phase order.
	// Returns ErrAlreadyShutdown if already complete. The context controls
	// the total time budget; handlers receive this same context.
	Shutdown(ctx context.Context) error

	// ShutdownWithTimeout initiates shutdown with a timeout.
	// Uses DefaultTimeout when timeout is zero.
	ShutdownWithTimeout(timeout time.Duration) error

	// HandleSignals registers for SIGTERM and SIGINT. When a signal arrives,
	// Shutdown is called with the configured DefaultTimeout.
	HandleSignals()

	// Done returns a channel closed when shutdown is complete.
	Done() <-chan struct{}

	// Err returns any error from shutdown. Valid only after Done is closed.
	Err() error

	// Summary returns the per-handler outcome. Valid only after Done is closed.
	Summary() *Summary
}

// sequence implements Sequence.
type sequence struct {
	config Config

	mu           sync.Mutex
	handlers     []entry
	shutdownOnce sync.Once
	shutdownErr  error
	done         chan struct{}
	summary      *Summary
	signalChan   chan os.Signal
	start        time.Time
}

// New creates a new shutdown sequence with the given configuration.
// Zero-value Config fields are filled from Defaults.
func New(cfg Config) Sequence {
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = Defaults().DefaultTimeout
	}
	if cfg.DefaultPhase == 0 {
		cfg.DefaultPhase = Defaults().DefaultPhase
	}

	return &sequence{
		config:     cfg,
		handlers:   make([]entry, 0),
		done:       make(chan struct{}),
		signalChan: make(chan os.Signal, 1),
	}
}

// Register adds a handler at the default phase.
func (s *sequence) Register(name string, h Handler) {
	s.RegisterWithPhase(name, h, s.config.DefaultPhase)
}

// RegisterWithPhase adds a handler at a specific phase.
func (s *sequence) RegisterWithPhase(name string, h Handler, phase int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handlers = append(s.handlers, entry{name: name, handler: h, phase: phase})
}

// Shutdown initiates graceful shutdown.
func (s *sequence) Shutdown(ctx context.Context) error {
	var err error
	s.shutdownOnce.Do(func() {
		s.start = time.Now()
		err = s.run(ctx)
		s.shutdownErr = err
		close(s.done)
	})

	select {
	case <-s.done:
		return s.shutdownErr
	default:
		return ErrAlreadyShutdown
	}
}

// ShutdownWithTimeout initiates shutdown with a timeout.
func (s *sequence) ShutdownWithTimeout(timeout time.Duration) error {
	if timeout == 0 {
		timeout = s.config.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.Shutdown(ctx)
}

// HandleSignals registers for SIGTERM and SIGINT.
func (s *sequence) HandleSignals() {
	signal.Notify(s.signalChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-s.signalChan
		ctx, cancel := context.WithTimeout(context.Background(), s.config.DefaultTimeout)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()
}

// Done returns a channel closed when shutdown is complete.
func (s *sequence) Done() <-chan struct{} {
	return s.done
}

// Err returns any error from shutdown.
func (s *sequence) Err() error {
	select {
	case <-s.done:
		return s.shutdownErr
	default:
		return nil
	}
}

// Summary returns the detailed shutdown summary.
func (s *sequence) Summary() *Summary {
	select {
	case <-s.done:
		return s.summary
	default:
		return nil
	}
}

// run performs the shutdown sequence.
func (s *sequence) run(ctx context.Context) error {
	s.mu.Lock()
	handlers := make([]entry, len(s.handlers))
	copy(handlers, s.handlers)
	s.mu.Unlock()

	sort.Slice(handlers, func(i, j int) bool {
		return handlers[i].phase < handlers[j].phase
	})

	groups := groupByPhase(handlers)
	sum := &Summary{Results: make([]Result, 0, len(handlers))}
	var overallErr error

	for _, group := range groups {
		select {
		case <-ctx.Done():
			sum.Err = ErrTimeout
			sum.TotalDuration = time.Since(s.start)
			s.summary = sum
			return ErrTimeout
		default:
		}

		results := s.execute(ctx, group)
		sum.Results = append(sum.Results, results...)

		for _, r := range results {
			if r.Err != nil && overallErr == nil {
				overallErr = ErrHandlerFailed
			}
			if !s.config.ContinueOnError && r.Err != nil {
				sum.Err = overallErr
				sum.TotalDuration = time.Since(s.start)
				s.summary = sum
				return overallErr
			}
		}
	}

	sum.Err = overallErr
	sum.TotalDuration = time.Since(s.start)
	s.summary = sum
	return overallErr
}

// execute runs all handlers in a phase concurrently.
func (s *sequence) execute(ctx context.Context, handlers []entry) []Result {
	results := make([]Result, len(handlers))
	var wg sync.WaitGroup

	for i, e := range handlers {
		wg.Add(1)
		go func(idx int, e entry) {
			defer wg.Done()

			start := time.Now()
			err := e.handler.Shutdown(ctx)
			r := Result{
				Name:     e.name,
				Phase:    e.phase,
				Duration: time.Since(start),
				Err:      err,
			}
			results[idx] = r

			if s.config.Progress != nil {
				s.config.Progress(r)
			}
		}(i, e)
	}

	wg.Wait()
	return results
}

// groupByPhase groups entries by phase. Assumes entries are sorted by phase.
func groupByPhase(handlers []entry) [][]entry {
	if len(handlers) == 0 {
		return nil
	}

	var groups [][]entry
	var current []entry
	currentPhase := handlers[0].phase

	for _, h := range handlers {
		if h.phase != currentPhase {
			groups = append(groups, current)
			current = nil
			currentPhase = h.phase
		}
		current = append(current, h)
	}

	if len(current) > 0 {
		groups = append(groups, current)
	}

	return groups
}

// trigger simulates a shutdown signal. For testing only.
func (s *sequence) trigger() {
	select {
	case s.signalChan <- syscall.SIGTERM:
	default:
	}
}

// reset clears all state so the sequence can be reused. For testing only.
func (s *sequence) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handlers = make([]entry, 0)
	s.shutdownOnce = sync.Once{}
	s.shutdownErr = nil
	s.done = make(chan struct{})
	s.summary = nil
}
