package executor

import (
	"context"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/victoralfred/goexec/internal/envutil"
	internalexec "github.com/victoralfred/goexec/internal/exec"
)

// Executor is the single abstraction for all process invocation.
// All command execution MUST go through this interface.
type Executor interface {
	// Execute runs a command synchronously with the given context.
	Execute(ctx context.Context, cmd *Command) (*Result, error)

	// ExecuteAsync runs a command asynchronously, returning a Future.
	ExecuteAsync(ctx context.Context, cmd *Command) Future[*Result]

	// ExecuteBatch runs multiple commands with optional batching optimization.
	ExecuteBatch(ctx context.Context, cmds []*Command) ([]*Result, error)

	// Stream executes a command with streaming I/O.
	Stream(ctx context.Context, cmd *Command, stdout, stderr io.Writer) error

	// Shutdown gracefully shuts down the executor, waiting for pending commands.
	Shutdown(ctx context.Context) error
}

// Policy defines the security policy interface.
type Policy interface {
	// Validate checks if a command is allowed by the policy.
	Validate(ctx context.Context, cmd *Command) (*ValidationResult, error)
}

// ValidationResult contains the outcome of policy validation.
type ValidationResult struct {
	Reason     string
	Violations []Violation
	Allowed    bool
}

// Sandbox provides process isolation.
type Sandbox interface {
	// Prepare sets up the sandbox for a command.
	Prepare(ctx context.Context, cmd *Command) (SandboxConfig, error)
}

// SandboxConfig contains sandbox configuration.
type SandboxConfig interface {
	// Apply applies sandbox restrictions.
	Apply() error
}

// WorkerPool manages bounded worker pool.
type WorkerPool interface {
	// Submit submits a task to the pool.
	Submit(ctx context.Context, task func()) error
}

// RateLimiter controls execution rate.
type RateLimiter interface {
	// Allow checks if execution is allowed.
	Allow(binary string) bool
	// Wait blocks until execution is allowed.
	Wait(ctx context.Context, binary string) error
}

// CircuitBreaker provides circuit breaker functionality.
type CircuitBreaker interface {
	// Allow checks if execution is allowed.
	Allow(binary string) bool
	// RecordSuccess records a successful execution.
	RecordSuccess(binary string)
	// RecordFailure records a failed execution.
	RecordFailure(binary string)
}

// Hook defines extension points.
type Hook interface {
	// PreExecute is called before command execution.
	PreExecute(ctx context.Context, cmd *Command) (*Command, error)
	// PostExecute is called after command execution.
	PostExecute(ctx context.Context, cmd *Command, result *Result, err error) error
}

// Telemetry provides observability.
type Telemetry interface {
	// StartSpan starts a new trace span.
	StartSpan(ctx context.Context, name string) (context.Context, func())
	// RecordMetric records a metric.
	RecordMetric(name string, value float64, labels map[string]string)
}

// executor is the default implementation.
type executor struct {
	policy         Policy
	sandbox        Sandbox
	pool           WorkerPool
	rateLimiter    RateLimiter
	circuitBreaker CircuitBreaker
	telemetry      Telemetry
	runner         *internalexec.Runner
	hooks          []Hook
	wg             sync.WaitGroup
	mu             sync.RWMutex // protects shutdown check and wg.Add
	defaultTimeout time.Duration
	shutdown       int32
}

// Builder creates configured Executor instances.
type Builder struct {
	policy         Policy
	sandbox        Sandbox
	pool           WorkerPool
	rateLimiter    RateLimiter
	circuitBreaker CircuitBreaker
	telemetry      Telemetry
	hooks          []Hook
	defaultTimeout time.Duration
}

// NewBuilder creates a new executor builder.
func NewBuilder() *Builder {
	return &Builder{
		defaultTimeout: 30 * time.Second,
	}
}

// WithPolicy sets the security policy.
func (b *Builder) WithPolicy(policy Policy) *Builder {
	b.policy = policy
	return b
}

// WithSandbox sets the sandbox.
func (b *Builder) WithSandbox(sandbox Sandbox) *Builder {
	b.sandbox = sandbox
	return b
}

// WithPool sets the worker pool.
func (b *Builder) WithPool(pool WorkerPool) *Builder {
	b.pool = pool
	return b
}

// WithRateLimiter sets the rate limiter.
func (b *Builder) WithRateLimiter(limiter RateLimiter) *Builder {
	b.rateLimiter = limiter
	return b
}

// WithCircuitBreaker sets the circuit breaker.
func (b *Builder) WithCircuitBreaker(cb CircuitBreaker) *Builder {
	b.circuitBreaker = cb
	return b
}

// WithHooks adds execution hooks.
func (b *Builder) WithHooks(hooks ...Hook) *Builder {
	b.hooks = append(b.hooks, hooks...)
	return b
}

// WithTelemetry sets the telemetry provider.
func (b *Builder) WithTelemetry(telemetry Telemetry) *Builder {
	b.telemetry = telemetry
	return b
}

// WithDefaultTimeout sets the default execution timeout.
func (b *Builder) WithDefaultTimeout(timeout time.Duration) *Builder {
	b.defaultTimeout = timeout
	return b
}

// Build creates the executor.
func (b *Builder) Build() (Executor, error) {
	return &executor{
		runner:         internalexec.NewRunner(),
		policy:         b.policy,
		sandbox:        b.sandbox,
		pool:           b.pool,
		rateLimiter:    b.rateLimiter,
		circuitBreaker: b.circuitBreaker,
		hooks:          b.hooks,
		telemetry:      b.telemetry,
		defaultTimeout: b.defaultTimeout,
	}, nil
}

// Execute runs a command synchronously.
func (e *executor) Execute(ctx context.Context, cmd *Command) (*Result, error) {
	// Use mutex to ensure shutdown check and wg.Add are atomic
	// This prevents a race where Shutdown starts wg.Wait() between our check and Add
	e.mu.RLock()
	if atomic.LoadInt32(&e.shutdown) == 1 {
		e.mu.RUnlock()
		return nil, ErrExecutorShutdown
	}
	e.wg.Add(1)
	e.mu.RUnlock()

	defer e.wg.Done()

	// Start telemetry span
	if e.telemetry != nil {
		var endSpan func()
		ctx, endSpan = e.telemetry.StartSpan(ctx, "executor.Execute")
		defer endSpan()
	}

	// Generate command ID
	commandID := uuid.New().String()

	// Run pre-execute hooks
	var err error
	cmd, err = e.runPreHooks(ctx, cmd)
	if err != nil {
		return nil, err
	}

	// Validate against policy
	if e.policy != nil {
		result, err := e.policy.Validate(ctx, cmd)
		if err != nil {
			return nil, err
		}
		if !result.Allowed {
			return &Result{
				Status:    StatusPolicyDenied,
				CommandID: commandID,
			}, NewPolicyError(cmd.Binary, result.Violations)
		}
	}

	// Check rate limiter
	if e.rateLimiter != nil {
		if err := e.rateLimiter.Wait(ctx, cmd.Binary); err != nil {
			return &Result{
				Status:    StatusRateLimited,
				CommandID: commandID,
			}, NewRateLimitError(cmd.Binary)
		}
	}

	// Check circuit breaker
	if e.circuitBreaker != nil {
		if !e.circuitBreaker.Allow(cmd.Binary) {
			return &Result{
				Status:    StatusCircuitOpen,
				CommandID: commandID,
			}, NewCircuitOpenError(cmd.Binary)
		}
	}

	// Determine timeout
	timeout := cmd.Timeout
	if timeout == 0 {
		timeout = e.defaultTimeout
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build run configuration
	// Merge command environment with minimal safe environment
	// This ensures we always have a safe base, even if cmd.Env is empty
	minimalEnv := envutil.MinimalEnvironment()
	mergedEnv := envutil.MergeEnvironment(minimalEnv, cmd.Env)
	config := &internalexec.RunConfig{
		Binary:     cmd.Binary,
		Args:       cmd.Args,
		Env:        internalexec.BuildEnv(mergedEnv),
		WorkingDir: cmd.WorkingDir,
		Stdin:      cmd.Stdin,
	}

	// Execute command
	runResult, runErr := e.runner.Run(execCtx, config)

	// Build result
	result := e.buildResult(runResult, runErr, commandID)

	// Record circuit breaker result
	if e.circuitBreaker != nil {
		if result.Success() {
			e.circuitBreaker.RecordSuccess(cmd.Binary)
		} else {
			e.circuitBreaker.RecordFailure(cmd.Binary)
		}
	}

	// Record metrics
	if e.telemetry != nil {
		e.telemetry.RecordMetric("executor.execution_duration_ms", float64(result.Duration.Milliseconds()), map[string]string{
			"binary":   cmd.Binary,
			"status":   result.Status.String(),
			"exitcode": strconv.Itoa(result.ExitCode),
		})
	}

	// Run post-execute hooks
	if hookErr := e.runPostHooks(ctx, cmd, result, runErr); hookErr != nil {
		return result, hookErr
	}

	return result, runErr
}

// ExecuteAsync runs a command asynchronously.
func (e *executor) ExecuteAsync(ctx context.Context, cmd *Command) Future[*Result] {
	asyncCtx, cancel := context.WithCancel(ctx)
	future := NewResultFuture(cancel)

	go func() {
		result, err := e.Execute(asyncCtx, cmd)
		future.Complete(result, err)
	}()

	return future
}

// ExecuteBatch runs multiple commands.
func (e *executor) ExecuteBatch(ctx context.Context, cmds []*Command) ([]*Result, error) {
	results := make([]*Result, len(cmds))
	errs := make([]error, len(cmds))

	var wg sync.WaitGroup
	for i, cmd := range cmds {
		wg.Add(1)
		go func(idx int, c *Command) {
			defer wg.Done()
			results[idx], errs[idx] = e.Execute(ctx, c)
		}(i, cmd)
	}

	wg.Wait()

	// Return first error encountered
	for _, err := range errs {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Stream executes a command with streaming I/O.
func (e *executor) Stream(ctx context.Context, cmd *Command, stdout, stderr io.Writer) error {
	// Use mutex to ensure shutdown check and wg.Add are atomic
	// This prevents a race where Shutdown starts wg.Wait() between our check and Add
	e.mu.RLock()
	if atomic.LoadInt32(&e.shutdown) == 1 {
		e.mu.RUnlock()
		return ErrExecutorShutdown
	}
	e.wg.Add(1)
	e.mu.RUnlock()

	defer e.wg.Done()

	// Validate against policy
	if e.policy != nil {
		result, err := e.policy.Validate(ctx, cmd)
		if err != nil {
			return err
		}
		if !result.Allowed {
			return NewPolicyError(cmd.Binary, result.Violations)
		}
	}

	// Determine timeout
	timeout := cmd.Timeout
	if timeout == 0 {
		timeout = e.defaultTimeout
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build run configuration with streaming
	// Merge command environment with minimal safe environment
	// This ensures we always have a safe base, even if cmd.Env is empty
	minimalEnv := envutil.MinimalEnvironment()
	mergedEnv := envutil.MergeEnvironment(minimalEnv, cmd.Env)
	config := &internalexec.RunConfig{
		Binary:     cmd.Binary,
		Args:       cmd.Args,
		Env:        internalexec.BuildEnv(mergedEnv),
		WorkingDir: cmd.WorkingDir,
		Stdin:      cmd.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
	}

	_, err := e.runner.Run(execCtx, config)
	return err
}

// Shutdown gracefully shuts down the executor.
func (e *executor) Shutdown(ctx context.Context) error {
	// Acquire write lock to prevent new executions from starting
	// Any Execute calls will block on RLock until we release
	e.mu.Lock()
	atomic.StoreInt32(&e.shutdown, 1)
	e.mu.Unlock()

	// Now wait for any in-progress executions to complete
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runPreHooks runs pre-execute hooks.
// Hooks are read-only after executor creation, so no lock needed.
func (e *executor) runPreHooks(ctx context.Context, cmd *Command) (*Command, error) {
	// Create a local copy of hooks slice to avoid potential races if hooks are modified
	// In practice, hooks are immutable after Build(), but defensive copying is safer
	hooks := e.hooks
	if len(hooks) == 0 {
		return cmd, nil
	}

	current := cmd
	for _, hook := range hooks {
		modified, err := hook.PreExecute(ctx, current)
		if err != nil {
			return nil, err
		}
		current = modified
	}
	return current, nil
}

// runPostHooks runs post-execute hooks.
// Hooks are read-only after executor creation, so no lock needed.
func (e *executor) runPostHooks(ctx context.Context, cmd *Command, result *Result, execErr error) error {
	// Create a local copy of hooks slice to avoid potential races
	hooks := e.hooks
	if len(hooks) == 0 {
		return nil
	}

	for _, hook := range hooks {
		if err := hook.PostExecute(ctx, cmd, result, execErr); err != nil {
			return err
		}
	}
	return nil
}

// buildResult builds a Result from the internal run result.
func (e *executor) buildResult(runResult *internalexec.RunResult, runErr error, commandID string) *Result {
	result := &Result{
		CommandID: commandID,
	}

	if runResult == nil {
		result.Status = StatusError
		return result
	}

	result.ExitCode = runResult.ExitCode
	result.Stdout = runResult.Stdout
	result.Stderr = runResult.Stderr
	result.Duration = runResult.Duration

	if runResult.Signal != 0 {
		result.Signal = runResult.Signal.String()
	}

	if runResult.ProcessState != nil {
		result.CPUTime = runResult.ProcessState.UserTime + runResult.ProcessState.SystemTime
		result.ResourceUsage = &ResourceUsage{
			UserTime:   runResult.ProcessState.UserTime,
			SystemTime: runResult.ProcessState.SystemTime,
		}
	}

	// Determine status
	switch {
	case runErr == nil && runResult.ExitCode == 0:
		result.Status = StatusSuccess
	case runErr != nil && runErr == context.DeadlineExceeded:
		result.Status = StatusTimeout
	case runErr != nil && runErr == context.Canceled:
		result.Status = StatusCanceled
	case runResult.Signal != 0:
		result.Status = StatusKilled
	default:
		result.Status = StatusError
	}

	return result
}
