// Package resilience provides rate limiting and circuit breaker functionality.
package resilience

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter controls execution rate.
type RateLimiter interface {
	// Allow checks if execution is allowed for the given binary.
	Allow(binary string) bool

	// Wait blocks until execution is allowed or context is canceled.
	Wait(ctx context.Context, binary string) error

	// SetLimit updates the rate limit for a binary.
	SetLimit(binary string, limit rate.Limit, burst int)
}

// RateLimiterConfig configures the rate limiter.
type RateLimiterConfig struct {
	// DefaultLimit is the default requests per second.
	DefaultLimit float64

	// DefaultBurst is the default burst size.
	DefaultBurst int

	// PerBinary enables per-binary rate limiting.
	PerBinary bool

	// BinaryLimits contains per-binary rate limits.
	BinaryLimits map[string]BinaryLimit
}

// BinaryLimit defines rate limit for a specific binary.
type BinaryLimit struct {
	Limit float64
	Burst int
}

// DefaultRateLimiterConfig returns default configuration.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		DefaultLimit: 100,
		DefaultBurst: 150,
		PerBinary:    true,
		BinaryLimits: make(map[string]BinaryLimit),
	}
}

// rateLimiter implements RateLimiter.
type rateLimiter struct {
	config         RateLimiterConfig
	globalLimiter  *rate.Limiter
	binaryLimiters map[string]*rate.Limiter
	mu             sync.RWMutex
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config RateLimiterConfig) RateLimiter {
	rl := &rateLimiter{
		config:         config,
		globalLimiter:  rate.NewLimiter(rate.Limit(config.DefaultLimit), config.DefaultBurst),
		binaryLimiters: make(map[string]*rate.Limiter),
	}

	// Initialize per-binary limiters
	for binary, limit := range config.BinaryLimits {
		rl.binaryLimiters[binary] = rate.NewLimiter(rate.Limit(limit.Limit), limit.Burst)
	}

	return rl
}

// Allow implements RateLimiter.Allow.
func (rl *rateLimiter) Allow(binary string) bool {
	if !rl.config.PerBinary {
		return rl.globalLimiter.Allow()
	}

	limiter := rl.getLimiter(binary)
	return limiter.Allow()
}

// Wait implements RateLimiter.Wait.
func (rl *rateLimiter) Wait(ctx context.Context, binary string) error {
	if !rl.config.PerBinary {
		return rl.globalLimiter.Wait(ctx)
	}

	limiter := rl.getLimiter(binary)
	return limiter.Wait(ctx)
}

// SetLimit implements RateLimiter.SetLimit.
func (rl *rateLimiter) SetLimit(binary string, limit rate.Limit, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, ok := rl.binaryLimiters[binary]; ok {
		limiter.SetLimit(limit)
		limiter.SetBurst(burst)
	} else {
		rl.binaryLimiters[binary] = rate.NewLimiter(limit, burst)
	}
}

func (rl *rateLimiter) getLimiter(binary string) *rate.Limiter {
	rl.mu.RLock()
	limiter, ok := rl.binaryLimiters[binary]
	rl.mu.RUnlock()

	if ok {
		return limiter
	}

	// Create new limiter with default settings
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if existing, ok := rl.binaryLimiters[binary]; ok {
		return existing
	}

	newLimiter := rate.NewLimiter(rate.Limit(rl.config.DefaultLimit), rl.config.DefaultBurst)
	rl.binaryLimiters[binary] = newLimiter
	return newLimiter
}

// TokenBucket implements a simple token bucket rate limiter.
type TokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(capacity, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Take attempts to take n tokens from the bucket.
func (tb *TokenBucket) Take(n float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= n {
		tb.tokens -= n
		return true
	}

	return false
}

// TakeWait takes n tokens, waiting if necessary.
func (tb *TokenBucket) TakeWait(ctx context.Context, n float64) error {
	for {
		tb.mu.Lock()
		tb.refill()

		if tb.tokens >= n {
			tb.tokens -= n
			tb.mu.Unlock()
			return nil
		}

		// Calculate wait time
		needed := n - tb.tokens
		waitTime := time.Duration(needed / tb.refillRate * float64(time.Second))
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Try again
		}
	}
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now
}
