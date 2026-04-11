package shutdown

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBasicShutdown(t *testing.T) {
	seq := New(Defaults())

	called := false
	seq.Register("test", Func(func(ctx context.Context) error {
		called = true
		return nil
	}))

	if err := seq.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}

	select {
	case <-seq.Done():
	default:
		t.Fatal("expected Done to be closed")
	}
	if seq.Err() != nil {
		t.Fatalf("expected Err() nil, got %v", seq.Err())
	}

	summary := seq.Summary()
	if summary == nil {
		t.Fatal("expected Summary to be non-nil")
	}
	if len(summary.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(summary.Results))
	}
	if summary.Results[0].Name != "test" {
		t.Fatalf("expected name 'test', got %s", summary.Results[0].Name)
	}
	if summary.Failed() {
		t.Fatal("expected Failed() false")
	}
}

func TestPhaseOrdering(t *testing.T) {
	seq := New(Defaults())

	var order []int
	var mu sync.Mutex
	record := func(phase int) { mu.Lock(); order = append(order, phase); mu.Unlock() }

	seq.RegisterWithPhase("phase30", Func(func(ctx context.Context) error { record(30); return nil }), 30)
	seq.RegisterWithPhase("phase10", Func(func(ctx context.Context) error { record(10); return nil }), 10)
	seq.RegisterWithPhase("phase20", Func(func(ctx context.Context) error { record(20); return nil }), 20)

	if err := seq.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 || order[0] != 10 || order[1] != 20 || order[2] != 30 {
		t.Fatalf("expected [10, 20, 30], got %v", order)
	}
}

func TestConcurrentPhase(t *testing.T) {
	seq := New(Defaults())

	var wg sync.WaitGroup
	wg.Add(2)
	started := make(chan struct{}, 2)

	seq.RegisterWithPhase("h1", Func(func(ctx context.Context) error {
		started <- struct{}{}
		wg.Done()
		wg.Wait()
		return nil
	}), 10)
	seq.RegisterWithPhase("h2", Func(func(ctx context.Context) error {
		started <- struct{}{}
		wg.Done()
		wg.Wait()
		return nil
	}), 10)

	done := make(chan error)
	go func() { done <- seq.ShutdownWithTimeout(5 * time.Second) }()

	timeout := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-timeout:
			t.Fatal("handlers did not start concurrently")
		}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestTimeout(t *testing.T) {
	seq := New(Defaults())

	var ctxCancelled bool
	seq.Register("slow", Func(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			ctxCancelled = true
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}))

	start := time.Now()
	err := seq.ShutdownWithTimeout(100 * time.Millisecond)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("shutdown took too long: %v", time.Since(start))
	}
	if !ctxCancelled {
		t.Fatal("expected context to be cancelled")
	}
	if !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	seq := New(Defaults())

	var called bool
	seq.Register("test", Func(func(ctx context.Context) error {
		called = true
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := seq.Shutdown(ctx); !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if called {
		t.Fatal("expected handler not to be called with cancelled context")
	}
}

func TestHandlerError(t *testing.T) {
	cfg := Defaults()
	cfg.ContinueOnError = false
	seq := New(cfg)

	expected := errors.New("handler failed")
	seq.Register("failing", Func(func(ctx context.Context) error { return expected }))

	if err := seq.ShutdownWithTimeout(5 * time.Second); !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}

	summary := seq.Summary()
	if !summary.Failed() {
		t.Fatal("expected Failed() true")
	}
	failed := summary.FailedHandlers()
	if len(failed) != 1 || failed[0] != "failing" {
		t.Fatalf("expected ['failing'], got %v", failed)
	}
	if summary.Results[0].Err != expected {
		t.Fatalf("expected original error, got %v", summary.Results[0].Err)
	}
}

func TestContinueOnError(t *testing.T) {
	cfg := Defaults()
	cfg.ContinueOnError = true
	seq := New(cfg)

	var called []string
	var mu sync.Mutex
	record := func(name string) { mu.Lock(); called = append(called, name); mu.Unlock() }

	seq.RegisterWithPhase("h1", Func(func(ctx context.Context) error { record("h1"); return errors.New("h1 failed") }), 10)
	seq.RegisterWithPhase("h2", Func(func(ctx context.Context) error { record("h2"); return nil }), 20)
	seq.RegisterWithPhase("h3", Func(func(ctx context.Context) error { record("h3"); return errors.New("h3 failed") }), 30)

	if err := seq.ShutdownWithTimeout(5 * time.Second); !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}
	if len(called) != 3 {
		t.Fatalf("expected all 3 called, got %d: %v", len(called), called)
	}
	if len(seq.Summary().FailedHandlers()) != 2 {
		t.Fatalf("expected 2 failed, got %v", seq.Summary().FailedHandlers())
	}
}

func TestStopOnError(t *testing.T) {
	cfg := Defaults()
	cfg.ContinueOnError = false
	seq := New(cfg)

	var count int32
	seq.RegisterWithPhase("h1", Func(func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return errors.New("h1 failed")
	}), 10)
	seq.RegisterWithPhase("h2", Func(func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}), 20)

	if err := seq.ShutdownWithTimeout(5 * time.Second); !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("expected ErrHandlerFailed, got %v", err)
	}
	if atomic.LoadInt32(&count) != 1 {
		t.Fatalf("expected 1 handler called, got %d", atomic.LoadInt32(&count))
	}
}

func TestIdempotent(t *testing.T) {
	seq := New(Defaults())

	var count int32
	seq.Register("test", Func(func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}))

	if err := seq.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := seq.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if atomic.LoadInt32(&count) != 1 {
		t.Fatalf("expected handler called once, got %d", atomic.LoadInt32(&count))
	}
}

func TestIdempotentError(t *testing.T) {
	cfg := Defaults()
	cfg.ContinueOnError = false
	seq := New(cfg)

	seq.Register("failing", Func(func(ctx context.Context) error { return errors.New("failure") }))

	if err := seq.ShutdownWithTimeout(5 * time.Second); !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("first shutdown: expected ErrHandlerFailed, got %v", err)
	}
	if err := seq.ShutdownWithTimeout(5 * time.Second); !errors.Is(err, ErrHandlerFailed) {
		t.Fatalf("second shutdown: expected ErrHandlerFailed, got %v", err)
	}
}

func TestSignalHandling(t *testing.T) {
	seq := New(Config{
		DefaultTimeout:  1 * time.Second,
		DefaultPhase:    100,
		ContinueOnError: true,
	})

	var called bool
	seq.Register("test", Func(func(ctx context.Context) error {
		called = true
		return nil
	}))

	seq.HandleSignals()
	seq.(*sequence).trigger()

	select {
	case <-seq.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not complete after signal")
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
	if seq.Err() != nil {
		t.Fatalf("expected no error, got %v", seq.Err())
	}
}

func TestLateRegistration(t *testing.T) {
	seq := New(Defaults())

	started := make(chan struct{})
	proceed := make(chan struct{})
	var h1Called, h2Called bool

	seq.Register("h1", Func(func(ctx context.Context) error {
		h1Called = true
		close(started)
		<-proceed
		return nil
	}))

	done := make(chan error)
	go func() { done <- seq.ShutdownWithTimeout(5 * time.Second) }()

	<-started
	seq.Register("h2", Func(func(ctx context.Context) error {
		h2Called = true
		return nil
	}))
	close(proceed)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}
	if !h1Called {
		t.Fatal("expected h1 to be called")
	}
	if h2Called {
		t.Fatal("expected h2 NOT to be called (registered after shutdown snapshot)")
	}
}

func TestRegisterWithPhase(t *testing.T) {
	seq := New(Defaults())

	var called bool
	seq.RegisterWithPhase("mock", Func(func(ctx context.Context) error {
		called = true
		return nil
	}), 50)

	if err := seq.ShutdownWithTimeout(1 * time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
	if seq.Summary().Results[0].Phase != 50 {
		t.Fatalf("expected phase 50, got %d", seq.Summary().Results[0].Phase)
	}
}

func TestProgress(t *testing.T) {
	var results []Result
	var mu sync.Mutex

	cfg := Defaults()
	cfg.Progress = func(r Result) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}

	seq := New(cfg)
	seq.RegisterWithPhase("h1", Func(func(ctx context.Context) error { return nil }), 10)
	seq.RegisterWithPhase("h2", Func(func(ctx context.Context) error { return errors.New("failed") }), 20)

	seq.ShutdownWithTimeout(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(results) != 2 {
		t.Fatalf("expected 2 progress callbacks, got %d", len(results))
	}

	var h1, h2 *Result
	for i := range results {
		if results[i].Name == "h1" {
			h1 = &results[i]
		}
		if results[i].Name == "h2" {
			h2 = &results[i]
		}
	}
	if h1 == nil || h1.Err != nil {
		t.Fatal("expected h1 to succeed")
	}
	if h2 == nil || h2.Err == nil {
		t.Fatal("expected h2 to fail")
	}
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.DefaultTimeout != 30*time.Second {
		t.Fatalf("expected 30s, got %v", cfg.DefaultTimeout)
	}
	if cfg.DefaultPhase != 100 {
		t.Fatalf("expected 100, got %d", cfg.DefaultPhase)
	}
	if !cfg.ContinueOnError {
		t.Fatal("expected ContinueOnError true")
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	cfg.DefaultTimeout = -1 * time.Second
	if !errors.Is(cfg.Validate(), ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", cfg.Validate())
	}
}

func TestDefaultsApplied(t *testing.T) {
	seq := New(Config{})
	var called bool
	seq.Register("test", Func(func(ctx context.Context) error {
		called = true
		return nil
	}))
	if err := seq.ShutdownWithTimeout(0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestShutdownWithContext(t *testing.T) {
	seq := New(Defaults())
	var called bool
	seq.Register("test", Func(func(ctx context.Context) error {
		called = true
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := seq.Shutdown(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestSummaryBeforeDone(t *testing.T) {
	seq := New(Defaults())
	if seq.Summary() != nil {
		t.Fatal("expected Summary nil before shutdown")
	}
	if seq.Err() != nil {
		t.Fatal("expected Err nil before shutdown")
	}
}

func TestEmpty(t *testing.T) {
	seq := New(Defaults())
	if err := seq.ShutdownWithTimeout(1 * time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	summary := seq.Summary()
	if summary == nil || len(summary.Results) != 0 || summary.Failed() {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestReset(t *testing.T) {
	seq := New(Defaults())
	var count int32

	seq.Register("test", Func(func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}))
	if err := seq.ShutdownWithTimeout(1 * time.Second); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if atomic.LoadInt32(&count) != 1 {
		t.Fatalf("expected 1 call, got %d", atomic.LoadInt32(&count))
	}

	seq.(*sequence).reset()

	seq.Register("test2", Func(func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	}))
	if err := seq.ShutdownWithTimeout(1 * time.Second); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if atomic.LoadInt32(&count) != 2 {
		t.Fatalf("expected 2 calls after reset, got %d", atomic.LoadInt32(&count))
	}

	select {
	case <-seq.Done():
	default:
		t.Fatal("expected Done closed after second shutdown")
	}
}

func TestFuncAdapter(t *testing.T) {
	var called bool
	fn := Func(func(ctx context.Context) error {
		called = true
		return nil
	})

	var _ Handler = fn

	if err := fn.Shutdown(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected function to be called")
	}
}

func TestHandlerDuration(t *testing.T) {
	seq := New(Defaults())
	sleep := 100 * time.Millisecond
	seq.Register("sleepy", Func(func(ctx context.Context) error {
		time.Sleep(sleep)
		return nil
	}))

	seq.ShutdownWithTimeout(5 * time.Second)

	summary := seq.Summary()
	if len(summary.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(summary.Results))
	}
	if summary.Results[0].Duration < sleep {
		t.Fatalf("expected duration >= %v, got %v", sleep, summary.Results[0].Duration)
	}
	if summary.TotalDuration < sleep {
		t.Fatalf("expected total >= %v, got %v", sleep, summary.TotalDuration)
	}
}

func TestSamePhaseMultiple(t *testing.T) {
	seq := New(Defaults())
	var results []string
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		seq.RegisterWithPhase(name, Func(func(ctx context.Context) error {
			mu.Lock()
			results = append(results, name)
			mu.Unlock()
			return nil
		}), 10)
	}

	if err := seq.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 handlers called, got %d", len(results))
	}
}

func TestSummaryFailedHandlers(t *testing.T) {
	summary := &Summary{
		Results: []Result{
			{Name: "success", Err: nil},
			{Name: "fail1", Err: errors.New("e1")},
			{Name: "fail2", Err: errors.New("e2")},
		},
		Err: ErrHandlerFailed,
	}

	failed := summary.FailedHandlers()
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed, got %d", len(failed))
	}
	expected := map[string]bool{"fail1": true, "fail2": true}
	for _, name := range failed {
		if !expected[name] {
			t.Fatalf("unexpected failed handler: %s", name)
		}
	}
}

func TestSummaryNoFailures(t *testing.T) {
	summary := &Summary{
		Results: []Result{{Name: "s1"}, {Name: "s2"}},
	}
	if len(summary.FailedHandlers()) != 0 {
		t.Fatalf("expected 0 failed, got %v", summary.FailedHandlers())
	}
}

func TestSequenceInterface(t *testing.T) {
	var _ Sequence = (*sequence)(nil)
}

func TestDefaultPhase(t *testing.T) {
	cfg := Defaults()
	cfg.DefaultPhase = 50
	seq := New(cfg)

	seq.Register("test", Func(func(ctx context.Context) error { return nil }))
	seq.ShutdownWithTimeout(1 * time.Second)

	if seq.Summary().Results[0].Phase != 50 {
		t.Fatalf("expected phase 50, got %d", seq.Summary().Results[0].Phase)
	}
}

func TestGroupByPhaseEmpty(t *testing.T) {
	if groupByPhase(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
	if groupByPhase([]entry{}) != nil {
		t.Fatal("expected nil for empty slice")
	}
}

func TestGroupByPhase(t *testing.T) {
	handlers := []entry{
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
	if len(groups[0]) != 2 {
		t.Fatalf("expected 2 in phase 10, got %d", len(groups[0]))
	}
	if len(groups[1]) != 1 {
		t.Fatalf("expected 1 in phase 20, got %d", len(groups[1]))
	}
	if len(groups[2]) != 3 {
		t.Fatalf("expected 3 in phase 30, got %d", len(groups[2]))
	}
}
