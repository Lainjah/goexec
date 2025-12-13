package resilience

import (
	"sync"
	"time"
)

// CircuitBreaker provides circuit breaker functionality.
type CircuitBreaker interface {
	// Allow checks if execution is allowed.
	Allow(binary string) bool

	// RecordSuccess records a successful execution.
	RecordSuccess(binary string)

	// RecordFailure records a failed execution.
	RecordFailure(binary string)

	// State returns the current state for a binary.
	State(binary string) CircuitState

	// Reset resets the circuit breaker for a binary.
	Reset(binary string)
}

// CircuitState represents the circuit breaker state.
type CircuitState int

const (
	// StateClosed allows requests through.
	StateClosed CircuitState = iota
	// StateOpen blocks all requests.
	StateOpen
	// StateHalfOpen allows limited requests for testing.
	StateHalfOpen
)

// String returns the string representation of the state.
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening.
	FailureThreshold int

	// SuccessThreshold is the number of successes to close from half-open.
	SuccessThreshold int

	// Timeout is the duration to wait before transitioning to half-open.
	Timeout time.Duration

	// PerBinary enables per-binary circuit breakers.
	PerBinary bool

	// OnStateChange is called when state changes.
	OnStateChange func(binary string, from, to CircuitState)
}

// DefaultCircuitBreakerConfig returns default configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		PerBinary:        true,
	}
}

// circuitBreaker implements CircuitBreaker.
type circuitBreaker struct {
	config   CircuitBreakerConfig
	global   *breaker
	breakers map[string]*breaker
	mu       sync.RWMutex
}

// breaker represents a single circuit breaker.
type breaker struct {
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	config          *CircuitBreakerConfig
	mu              sync.Mutex
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) CircuitBreaker {
	return &circuitBreaker{
		config:   config,
		global:   newBreaker(&config),
		breakers: make(map[string]*breaker),
	}
}

// Allow implements CircuitBreaker.Allow.
func (cb *circuitBreaker) Allow(binary string) bool {
	if !cb.config.PerBinary {
		return cb.global.allow()
	}

	b := cb.getBreaker(binary)
	return b.allow()
}

// RecordSuccess implements CircuitBreaker.RecordSuccess.
func (cb *circuitBreaker) RecordSuccess(binary string) {
	if !cb.config.PerBinary {
		cb.global.recordSuccess()
		return
	}

	b := cb.getBreaker(binary)
	b.recordSuccess()
}

// RecordFailure implements CircuitBreaker.RecordFailure.
func (cb *circuitBreaker) RecordFailure(binary string) {
	if !cb.config.PerBinary {
		cb.global.recordFailure()
		return
	}

	b := cb.getBreaker(binary)
	b.recordFailure()
}

// State implements CircuitBreaker.State.
func (cb *circuitBreaker) State(binary string) CircuitState {
	if !cb.config.PerBinary {
		return cb.global.getState()
	}

	b := cb.getBreaker(binary)
	return b.getState()
}

// Reset implements CircuitBreaker.Reset.
func (cb *circuitBreaker) Reset(binary string) {
	if !cb.config.PerBinary {
		cb.global.reset()
		return
	}

	b := cb.getBreaker(binary)
	b.reset()
}

func (cb *circuitBreaker) getBreaker(binary string) *breaker {
	cb.mu.RLock()
	b, ok := cb.breakers[binary]
	cb.mu.RUnlock()

	if ok {
		return b
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Double-check
	if existing, ok := cb.breakers[binary]; ok {
		return existing
	}

	newB := newBreaker(&cb.config)
	cb.breakers[binary] = newB
	return newB
}

func newBreaker(config *CircuitBreakerConfig) *breaker {
	return &breaker{
		state:  StateClosed,
		config: config,
	}
}

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if timeout has passed
		if time.Since(b.lastFailureTime) > b.config.Timeout {
			b.toHalfOpen()
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited requests
		return true
	}

	return false
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		b.failures = 0

	case StateHalfOpen:
		b.successes++
		if b.successes >= b.config.SuccessThreshold {
			b.toClosed()
		}
	}
}

func (b *breaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	b.lastFailureTime = time.Now()

	switch b.state {
	case StateClosed:
		if b.failures >= b.config.FailureThreshold {
			b.toOpen()
		}

	case StateHalfOpen:
		b.toOpen()
	}
}

func (b *breaker) getState() CircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check for automatic transition
	if b.state == StateOpen && time.Since(b.lastFailureTime) > b.config.Timeout {
		b.toHalfOpen()
	}

	return b.state
}

func (b *breaker) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state = StateClosed
	b.failures = 0
	b.successes = 0
}

func (b *breaker) toClosed() {
	oldState := b.state
	b.state = StateClosed
	b.failures = 0
	b.successes = 0

	if b.config.OnStateChange != nil {
		b.config.OnStateChange("", oldState, StateClosed)
	}
}

func (b *breaker) toOpen() {
	oldState := b.state
	b.state = StateOpen
	b.successes = 0

	if b.config.OnStateChange != nil {
		b.config.OnStateChange("", oldState, StateOpen)
	}
}

func (b *breaker) toHalfOpen() {
	oldState := b.state
	b.state = StateHalfOpen
	b.failures = 0
	b.successes = 0

	if b.config.OnStateChange != nil {
		b.config.OnStateChange("", oldState, StateHalfOpen)
	}
}
