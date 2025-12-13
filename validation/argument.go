package validation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/victoralfred/goexec/executor"
)

// ArgumentValidatorConfig configures the argument validator.
type ArgumentValidatorConfig struct {
	DeniedPatterns      []string
	MaxArgs             int
	MaxArgLength        int
	AllowShellMetachars bool
}

// ArgumentValidator validates command arguments.
type ArgumentValidator struct {
	config         *ArgumentValidatorConfig
	shellMetachars string
	deniedRegexps  []*regexp.Regexp
}

// NewArgumentValidator creates a new argument validator.
func NewArgumentValidator(config *ArgumentValidatorConfig) *ArgumentValidator {
	if config == nil {
		config = &ArgumentValidatorConfig{
			MaxArgs:      100,
			MaxArgLength: 4096,
			DeniedPatterns: []string{
				`^\s*;\s*`,           // Command injection via semicolon
				`\|\s*`,              // Pipe injection
				`&\s*`,               // Background/AND injection
				`\$\(`,               // Command substitution
				"\\`",                // Backtick substitution
				`>\s*`,               // Redirect output
				`<\s*`,               // Redirect input
				`\$\{`,               // Variable expansion
				`\n`,                 // Newline injection
				`\r`,                 // Carriage return injection
				`--exec\s*=`,         // Git exec injection
				`--upload-pack\s*=`,  // Git upload-pack injection
				`--receive-pack\s*=`, // Git receive-pack injection
			},
			AllowShellMetachars: false,
		}
	}

	v := &ArgumentValidator{
		config:         config,
		shellMetachars: ";|&$`'\"\\<>(){}[]!#~*?",
	}

	// Compile denied patterns
	for _, pattern := range config.DeniedPatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			v.deniedRegexps = append(v.deniedRegexps, re)
		}
	}

	return v
}

// Name returns the validator name.
func (v *ArgumentValidator) Name() string {
	return "argument_validator"
}

// Priority returns the execution priority.
func (v *ArgumentValidator) Priority() int {
	return 20
}

// Validate validates command arguments.
func (v *ArgumentValidator) Validate(ctx context.Context, cmd *executor.Command) error {
	// Check argument count
	if len(cmd.Args) > v.config.MaxArgs {
		return fmt.Errorf("%w: too many arguments (%d > %d)",
			executor.ErrArgumentNotAllowed, len(cmd.Args), v.config.MaxArgs)
	}

	// Validate each argument
	for i, arg := range cmd.Args {
		if err := v.validateArgument(arg, i); err != nil {
			return err
		}
	}

	return nil
}

// validateArgument validates a single argument.
func (v *ArgumentValidator) validateArgument(arg string, position int) error {
	// Check length
	if len(arg) > v.config.MaxArgLength {
		return fmt.Errorf("%w: argument %d too long (%d > %d)",
			executor.ErrArgumentNotAllowed, position, len(arg), v.config.MaxArgLength)
	}

	// Check for null bytes
	if strings.ContainsRune(arg, 0) {
		return fmt.Errorf("%w: argument %d contains null byte",
			executor.ErrArgumentNotAllowed, position)
	}

	// Check denied patterns
	for _, re := range v.deniedRegexps {
		if re.MatchString(arg) {
			return fmt.Errorf("%w: argument %d matches denied pattern",
				executor.ErrArgumentNotAllowed, position)
		}
	}

	// Check shell metacharacters if not allowed
	if !v.config.AllowShellMetachars {
		for _, char := range v.shellMetachars {
			if strings.ContainsRune(arg, char) {
				return fmt.Errorf("%w: argument %d contains shell metacharacter '%c'",
					executor.ErrArgumentNotAllowed, position, char)
			}
		}
	}

	return nil
}

// SanitizeArgument sanitizes a single argument.
func SanitizeArgument(arg string) string {
	// Remove null bytes
	arg = strings.ReplaceAll(arg, "\x00", "")

	// Remove control characters
	var result strings.Builder
	for _, r := range arg {
		if r >= 32 || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// EscapeShellArg escapes an argument for safe shell usage.
func EscapeShellArg(arg string) string {
	// If argument is empty or safe, return as-is
	if arg == "" {
		return "''"
	}

	// Check if argument needs escaping
	needsEscape := false
	for _, c := range arg {
		if !isShellSafe(c) {
			needsEscape = true
			break
		}
	}

	if !needsEscape {
		return arg
	}

	// Use single quotes and escape any single quotes within
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

// isShellSafe returns true if a character is safe in shell context.
func isShellSafe(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '/' || c == ':'
}

// ArgPattern defines a pattern for argument validation.
type ArgPattern struct {
	compiled    *regexp.Regexp
	Pattern     string
	Description string
	Position    int
	Required    bool
}

// Compile compiles the argument pattern.
func (p *ArgPattern) Compile() error {
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", p.Pattern, err)
	}
	p.compiled = re
	return nil
}

// Matches returns true if the argument matches this pattern.
func (p *ArgPattern) Matches(arg string, position int) bool {
	if p.compiled == nil {
		return false
	}

	// Check position if specified
	if p.Position >= 0 && p.Position != position {
		return false
	}

	return p.compiled.MatchString(arg)
}

// ArgumentMatcher matches arguments against allowed patterns.
type ArgumentMatcher struct {
	patterns []*ArgPattern
}

// NewArgumentMatcher creates a new argument matcher.
func NewArgumentMatcher(patterns []*ArgPattern) (*ArgumentMatcher, error) {
	m := &ArgumentMatcher{
		patterns: make([]*ArgPattern, len(patterns)),
	}

	for i, p := range patterns {
		// Make a copy
		pattern := *p
		if err := pattern.Compile(); err != nil {
			return nil, fmt.Errorf("pattern %d: %w", i, err)
		}
		m.patterns[i] = &pattern
	}

	return m, nil
}

// MatchAll checks if all arguments match allowed patterns.
func (m *ArgumentMatcher) MatchAll(args []string) (matched bool, reason string) {
	for i, arg := range args {
		argMatched := false
		for _, p := range m.patterns {
			if p.Matches(arg, i) {
				argMatched = true
				break
			}
		}
		if !argMatched {
			return false, fmt.Sprintf("argument %d (%q) does not match any allowed pattern", i, arg)
		}
	}

	// Check required patterns
	for _, p := range m.patterns {
		if p.Required {
			found := false
			for i, arg := range args {
				if p.Matches(arg, i) {
					found = true
					break
				}
			}
			if !found {
				return false, fmt.Sprintf("required pattern %q not found", p.Description)
			}
		}
	}

	return true, ""
}
