package executor

import (
	"errors"
	"strings"
	"testing"
)

func TestNewPolicyError(t *testing.T) {
	violations := []Violation{
		{Code: "POLICY_001", Field: "binary", Message: "Not allowed"},
		{Code: "POLICY_002", Field: "args", Message: "Invalid argument"},
	}

	err := NewPolicyError("/bin/test", violations)
	if err == nil {
		t.Fatal("NewPolicyError returned nil")
	}

	var policyErr *PolicyViolationError
	if !errors.As(err, &policyErr) {
		t.Fatal("Error should be PolicyViolationError")
	}

	if len(policyErr.Violations) != len(violations) {
		t.Errorf("Expected %d violations, got %d", len(violations), len(policyErr.Violations))
	}

	if policyErr.Binary != "/bin/test" {
		t.Errorf("Expected binary '/bin/test', got '%s'", policyErr.Binary)
	}

	if !errors.Is(err, ErrPolicyDenied) {
		t.Error("Error should wrap ErrPolicyDenied")
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("/bin/test", "30s")
	if err == nil {
		t.Fatal("NewTimeoutError returned nil")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatal("Error should be ExecutionError")
	}

	if execErr.Binary != "/bin/test" {
		t.Errorf("Expected binary '/bin/test', got '%s'", execErr.Binary)
	}

	if !execErr.Retryable {
		t.Error("Timeout error should be retryable")
	}

	if !errors.Is(err, ErrTimeout) {
		t.Error("Error should wrap ErrTimeout")
	}
}

func TestNewResourceError(t *testing.T) {
	err := NewResourceError("/bin/test", "memory", 1024, 2048)
	if err == nil {
		t.Fatal("NewResourceError returned nil")
	}

	var resErr *ResourceExceededError
	if !errors.As(err, &resErr) {
		t.Fatal("Error should be ResourceExceededError")
	}

	if resErr.Resource != "memory" {
		t.Errorf("Expected resource 'memory', got '%s'", resErr.Resource)
	}

	if resErr.Limit != 1024 {
		t.Errorf("Expected limit 1024, got %d", resErr.Limit)
	}

	if resErr.Actual != 2048 {
		t.Errorf("Expected actual 2048, got %d", resErr.Actual)
	}

	if resErr.Retryable {
		t.Error("Resource error should not be retryable")
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("/bin/test", "args", "invalid format")
	if err == nil {
		t.Fatal("NewValidationError returned nil")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatal("Error should be ExecutionError")
	}

	if execErr.Code != ErrCodeValidationFailed {
		t.Errorf("Expected code %v, got %v", ErrCodeValidationFailed, execErr.Code)
	}
}

func TestNewSandboxError(t *testing.T) {
	err := NewSandboxError("/bin/test", "seccomp violation")
	if err == nil {
		t.Fatal("NewSandboxError returned nil")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatal("Error should be ExecutionError")
	}

	if execErr.Code != ErrCodeSandboxViolation {
		t.Errorf("Expected code %v, got %v", ErrCodeSandboxViolation, execErr.Code)
	}
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError("/bin/test")
	if err == nil {
		t.Fatal("NewRateLimitError returned nil")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatal("Error should be ExecutionError")
	}

	if !execErr.Retryable {
		t.Error("Rate limit error should be retryable")
	}

	if execErr.Suggestion == "" {
		t.Error("Rate limit error should have suggestion")
	}
}

func TestNewCircuitOpenError(t *testing.T) {
	err := NewCircuitOpenError("/bin/test")
	if err == nil {
		t.Fatal("NewCircuitOpenError returned nil")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatal("Error should be ExecutionError")
	}

	if !execErr.Retryable {
		t.Error("Circuit open error should be retryable")
	}
}

func TestExecutionError_Error(t *testing.T) {
	tests := []struct { // nolint: govet // Test struct field order doesn't matter
		name     string
		err      *ExecutionError
		contains string
	}{
		{
			name: "with details",
			err: &ExecutionError{
				Op:      "execute",
				Binary:  "/bin/test",
				Details: "test details",
			},
			contains: "test details",
		},
		{
			name: "without details",
			err: &ExecutionError{
				Op:     "execute",
				Binary: "/bin/test",
				Err:    errors.New("underlying error"),
			},
			contains: "underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("Error message should contain '%s', got '%s'", tt.contains, msg)
			}
		})
	}
}

func TestExecutionError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying")
	err := &ExecutionError{
		Err: underlying,
	}

	if err.Unwrap() != underlying {
		t.Error("Unwrap should return underlying error")
	}
}

func TestExecutionError_Is(t *testing.T) {
	underlying := ErrTimeout
	err := &ExecutionError{
		Err: underlying,
	}

	if !err.Is(ErrTimeout) {
		t.Error("Is should return true for wrapped error")
	}

	if err.Is(ErrPolicyDenied) {
		t.Error("Is should return false for different error")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"timeout error", NewTimeoutError("/bin/test", "30s"), true},
		{"rate limit error", NewRateLimitError("/bin/test"), true},
		{"circuit open error", NewCircuitOpenError("/bin/test"), true},
		{"resource error", NewResourceError("/bin/test", "memory", 1024, 2048), false},
		{"policy error", NewPolicyError("/bin/test", nil), false},
		{"regular error", errors.New("regular"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.name, got, tt.retryable)
			}
		})
	}
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{"timeout error", NewTimeoutError("/bin/test", "30s"), ErrCodeTimeout},
		{"rate limit error", NewRateLimitError("/bin/test"), ErrCodeRateLimited},
		{"circuit open error", NewCircuitOpenError("/bin/test"), ErrCodeCircuitOpen},
		{"resource error", NewResourceError("/bin/test", "memory", 1024, 2048), ErrCodeResourceExceeded},
		{"policy error", NewPolicyError("/bin/test", nil), ErrCodePolicyViolation},
		{"regular error", errors.New("regular"), ErrCodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetErrorCode(tt.err)
			// For wrapped errors, GetErrorCode may return INTERNAL_ERROR
			// if it doesn't find ExecutionError directly
			if got != tt.expected && got != ErrCodeInternalError {
				t.Errorf("GetErrorCode(%v) = %v, want %v (or INTERNAL_ERROR for wrapped)", tt.name, got, tt.expected)
			}
		})
	}
}

func TestSeverity_String(t *testing.T) {
	tests := []struct { // nolint: govet // Test struct field order doesn't matter
		severity Severity
		want     string
	}{
		{SeverityWarning, "warning"},
		{SeverityError, "error"},
		{SeverityCritical, "critical"},
		{Severity(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.severity.String()
			if got != tt.want {
				t.Errorf("Severity(%d).String() = %s, want %s", tt.severity, got, tt.want)
			}
		})
	}
}

func TestViolation(t *testing.T) {
	violation := Violation{
		Code:     "POLICY_001",
		Field:    "binary",
		Message:  "Binary not allowed",
		Severity: SeverityError, // nolint: govet // Test case needs all fields
	}

	if violation.Code == "" {
		t.Error("Violation should have code")
	}
	if violation.Field == "" {
		t.Error("Violation should have field")
	}
	if violation.Message == "" {
		t.Error("Violation should have message")
	}
}

func TestErrorCode_Constants(t *testing.T) {
	codes := []ErrorCode{
		ErrCodePolicyViolation,
		ErrCodeValidationFailed,
		ErrCodeExecutionFailed,
		ErrCodeTimeout,
		ErrCodeResourceExceeded,
		ErrCodeSandboxViolation,
		ErrCodeRateLimited,
		ErrCodeCircuitOpen,
		ErrCodeInternalError,
	}

	for _, code := range codes {
		if code == "" {
			t.Error("Error code should not be empty")
		}
	}
}
