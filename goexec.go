// Package goexec provides a secure, hardened command execution library.
//
// GoExec is a production-grade Go library that centralizes all process invocation
// behind a minimal API, banning direct os/exec usage elsewhere. It enforces strict
// security controls including binary allowlisting, argument validation, path sanitization,
// and sandbox integration.
//
// # Quick Start
//
// The simplest way to use goexec:
//
//	// Create an executor with default settings
//	exec, err := goexec.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer exec.Shutdown(context.Background())
//
//	// Execute a command
//	cmd, _ := goexec.Cmd("/usr/bin/ls", "-la").Build()
//	result, err := exec.Execute(ctx, cmd)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Stdout)
//
// # With Policy Configuration
//
// For production use, configure security policies:
//
//	// Load policy from YAML file
//	pol, err := goexec.LoadPolicy("/etc/goexec", "policy.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create executor with policy
//	exec, err := goexec.NewBuilder().
//	    WithPolicy(pol).
//	    WithDefaultTimeout(30 * time.Second).
//	    Build()
//
// # Security Model
//
// GoExec implements defense-in-depth security:
//
//   - Binary Allowlisting: Only explicitly allowed binaries can execute
//   - Argument Validation: Arguments validated against patterns and denied values
//   - Path Sanitization: Prevents path traversal and symlink attacks
//   - Environment Filtering: Controls which environment variables pass through
//   - Resource Limits: CPU, memory, process limits via cgroups and rlimits
//   - Sandboxing: Optional seccomp-bpf and AppArmor integration (Linux)
//
// # Architecture
//
// The library is organized into focused packages:
//
//   - goexec (this package): Main entry point and convenience functions
//   - executor: Core Executor interface and implementation
//   - policy: YAML policy loading and validation
//   - validation: Input sanitization and validation
//   - sandbox: Linux sandboxing (seccomp, AppArmor, cgroups)
//   - pool: Bounded worker pool with backpressure
//   - resilience: Rate limiting and circuit breaker
//   - observability: OpenTelemetry metrics and audit logging
//   - hooks: Extension points for custom behavior
//
// # Thread Safety
//
// All types in this package are safe for concurrent use by multiple goroutines.
// The Executor can be shared across goroutines without additional synchronization.
//
// # File I/O
//
// All file operations in this library use github.com/victoralfred/gowritter/safepath
// for secure path handling and I/O operations.
package goexec

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/victoralfred/goexec/executor"
	"github.com/victoralfred/goexec/policy"
	"github.com/victoralfred/goexec/validation"
)

// =============================================================================
// Core Types
// =============================================================================

// Executor is the primary interface for command execution.
// All command execution MUST go through this interface to ensure security
// controls are applied consistently.
//
// The Executor interface provides:
//   - Synchronous execution with Execute
//   - Asynchronous execution with ExecuteAsync
//   - Batch execution with ExecuteBatch
//   - Streaming I/O with Stream
//   - Graceful shutdown with Shutdown
type Executor = executor.Executor

// Command represents a command to be executed.
// Use Command() to create commands.
type Command = executor.Command

// Result contains the outcome of command execution.
type Result = executor.Result

// ResourceUsage contains resource consumption metrics.
type ResourceUsage = executor.ResourceUsage

// ResourceLimits defines resource constraints for command execution.
type ResourceLimits = executor.ResourceLimits

// Builder creates configured Executor instances.
type Builder = executor.Builder

// CommandBuilder creates commands with a fluent interface.
type CommandBuilder = executor.CommandBuilder

// ValidationResult contains the outcome of policy validation.
type ValidationResult = executor.ValidationResult

// Violation represents a policy violation.
type Violation = executor.Violation

// Priority represents command execution priority.
type Priority = executor.Priority

// Priority constants.
const (
	PriorityLow      = executor.PriorityLow
	PriorityNormal   = executor.PriorityNormal
	PriorityHigh     = executor.PriorityHigh
	PriorityCritical = executor.PriorityCritical
)

// =============================================================================
// Policy Types
// =============================================================================

// PolicyLoader loads and manages policies from YAML files.
type PolicyLoader = policy.Loader

// PolicyConfig represents a loaded policy configuration.
// Deprecated: Use policy.Config instead.
type PolicyConfig = policy.Config

// CompiledPolicy is a compiled and ready-to-use policy.
type CompiledPolicy = policy.CompiledPolicy

// =============================================================================
// Error Variables
// =============================================================================

// Common errors returned by the library.
var (
	// ErrArgumentNotAllowed indicates an argument was denied.
	ErrArgumentNotAllowed = executor.ErrArgumentNotAllowed

	// ErrInvalidPath indicates an invalid binary or working directory path.
	ErrInvalidPath = executor.ErrInvalidPath

	// ErrPathTraversal indicates a path traversal attempt was detected.
	ErrPathTraversal = executor.ErrPathTraversal

	// ErrExecutorShutdown indicates the executor has been shut down.
	ErrExecutorShutdown = executor.ErrExecutorShutdown

	// ErrInvalidCommand indicates an invalid command configuration.
	ErrInvalidCommand = executor.ErrInvalidCommand

	// ErrTimeout indicates execution exceeded the timeout.
	ErrTimeout = errors.New("execution timeout")

	// ErrRateLimited indicates the rate limit was exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker open")
)

// =============================================================================
// Status Constants
// =============================================================================

// Execution status values.
const (
	StatusSuccess      = executor.StatusSuccess
	StatusError        = executor.StatusError
	StatusTimeout      = executor.StatusTimeout
	StatusCanceled     = executor.StatusCanceled
	StatusKilled       = executor.StatusKilled
	StatusPolicyDenied = executor.StatusPolicyDenied
	StatusRateLimited  = executor.StatusRateLimited
	StatusCircuitOpen  = executor.StatusCircuitOpen
)

// =============================================================================
// Factory Functions
// =============================================================================

// New creates a new Executor with default settings.
// This is the simplest way to get started with goexec.
//
// For production use, consider using NewBuilder to configure
// security policies and other settings.
//
// Example:
//
//	exec, err := goexec.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer exec.Shutdown(context.Background())
func New() (Executor, error) {
	return executor.NewBuilder().Build()
}

// NewBuilder creates a new executor builder for configuring the Executor.
//
// Example:
//
//	exec, err := goexec.NewBuilder().
//	    WithPolicy(policy).
//	    WithDefaultTimeout(30 * time.Second).
//	    Build()
func NewBuilder() *Builder {
	return executor.NewBuilder()
}

// =============================================================================
// Command Construction
// =============================================================================

// Cmd creates a new CommandBuilder with the specified binary and arguments.
// Call Build() on the returned builder to get the final Command.
//
// Example:
//
//	cmd, err := goexec.Cmd("/usr/bin/git", "status").Build()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	result, err := exec.Execute(ctx, cmd)
func Cmd(binary string, args ...string) *CommandBuilder {
	return executor.NewCommand(binary, args...)
}

// MustCmd creates a command and panics on error.
// Use only when the binary path is known to be valid.
//
// Example:
//
//	cmd := goexec.MustCmd("/usr/bin/ls", "-la")
func MustCmd(binary string, args ...string) *Command {
	return executor.NewCommand(binary, args...).MustBuild()
}

// =============================================================================
// Policy Loading
// =============================================================================

// LoadPolicy loads a security policy from a YAML file.
// The basePath is the directory containing the policy file.
// The policyFile is the name of the policy file relative to basePath.
//
// Example policy.yaml:
//
//	version: "1.0"
//	binaries:
//	  - path: /usr/bin/git
//	    enabled: true
//	    allowed_args:
//	      - pattern: "^(clone|pull|push|status|log)$"
//
// Example:
//
//	loader, err := goexec.LoadPolicy("/etc/goexec", "policy.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	pol, err := loader.Load(ctx)
func LoadPolicy(basePath, policyFile string) (*PolicyLoader, error) {
	return policy.NewLoader(basePath, policyFile)
}

// LoadPolicyWithValidation loads a policy with custom validators.
//
// Example:
//
//	loader, err := goexec.LoadPolicyWithValidation(
//	    "/etc/goexec", "policy.yaml",
//	    policy.WithValidator(&policy.DefaultValidator{}),
//	)
func LoadPolicyWithValidation(basePath, policyFile string, opts ...policy.LoaderOption) (*PolicyLoader, error) {
	return policy.NewLoader(basePath, policyFile, opts...)
}

// LoadPolicyFromPath loads a policy from a full file path.
// This is a convenience function that splits the path into directory and filename.
//
// Example:
//
//	loader, err := goexec.LoadPolicyFromPath("/etc/goexec/policy.yaml")
func LoadPolicyFromPath(path string) (*PolicyLoader, error) {
	dir := filepath.Dir(path)
	file := filepath.Base(path)
	return policy.NewLoader(dir, file)
}

// ExamplePolicy returns an example policy configuration.
// Use this as a starting point for creating your own policies.
func ExamplePolicy() *PolicyConfig {
	return policy.ExamplePolicy()
}

// =============================================================================
// Validation
// =============================================================================

// ValidatePath validates a file path for safety.
// Returns an error if the path contains traversal attempts or other issues.
func ValidatePath(path string) error {
	v := validation.NewPathValidator(nil)
	cmd := &Command{Binary: path}
	return v.Validate(context.Background(), cmd)
}

// SanitizePath cleans a path and validates it for safety.
// Returns the cleaned path or an error if invalid.
func SanitizePath(path string) (string, error) {
	return validation.SanitizePath(path)
}

// ValidateArguments validates command arguments against security rules.
func ValidateArguments(args []string) error {
	v := validation.NewArgumentValidator(nil)
	cmd := &Command{Args: args}
	return v.Validate(context.Background(), cmd)
}

// =============================================================================
// Convenience Functions
// =============================================================================

// Execute is a convenience function for one-off command execution.
// For repeated executions, create an Executor instance instead.
//
// Example:
//
//	result, err := goexec.Execute(ctx, "/usr/bin/ls", "-la")
func Execute(ctx context.Context, binary string, args ...string) (*Result, error) {
	exec, err := New()
	if err != nil {
		return nil, err
	}
	defer func() {
		// Ignore shutdown errors in defer - cleanup failure doesn't affect result
		//nolint:errcheck // Shutdown errors are non-critical in cleanup context
		_ = exec.Shutdown(context.Background())
	}()

	cmd, err := Cmd(binary, args...).Build()
	if err != nil {
		return nil, err
	}

	return exec.Execute(ctx, cmd)
}

// ExecuteWithTimeout is a convenience function with explicit timeout.
//
// Example:
//
//	result, err := goexec.ExecuteWithTimeout(ctx, 30*time.Second, "/usr/bin/ls", "-la")
func ExecuteWithTimeout(ctx context.Context, timeout time.Duration, binary string, args ...string) (*Result, error) {
	exec, err := NewBuilder().WithDefaultTimeout(timeout).Build()
	if err != nil {
		return nil, err
	}
	defer func() {
		// Ignore shutdown errors in defer - cleanup failure doesn't affect result
		//nolint:errcheck // Shutdown errors are non-critical in cleanup context
		_ = exec.Shutdown(context.Background())
	}()

	cmd, err := Cmd(binary, args...).Build()
	if err != nil {
		return nil, err
	}

	return exec.Execute(ctx, cmd)
}

// Stream is a convenience function for streaming command output.
//
// Example:
//
//	err := goexec.Stream(ctx, os.Stdout, os.Stderr, "/usr/bin/tail", "-f", "/var/log/syslog")
func Stream(ctx context.Context, stdout, stderr io.Writer, binary string, args ...string) error {
	exec, err := New()
	if err != nil {
		return err
	}
	defer func() {
		// Ignore shutdown errors in defer - cleanup failure doesn't affect result
		//nolint:errcheck // Shutdown errors are non-critical in cleanup context
		_ = exec.Shutdown(context.Background())
	}()

	cmd, err := Cmd(binary, args...).Build()
	if err != nil {
		return err
	}

	return exec.Stream(ctx, cmd, stdout, stderr)
}

// =============================================================================
// Version Information
// =============================================================================

// Version returns the library version.
func Version() string {
	return "1.0.0"
}

// =============================================================================
// Package Accessors
// =============================================================================

// These functions provide access to subpackage functionality.
// For advanced use cases, import the subpackages directly:
//
//   - github.com/victoralfred/goexec/executor    - Core execution interface
//   - github.com/victoralfred/goexec/policy      - YAML policy loading
//   - github.com/victoralfred/goexec/validation  - Input validation
//   - github.com/victoralfred/goexec/sandbox     - Linux sandboxing
//   - github.com/victoralfred/goexec/pool        - Worker pool
//   - github.com/victoralfred/goexec/resilience  - Rate limiting & circuit breaker
//   - github.com/victoralfred/goexec/observability - Metrics & tracing
//   - github.com/victoralfred/goexec/hooks       - Extension points
//   - github.com/victoralfred/goexec/config      - Configuration
