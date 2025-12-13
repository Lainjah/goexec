package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestNewRateLimiter(t *testing.T) {
	config := DefaultRateLimiterConfig()
	rl := NewRateLimiter(config)

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}

	// Should allow requests
	if !rl.Allow("test") {
		t.Error("Rate limiter should allow initial requests")
	}
}

func TestRateLimiter_GlobalMode(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = false
	config.DefaultLimit = 10.0
	config.DefaultBurst = 5
	rl := NewRateLimiter(config)

	// All binaries should use same limiter
	allowed1 := rl.Allow("binary1")
	allowed2 := rl.Allow("binary2")

	if !allowed1 || !allowed2 {
		t.Error("Should allow initial requests in global mode")
	}
}

func TestRateLimiter_PerBinaryMode(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	config.DefaultLimit = 100.0
	config.DefaultBurst = 10
	rl := NewRateLimiter(config)

	// Each binary should have separate limiter
	if !rl.Allow("binary1") {
		t.Error("Should allow request for binary1")
	}
	if !rl.Allow("binary2") {
		t.Error("Should allow request for binary2")
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.DefaultLimit = 10.0
	config.DefaultBurst = 2
	rl := NewRateLimiter(config)

	ctx := context.Background()

	// Should wait without error
	err := rl.Wait(ctx, "test")
	if err != nil {
		t.Errorf("Wait should not error initially: %v", err)
	}
}

func TestRateLimiter_Wait_ContextCanceled(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.DefaultLimit = 0.1 // Very low limit
	rl := NewRateLimiter(config)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx, "test")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRateLimiter_Wait_ContextTimeout(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.DefaultLimit = 0.1 // Very low limit
	rl := NewRateLimiter(config)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx, "test")
	if err == nil {
		t.Log("Wait completed before timeout")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("Wait error: %v", err)
	}
}

func TestRateLimiter_SetLimit(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	rl := NewRateLimiter(config)

	// Set custom limit
	rl.SetLimit("test", rate.Limit(50.0), 10)

	// Should use new limit
	if !rl.Allow("test") {
		t.Error("Should allow with new limit")
	}
}

func TestRateLimiter_SetLimit_Existing(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	rl := NewRateLimiter(config)

	// Get limiter (creates it)
	rl.Allow("test")

	// Update limit
	rl.SetLimit("test", rate.Limit(100.0), 20)

	// Should use updated limit
	if !rl.Allow("test") {
		t.Error("Should allow with updated limit")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	rl := NewRateLimiter(config)

	var wg sync.WaitGroup
	var allowed int32
	concurrency := 50

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow("test") {
				atomic.AddInt32(&allowed, 1)
			}
		}()
	}

	wg.Wait()

	// Should allow some requests
	if atomic.LoadInt32(&allowed) == 0 {
		t.Error("Should allow some concurrent requests")
	}
}

func TestRateLimiter_DefaultConfig(t *testing.T) {
	config := DefaultRateLimiterConfig()

	if config.DefaultLimit <= 0 {
		t.Error("DefaultLimit should be positive")
	}
	if config.DefaultBurst <= 0 {
		t.Error("DefaultBurst should be positive")
	}
}

func TestTokenBucket_New(t *testing.T) {
	capacity := 10.0
	refillRate := 2.0
	tb := NewTokenBucket(capacity, refillRate)

	if tb == nil {
		t.Fatal("NewTokenBucket returned nil")
	}
}

func TestTokenBucket_Take(t *testing.T) {
	capacity := 10.0
	refillRate := 2.0
	tb := NewTokenBucket(capacity, refillRate)

	// Should allow taking tokens up to capacity
	for i := 0; i < int(capacity); i++ {
		if !tb.Take(1.0) {
			t.Errorf("Should allow taking token %d", i+1)
		}
	}

	// Should reject when empty
	if tb.Take(1.0) {
		t.Error("Should reject when no tokens available")
	}
}

func TestTokenBucket_Take_Multiple(t *testing.T) {
	capacity := 10.0
	refillRate := 2.0
	tb := NewTokenBucket(capacity, refillRate)

	// Take multiple tokens
	if !tb.Take(5.0) {
		t.Error("Should allow taking 5 tokens")
	}

	if !tb.Take(5.0) {
		t.Error("Should allow taking another 5 tokens")
	}

	// Should be empty now
	if tb.Take(1.0) {
		t.Error("Should reject when empty")
	}
}

func TestTokenBucket_TakeWait(t *testing.T) {
	capacity := 10.0
	refillRate := 10.0 // Fast refill
	tb := NewTokenBucket(capacity, refillRate)

	// Empty the bucket
	tb.Take(capacity)

	// TakeWait should eventually succeed
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := tb.TakeWait(ctx, 1.0)
	if err != nil {
		t.Errorf("TakeWait should succeed after refill: %v", err)
	}
}

func TestTokenBucket_TakeWait_ContextCanceled(t *testing.T) {
	capacity := 10.0
	refillRate := 0.1 // Slow refill
	tb := NewTokenBucket(capacity, refillRate)

	// Empty the bucket
	tb.Take(capacity)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := tb.TakeWait(ctx, 1.0)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	capacity := 10.0
	refillRate := 100.0 // Very fast refill
	tb := NewTokenBucket(capacity, refillRate)

	// Empty the bucket
	tb.Take(capacity)

	// Wait for refill
	time.Sleep(100 * time.Millisecond)

	// Should be able to take some tokens
	if !tb.Take(1.0) {
		t.Error("Should allow tokens after refill")
	}
}

func TestTokenBucket_Refill_CapacityCap(t *testing.T) {
	capacity := 10.0
	refillRate := 1000.0 // Very fast refill
	tb := NewTokenBucket(capacity, refillRate)

	// Empty and wait
	tb.Take(capacity)
	time.Sleep(200 * time.Millisecond)

	// Should not exceed capacity
	if !tb.Take(capacity) {
		t.Error("Should allow up to capacity")
	}

	if tb.Take(1.0) {
		t.Error("Should not exceed capacity")
	}
}

func TestRateLimiter_BinaryLimits(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	config.BinaryLimits = map[string]BinaryLimit{
		"binary1": {Limit: 50.0, Burst: 10},
		"binary2": {Limit: 100.0, Burst: 20},
	}

	rl := NewRateLimiter(config)

	// Each binary should use its configured limit
	if !rl.Allow("binary1") {
		t.Error("binary1 should be allowed")
	}
	if !rl.Allow("binary2") {
		t.Error("binary2 should be allowed")
	}
}

func TestRateLimiter_NewBinaryDefaults(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	config.DefaultLimit = 25.0
	config.DefaultBurst = 5
	rl := NewRateLimiter(config)

	// New binary should use defaults
	if !rl.Allow("newbinary") {
		t.Error("New binary should use default limits")
	}
}

func TestRateLimiter_ConcurrentBinaryCreation(t *testing.T) {
	config := DefaultRateLimiterConfig()
	config.PerBinary = true
	rl := NewRateLimiter(config)

	var wg sync.WaitGroup
	binaryCount := 20

	for i := 0; i < binaryCount; i++ {
		wg.Add(1)
		binary := "binary" + string(rune('a'+i))
		go func(b string) {
			defer wg.Done()
			rl.Allow(b)
			_ = rl.Wait(context.Background(), b)
		}(binary)
	}

	wg.Wait()

	// Should not panic and all binaries should work
	for i := 0; i < binaryCount; i++ {
		binary := "binary" + string(rune('a'+i))
		if !rl.Allow(binary) {
			t.Errorf("Should allow requests for %s", binary)
		}
	}
}

