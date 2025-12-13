package resilience

import (
	"sync"
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	if cb == nil {
		t.Fatal("NewCircuitBreaker returned nil")
	}

	// Should start in closed state
	if !cb.Allow("test") {
		t.Error("Circuit breaker should allow requests in closed state")
	}
}

func TestCircuitBreaker_StateTransition(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 3
	config.SuccessThreshold = 2
	config.Timeout = 100 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Start closed
	if cb.State("test") != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.State("test"))
	}

	// Record failures until threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure("test")
	}

	// Should be open now
	if cb.State("test") != StateOpen {
		t.Errorf("Expected StateOpen, got %v", cb.State("test"))
	}

	if cb.Allow("test") {
		t.Error("Circuit breaker should not allow requests in open state")
	}
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.SuccessThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	if cb.State("test") != StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	state := cb.State("test")
	if state != StateHalfOpen {
		t.Errorf("Expected StateHalfOpen, got %v", state)
	}

	// Should allow requests in half-open
	if !cb.Allow("test") {
		t.Error("Circuit breaker should allow requests in half-open state")
	}
}

func TestCircuitBreaker_CloseFromHalfOpen(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.SuccessThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Open circuit
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Transition to half-open by calling Allow
	cb.Allow("test")

	// Record successes
	cb.RecordSuccess("test")
	cb.RecordSuccess("test")

	// Should be closed
	if cb.State("test") != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.State("test"))
	}
}

func TestCircuitBreaker_ReopenFromHalfOpen(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.SuccessThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Open circuit
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Record failure - should reopen immediately
	cb.RecordFailure("test")

	if cb.State("test") != StateOpen {
		t.Errorf("Expected StateOpen, got %v", cb.State("test"))
	}
}

func TestCircuitBreaker_PerBinary(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.PerBinary = true
	config.FailureThreshold = 2
	cb := NewCircuitBreaker(config)

	// Open circuit for binary1
	cb.RecordFailure("binary1")
	cb.RecordFailure("binary1")

	if cb.State("binary1") != StateOpen {
		t.Error("binary1 should be open")
	}

	// binary2 should still be closed
	if cb.State("binary2") != StateClosed {
		t.Error("binary2 should still be closed")
	}

	if !cb.Allow("binary2") {
		t.Error("binary2 should be allowed")
	}
}

func TestCircuitBreaker_Global(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.PerBinary = false
	config.FailureThreshold = 2
	cb := NewCircuitBreaker(config)

	// Open global circuit
	cb.RecordFailure("any")
	cb.RecordFailure("any")

	// All binaries should be blocked
	if cb.Allow("binary1") {
		t.Error("binary1 should be blocked when global circuit is open")
	}
	if cb.Allow("binary2") {
		t.Error("binary2 should be blocked when global circuit is open")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	cb := NewCircuitBreaker(config)

	// Open circuit
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	if cb.State("test") != StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Reset
	cb.Reset("test")

	// Should be closed
	if cb.State("test") != StateClosed {
		t.Errorf("Expected StateClosed after reset, got %v", cb.State("test"))
	}

	if !cb.Allow("test") {
		t.Error("Should allow requests after reset")
	}
}

func TestCircuitBreaker_StateString(t *testing.T) {
	tests := []struct { //nolint:govet // fieldalignment: test struct field order optimized for readability not memory
		state CircuitState
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.want)
			}
		})
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.PerBinary = true
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	concurrency := 50

	// Concurrent operations on same binary
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow("test")
			cb.RecordSuccess("test")
			cb.RecordFailure("test")
			cb.State("test")
		}()
	}

	wg.Wait()

	// Should not panic and should have consistent state
	state := cb.State("test")
	if state != StateClosed && state != StateOpen && state != StateHalfOpen {
		t.Errorf("Invalid state after concurrent access: %v", state)
	}
}

func TestCircuitBreaker_ConcurrentDifferentBinaries(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.PerBinary = true
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	binaryCount := 10
	opsPerBinary := 10

	for i := 0; i < binaryCount; i++ {
		binary := "binary" + string(rune('0'+i))
		for j := 0; j < opsPerBinary; j++ {
			wg.Add(1)
			go func(b string) {
				defer wg.Done()
				cb.Allow(b)
				cb.RecordSuccess(b)
				cb.State(b)
			}(binary)
		}
	}

	wg.Wait()

	// All binaries should have valid states
	for i := 0; i < binaryCount; i++ {
		binary := "binary" + string(rune('0'+i))
		state := cb.State(binary)
		if state != StateClosed && state != StateOpen && state != StateHalfOpen {
			t.Errorf("Invalid state for %s: %v", binary, state)
		}
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	var stateChanges []CircuitState
	var mu sync.Mutex

	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.OnStateChange = func(binary string, from, to CircuitState) {
		mu.Lock()
		defer mu.Unlock()
		stateChanges = append(stateChanges, to)
	}
	cb := NewCircuitBreaker(config)

	// Open circuit
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	mu.Lock()
	changes := stateChanges
	mu.Unlock()

	if len(changes) == 0 {
		t.Error("OnStateChange should have been called")
	}
}

func TestCircuitBreaker_Allow_AutoHalfOpen(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Open circuit
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Allow should automatically transition to half-open
	if !cb.Allow("test") {
		t.Error("Allow should transition to half-open and return true")
	}

	if cb.State("test") != StateHalfOpen {
		t.Errorf("Expected StateHalfOpen, got %v", cb.State("test"))
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold <= 0 {
		t.Error("FailureThreshold should be positive")
	}
	if config.SuccessThreshold <= 0 {
		t.Error("SuccessThreshold should be positive")
	}
	if config.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
}

func TestCircuitBreaker_SuccessClearsFailures(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 5
	cb := NewCircuitBreaker(config)

	// Record some failures
	cb.RecordFailure("test")
	cb.RecordFailure("test")
	cb.RecordFailure("test")

	// Record success - should clear failures
	cb.RecordSuccess("test")

	// Should still be closed
	if cb.State("test") != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.State("test"))
	}
}
