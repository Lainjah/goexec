// Package policy provides YAML-based policy-as-code for command execution.
package policy

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/victoralfred/goexec/executor"
)

// Policy defines the security policy for command execution.
type Policy interface {
	// Validate checks if a command is allowed by the policy.
	Validate(ctx context.Context, cmd *executor.Command) (*ValidationResult, error)

	// GetBinaryPolicy returns the policy for a specific binary.
	GetBinaryPolicy(binary string) (*BinaryPolicy, error)

	// Reload reloads the policy from its source.
	Reload(ctx context.Context) error

	// Version returns the policy version for audit purposes.
	Version() string
}

// ValidationResult contains the outcome of policy validation.
type ValidationResult struct {
	Allowed        bool
	Reason         string
	Violations     []executor.Violation
	SuggestedFixes []string
}

// BinaryPolicy defines rules for a specific binary.
type BinaryPolicy struct {
	// Path is the absolute path to the binary.
	Path string

	// Enabled indicates if this binary is allowed.
	Enabled bool

	// AllowedArgs are patterns for allowed arguments.
	AllowedArgs []ArgPattern

	// DeniedArgs are patterns for denied arguments.
	DeniedArgs []ArgPattern

	// AllowedEnv are environment variables allowed for this binary.
	AllowedEnv []string

	// DeniedEnv are environment variables denied for this binary.
	DeniedEnv []string

	// ResourceLimits are resource limits for this binary.
	ResourceLimits *executor.ResourceLimits

	// SandboxProfile is the sandbox profile name.
	SandboxProfile string

	// RateLimit is the rate limit configuration.
	RateLimit *RateLimitConfig

	// RequireAudit indicates if audit logging is required.
	RequireAudit bool

	// MaxInstances is the maximum concurrent instances.
	MaxInstances int

	// AllowedWorkdirs are allowed working directories.
	AllowedWorkdirs []string

	// compiledAllowed are compiled allowed arg patterns.
	compiledAllowed []*regexp.Regexp

	// compiledDenied are compiled denied arg patterns.
	compiledDenied []*regexp.Regexp
}

// ArgPattern defines a pattern for argument validation.
type ArgPattern struct {
	// Pattern is the regex pattern.
	Pattern string `yaml:"pattern"`

	// Position is the expected position (-1 for any).
	Position int `yaml:"position"`

	// Description describes what this pattern allows.
	Description string `yaml:"description"`

	// Required indicates if this argument is required.
	Required bool `yaml:"required"`
}

// RateLimitConfig defines rate limiting parameters.
type RateLimitConfig struct {
	// RequestsPerSecond is the allowed requests per second.
	RequestsPerSecond float64 `yaml:"requests_per_second"`

	// BurstSize is the maximum burst size.
	BurstSize int `yaml:"burst"`
}

// CompiledPolicy is a validated, optimized policy ready for use.
type CompiledPolicy struct {
	raw         *Config
	version     string
	hash        string
	binaryIndex map[string]*BinaryPolicy
	loadedAt    time.Time
	mu          sync.RWMutex
}

// NewCompiledPolicy creates a new compiled policy from configuration.
func NewCompiledPolicy(config *Config) (*CompiledPolicy, error) {
	cp := &CompiledPolicy{
		raw:         config,
		version:     config.Version,
		binaryIndex: make(map[string]*BinaryPolicy),
		loadedAt:    time.Now(),
	}

	// Build binary index
	for i := range config.Binaries {
		bc := &config.Binaries[i]
		bp := &BinaryPolicy{
			Path:            bc.Path,
			Enabled:         bc.Enabled,
			AllowedArgs:     bc.AllowedArgs,
			DeniedArgs:      bc.DeniedArgs,
			AllowedEnv:      bc.AllowedEnv,
			DeniedEnv:       bc.DeniedEnv,
			SandboxProfile:  bc.Sandbox,
			RateLimit:       bc.RateLimit,
			RequireAudit:    bc.RequireAudit,
			MaxInstances:    bc.MaxInstances,
			AllowedWorkdirs: bc.AllowedWorkdirs,
		}

		// Convert resource limits
		if bc.Limits != nil {
			bp.ResourceLimits = &executor.ResourceLimits{
				MaxCPUTime:     bc.Limits.MaxCPUTime.Duration,
				MaxWallTime:    bc.Limits.MaxWallTime.Duration,
				MaxMemoryBytes: bc.Limits.MaxMemory.Bytes,
				MaxOutputBytes: bc.Limits.MaxOutput.Bytes,
				MaxProcesses:   bc.Limits.MaxProcesses,
				MaxOpenFiles:   bc.Limits.MaxOpenFiles,
			}
		}

		// Compile patterns
		if err := bp.compile(); err != nil {
			return nil, fmt.Errorf("compiling policy for %s: %w", bc.Path, err)
		}

		cp.binaryIndex[bc.Path] = bp
	}

	return cp, nil
}

// compile compiles the regex patterns.
func (bp *BinaryPolicy) compile() error {
	// Compile allowed patterns
	for _, ap := range bp.AllowedArgs {
		re, err := regexp.Compile(ap.Pattern)
		if err != nil {
			return fmt.Errorf("invalid allowed pattern %q: %w", ap.Pattern, err)
		}
		bp.compiledAllowed = append(bp.compiledAllowed, re)
	}

	// Compile denied patterns
	for _, dp := range bp.DeniedArgs {
		re, err := regexp.Compile(dp.Pattern)
		if err != nil {
			return fmt.Errorf("invalid denied pattern %q: %w", dp.Pattern, err)
		}
		bp.compiledDenied = append(bp.compiledDenied, re)
	}

	return nil
}

// Validate implements Policy.Validate.
func (cp *CompiledPolicy) Validate(ctx context.Context, cmd *executor.Command) (*ValidationResult, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	result := &ValidationResult{Allowed: true}

	// Check if binary is in policy
	bp, ok := cp.binaryIndex[cmd.Binary]
	if !ok {
		result.Allowed = false
		result.Reason = "binary not in policy"
		result.Violations = append(result.Violations, executor.Violation{
			Code:     "BINARY_NOT_ALLOWED",
			Field:    "binary",
			Message:  fmt.Sprintf("binary %s is not in the allowlist", cmd.Binary),
			Severity: executor.SeverityError,
		})
		return result, nil
	}

	// Check if enabled
	if !bp.Enabled {
		result.Allowed = false
		result.Reason = "binary is disabled"
		result.Violations = append(result.Violations, executor.Violation{
			Code:     "BINARY_DISABLED",
			Field:    "binary",
			Message:  fmt.Sprintf("binary %s is disabled in policy", cmd.Binary),
			Severity: executor.SeverityError,
		})
		return result, nil
	}

	// Validate arguments
	if violations := bp.validateArgs(cmd.Args); len(violations) > 0 {
		result.Allowed = false
		result.Reason = "argument validation failed"
		result.Violations = append(result.Violations, violations...)
	}

	// Validate environment
	if violations := bp.validateEnv(cmd.Env); len(violations) > 0 {
		result.Allowed = false
		result.Reason = "environment validation failed"
		result.Violations = append(result.Violations, violations...)
	}

	// Validate working directory
	if cmd.WorkingDir != "" && len(bp.AllowedWorkdirs) > 0 {
		if violations := bp.validateWorkdir(cmd.WorkingDir); len(violations) > 0 {
			result.Allowed = false
			result.Reason = "working directory not allowed"
			result.Violations = append(result.Violations, violations...)
		}
	}

	return result, nil
}

// validateArgs validates arguments against the policy.
func (bp *BinaryPolicy) validateArgs(args []string) []executor.Violation {
	var violations []executor.Violation

	for i, arg := range args {
		// Check denied patterns first
		for j, re := range bp.compiledDenied {
			if re.MatchString(arg) {
				violations = append(violations, executor.Violation{
					Code:     "ARGUMENT_DENIED",
					Field:    fmt.Sprintf("args[%d]", i),
					Message:  fmt.Sprintf("argument %q matches denied pattern: %s", arg, bp.DeniedArgs[j].Description),
					Severity: executor.SeverityError,
				})
			}
		}

		// Check if matches any allowed pattern
		if len(bp.compiledAllowed) > 0 {
			matched := false
			for _, re := range bp.compiledAllowed {
				if re.MatchString(arg) {
					matched = true
					break
				}
			}
			if !matched {
				violations = append(violations, executor.Violation{
					Code:     "ARGUMENT_NOT_ALLOWED",
					Field:    fmt.Sprintf("args[%d]", i),
					Message:  fmt.Sprintf("argument %q does not match any allowed pattern", arg),
					Severity: executor.SeverityError,
				})
			}
		}
	}

	return violations
}

// validateEnv validates environment against the policy.
func (bp *BinaryPolicy) validateEnv(env map[string]string) []executor.Violation {
	var violations []executor.Violation

	for key := range env {
		// Check denied
		for _, denied := range bp.DeniedEnv {
			if matchesWildcard(key, denied) {
				violations = append(violations, executor.Violation{
					Code:     "ENV_DENIED",
					Field:    fmt.Sprintf("env[%s]", key),
					Message:  fmt.Sprintf("environment variable %s is denied", key),
					Severity: executor.SeverityError,
				})
			}
		}

		// Check allowed
		if len(bp.AllowedEnv) > 0 {
			allowed := false
			for _, a := range bp.AllowedEnv {
				if matchesWildcard(key, a) {
					allowed = true
					break
				}
			}
			if !allowed {
				violations = append(violations, executor.Violation{
					Code:     "ENV_NOT_ALLOWED",
					Field:    fmt.Sprintf("env[%s]", key),
					Message:  fmt.Sprintf("environment variable %s is not in allowlist", key),
					Severity: executor.SeverityError,
				})
			}
		}
	}

	return violations
}

// validateWorkdir validates working directory against allowed patterns.
func (bp *BinaryPolicy) validateWorkdir(workdir string) []executor.Violation {
	for _, pattern := range bp.AllowedWorkdirs {
		if matchesWildcard(workdir, pattern) {
			return nil
		}
	}

	return []executor.Violation{{
		Code:     "WORKDIR_NOT_ALLOWED",
		Field:    "workdir",
		Message:  fmt.Sprintf("working directory %s is not allowed", workdir),
		Severity: executor.SeverityError,
	}}
}

// GetBinaryPolicy implements Policy.GetBinaryPolicy.
func (cp *CompiledPolicy) GetBinaryPolicy(binary string) (*BinaryPolicy, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	bp, ok := cp.binaryIndex[binary]
	if !ok {
		return nil, fmt.Errorf("no policy for binary %s", binary)
	}
	return bp, nil
}

// Reload implements Policy.Reload.
func (cp *CompiledPolicy) Reload(ctx context.Context) error {
	// This would be implemented by the loader
	return nil
}

// Version implements Policy.Version.
func (cp *CompiledPolicy) Version() string {
	return cp.version
}

// matchesWildcard checks if a string matches a wildcard pattern.
func matchesWildcard(s, pattern string) bool {
	// Simple wildcard matching: * matches any sequence
	if pattern == "*" {
		return true
	}

	// Convert to regex
	escaped := regexp.QuoteMeta(pattern)
	escaped = "^" + escaped + "$"
	// Replace \* with .*
	escaped = regexp.MustCompile(`\\\*`).ReplaceAllString(escaped, ".*")

	re, err := regexp.Compile(escaped)
	if err != nil {
		return false
	}

	return re.MatchString(s)
}

// PermissivePolicy returns a policy that allows everything.
// WARNING: Only use for testing.
func PermissivePolicy() Policy {
	return &permissivePolicy{}
}

type permissivePolicy struct{}

func (p *permissivePolicy) Validate(ctx context.Context, cmd *executor.Command) (*ValidationResult, error) {
	return &ValidationResult{Allowed: true}, nil
}

func (p *permissivePolicy) GetBinaryPolicy(binary string) (*BinaryPolicy, error) {
	return &BinaryPolicy{Path: binary, Enabled: true}, nil
}

func (p *permissivePolicy) Reload(ctx context.Context) error { return nil }
func (p *permissivePolicy) Version() string                  { return "permissive" }
