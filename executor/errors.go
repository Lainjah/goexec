package executor

import (
	"errors"
	"fmt"
)

// Sentinel errors for common conditions.
var (
	// ErrPolicyDenied indicates command was denied by policy.
	ErrPolicyDenied = errors.New("command denied by policy")

	// ErrBinaryNotAllowed indicates binary is not in allowlist.
	ErrBinaryNotAllowed = errors.New("binary not in allowlist")

	// ErrArgumentNotAllowed indicates argument is not allowed.
	ErrArgumentNotAllowed = errors.New("argument not in allowlist")

	// ErrPathTraversal indicates path traversal was detected.
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrInvalidPath indicates an invalid path.
	ErrInvalidPath = errors.New("invalid path")

	// ErrTimeout indicates command timed out.
	ErrTimeout = errors.New("command timed out")

	// ErrContextCanceled indicates context was canceled.
	ErrContextCanceled = errors.New("context canceled")

	// ErrResourceExceeded indicates resource limit was exceeded.
	ErrResourceExceeded = errors.New("resource limit exceeded")

	// ErrSandboxViolation indicates sandbox security violation.
	ErrSandboxViolation = errors.New("sandbox security violation")

	// ErrRateLimited indicates rate limit was exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrCircuitOpen indicates circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker open")

	// ErrPoolFull indicates worker pool is full.
	ErrPoolFull = errors.New("worker pool full")

	// ErrPoolShutdown indicates worker pool is shutdown.
	ErrPoolShutdown = errors.New("worker pool shutdown")

	// ErrInvalidCommand indicates invalid command configuration.
	ErrInvalidCommand = errors.New("invalid command")

	// ErrExecutorShutdown indicates executor is shutdown.
	ErrExecutorShutdown = errors.New("executor shutdown")
)

// ErrorCode provides structured error classification.
type ErrorCode string

const (
	// ErrCodePolicyViolation indicates a policy violation.
	ErrCodePolicyViolation ErrorCode = "POLICY_VIOLATION"

	// ErrCodeValidationFailed indicates validation failure.
	ErrCodeValidationFailed ErrorCode = "VALIDATION_FAILED"

	// ErrCodeExecutionFailed indicates execution failure.
	ErrCodeExecutionFailed ErrorCode = "EXECUTION_FAILED"

	// ErrCodeTimeout indicates timeout.
	ErrCodeTimeout ErrorCode = "TIMEOUT"

	// ErrCodeResourceExceeded indicates resource limit exceeded.
	ErrCodeResourceExceeded ErrorCode = "RESOURCE_EXCEEDED"

	// ErrCodeSandboxViolation indicates sandbox violation.
	ErrCodeSandboxViolation ErrorCode = "SANDBOX_VIOLATION"

	// ErrCodeRateLimited indicates rate limiting.
	ErrCodeRateLimited ErrorCode = "RATE_LIMITED"

	// ErrCodeCircuitOpen indicates circuit breaker open.
	ErrCodeCircuitOpen ErrorCode = "CIRCUIT_OPEN"

	// ErrCodeInternalError indicates internal error.
	ErrCodeInternalError ErrorCode = "INTERNAL_ERROR"
)

// ExecutionError provides detailed error information.
type ExecutionError struct {
	// Op is the operation that failed.
	Op string

	// Binary is the binary being executed.
	Binary string

	// Err is the underlying error.
	Err error

	// Code is the structured error code.
	Code ErrorCode

	// Details provides human-readable details.
	Details string

	// Suggestion provides a suggested fix.
	Suggestion string

	// Retryable indicates if the operation can be retried.
	Retryable bool
}

// Error returns the error message.
func (e *ExecutionError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s: %s", e.Op, e.Binary, e.Details)
	}
	return fmt.Sprintf("%s: %s: %v", e.Op, e.Binary, e.Err)
}

// Unwrap returns the underlying error.
func (e *ExecutionError) Unwrap() error {
	return e.Err
}

// Is reports whether the error matches the target.
func (e *ExecutionError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// PolicyViolationError contains details about policy violations.
type PolicyViolationError struct {
	ExecutionError
	Violations    []Violation
	PolicyVersion string
}

// Violation describes a specific policy violation.
type Violation struct {
	// Code is the violation code.
	Code string

	// Field is the field that violated the policy.
	Field string

	// Message describes the violation.
	Message string

	// Severity is the violation severity.
	Severity Severity
}

// Severity represents violation severity.
type Severity int

const (
	// SeverityWarning is a warning that doesn't block execution.
	SeverityWarning Severity = iota
	// SeverityError is an error that blocks execution.
	SeverityError
	// SeverityCritical is a critical error requiring immediate attention.
	SeverityCritical
)

// String returns the string representation of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ResourceExceededError contains details about resource limit violations.
type ResourceExceededError struct {
	ExecutionError
	// Resource is the resource that was exceeded.
	Resource string
	// Limit is the configured limit.
	Limit int64
	// Actual is the actual usage.
	Actual int64
}

// Error constructors for consistent error creation.

// NewPolicyError creates a policy violation error.
func NewPolicyError(binary string, violations []Violation) error {
	return &PolicyViolationError{
		ExecutionError: ExecutionError{
			Op:        "policy_check",
			Binary:    binary,
			Err:       ErrPolicyDenied,
			Code:      ErrCodePolicyViolation,
			Retryable: false,
		},
		Violations: violations,
	}
}

// NewTimeoutError creates a timeout error.
func NewTimeoutError(binary string, duration string) error {
	return &ExecutionError{
		Op:        "execute",
		Binary:    binary,
		Err:       ErrTimeout,
		Code:      ErrCodeTimeout,
		Details:   fmt.Sprintf("execution exceeded timeout of %s", duration),
		Retryable: true,
	}
}

// NewResourceError creates a resource exceeded error.
func NewResourceError(binary, resource string, limit, actual int64) error {
	return &ResourceExceededError{
		ExecutionError: ExecutionError{
			Op:        "execute",
			Binary:    binary,
			Err:       ErrResourceExceeded,
			Code:      ErrCodeResourceExceeded,
			Details:   fmt.Sprintf("%s limit exceeded: %d > %d", resource, actual, limit),
			Retryable: false,
		},
		Resource: resource,
		Limit:    limit,
		Actual:   actual,
	}
}

// NewValidationError creates a validation error.
func NewValidationError(binary, field, message string) error {
	return &ExecutionError{
		Op:        "validate",
		Binary:    binary,
		Err:       ErrInvalidCommand,
		Code:      ErrCodeValidationFailed,
		Details:   fmt.Sprintf("%s: %s", field, message),
		Retryable: false,
	}
}

// NewSandboxError creates a sandbox violation error.
func NewSandboxError(binary, details string) error {
	return &ExecutionError{
		Op:        "sandbox",
		Binary:    binary,
		Err:       ErrSandboxViolation,
		Code:      ErrCodeSandboxViolation,
		Details:   details,
		Retryable: false,
	}
}

// NewRateLimitError creates a rate limit error.
func NewRateLimitError(binary string) error {
	return &ExecutionError{
		Op:         "rate_limit",
		Binary:     binary,
		Err:        ErrRateLimited,
		Code:       ErrCodeRateLimited,
		Details:    "rate limit exceeded, retry later",
		Suggestion: "wait before retrying",
		Retryable:  true,
	}
}

// NewCircuitOpenError creates a circuit breaker open error.
func NewCircuitOpenError(binary string) error {
	return &ExecutionError{
		Op:         "circuit_breaker",
		Binary:     binary,
		Err:        ErrCircuitOpen,
		Code:       ErrCodeCircuitOpen,
		Details:    "circuit breaker is open due to recent failures",
		Suggestion: "wait for circuit to close",
		Retryable:  true,
	}
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	var execErr *ExecutionError
	if errors.As(err, &execErr) {
		return execErr.Retryable
	}
	return false
}

// GetErrorCode extracts the error code from an error.
func GetErrorCode(err error) ErrorCode {
	var execErr *ExecutionError
	if errors.As(err, &execErr) {
		return execErr.Code
	}
	return ErrCodeInternalError
}
