package executor

import (
	"time"
)

// Result contains the outcome of command execution.
type Result struct {
	ResourceUsage *ResourceUsage
	Signal        string
	TraceID       string
	CommandID     string
	Stdout        []byte
	Stderr        []byte
	Status        ExitStatus
	ExitCode      int
	Duration      time.Duration
	CPUTime       time.Duration
	MemoryPeak    int64
}

// ExitStatus represents the outcome of command execution.
type ExitStatus int

const (
	// StatusSuccess indicates successful execution (exit code 0).
	StatusSuccess ExitStatus = iota
	// StatusError indicates non-zero exit code.
	StatusError
	// StatusTimeout indicates execution timeout.
	StatusTimeout
	// StatusCanceled indicates context was canceled.
	StatusCanceled
	// StatusKilled indicates process was killed by signal.
	StatusKilled
	// StatusResourceExceeded indicates resource limit exceeded.
	StatusResourceExceeded
	// StatusPolicyDenied indicates command was denied by policy.
	StatusPolicyDenied
	// StatusSandboxViolation indicates sandbox security violation.
	StatusSandboxViolation
	// StatusRateLimited indicates rate limit exceeded.
	StatusRateLimited
	// StatusCircuitOpen indicates circuit breaker is open.
	StatusCircuitOpen
)

// String returns the string representation of the exit status.
func (s ExitStatus) String() string {
	switch s {
	case StatusSuccess:
		return "success"
	case StatusError:
		return "error"
	case StatusTimeout:
		return "timeout"
	case StatusCanceled:
		return "canceled"
	case StatusKilled:
		return "killed"
	case StatusResourceExceeded:
		return "resource_exceeded"
	case StatusPolicyDenied:
		return "policy_denied"
	case StatusSandboxViolation:
		return "sandbox_violation"
	case StatusRateLimited:
		return "rate_limited"
	case StatusCircuitOpen:
		return "circuit_open"
	default:
		return "unknown"
	}
}

// IsSuccess returns true if the command succeeded.
func (s ExitStatus) IsSuccess() bool {
	return s == StatusSuccess
}

// IsRetryable returns true if the operation can be retried.
func (s ExitStatus) IsRetryable() bool {
	switch s {
	case StatusTimeout, StatusRateLimited, StatusCircuitOpen:
		return true
	default:
		return false
	}
}

// ResourceUsage contains detailed resource consumption metrics.
type ResourceUsage struct {
	// UserTime is the user CPU time consumed.
	UserTime time.Duration

	// SystemTime is the system CPU time consumed.
	SystemTime time.Duration

	// MaxRSS is the maximum resident set size in bytes.
	MaxRSS int64

	// MinorFaults is the number of minor page faults.
	MinorFaults int64

	// MajorFaults is the number of major page faults.
	MajorFaults int64

	// IOReadBytes is the number of bytes read from I/O.
	IOReadBytes int64

	// IOWriteBytes is the number of bytes written to I/O.
	IOWriteBytes int64

	// Syscalls is the number of syscalls (if sandboxed with auditing).
	Syscalls int64
}

// TotalCPUTime returns the total CPU time (user + system).
func (r *ResourceUsage) TotalCPUTime() time.Duration {
	return r.UserTime + r.SystemTime
}

// Success returns true if the result indicates success.
func (r *Result) Success() bool {
	return r.Status == StatusSuccess && r.ExitCode == 0
}

// Failed returns true if the result indicates failure.
func (r *Result) Failed() bool {
	return !r.Success()
}

// StdoutString returns stdout as a string.
func (r *Result) StdoutString() string {
	return string(r.Stdout)
}

// StderrString returns stderr as a string.
func (r *Result) StderrString() string {
	return string(r.Stderr)
}

// Future represents an asynchronous result.
type Future[T any] interface {
	// Wait blocks until the result is available.
	Wait() (T, error)

	// Done returns a channel that is closed when the result is ready.
	Done() <-chan struct{}

	// Cancel attempts to cancel the operation.
	Cancel()
}

// ResultFuture implements Future for Result.
type ResultFuture struct {
	result *Result
	err    error
	done   chan struct{}
	cancel func()
}

// NewResultFuture creates a new result future.
func NewResultFuture(cancel func()) *ResultFuture {
	return &ResultFuture{
		done:   make(chan struct{}),
		cancel: cancel,
	}
}

// Complete sets the result and signals completion.
func (f *ResultFuture) Complete(result *Result, err error) {
	f.result = result
	f.err = err
	close(f.done)
}

// Wait blocks until the result is available.
func (f *ResultFuture) Wait() (*Result, error) {
	<-f.done
	return f.result, f.err
}

// Done returns a channel that is closed when the result is ready.
func (f *ResultFuture) Done() <-chan struct{} {
	return f.done
}

// Cancel attempts to cancel the operation.
func (f *ResultFuture) Cancel() {
	if f.cancel != nil {
		f.cancel()
	}
}
