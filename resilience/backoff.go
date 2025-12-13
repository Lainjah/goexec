package resilience

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"time"
)

// Backoff provides backoff strategies.
type Backoff interface {
	// Next returns the next backoff duration.
	Next() time.Duration

	// Reset resets the backoff state.
	Reset()
}

// BackoffConfig configures backoff behavior.
type BackoffConfig struct {
	// InitialInterval is the first backoff interval.
	InitialInterval time.Duration

	// MaxInterval is the maximum backoff interval.
	MaxInterval time.Duration

	// Multiplier is the factor to multiply interval by after each retry.
	Multiplier float64

	// MaxRetries is the maximum number of retries (0 for unlimited).
	MaxRetries int

	// Jitter adds randomness to backoff intervals.
	Jitter bool

	// JitterFactor is the maximum jitter factor (0.0 to 1.0).
	JitterFactor float64
}

// DefaultBackoffConfig returns default backoff configuration.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      10,
		Jitter:          true,
		JitterFactor:    0.1,
	}
}

// secureFloat64 generates a cryptographically secure random float64 in [0.0, 1.0)
// For backoff jitter, we use crypto/rand to avoid predictability.
// This is thread-safe without requiring synchronization.
func secureFloat64() float64 {
	// Generate random bytes and convert to float
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback: if crypto/rand fails, use a time-based approach
		// This is acceptable for jitter purposes but not ideal
		val := time.Now().UnixNano()
		return float64(val&0x7FFFFFFF) / float64(0x7FFFFFFF)
	}

	// Convert bytes to uint64 and normalize to [0.0, 1.0)
	// Use only 53 bits to maintain float64 precision
	val := binary.BigEndian.Uint64(buf[:])
	val = val >> 11 // Keep 53 bits for float64 mantissa
	return float64(val) / float64(1<<53)
}

// ExponentialBackoff implements exponential backoff.
type ExponentialBackoff struct {
	config   BackoffConfig
	current  time.Duration
	attempts int
}

// NewExponentialBackoff creates a new exponential backoff.
func NewExponentialBackoff(config BackoffConfig) *ExponentialBackoff {
	return &ExponentialBackoff{
		config:  config,
		current: config.InitialInterval,
	}
}

// Next implements Backoff.Next.
func (b *ExponentialBackoff) Next() time.Duration {
	if b.config.MaxRetries > 0 && b.attempts >= b.config.MaxRetries {
		return 0 // No more retries
	}

	b.attempts++

	interval := b.current
	if b.config.Jitter {
		interval = b.addJitter(interval)
	}

	// Calculate next interval
	next := time.Duration(float64(b.current) * b.config.Multiplier)
	if next > b.config.MaxInterval {
		next = b.config.MaxInterval
	}
	b.current = next

	return interval
}

// Reset implements Backoff.Reset.
func (b *ExponentialBackoff) Reset() {
	b.current = b.config.InitialInterval
	b.attempts = 0
}

// Attempts returns the number of attempts so far.
func (b *ExponentialBackoff) Attempts() int {
	return b.attempts
}

func (b *ExponentialBackoff) addJitter(d time.Duration) time.Duration {
	if b.config.JitterFactor <= 0 {
		return d
	}

	jitter := float64(d) * b.config.JitterFactor
	// Use secure random number generation
	return time.Duration(float64(d) + jitter*(secureFloat64()*2-1))
}

// ConstantBackoff implements constant backoff.
type ConstantBackoff struct {
	interval   time.Duration
	maxRetries int
	attempts   int
}

// NewConstantBackoff creates a new constant backoff.
func NewConstantBackoff(interval time.Duration, maxRetries int) *ConstantBackoff {
	return &ConstantBackoff{
		interval:   interval,
		maxRetries: maxRetries,
	}
}

// Next implements Backoff.Next.
func (b *ConstantBackoff) Next() time.Duration {
	if b.maxRetries > 0 && b.attempts >= b.maxRetries {
		return 0
	}
	b.attempts++
	return b.interval
}

// Reset implements Backoff.Reset.
func (b *ConstantBackoff) Reset() {
	b.attempts = 0
}

// LinearBackoff implements linear backoff.
type LinearBackoff struct {
	initial    time.Duration
	increment  time.Duration
	max        time.Duration
	maxRetries int
	current    time.Duration
	attempts   int
}

// NewLinearBackoff creates a new linear backoff.
func NewLinearBackoff(initial, increment, max time.Duration, maxRetries int) *LinearBackoff {
	return &LinearBackoff{
		initial:    initial,
		increment:  increment,
		max:        max,
		maxRetries: maxRetries,
		current:    initial,
	}
}

// Next implements Backoff.Next.
func (b *LinearBackoff) Next() time.Duration {
	if b.maxRetries > 0 && b.attempts >= b.maxRetries {
		return 0
	}
	b.attempts++

	interval := b.current
	b.current += b.increment
	if b.current > b.max {
		b.current = b.max
	}

	return interval
}

// Reset implements Backoff.Reset.
func (b *LinearBackoff) Reset() {
	b.current = b.initial
	b.attempts = 0
}

// RetryWithBackoff retries an operation with backoff.
func RetryWithBackoff(ctx context.Context, backoff Backoff, fn func() error) error {
	var lastErr error

	for {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		wait := backoff.Next()
		if wait == 0 {
			return lastErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			// Continue retry
		}
	}
}

// RetryFunc is a function that can be retried.
type RetryFunc func() (interface{}, error)

// RetryWithBackoffResult retries an operation and returns the result.
func RetryWithBackoffResult(ctx context.Context, backoff Backoff, fn RetryFunc) (interface{}, error) {
	var lastErr error

	for {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		wait := backoff.Next()
		if wait == 0 {
			return nil, lastErr
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
			// Continue retry
		}
	}
}

// DecorrelatedJitterBackoff implements AWS-style decorrelated jitter.
type DecorrelatedJitterBackoff struct {
	base       time.Duration
	cap        time.Duration
	maxRetries int
	sleep      time.Duration
	attempts   int
}

// NewDecorrelatedJitterBackoff creates a new decorrelated jitter backoff.
func NewDecorrelatedJitterBackoff(base, cap time.Duration, maxRetries int) *DecorrelatedJitterBackoff {
	return &DecorrelatedJitterBackoff{
		base:       base,
		cap:        cap,
		maxRetries: maxRetries,
		sleep:      base,
	}
}

// Next implements Backoff.Next.
func (b *DecorrelatedJitterBackoff) Next() time.Duration {
	if b.maxRetries > 0 && b.attempts >= b.maxRetries {
		return 0
	}
	b.attempts++

	// sleep = min(cap, random_between(base, sleep * 3))
	maxSleep := time.Duration(float64(b.sleep) * 3)
	// Use secure random number generation
	b.sleep = time.Duration(float64(b.base) + secureFloat64()*float64(maxSleep-b.base))
	if b.sleep > b.cap {
		b.sleep = b.cap
	}

	return b.sleep
}

// Reset implements Backoff.Reset.
func (b *DecorrelatedJitterBackoff) Reset() {
	b.sleep = b.base
	b.attempts = 0
}

// CalculateBackoff calculates exponential backoff duration.
func CalculateBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
