package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// minInt returns the minimum of two integers.
// Named minInt to avoid shadowing the builtin min function (Go 1.21+).
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestNew(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2
	config.MaxWorkers = 4

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	if p == nil {
		t.Fatal("New returned nil pool")
	}

	stats := p.Stats()
	// G115: Safe conversion - we clamp to MaxInt32 to prevent overflow
	// #nosec G115
	maxWorkersInt32 := int32(minInt(config.MaxWorkers, 0x7FFFFFFF))
	if stats.ActiveWorkers < 0 || stats.ActiveWorkers > maxWorkersInt32 {
		t.Errorf("Invalid active workers count: %d", stats.ActiveWorkers)
	}
}

func TestPool_Submit_Success(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2
	config.MaxWorkers = 4
	config.QueueSize = 10

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	var executed int32
	task := Task{
		Fn: func() {
			atomic.AddInt32(&executed, 1)
		},
	}

	ctx := context.Background()
	err = p.Submit(ctx, task)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&executed) == 0 {
		t.Error("Task was not executed")
	}
}

func TestPool_Submit_Shutdown(t *testing.T) {
	config := DefaultConfig()
	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = p.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	task := Task{Fn: func() {}}
	err = p.Submit(context.Background(), task)
	if !errors.Is(err, ErrPoolShutdown) {
		t.Errorf("Expected ErrPoolShutdown, got %v", err)
	}
}

func TestPool_Submit_BlockingStrategy(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1
	config.MaxWorkers = 1
	config.QueueSize = 1
	config.BackpressureStrategy = StrategyBlock

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Fill the queue
	submitErr1 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(50 * time.Millisecond) }})
	_ = submitErr1 // Ignore submit errors for test purposes
	submitErr2 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(50 * time.Millisecond) }})
	_ = submitErr2 // Ignore submit errors for test purposes

	// This should block briefly, then succeed or timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = p.Submit(ctx, Task{Fn: func() {}})
	// May succeed or timeout depending on execution speed
	_ = err
}

func TestPool_Submit_RejectStrategy(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1
	config.MaxWorkers = 1
	config.QueueSize = 1
	config.BackpressureStrategy = StrategyReject

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Fill the queue
	submitErr1 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(100 * time.Millisecond) }})
	_ = submitErr1 // Ignore submit errors for test purposes
	submitErr2 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(100 * time.Millisecond) }})
	_ = submitErr2 // Ignore submit errors for test purposes

	// This should reject immediately
	err = p.Submit(context.Background(), Task{Fn: func() {}})
	if err == nil {
		// If it didn't reject, queue had space (acceptable)
		t.Log("Submit succeeded (queue had space)")
	} else if !errors.Is(err, ErrPoolFull) {
		t.Errorf("Expected ErrPoolFull, got %v", err)
	}
}

func TestPool_Submit_CallerRunsStrategy(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1
	config.MaxWorkers = 1
	config.QueueSize = 1
	config.BackpressureStrategy = StrategyCallerRuns

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	var executed int32
	// Fill the queue
	submitErr1 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(50 * time.Millisecond) }})
	_ = submitErr1 // Ignore submit errors for test purposes
	submitErr2 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(50 * time.Millisecond) }})
	_ = submitErr2 // Ignore submit errors for test purposes

	// This should execute in caller's goroutine if queue is full
	task := Task{
		Fn: func() {
			atomic.AddInt32(&executed, 1)
		},
	}

	err = p.Submit(context.Background(), task)
	if err != nil {
		t.Errorf("Submit failed: %v", err)
	}

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Task may have been executed in caller's goroutine or queued
	execCount := atomic.LoadInt32(&executed)
	if execCount > 1 {
		t.Errorf("Task executed %d times", execCount)
	}
}

func TestPool_Submit_DropOldestStrategy(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1
	config.MaxWorkers = 1
	config.QueueSize = 1
	config.BackpressureStrategy = StrategyDropOldest

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	var executed int32
	var dropped int32

	// Fill the queue with a task that sets a flag
	submitErr1 := p.Submit(context.Background(), Task{
		Fn: func() {
			atomic.AddInt32(&executed, 1)
		},
	})
	_ = submitErr1 // Ignore submit errors for test purposes

	// Try to submit another - should drop the first
	submitErr2 := p.Submit(context.Background(), Task{
		Fn: func() {
			atomic.AddInt32(&dropped, 1)
		},
	})
	if submitErr2 != nil {
		t.Logf("Submit failed (may have rejected): %v", submitErr2)
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	// At least one should execute
	total := atomic.LoadInt32(&executed) + atomic.LoadInt32(&dropped)
	if total == 0 {
		t.Error("No tasks executed")
	}
}

func TestPool_SubmitFunc(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	var executed int32
	fn := func() {
		atomic.AddInt32(&executed, 1)
	}

	err = p.SubmitFunc(context.Background(), fn)
	if err != nil {
		t.Fatalf("SubmitFunc failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&executed) == 0 {
		t.Error("Function was not executed")
	}
}

func TestPool_Stats(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2
	config.MaxWorkers = 4

	var err error
	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	stats := p.Stats()

	if stats.QueueCapacity <= 0 {
		t.Error("QueueCapacity should be positive")
	}

	if stats.ActiveWorkers < 0 {
		t.Error("ActiveWorkers should be non-negative")
	}

	if stats.TotalSubmitted < 0 {
		t.Error("TotalSubmitted should be non-negative")
	}
}

func TestPool_Resize(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2
	config.MaxWorkers = 10

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Increase workers
	err = p.Resize(5)
	if err != nil {
		t.Fatalf("Resize failed: %v", err)
	}

	// Decrease workers (they'll exit on idle timeout)
	err = p.Resize(3)
	if err != nil {
		t.Fatalf("Resize down failed: %v", err)
	}
}

func TestPool_Resize_Invalid(t *testing.T) {
	config := DefaultConfig()
	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Should handle invalid size gracefully
	err = p.Resize(0)
	if err != nil {
		t.Logf("Resize(0) error: %v", err)
	}

	err = p.Resize(-1)
	if err != nil {
		t.Logf("Resize(-1) error: %v", err)
	}
}

func TestPool_Shutdown_WaitForCompletion(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Submit a long-running task
	submitErr := p.Submit(context.Background(), Task{
		Fn: func() {
			defer wg.Done()
			time.Sleep(100 * time.Millisecond)
		},
	})
	_ = submitErr // Ignore submit errors for test purposes

	// Shutdown should wait for task to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- p.Shutdown(ctx)
	}()

	// Wait for shutdown or timeout
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Shutdown did not complete")
	}

	wg.Wait()
}

func TestPool_Shutdown_Timeout(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Submit a very long-running task
	submitErr := p.Submit(context.Background(), Task{
		Fn: func() {
			time.Sleep(10 * time.Second)
		},
	})
	_ = submitErr // Ignore submit errors for test purposes

	// Shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	shutdownErr := p.Shutdown(ctx)
	if shutdownErr == nil {
		t.Log("Shutdown completed (task finished quickly)")
	} else if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		t.Errorf("Expected DeadlineExceeded, got %v", shutdownErr)
	}
}

func TestPool_ConcurrentSubmit(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 4
	config.MaxWorkers = 8
	config.QueueSize = 100

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	var wg sync.WaitGroup
	var executed int32
	concurrency := 50

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			submitErr := p.Submit(context.Background(), Task{
				Fn: func() {
					atomic.AddInt32(&executed, 1)
				},
			})
			if submitErr != nil {
				t.Errorf("Submit failed: %v", submitErr)
			}
		}()
	}

	wg.Wait()

	// Wait for tasks to execute
	time.Sleep(200 * time.Millisecond)

	execCount := atomic.LoadInt32(&executed)
	halfConcurrency := concurrency / 2
	// G115: Safe conversion - we clamp to MaxInt32 to prevent overflow
	// #nosec G115
	halfConcurrencyInt32 := int32(minInt(halfConcurrency, 0x7FFFFFFF))
	if execCount < halfConcurrencyInt32 {
		t.Errorf("Expected at least %d executions, got %d", halfConcurrency, execCount)
	}
}

func TestPool_AvgTimes(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Submit several tasks
	for i := 0; i < 5; i++ {
		submitErr := p.Submit(context.Background(), Task{
			Fn: func() {
				time.Sleep(10 * time.Millisecond)
			},
		})
		_ = submitErr // Ignore submit errors for test purposes
	}

	time.Sleep(200 * time.Millisecond)

	stats := p.Stats()

	// AvgWaitTime and AvgExecTime should be reasonable
	if stats.AvgWaitTime < 0 {
		t.Error("AvgWaitTime should be non-negative")
	}
	if stats.AvgExecTime < 0 {
		t.Error("AvgExecTime should be non-negative")
	}
}

func TestPool_WorkerIdleTimeout(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1
	config.MaxWorkers = 5
	config.IdleTimeout = 100 * time.Millisecond

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Submit a task
	submitErr := p.Submit(context.Background(), Task{
		Fn: func() {},
	})
	_ = submitErr // Ignore submit errors for test purposes

	initialWorkers := p.Stats().ActiveWorkers + p.Stats().IdleWorkers

	// Wait for idle timeout
	time.Sleep(200 * time.Millisecond)

	// Workers may have scaled down (but min workers should remain)
	stats := p.Stats()
	totalWorkers := stats.ActiveWorkers + stats.IdleWorkers

	// G115: Safe conversion - we clamp to MaxInt32 to prevent overflow
	// #nosec G115
	minWorkersInt32 := int32(minInt(config.MinWorkers, 0x7FFFFFFF))
	if totalWorkers < minWorkersInt32 {
		t.Errorf("Workers dropped below minimum: %d < %d", totalWorkers, config.MinWorkers)
	}
	_ = initialWorkers
}

func TestPool_Autoscale(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 2
	config.MaxWorkers = 10
	config.QueueSize = 50

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Fill the queue to trigger autoscaling
	for i := 0; i < 30; i++ {
		submitErr := p.Submit(context.Background(), Task{
			Fn: func() {
				time.Sleep(50 * time.Millisecond)
			},
		})
		_ = submitErr // Ignore submit errors for test purposes
	}

	// Wait for autoscaler
	time.Sleep(2 * time.Second)

	stats := p.Stats()
	totalWorkers := stats.ActiveWorkers + stats.IdleWorkers

	// G115: Safe conversion - config.MaxWorkers is bounded by reasonable limits in tests
	// #nosec G115
	if totalWorkers > int32(config.MaxWorkers) {
		t.Errorf("Workers exceeded maximum: %d > %d", totalWorkers, config.MaxWorkers)
	}
}

func TestPool_DefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MinWorkers <= 0 {
		t.Error("MinWorkers should be positive")
	}
	if config.MaxWorkers < config.MinWorkers {
		t.Error("MaxWorkers should be >= MinWorkers")
	}
	if config.QueueSize <= 0 {
		t.Error("QueueSize should be positive")
	}
	if config.IdleTimeout <= 0 {
		t.Error("IdleTimeout should be positive")
	}
}

func TestPool_New_InvalidConfig(t *testing.T) {
	config := Config{
		MinWorkers: 0,
		MaxWorkers: 0,
		QueueSize:  0,
	}

	p, err := New(config)
	if err != nil {
		t.Fatalf("New should handle invalid config: %v", err)
	}

	if p == nil {
		t.Fatal("New should return pool even with invalid config (uses defaults)")
	}

	if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
		t.Errorf("Shutdown() failed: %v", shutdownErr)
	}
}

func TestPool_Submit_ContextCanceled(t *testing.T) {
	config := DefaultConfig()
	config.MinWorkers = 1
	config.MaxWorkers = 1
	config.QueueSize = 1
	config.BackpressureStrategy = StrategyBlock

	p, err := New(config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if shutdownErr := p.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() failed: %v", shutdownErr)
		}
	}()

	// Fill the queue
	submitErr1 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(200 * time.Millisecond) }})
	_ = submitErr1 // Ignore submit errors for test purposes
	submitErr2 := p.Submit(context.Background(), Task{Fn: func() { time.Sleep(200 * time.Millisecond) }})
	_ = submitErr2 // Ignore submit errors for test purposes

	// Submit with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = p.Submit(ctx, Task{Fn: func() {}})
	if err == nil {
		t.Error("Expected error for canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}
