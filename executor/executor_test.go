package executor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	internalexec "github.com/victoralfred/goexec/internal/exec"
)

// mockRunner is a mock implementation of the internal runner
type mockRunner struct {
	runFunc func(ctx context.Context, config *internalexec.RunConfig) (*internalexec.RunResult, error)
}

func (m *mockRunner) Run(ctx context.Context, config *internalexec.RunConfig) (*internalexec.RunResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, config)
	}
	return &internalexec.RunResult{
		ExitCode: 0,
		Stdout:   []byte("output"),
		Stderr:   []byte(""),
		Duration: 100 * time.Millisecond,
		ProcessState: &internalexec.ProcessState{
			Pid:        1234,
			UserTime:   50 * time.Millisecond,
			SystemTime: 50 * time.Millisecond,
		},
	}, nil
}

// mockPolicy is a mock policy implementation
type mockPolicy struct {
	validateFunc func(ctx context.Context, cmd *Command) (*ValidationResult, error)
}

func (m *mockPolicy) Validate(ctx context.Context, cmd *Command) (*ValidationResult, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, cmd)
	}
	return &ValidationResult{Allowed: true}, nil
}

// mockRateLimiter is a mock rate limiter
type mockRateLimiter struct {
	allowFunc func(binary string) bool
	waitFunc  func(ctx context.Context, binary string) error
}

func (m *mockRateLimiter) Allow(binary string) bool {
	if m.allowFunc != nil {
		return m.allowFunc(binary)
	}
	return true
}

func (m *mockRateLimiter) Wait(ctx context.Context, binary string) error {
	if m.waitFunc != nil {
		return m.waitFunc(ctx, binary)
	}
	return nil
}

// mockCircuitBreaker is a mock circuit breaker
type mockCircuitBreaker struct {
	allowFunc         func(binary string) bool
	recordSuccessFunc func(binary string)
	recordFailureFunc func(binary string)
}

func (m *mockCircuitBreaker) Allow(binary string) bool {
	if m.allowFunc != nil {
		return m.allowFunc(binary)
	}
	return true
}

func (m *mockCircuitBreaker) RecordSuccess(binary string) {
	if m.recordSuccessFunc != nil {
		m.recordSuccessFunc(binary)
	}
}

func (m *mockCircuitBreaker) RecordFailure(binary string) {
	if m.recordFailureFunc != nil {
		m.recordFailureFunc(binary)
	}
}

// mockTelemetry is a mock telemetry implementation
type mockTelemetry struct {
	startSpanFunc    func(ctx context.Context, name string) (context.Context, func())
	recordMetricFunc func(name string, value float64, labels map[string]string)
}

func (m *mockTelemetry) StartSpan(ctx context.Context, name string) (context.Context, func()) {
	if m.startSpanFunc != nil {
		return m.startSpanFunc(ctx, name)
	}
	return ctx, func() {}
}

func (m *mockTelemetry) RecordMetric(name string, value float64, labels map[string]string) {
	if m.recordMetricFunc != nil {
		m.recordMetricFunc(name, value, labels)
	}
}

// mockHook is a mock hook implementation
type mockHook struct {
	preExecuteFunc  func(ctx context.Context, cmd *Command) (*Command, error)
	postExecuteFunc func(ctx context.Context, cmd *Command, result *Result, err error) error
}

func (m *mockHook) PreExecute(ctx context.Context, cmd *Command) (*Command, error) {
	if m.preExecuteFunc != nil {
		return m.preExecuteFunc(ctx, cmd)
	}
	return cmd, nil
}

func (m *mockHook) PostExecute(ctx context.Context, cmd *Command, result *Result, err error) error {
	if m.postExecuteFunc != nil {
		return m.postExecuteFunc(ctx, cmd, result, err)
	}
	return nil
}

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()
	if builder == nil {
		t.Fatal("NewBuilder() returned nil")
	}

	exec, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if exec == nil {
		t.Fatal("Build() returned nil executor")
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	exec, _ := NewBuilder().Build()
	defer exec.Shutdown(context.Background())

	cmd, err := NewCommand("/bin/echo", "hello").Build()
	if err != nil {
		t.Fatalf("Failed to build command: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		t.Logf("Execute failed (may be expected if /bin/echo doesn't exist): %v", err)
		return
	}

	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.CommandID == "" {
		t.Error("CommandID should not be empty")
	}
}

func TestExecutor_Execute_Shutdown(t *testing.T) {
	exec, _ := NewBuilder().Build()
	exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "hello").Build()
	ctx := context.Background()

	_, err := exec.Execute(ctx, cmd)
	if !errors.Is(err, ErrExecutorShutdown) {
		t.Errorf("Expected ErrExecutorShutdown, got %v", err)
	}
}

func TestExecutor_Execute_PolicyDenied(t *testing.T) {
	policy := &mockPolicy{
		validateFunc: func(ctx context.Context, cmd *Command) (*ValidationResult, error) {
			return &ValidationResult{
				Allowed: false,
				Violations: []Violation{
					{Code: "POLICY_001", Field: "binary", Message: "Binary not allowed"},
				},
			}, nil
		},
	}

	exec, _ := NewBuilder().WithPolicy(policy).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/forbidden", "arg").Build()
	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	if err == nil {
		t.Error("Expected error for denied command")
	}
	if result == nil {
		t.Fatal("Result should not be nil even on policy denial")
	}
	if result.Status != StatusPolicyDenied {
		t.Errorf("Expected StatusPolicyDenied, got %v", result.Status)
	}
}

func TestExecutor_Execute_RateLimited(t *testing.T) {
	rateLimiter := &mockRateLimiter{
		waitFunc: func(ctx context.Context, binary string) error {
			return errors.New("rate limit exceeded")
		},
	}

	exec, _ := NewBuilder().WithRateLimiter(rateLimiter).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "hello").Build()
	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	if err == nil {
		t.Error("Expected error for rate limited command")
	}
	if result != nil && result.Status != StatusRateLimited {
		t.Errorf("Expected StatusRateLimited, got %v", result.Status)
	}
}

func TestExecutor_Execute_CircuitOpen(t *testing.T) {
	circuitBreaker := &mockCircuitBreaker{
		allowFunc: func(binary string) bool {
			return false
		},
	}

	exec, _ := NewBuilder().WithCircuitBreaker(circuitBreaker).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "hello").Build()
	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	if err == nil {
		t.Error("Expected error for circuit open")
	}
	if result != nil && result.Status != StatusCircuitOpen {
		t.Errorf("Expected StatusCircuitOpen, got %v", result.Status)
	}
}

func TestExecutor_Execute_Timeout(t *testing.T) {
	exec, _ := NewBuilder().WithDefaultTimeout(100 * time.Millisecond).Build()
	defer exec.Shutdown(context.Background())

	// Use a command that will timeout
	cmd, _ := NewCommand("/bin/sleep", "10").Build()
	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	// May fail for different reasons (command not found, permission denied, etc.)
	if result != nil && result.Status == StatusTimeout {
		if result.Duration < 100*time.Millisecond || result.Duration > 500*time.Millisecond {
			t.Errorf("Duration should be around timeout, got %v", result.Duration)
		}
	}
	_ = err // Error is expected here
}

func TestExecutor_Execute_Hooks(t *testing.T) {
	var preCalled, postCalled bool
	var preCmd, postCmd *Command
	var postResult *Result

	hook := &mockHook{
		preExecuteFunc: func(ctx context.Context, cmd *Command) (*Command, error) {
			preCalled = true
			preCmd = cmd
			return cmd, nil
		},
		postExecuteFunc: func(ctx context.Context, cmd *Command, result *Result, err error) error {
			postCalled = true
			postCmd = cmd
			postResult = result
			return nil
		},
	}

	exec, _ := NewBuilder().WithHooks(hook).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "test").Build()
	ctx := context.Background()

	exec.Execute(ctx, cmd) // Ignore error

	if !preCalled {
		t.Error("PreExecute hook was not called")
	}
	if preCmd == nil {
		t.Error("PreExecute hook did not receive command")
	}
	if !postCalled {
		t.Error("PostExecute hook was not called")
	}
	if postCmd == nil {
		t.Error("PostExecute hook did not receive command")
	}
	if postResult == nil {
		t.Error("PostExecute hook did not receive result")
	}
}

func TestExecutor_Execute_Telemetry(t *testing.T) {
	var spanStarted bool
	var metricRecorded bool

	telemetry := &mockTelemetry{
		startSpanFunc: func(ctx context.Context, name string) (context.Context, func()) {
			spanStarted = true
			return ctx, func() {}
		},
		recordMetricFunc: func(name string, value float64, labels map[string]string) {
			metricRecorded = true
			if name != "executor.execution_duration_ms" {
				t.Errorf("Unexpected metric name: %s", name)
			}
		},
	}

	exec, _ := NewBuilder().WithTelemetry(telemetry).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "test").Build()
	ctx := context.Background()

	exec.Execute(ctx, cmd) // Ignore error

	if !spanStarted {
		t.Error("Telemetry span was not started")
	}
	if !metricRecorded {
		t.Error("Telemetry metric was not recorded")
	}
}

func TestExecutor_ExecuteAsync(t *testing.T) {
	exec, _ := NewBuilder().Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "hello").Build()
	ctx := context.Background()

	future := exec.ExecuteAsync(ctx, cmd)
	if future == nil {
		t.Fatal("ExecuteAsync returned nil future")
	}

	select {
	case <-future.Done():
		result, err := future.Wait()
		if result == nil && err == nil {
			t.Error("Future completed but both result and error are nil")
		}
	case <-time.After(5 * time.Second):
		t.Error("Future did not complete within timeout")
	}
}

func TestExecutor_ExecuteBatch(t *testing.T) {
	exec, _ := NewBuilder().Build()
	defer exec.Shutdown(context.Background())

	cmds := []*Command{
		NewCommand("/bin/echo", "cmd1").MustBuild(),
		NewCommand("/bin/echo", "cmd2").MustBuild(),
		NewCommand("/bin/echo", "cmd3").MustBuild(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := exec.ExecuteBatch(ctx, cmds)
	if err != nil {
		t.Logf("ExecuteBatch failed (may be expected): %v", err)
		return
	}

	if len(results) != len(cmds) {
		t.Errorf("Expected %d results, got %d", len(cmds), len(results))
	}
}

func TestExecutor_Stream(t *testing.T) {
	exec, _ := NewBuilder().Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "streamed output").Build()
	ctx := context.Background()

	var stdout, stderr strings.Builder

	err := exec.Stream(ctx, cmd, &stdout, &stderr)
	if err != nil {
		t.Logf("Stream failed (may be expected): %v", err)
		return
	}

	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Log("No output captured (command may not exist)")
	}
}

func TestExecutor_Shutdown(t *testing.T) {
	exec, _ := NewBuilder().Build()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := exec.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestExecutor_Shutdown_WithTimeout(t *testing.T) {
	exec, _ := NewBuilder().Build()

	// Start a long-running operation
	cmd, _ := NewCommand("/bin/sleep", "1").Build()
	go func() {
		ctx := context.Background()
		exec.Execute(ctx, cmd)
	}()

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err := exec.Shutdown(ctx)
	if err == nil {
		t.Log("Shutdown completed (operations finished quickly)")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("Shutdown error: %v", err)
	}
}

func TestExecutor_Execute_CircuitBreakerRecording(t *testing.T) {
	var successRecorded, failureRecorded bool

	circuitBreaker := &mockCircuitBreaker{
		allowFunc: func(binary string) bool { return true },
		recordSuccessFunc: func(binary string) {
			successRecorded = true
		},
		recordFailureFunc: func(binary string) {
			failureRecorded = true
		},
	}

	exec, _ := NewBuilder().WithCircuitBreaker(circuitBreaker).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "test").Build()
	ctx := context.Background()

	exec.Execute(ctx, cmd) // Ignore error

	// At least one should be called (success or failure)
	if !successRecorded && !failureRecorded {
		t.Error("Circuit breaker was not notified of result")
	}
}

func TestExecutor_Execute_CommandTimeout(t *testing.T) {
	exec, _ := NewBuilder().WithDefaultTimeout(30 * time.Second).Build()
	defer exec.Shutdown(context.Background())

	// Command with explicit timeout
	cmd, _ := NewCommand("/bin/echo", "test").
		WithTimeout(100 * time.Millisecond).
		Build()

	ctx := context.Background()

	result, _ := exec.Execute(ctx, cmd)
	if result != nil && result.CommandID == "" {
		t.Error("CommandID should not be empty")
	}
}

func TestExecutor_Execute_EmptyArgs(t *testing.T) {
	exec, _ := NewBuilder().Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo").Build()
	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		t.Logf("Execute with empty args failed (may be expected): %v", err)
		return
	}
	if result == nil {
		t.Fatal("Result should not be nil")
	}
}

func TestExecutor_Execute_WithStdin(t *testing.T) {
	exec, _ := NewBuilder().Build()
	defer exec.Shutdown(context.Background())

	stdin := strings.NewReader("test input")
	cmd, _ := NewCommand("/bin/cat").
		WithStdin(stdin).
		Build()

	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		t.Logf("Execute with stdin failed (may be expected): %v", err)
		return
	}
	if result == nil {
		t.Fatal("Result should not be nil")
	}
}

func TestResultFuture(t *testing.T) {
	cancelCalled := false
	cancel := func() {
		cancelCalled = true
	}

	future := NewResultFuture(cancel)
	if future == nil {
		t.Fatal("NewResultFuture returned nil")
	}

	// Test Cancel before completion
	future.Cancel()
	if !cancelCalled {
		t.Error("Cancel did not call the cancel function")
	}

	// Complete the future
	result := &Result{CommandID: "test", Status: StatusSuccess}
	future.Complete(result, nil)

	// Wait should return immediately
	gotResult, err := future.Wait()
	if err != nil {
		t.Errorf("Wait returned error: %v", err)
	}
	if gotResult.CommandID != "test" {
		t.Errorf("Expected CommandID 'test', got %s", gotResult.CommandID)
	}

	// Done channel should be closed
	select {
	case <-future.Done():
		// Expected
	default:
		t.Error("Done channel should be closed after completion")
	}
}

func TestResultFuture_WithError(t *testing.T) {
	future := NewResultFuture(nil)

	testErr := errors.New("test error")
	future.Complete(nil, testErr)

	_, err := future.Wait()
	if err == nil {
		t.Error("Expected error from Wait")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("Expected testErr, got %v", err)
	}
}

func TestResultFuture_ConcurrentAccess(t *testing.T) {
	future := NewResultFuture(nil)

	var wg sync.WaitGroup
	var completedOnce sync.Once

	// Try to complete multiple times (only first should work)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			result := &Result{CommandID: "test", Status: StatusSuccess}
			completedOnce.Do(func() {
				future.Complete(result, nil)
			})
		}()
	}

	// Multiple waiters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := future.Wait()
			if err != nil {
				t.Errorf("Wait returned error: %v", err)
			}
			if result == nil {
				t.Error("Wait returned nil result")
			}
		}()
	}

	wg.Wait()

	// Should complete successfully despite concurrent access
	result, err := future.Wait()
	if err != nil {
		t.Errorf("Final Wait returned error: %v", err)
	}
	if result == nil {
		t.Error("Final Wait returned nil result")
	}
}

func TestResult_Methods(t *testing.T) {
	result := &Result{
		Status:    StatusSuccess,
		ExitCode:  0,
		Stdout:    []byte("output"),
		Stderr:    []byte("errors"),
		Duration:  100 * time.Millisecond,
		CommandID: "test-123",
	}

	if !result.Success() {
		t.Error("Expected Success() to return true")
	}

	if result.Failed() {
		t.Error("Expected Failed() to return false")
	}

	if result.StdoutString() != "output" {
		t.Errorf("Expected stdout 'output', got %s", result.StdoutString())
	}

	if result.StderrString() != "errors" {
		t.Errorf("Expected stderr 'errors', got %s", result.StderrString())
	}

	result.Status = StatusError
	result.ExitCode = 1
	if result.Success() {
		t.Error("Expected Success() to return false for error status")
	}
	if !result.Failed() {
		t.Error("Expected Failed() to return true for error status")
	}
}

func TestExitStatus_String(t *testing.T) {
	tests := []struct {
		status ExitStatus
		want   string
	}{
		{StatusSuccess, "success"},
		{StatusError, "error"},
		{StatusTimeout, "timeout"},
		{StatusCanceled, "canceled"},
		{StatusKilled, "killed"},
		{StatusResourceExceeded, "resource_exceeded"},
		{StatusPolicyDenied, "policy_denied"},
		{StatusSandboxViolation, "sandbox_violation"},
		{StatusRateLimited, "rate_limited"},
		{StatusCircuitOpen, "circuit_open"},
		{ExitStatus(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("Status(%d).String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestExitStatus_IsRetryable(t *testing.T) {
	tests := []struct {
		status    ExitStatus
		retryable bool
	}{
		{StatusSuccess, false},
		{StatusError, false},
		{StatusTimeout, true},
		{StatusCanceled, false},
		{StatusRateLimited, true},
		{StatusCircuitOpen, true},
	}

	for _, tt := range tests {
		got := tt.status.IsRetryable()
		if got != tt.retryable {
			t.Errorf("Status(%v).IsRetryable() = %v, want %v", tt.status, got, tt.retryable)
		}
	}
}

func TestResourceUsage(t *testing.T) {
	usage := &ResourceUsage{
		UserTime:   50 * time.Millisecond,
		SystemTime: 30 * time.Millisecond,
	}

	total := usage.TotalCPUTime()
	expected := 80 * time.Millisecond
	if total != expected {
		t.Errorf("TotalCPUTime() = %v, want %v", total, expected)
	}
}

func TestExecutor_Execute_PreHookError(t *testing.T) {
	hookErr := errors.New("hook error")
	hook := &mockHook{
		preExecuteFunc: func(ctx context.Context, cmd *Command) (*Command, error) {
			return nil, hookErr
		},
	}

	exec, _ := NewBuilder().WithHooks(hook).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "test").Build()
	ctx := context.Background()

	_, err := exec.Execute(ctx, cmd)
	if err == nil {
		t.Error("Expected error from pre-execute hook")
	}
	if !errors.Is(err, hookErr) {
		t.Errorf("Expected hookErr, got %v", err)
	}
}

func TestExecutor_Execute_PostHookError(t *testing.T) {
	hookErr := errors.New("hook error")
	hook := &mockHook{
		postExecuteFunc: func(ctx context.Context, cmd *Command, result *Result, err error) error {
			return hookErr
		},
	}

	exec, _ := NewBuilder().WithHooks(hook).Build()
	defer exec.Shutdown(context.Background())

	cmd, _ := NewCommand("/bin/echo", "test").Build()
	ctx := context.Background()

	result, err := exec.Execute(ctx, cmd)
	if err == nil {
		t.Error("Expected error from post-execute hook")
	}
	if result == nil {
		t.Error("Result should not be nil even on hook error")
	}
}
