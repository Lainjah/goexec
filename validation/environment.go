package validation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/victoralfred/goexec/executor"
)

// EnvironmentValidatorConfig configures the environment validator.
type EnvironmentValidatorConfig struct {
	// AllowedVars are environment variables that are allowed.
	// Supports wildcards: "PATH", "LC_*", etc.
	AllowedVars []string

	// DeniedVars are environment variables that are denied.
	// Supports wildcards: "*_SECRET", "*_PASSWORD", etc.
	DeniedVars []string

	// MaxVars is the maximum number of environment variables.
	MaxVars int

	// MaxKeyLength is the maximum length of a variable name.
	MaxKeyLength int

	// MaxValueLength is the maximum length of a variable value.
	MaxValueLength int

	// AllowEmpty allows empty values.
	AllowEmpty bool
}

// EnvironmentValidator validates environment variables.
type EnvironmentValidator struct {
	config        *EnvironmentValidatorConfig
	allowedRegexp []*regexp.Regexp
	deniedRegexp  []*regexp.Regexp
}

// NewEnvironmentValidator creates a new environment validator.
func NewEnvironmentValidator(config *EnvironmentValidatorConfig) *EnvironmentValidator {
	if config == nil {
		config = &EnvironmentValidatorConfig{
			AllowedVars: []string{
				"PATH",
				"HOME",
				"USER",
				"LANG",
				"LC_*",
				"TZ",
				"TERM",
				"SHELL",
			},
			DeniedVars: []string{
				"*_SECRET*",
				"*_PASSWORD*",
				"*_TOKEN*",
				"*_KEY*",
				"*_CREDENTIAL*",
				"AWS_*",
				"GITHUB_*",
				"DOCKER_*",
				"SSH_*",
				"GPG_*",
				"LD_PRELOAD",
				"LD_LIBRARY_PATH",
				"DYLD_*",
			},
			MaxVars:        50,
			MaxKeyLength:   256,
			MaxValueLength: 8192,
			AllowEmpty:     false,
		}
	}

	v := &EnvironmentValidator{
		config: config,
	}

	// Compile allowed patterns
	for _, pattern := range config.AllowedVars {
		if re := wildcardToRegexp(pattern); re != nil {
			v.allowedRegexp = append(v.allowedRegexp, re)
		}
	}

	// Compile denied patterns
	for _, pattern := range config.DeniedVars {
		if re := wildcardToRegexp(pattern); re != nil {
			v.deniedRegexp = append(v.deniedRegexp, re)
		}
	}

	return v
}

// Name returns the validator name.
func (v *EnvironmentValidator) Name() string {
	return "environment_validator"
}

// Priority returns the execution priority.
func (v *EnvironmentValidator) Priority() int {
	return 30
}

// Validate validates command environment.
func (v *EnvironmentValidator) Validate(ctx context.Context, cmd *executor.Command) error {
	// Check count
	if len(cmd.Env) > v.config.MaxVars {
		return fmt.Errorf("too many environment variables (%d > %d)",
			len(cmd.Env), v.config.MaxVars)
	}

	// Validate each variable
	for key, value := range cmd.Env {
		if err := v.validateVar(key, value); err != nil {
			return err
		}
	}

	return nil
}

// validateVar validates a single environment variable.
func (v *EnvironmentValidator) validateVar(key, value string) error {
	// Check key length
	if len(key) > v.config.MaxKeyLength {
		return fmt.Errorf("environment key %q too long (%d > %d)",
			key, len(key), v.config.MaxKeyLength)
	}

	// Check value length
	if len(value) > v.config.MaxValueLength {
		return fmt.Errorf("environment value for %q too long (%d > %d)",
			key, len(value), v.config.MaxValueLength)
	}

	// Check empty value
	if !v.config.AllowEmpty && value == "" {
		return fmt.Errorf("empty environment value for %q not allowed", key)
	}

	// Check key format (must be valid identifier)
	if !isValidEnvKey(key) {
		return fmt.Errorf("invalid environment key %q", key)
	}

	// Check against denied patterns first
	for _, re := range v.deniedRegexp {
		if re.MatchString(key) {
			return fmt.Errorf("environment variable %q matches denied pattern", key)
		}
	}

	// Check against allowed patterns
	if len(v.allowedRegexp) > 0 {
		allowed := false
		for _, re := range v.allowedRegexp {
			if re.MatchString(key) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("environment variable %q not in allowlist", key)
		}
	}

	// Check value for dangerous content
	if err := validateEnvValue(value); err != nil {
		return fmt.Errorf("invalid value for %q: %w", key, err)
	}

	return nil
}

// wildcardToRegexp converts a wildcard pattern to a regexp.
func wildcardToRegexp(pattern string) *regexp.Regexp {
	// Escape special characters except *
	escaped := regexp.QuoteMeta(pattern)
	// Replace \* with .* for wildcard matching
	escaped = strings.ReplaceAll(escaped, "\\*", ".*")
	// Anchor the pattern
	escaped = "^" + escaped + "$"

	re, err := regexp.Compile(escaped)
	if err != nil {
		return nil
	}
	return re
}

// isValidEnvKey checks if a key is a valid environment variable name.
func isValidEnvKey(key string) bool {
	if len(key) == 0 {
		return false
	}

	// Must start with letter or underscore
	first := key[0]
	if !((first >= 'a' && first <= 'z') ||
		(first >= 'A' && first <= 'Z') ||
		first == '_') {
		return false
	}

	// Rest must be alphanumeric or underscore
	for i := 1; i < len(key); i++ {
		c := key[i]
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_') {
			return false
		}
	}

	return true
}

// validateEnvValue checks if a value is safe.
func validateEnvValue(value string) error {
	// Check for null bytes
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("value contains null byte")
	}

	return nil
}

// FilterEnvironment filters environment variables based on allowlist/denylist.
func FilterEnvironment(env map[string]string, allowed, denied []string) map[string]string {
	result := make(map[string]string)

	// Compile patterns
	var allowedRe, deniedRe []*regexp.Regexp
	for _, p := range allowed {
		if re := wildcardToRegexp(p); re != nil {
			allowedRe = append(allowedRe, re)
		}
	}
	for _, p := range denied {
		if re := wildcardToRegexp(p); re != nil {
			deniedRe = append(deniedRe, re)
		}
	}

	for key, value := range env {
		// Check denied first
		isDenied := false
		for _, re := range deniedRe {
			if re.MatchString(key) {
				isDenied = true
				break
			}
		}
		if isDenied {
			continue
		}

		// Check allowed
		if len(allowedRe) > 0 {
			isAllowed := false
			for _, re := range allowedRe {
				if re.MatchString(key) {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				continue
			}
		}

		result[key] = value
	}

	return result
}

// MinimalEnvironment returns a minimal safe environment.
func MinimalEnvironment() map[string]string {
	return map[string]string{
		"PATH":   "/usr/bin:/bin",
		"LANG":   "C.UTF-8",
		"LC_ALL": "C.UTF-8",
		"HOME":   "/tmp",
		"USER":   "nobody",
	}
}

// MergeEnvironment merges base environment with overrides.
// Overrides take precedence.
func MergeEnvironment(base, override map[string]string) map[string]string {
	result := make(map[string]string, len(base)+len(override))

	for k, v := range base {
		result[k] = v
	}

	for k, v := range override {
		result[k] = v
	}

	return result
}
