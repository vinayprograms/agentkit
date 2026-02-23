package shutdown

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBasicShutdownWithSingleHandler tests basic shutdown with a single handler.
func TestBasicShutdownWithSingleHandler(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	called := false
	coord.RegisterFunc("test", func(ctx context.Context) error {
		called = true
		return nil
	})

	err := coord.ShutdownWithTimeout(5 * time.Second)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !called {
		t.Fatal("expected handler to be called")
	}

	// Verify Done channel is closed
	select {
	case <-coord.Done():
		// OK
	default:
		t.Fatal("expected Done channel to be closed")
	}

	// Verify Err returns nil
	if coord.Err() != nil {
		t.Fatalf("expected Err() to be nil, got %v", coord.Err())
	}

	// Verify Result
	result := coord.Result()
	if result == nil {
		t.Fatal("expected Result to be non-nil")
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Name != "test" {
		t.Fatalf("expected handler name 'test', got %s", result.Results[0].Name)
	}
	if result.Failed() {
		t.Fatal("expected result.Failed() to be false")
	}
}

// TestMultiPhaseOrderedShutdown tests that lower phases execute first.
func TestMultiPhaseOrderedShutdown(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var order []int
	var mu sync.Mutex

	record := func(phase int) {
		mu.Lock()
		order = append(order, phase)
		mu.Unlock()
	}

	// Register handlers in reverse phase order
	coord.RegisterFuncWithPhase("phase30", func(ctx context.Context) error {
		record(30)
		return nil
	}, 30)

	coord.RegisterFuncWithPhase("phase10", func(ctx context.Context) error {
		record(10)
		return nil
	}, 10)

	coord.RegisterFuncWithPhase("phase20", func(ctx context.Context) error {
		record(20)
		return nil
	}, 20)

	err := coord.ShutdownWithTimeout(5 * time.Second)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify order: 10 → 20 → 30
	if len(order) != 3 {
		t.Fatalf("expected 3 handlers called, got %d", len(order))
	}
	if order[0] != 10 || order[1] != 20 || order[2] != 30 {
		t.Fatalf("expected order [10, 20, 30], got %v", order)
	}
}

// TestConcurrentHandlersInSamePhase tests that handlers in the same phase run concurrently.
func TestConcurrentHandlersInSamePhase(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var wg sync.WaitGroup
	wg.Add(2)

	started := make(chan struct{}, 2)
	phase := 10

	// Two handlers in the same phase should run concurrently
	coord.RegisterFuncWithPhase("handler1", func(ctx context.Context) error {
		started <- struct{}{}
		wg.Done()
		wg.Wait() // Wait for both to start
		return nil
	}, phase)

	coord.RegisterFuncWithPhase("handler2", func(ctx context.Context) error {
		started <- struct{}{}
		wg.Done()
		wg.Wait() // Wait for both to start
		return nil
	}, phase)

	done := make(chan error)
	go func() {
		done <- coord.ShutdownWithTimeout(5 * time.Second)
	}()

	// Wait for both handlers to start
	timeout := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
			// OK
		case <-timeout:
			t.Fatal("handlers did not start concurrently")
		}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

// TestTimeoutHandling tests that handlers exceeding timeout receive context cancellation.
func TestTimeoutHandling(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var ctxCancelled bool

	coord.RegisterFunc("slow", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			ctxCancelled = true
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	start := time.Now()
	err := coord.ShutdownWithTimeout(100 * time.Millisecond)
	duration := time.Since(start)

	// Should have returned quickly due to timeout
	if duration > 500*time.Millisecond {
		t.Fatalf("shutdown took too long: %v", duration)
	}

	// Context should have been cancelled
	if !ctxCancelled {
		t.Fatal("expected context to be cancelled")
	}

	// Error should indicate handler failure
	if !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}
}

// TestContextCancellation tests shutdown with a cancelled context.
func TestContextCancellation(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var handlerCalled bool
	coord.RegisterFunc("test", func(ctx context.Context) error {
		handlerCalled = true
		return nil
	})

	// Create an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := coord.Shutdown(ctx)

	// With a pre-cancelled context, we should get ErrTimeout
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}

	// Handler should not be called due to early context cancellation
	if handlerCalled {
		t.Fatal("expected handler not to be called with cancelled context")
	}
}

// TestErrorHandling tests that handler errors are properly reported.
func TestErrorHandling(t *testing.T) {
	config := DefaultConfig()
	config.ContinueOnError = false
	coord := NewCoordinator(config)

	expectedErr := errors.New("handler failed")

	coord.RegisterFunc("failing", func(ctx context.Context) error {
		return expectedErr
	})

	err := coord.ShutdownWithTimeout(5 * time.Second)
	if !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}

	result := coord.Result()
	if result == nil {
		t.Fatal("expected Result to be non-nil")
	}
	if !result.Failed() {
		t.Fatal("expected result.Failed() to be true")
	}

	failed := result.FailedHandlers()
	if len(failed) != 1 || failed[0] != "failing" {
		t.Fatalf("expected ['failing'], got %v", failed)
	}

	if result.Results[0].Err != expectedErr {
		t.Fatalf("expected handler error to be preserved, got %v", result.Results[0].Err)
	}
}

// TestContinueOnError tests that shutdown continues after handler errors when configured.
func TestContinueOnError(t *testing.T) {
	config := DefaultConfig()
	config.ContinueOnError = true
	coord := NewCoordinator(config)

	var handlersCalled []string
	var mu sync.Mutex

	record := func(name string) {
		mu.Lock()
		handlersCalled = append(handlersCalled, name)
		mu.Unlock()
	}

	coord.RegisterFuncWithPhase("handler1", func(ctx context.Context) error {
		record("handler1")
		return errors.New("handler1 failed")
	}, 10)

	coord.RegisterFuncWithPhase("handler2", func(ctx context.Context) error {
		record("handler2")
		return nil
	}, 20)

	coord.RegisterFuncWithPhase("handler3", func(ctx context.Context) error {
		record("handler3")
		return errors.New("handler3 failed")
	}, 30)

	err := coord.ShutdownWithTimeout(5 * time.Second)

	// Should have ErrHandlerFailed since handlers failed
	if !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}

	// All handlers should have been called despite errors
	if len(handlersCalled) != 3 {
		t.Fatalf("expected all 3 handlers called, got %d: %v", len(handlersCalled), handlersCalled)
	}

	result := coord.Result()
	failed := result.FailedHandlers()
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed handlers, got %d: %v", len(failed), failed)
	}
}

// TestContinueOnErrorFalse tests that shutdown stops after first handler error when not continuing.
func TestContinueOnErrorFalse(t *testing.T) {
	config := DefaultConfig()
	config.ContinueOnError = false
	coord := NewCoordinator(config)

	var handlersCalled int32

	coord.RegisterFuncWithPhase("handler1", func(ctx context.Context) error {
		atomic.AddInt32(&handlersCalled, 1)
		return errors.New("handler1 failed")
	}, 10)

	coord.RegisterFuncWithPhase("handler2", func(ctx context.Context) error {
		atomic.AddInt32(&handlersCalled, 1)
		return nil
	}, 20)

	err := coord.ShutdownWithTimeout(5 * time.Second)

	// Should have ErrHandlerFailed
	if !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}

	// Only first handler should have been called
	if atomic.LoadInt32(&handlersCalled) != 1 {
		t.Fatalf("expected only 1 handler called, got %d", atomic.LoadInt32(&handlersCalled))
	}
}

// TestDoubleShutdown tests that calling Shutdown twice is idempotent.
func TestDoubleShutdown(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var callCount int32
	coord.RegisterFunc("test", func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// First shutdown
	err1 := coord.ShutdownWithTimeout(5 * time.Second)
	if err1 != nil {
		t.Fatalf("expected no error on first shutdown, got %v", err1)
	}

	// Second shutdown should return the same result (nil error)
	err2 := coord.ShutdownWithTimeout(5 * time.Second)
	if err2 != nil {
		t.Fatalf("expected no error on second shutdown, got %v", err2)
	}

	// Handler should only be called once
	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("expected handler called once, got %d", atomic.LoadInt32(&callCount))
	}
}

// TestDoubleShutdownWithError tests double shutdown when first shutdown had an error.
func TestDoubleShutdownWithError(t *testing.T) {
	config := DefaultConfig()
	config.ContinueOnError = false
	coord := NewCoordinator(config)

	coord.RegisterFunc("failing", func(ctx context.Context) error {
		return errors.New("failure")
	})

	err1 := coord.ShutdownWithTimeout(5 * time.Second)
	if !errors.Is(err1, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed on first shutdown, got %v", err1)
	}

	// Second shutdown should return the same error
	err2 := coord.ShutdownWithTimeout(5 * time.Second)
	if !errors.Is(err2, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed on second shutdown, got %v", err2)
	}
}

// TestSignalHandling tests the HandleSignals and Trigger functionality.
func TestSignalHandling(t *testing.T) {
	coord := NewCoordinator(Config{
		DefaultTimeout:  1 * time.Second,
		DefaultPhase:    100,
		ContinueOnError: true,
	})

	var handlerCalled bool
	coord.RegisterFunc("test", func(ctx context.Context) error {
		handlerCalled = true
		return nil
	})

	coord.HandleSignals()

	// Trigger shutdown via signal simulation
	coord.Trigger()

	// Wait for shutdown to complete
	select {
	case <-coord.Done():
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not complete after signal trigger")
	}

	if !handlerCalled {
		t.Fatal("expected handler to be called")
	}

	if coord.Err() != nil {
		t.Fatalf("expected no error, got %v", coord.Err())
	}
}

// TestRegistrationAfterShutdownStarted tests that handlers can still be registered
// but the snapshot used during shutdown is fixed.
func TestRegistrationAfterShutdownStarted(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	started := make(chan struct{})
	proceed := make(chan struct{})

	var handler1Called, handler2Called bool

	coord.RegisterFunc("handler1", func(ctx context.Context) error {
		handler1Called = true
		close(started)
		<-proceed
		return nil
	})

	// Start shutdown in background
	shutdownDone := make(chan error)
	go func() {
		shutdownDone <- coord.ShutdownWithTimeout(5 * time.Second)
	}()

	// Wait for handler1 to start
	<-started

	// Try to register another handler during shutdown
	coord.RegisterFunc("handler2", func(ctx context.Context) error {
		handler2Called = true
		return nil
	})

	// Allow shutdown to complete
	close(proceed)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}

	// handler1 should have been called
	if !handler1Called {
		t.Fatal("expected handler1 to be called")
	}

	// handler2 was registered after snapshot was taken, so it should NOT be called
	if handler2Called {
		t.Fatal("expected handler2 NOT to be called (registered after shutdown started)")
	}
}

// TestRegisterWithPhase tests RegisterWithPhase method.
func TestRegisterWithPhase(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	type mockHandler struct {
		called bool
	}

	handler := &mockHandler{}
	handlerImpl := ShutdownFunc(func(ctx context.Context) error {
		handler.called = true
		return nil
	})

	coord.RegisterWithPhase("mock", handlerImpl, 50)

	err := coord.ShutdownWithTimeout(1 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handler.called {
		t.Fatal("expected handler to be called")
	}

	result := coord.Result()
	if result.Results[0].Phase != 50 {
		t.Fatalf("expected phase 50, got %d", result.Results[0].Phase)
	}
}

// TestOnProgress tests the progress callback.
func TestOnProgress(t *testing.T) {
	var progressResults []HandlerResult
	var mu sync.Mutex

	config := DefaultConfig()
	config.OnProgress = func(result HandlerResult) {
		mu.Lock()
		progressResults = append(progressResults, result)
		mu.Unlock()
	}

	coord := NewCoordinator(config)

	coord.RegisterFuncWithPhase("handler1", func(ctx context.Context) error {
		return nil
	}, 10)

	coord.RegisterFuncWithPhase("handler2", func(ctx context.Context) error {
		return errors.New("failed")
	}, 20)

	coord.ShutdownWithTimeout(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(progressResults) != 2 {
		t.Fatalf("expected 2 progress callbacks, got %d", len(progressResults))
	}

	// Find handler results (order may vary due to concurrency within phases)
	var h1, h2 *HandlerResult
	for i := range progressResults {
		if progressResults[i].Name == "handler1" {
			h1 = &progressResults[i]
		}
		if progressResults[i].Name == "handler2" {
			h2 = &progressResults[i]
		}
	}

	if h1 == nil || h1.Err != nil {
		t.Fatal("expected handler1 to succeed")
	}
	if h2 == nil || h2.Err == nil {
		t.Fatal("expected handler2 to fail")
	}
}

// TestDefaultConfig tests default configuration values.
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.DefaultTimeout != 30*time.Second {
		t.Fatalf("expected default timeout 30s, got %v", config.DefaultTimeout)
	}

	if config.DefaultPhase != 100 {
		t.Fatalf("expected default phase 100, got %d", config.DefaultPhase)
	}

	if !config.ContinueOnError {
		t.Fatal("expected ContinueOnError to be true by default")
	}
}

// TestConfigValidate tests configuration validation.
func TestConfigValidate(t *testing.T) {
	// Valid config
	config := DefaultConfig()
	if err := config.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	// Invalid config with negative timeout
	config.DefaultTimeout = -1 * time.Second
	if err := config.Validate(); err == nil {
		t.Fatal("expected error for negative timeout")
	}
	if !errors.Is(config.Validate(), ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", config.Validate())
	}
}

// TestNewCoordinatorDefaults tests that NewCoordinator applies defaults.
func TestNewCoordinatorDefaults(t *testing.T) {
	// Pass empty config
	coord := NewCoordinator(Config{})

	// Should have default timeout applied
	var handlerCalled bool
	coord.RegisterFunc("test", func(ctx context.Context) error {
		handlerCalled = true
		return nil
	})

	err := coord.ShutdownWithTimeout(0) // Uses default timeout
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Fatal("expected handler to be called")
	}
}

// TestShutdownWithContext tests Shutdown with a custom context.
func TestShutdownWithContext(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var handlerCalled bool
	coord.RegisterFunc("test", func(ctx context.Context) error {
		handlerCalled = true
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := coord.Shutdown(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Fatal("expected handler to be called")
	}
}

// TestResultBeforeDone tests that Result returns nil before shutdown completes.
func TestResultBeforeDone(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	// Before shutdown, Result should be nil
	if coord.Result() != nil {
		t.Fatal("expected Result to be nil before shutdown")
	}

	// Before shutdown, Err should be nil
	if coord.Err() != nil {
		t.Fatal("expected Err to be nil before shutdown")
	}
}

// TestEmptyShutdown tests shutdown with no registered handlers.
func TestEmptyShutdown(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	err := coord.ShutdownWithTimeout(1 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := coord.Result()
	if result == nil {
		t.Fatal("expected Result to be non-nil")
	}
	if len(result.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result.Results))
	}
	if result.Failed() {
		t.Fatal("expected result.Failed() to be false")
	}
}

// TestReset tests the Reset functionality.
func TestReset(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var callCount int32
	coord.RegisterFunc("test", func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// First shutdown
	err := coord.ShutdownWithTimeout(1 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("expected 1 call, got %d", atomic.LoadInt32(&callCount))
	}

	// Reset
	coord.Reset()

	// Register a new handler
	coord.RegisterFunc("test2", func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// Second shutdown after reset
	err = coord.ShutdownWithTimeout(1 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error after reset: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Fatalf("expected 2 calls after reset, got %d", atomic.LoadInt32(&callCount))
	}

	// Done channel should be closed again
	select {
	case <-coord.Done():
		// OK
	default:
		t.Fatal("expected Done channel to be closed after second shutdown")
	}
}

// TestShutdownFuncInterface tests that ShutdownFunc correctly implements ShutdownHandler.
func TestShutdownFuncInterface(t *testing.T) {
	var called bool
	fn := ShutdownFunc(func(ctx context.Context) error {
		called = true
		return nil
	})

	// Verify it implements the interface
	var _ ShutdownHandler = fn

	err := fn.OnShutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected function to be called")
	}
}

// TestHandlerDuration tests that handler duration is recorded correctly.
func TestHandlerDuration(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	sleepDuration := 100 * time.Millisecond
	coord.RegisterFunc("sleepy", func(ctx context.Context) error {
		time.Sleep(sleepDuration)
		return nil
	})

	coord.ShutdownWithTimeout(5 * time.Second)

	result := coord.Result()
	if result == nil {
		t.Fatal("expected Result to be non-nil")
	}

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}

	// Duration should be at least sleepDuration
	if result.Results[0].Duration < sleepDuration {
		t.Fatalf("expected duration >= %v, got %v", sleepDuration, result.Results[0].Duration)
	}

	// Total duration should also be at least sleepDuration
	if result.TotalDuration < sleepDuration {
		t.Fatalf("expected total duration >= %v, got %v", sleepDuration, result.TotalDuration)
	}
}

// TestMultipleHandlersSamePhase tests multiple handlers in the same phase.
func TestMultipleHandlersSamePhase(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	var results []string
	var mu sync.Mutex

	// All handlers in the same phase
	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		coord.RegisterFuncWithPhase(name, func(ctx context.Context) error {
			mu.Lock()
			results = append(results, name)
			mu.Unlock()
			return nil
		}, 10)
	}

	err := coord.ShutdownWithTimeout(5 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// All 5 handlers should have been called
	if len(results) != 5 {
		t.Fatalf("expected 5 handlers called, got %d", len(results))
	}
}

// TestShutdownResultFailedHandlers tests FailedHandlers method.
func TestShutdownResultFailedHandlers(t *testing.T) {
	result := &ShutdownResult{
		Results: []HandlerResult{
			{Name: "success", Err: nil},
			{Name: "fail1", Err: errors.New("error1")},
			{Name: "fail2", Err: errors.New("error2")},
		},
		Err: ErrHandlerFailed,
	}

	failed := result.FailedHandlers()
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed handlers, got %d", len(failed))
	}

	// Check that the correct handlers are reported as failed
	expectedFailed := map[string]bool{"fail1": true, "fail2": true}
	for _, name := range failed {
		if !expectedFailed[name] {
			t.Fatalf("unexpected failed handler: %s", name)
		}
	}
}

// TestShutdownResultFailedEmpty tests FailedHandlers when no handlers failed.
func TestShutdownResultFailedEmpty(t *testing.T) {
	result := &ShutdownResult{
		Results: []HandlerResult{
			{Name: "success1", Err: nil},
			{Name: "success2", Err: nil},
		},
		Err: nil,
	}

	failed := result.FailedHandlers()
	if len(failed) != 0 {
		t.Fatalf("expected 0 failed handlers, got %d: %v", len(failed), failed)
	}
}

// TestCoordinatorInterface tests that Coordinator implements ShutdownCoordinator.
func TestCoordinatorInterface(t *testing.T) {
	var _ ShutdownCoordinator = (*Coordinator)(nil)
}

// TestDefaultPhaseUsedWhenNotSpecified tests that default phase is used.
func TestDefaultPhaseUsedWhenNotSpecified(t *testing.T) {
	config := DefaultConfig()
	config.DefaultPhase = 50
	coord := NewCoordinator(config)

	coord.RegisterFunc("test", func(ctx context.Context) error {
		return nil
	})

	coord.ShutdownWithTimeout(1 * time.Second)

	result := coord.Result()
	if result.Results[0].Phase != 50 {
		t.Fatalf("expected phase 50, got %d", result.Results[0].Phase)
	}
}

// TestGroupByPhaseEmpty tests groupByPhase with empty input.
func TestGroupByPhaseEmpty(t *testing.T) {
	groups := groupByPhase(nil)
	if groups != nil {
		t.Fatalf("expected nil for empty input, got %v", groups)
	}

	groups = groupByPhase([]registration{})
	if groups != nil {
		t.Fatalf("expected nil for empty slice, got %v", groups)
	}
}

// TestGroupByPhase tests groupByPhase function.
func TestGroupByPhase(t *testing.T) {
	handlers := []registration{
		{name: "a", phase: 10},
		{name: "b", phase: 10},
		{name: "c", phase: 20},
		{name: "d", phase: 30},
		{name: "e", phase: 30},
		{name: "f", phase: 30},
	}

	groups := groupByPhase(handlers)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	// Phase 10 should have 2 handlers
	if len(groups[0]) != 2 {
		t.Fatalf("expected 2 handlers in phase 10, got %d", len(groups[0]))
	}

	// Phase 20 should have 1 handler
	if len(groups[1]) != 1 {
		t.Fatalf("expected 1 handler in phase 20, got %d", len(groups[1]))
	}

	// Phase 30 should have 3 handlers
	if len(groups[2]) != 3 {
		t.Fatalf("expected 3 handlers in phase 30, got %d", len(groups[2]))
	}
}
