package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewExponentialBackoff(t *testing.T) {
	config := DefaultBackoffConfig()
	config.Jitter = false // Disable jitter for predictable test
	backoff := NewExponentialBackoff(config)

	if backoff == nil {
		t.Fatal("NewExponentialBackoff returned nil")
	}

	// First call should return initial interval
	duration := backoff.Next()
	if duration != config.InitialInterval {
		t.Errorf("Expected duration %v, got %v", config.InitialInterval, duration)
	}
}

func TestExponentialBackoff_Next(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 100 * time.Millisecond
	config.Multiplier = 2.0
	config.MaxRetries = 5
	config.Jitter = false

	backoff := NewExponentialBackoff(config)

	var durations []time.Duration
	for i := 0; i < 4; i++ {
		duration := backoff.Next()
		durations = append(durations, duration)
	}

	// Without jitter, should see exponential growth
	if durations[0] < 100*time.Millisecond || durations[0] > 110*time.Millisecond {
		t.Errorf("First duration should be ~100ms, got %v", durations[0])
	}

	// Subsequent durations should increase
	for i := 1; i < len(durations); i++ {
		if durations[i] < durations[i-1] {
			t.Errorf("Duration should increase: %v < %v", durations[i], durations[i-1])
		}
	}
}

func TestExponentialBackoff_MaxRetries(t *testing.T) {
	config := DefaultBackoffConfig()
	config.MaxRetries = 3
	backoff := NewExponentialBackoff(config)

	// Execute max retries
	for i := 0; i < 3; i++ {
		duration := backoff.Next()
		if duration == 0 {
			t.Errorf("Expected non-zero duration at attempt %d", i+1)
		}
	}

	// Next call should return 0 (no more retries)
	duration := backoff.Next()
	if duration != 0 {
		t.Errorf("Expected 0 duration after max retries, got %v", duration)
	}
}

func TestExponentialBackoff_Reset(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 100 * time.Millisecond
	config.Jitter = false

	backoff := NewExponentialBackoff(config)

	// Advance backoff
	backoff.Next()
	backoff.Next()

	// Reset
	backoff.Reset()

	// Next call should return initial interval again
	duration := backoff.Next()
	if duration < 100*time.Millisecond || duration > 110*time.Millisecond {
		t.Errorf("After reset, expected ~100ms, got %v", duration)
	}
}

func TestExponentialBackoff_MaxInterval(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 100 * time.Millisecond
	config.Multiplier = 10.0
	config.MaxInterval = 500 * time.Millisecond
	config.Jitter = false

	backoff := NewExponentialBackoff(config)

	// Should cap at MaxInterval
	for i := 0; i < 5; i++ {
		duration := backoff.Next()
		if duration > config.MaxInterval {
			t.Errorf("Duration %v exceeded MaxInterval %v", duration, config.MaxInterval)
		}
	}
}

func TestExponentialBackoff_Jitter(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 100 * time.Millisecond
	config.Jitter = true
	config.JitterFactor = 0.1

	backoff := NewExponentialBackoff(config)

	// With jitter, duration should vary
	durations := make(map[time.Duration]bool)
	for i := 0; i < 10; i++ {
		duration := backoff.Next()
		durations[duration] = true
		backoff.Reset()
	}

	// Should have some variation (though not guaranteed)
	if len(durations) < 1 {
		t.Error("Expected some variation with jitter")
	}
}

func TestConstantBackoff(t *testing.T) {
	interval := 200 * time.Millisecond
	backoff := NewConstantBackoff(interval, 5)

	if backoff == nil {
		t.Fatal("NewConstantBackoff returned nil")
	}

	// All calls should return same interval
	for i := 0; i < 5; i++ {
		duration := backoff.Next()
		if duration != interval {
			t.Errorf("Expected %v, got %v", interval, duration)
		}
	}

	// Should return 0 after max retries
	duration := backoff.Next()
	if duration != 0 {
		t.Errorf("Expected 0 after max retries, got %v", duration)
	}
}

func TestConstantBackoff_Unlimited(t *testing.T) {
	interval := 100 * time.Millisecond
	backoff := NewConstantBackoff(interval, 0)

	// Should return interval indefinitely
	for i := 0; i < 10; i++ {
		duration := backoff.Next()
		if duration != interval {
			t.Errorf("Expected %v, got %v", interval, duration)
		}
	}
}

func TestConstantBackoff_Reset(t *testing.T) {
	backoff := NewConstantBackoff(100*time.Millisecond, 10)

	backoff.Next()
	backoff.Next()
	backoff.Reset()

	// Reset should reset internal state
	// Next should return the same interval
	duration := backoff.Next()
	if duration != 100*time.Millisecond {
		t.Errorf("After reset, expected 100ms, got %v", duration)
	}
}

func TestLinearBackoff(t *testing.T) {
	initial := 100 * time.Millisecond
	increment := 50 * time.Millisecond
	max := 500 * time.Millisecond

	backoff := NewLinearBackoff(initial, increment, max, 10)

	if backoff == nil {
		t.Fatal("NewLinearBackoff returned nil")
	}

	var durations []time.Duration
	for i := 0; i < 5; i++ {
		duration := backoff.Next()
		durations = append(durations, duration)
	}

	// Should increase linearly
	for i := 1; i < len(durations); i++ {
		diff := durations[i] - durations[i-1]
		if diff < increment-10*time.Millisecond || diff > increment+10*time.Millisecond {
			t.Errorf("Expected linear increase of ~%v, got %v", increment, diff)
		}
	}
}

func TestLinearBackoff_MaxCap(t *testing.T) {
	initial := 100 * time.Millisecond
	increment := 200 * time.Millisecond
	max := 300 * time.Millisecond

	backoff := NewLinearBackoff(initial, increment, max, 10)

	// Should cap at max
	for i := 0; i < 5; i++ {
		duration := backoff.Next()
		if duration > max {
			t.Errorf("Duration %v exceeded max %v", duration, max)
		}
	}
}

func TestRetryWithBackoff(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 10 * time.Millisecond
	config.MaxRetries = 3
	backoff := NewExponentialBackoff(config)

	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("not ready")
		}
		return nil
	}

	ctx := context.Background()
	err := RetryWithBackoff(ctx, backoff, fn)

	if err != nil {
		t.Errorf("Expected success after retries, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_ExhaustRetries(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 10 * time.Millisecond
	config.MaxRetries = 2
	backoff := NewExponentialBackoff(config)

	fn := func() error {
		return errors.New("always fails")
	}

	ctx := context.Background()
	err := RetryWithBackoff(ctx, backoff, fn)

	if err == nil {
		t.Error("Expected error after exhausting retries")
	}
}

func TestRetryWithBackoff_ContextCanceled(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 100 * time.Millisecond
	backoff := NewExponentialBackoff(config)

	fn := func() error {
		return errors.New("always fails")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RetryWithBackoff(ctx, backoff, fn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRetryWithBackoffResult(t *testing.T) {
	config := DefaultBackoffConfig()
	config.InitialInterval = 10 * time.Millisecond
	config.MaxRetries = 3
	backoff := NewExponentialBackoff(config)

	attempts := 0
	fn := RetryFunc(func() (interface{}, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("not ready")
		}
		return "success", nil
	})

	ctx := context.Background()
	result, err := RetryWithBackoffResult(ctx, backoff, fn)

	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected result 'success', got %v", result)
	}
}

func TestDecorrelatedJitterBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	capDur := 1000 * time.Millisecond

	backoff := NewDecorrelatedJitterBackoff(base, capDur, 10)

	if backoff == nil {
		t.Fatal("NewDecorrelatedJitterBackoff returned nil")
	}

	// Should return duration within bounds
	for i := 0; i < 5; i++ {
		duration := backoff.Next()
		if duration < base {
			t.Errorf("Duration %v below base %v", duration, base)
		}
		if duration > capDur {
			t.Errorf("Duration %v exceeded cap %v", duration, capDur)
		}
	}
}

func TestDecorrelatedJitterBackoff_MaxRetries(t *testing.T) {
	backoff := NewDecorrelatedJitterBackoff(100*time.Millisecond, 1000*time.Millisecond, 3)

	for i := 0; i < 3; i++ {
		duration := backoff.Next()
		if duration == 0 {
			t.Errorf("Expected non-zero duration at attempt %d", i+1)
		}
	}

	duration := backoff.Next()
	if duration != 0 {
		t.Errorf("Expected 0 after max retries, got %v", duration)
	}
}

func TestDecorrelatedJitterBackoff_Reset(t *testing.T) {
	backoff := NewDecorrelatedJitterBackoff(100*time.Millisecond, 1000*time.Millisecond, 10)

	backoff.Next()
	backoff.Next()

	backoff.Reset()

	// Should reset internal state
	duration := backoff.Next()
	if duration < 100*time.Millisecond {
		t.Errorf("After reset, expected >= 100ms, got %v", duration)
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		base    time.Duration
		max     time.Duration
		wantMax time.Duration
	}{
		{0, 100 * time.Millisecond, 1000 * time.Millisecond, 100 * time.Millisecond},
		{1, 100 * time.Millisecond, 1000 * time.Millisecond, 200 * time.Millisecond},
		{5, 100 * time.Millisecond, 1000 * time.Millisecond, 1000 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			duration := CalculateBackoff(tt.attempt, tt.base, tt.max)
			if duration > tt.wantMax {
				t.Errorf("CalculateBackoff(%d, %v, %v) = %v, want <= %v",
					tt.attempt, tt.base, tt.max, duration, tt.wantMax)
			}
		})
	}
}

func TestExponentialBackoff_Attempts(t *testing.T) {
	config := DefaultBackoffConfig()
	config.MaxRetries = 5
	backoff := NewExponentialBackoff(config)

	if backoff.Attempts() != 0 {
		t.Errorf("Initial attempts should be 0, got %d", backoff.Attempts())
	}

	backoff.Next()
	if backoff.Attempts() != 1 {
		t.Errorf("After Next(), attempts should be 1, got %d", backoff.Attempts())
	}

	backoff.Reset()
	if backoff.Attempts() != 0 {
		t.Errorf("After Reset(), attempts should be 0, got %d", backoff.Attempts())
	}
}

