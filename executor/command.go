// Package executor provides the core command execution abstraction.
package executor

import (
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// Command represents a command to be executed.
// Commands are immutable once built.
type Command struct {
	// Binary is the absolute path to the executable.
	Binary string

	// Args are the command arguments (excluding the binary name).
	Args []string

	// Env is the environment variables for the command.
	// If nil, a minimal safe environment is used.
	Env map[string]string

	// WorkingDir is the working directory for the command.
	WorkingDir string

	// Timeout is the maximum execution time.
	// If zero, the context deadline is used.
	Timeout time.Duration

	// Stdin provides input to the command.
	Stdin io.Reader

	// ResourceLimits configures resource constraints.
	ResourceLimits *ResourceLimits

	// SandboxProfile specifies the sandbox configuration name.
	SandboxProfile string

	// Metadata contains arbitrary key-value pairs for tracing/logging.
	Metadata map[string]string

	// Priority affects scheduling in the worker pool.
	Priority Priority
}

// ResourceLimits defines resource constraints for command execution.
type ResourceLimits struct {
	// MaxCPUTime is the maximum CPU time allowed.
	MaxCPUTime time.Duration

	// MaxWallTime is the maximum wall clock time allowed.
	MaxWallTime time.Duration

	// MaxMemoryBytes is the maximum memory in bytes.
	MaxMemoryBytes int64

	// MaxOutputBytes is the maximum stdout+stderr size.
	MaxOutputBytes int64

	// MaxFileSize is the maximum file size that can be created.
	MaxFileSize int64

	// MaxProcesses is the maximum number of processes/threads.
	MaxProcesses int

	// MaxOpenFiles is the maximum open file descriptors.
	MaxOpenFiles int
}

// Priority represents command execution priority.
type Priority int

const (
	// PriorityLow is for background tasks.
	PriorityLow Priority = iota
	// PriorityNormal is the default priority.
	PriorityNormal
	// PriorityHigh is for time-sensitive tasks.
	PriorityHigh
	// PriorityCritical is for urgent tasks.
	PriorityCritical
)

// String returns the string representation of the priority.
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// CommandBuilder provides a fluent API for constructing commands.
type CommandBuilder struct {
	cmd *Command
	err error
}

// NewCommand creates a new CommandBuilder with the specified binary and arguments.
func NewCommand(binary string, args ...string) *CommandBuilder {
	return &CommandBuilder{
		cmd: &Command{
			Binary:   binary,
			Args:     args,
			Env:      make(map[string]string),
			Metadata: make(map[string]string),
			Priority: PriorityNormal,
		},
	}
}

// WithWorkingDir sets the working directory.
func (b *CommandBuilder) WithWorkingDir(dir string) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.WorkingDir = dir
	return b
}

// WithTimeout sets the execution timeout.
func (b *CommandBuilder) WithTimeout(timeout time.Duration) *CommandBuilder {
	if b.err != nil {
		return b
	}
	if timeout <= 0 {
		b.err = fmt.Errorf("timeout must be positive")
		return b
	}
	b.cmd.Timeout = timeout
	return b
}

// WithEnv adds environment variables.
func (b *CommandBuilder) WithEnv(key, value string) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.Env[key] = value
	return b
}

// WithEnvMap adds multiple environment variables.
func (b *CommandBuilder) WithEnvMap(env map[string]string) *CommandBuilder {
	if b.err != nil {
		return b
	}
	for k, v := range env {
		b.cmd.Env[k] = v
	}
	return b
}

// WithStdin sets the standard input reader.
func (b *CommandBuilder) WithStdin(stdin io.Reader) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.Stdin = stdin
	return b
}

// WithResourceLimits sets resource constraints.
func (b *CommandBuilder) WithResourceLimits(limits *ResourceLimits) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.ResourceLimits = limits
	return b
}

// WithSandboxProfile sets the sandbox profile name.
func (b *CommandBuilder) WithSandboxProfile(profile string) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.SandboxProfile = profile
	return b
}

// WithMetadata adds metadata for tracing/logging.
func (b *CommandBuilder) WithMetadata(key, value string) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.Metadata[key] = value
	return b
}

// WithPriority sets the execution priority.
func (b *CommandBuilder) WithPriority(priority Priority) *CommandBuilder {
	if b.err != nil {
		return b
	}
	b.cmd.Priority = priority
	return b
}

// Build validates and returns the command.
func (b *CommandBuilder) Build() (*Command, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Validate binary path
	if b.cmd.Binary == "" {
		return nil, fmt.Errorf("%w: binary path is required", ErrInvalidCommand)
	}

	// Must be absolute path
	if !filepath.IsAbs(b.cmd.Binary) {
		return nil, fmt.Errorf("%w: binary must be an absolute path", ErrInvalidCommand)
	}

	// Validate working directory if set
	if b.cmd.WorkingDir != "" && !filepath.IsAbs(b.cmd.WorkingDir) {
		return nil, fmt.Errorf("%w: working directory must be an absolute path", ErrInvalidCommand)
	}

	return b.cmd, nil
}

// MustBuild validates and returns the command, panicking on error.
func (b *CommandBuilder) MustBuild() *Command {
	cmd, err := b.Build()
	if err != nil {
		panic(err)
	}
	return cmd
}

// Clone creates a deep copy of the command.
func (c *Command) Clone() *Command {
	clone := &Command{
		Binary:         c.Binary,
		Args:           make([]string, len(c.Args)),
		Env:            make(map[string]string, len(c.Env)),
		WorkingDir:     c.WorkingDir,
		Timeout:        c.Timeout,
		Stdin:          c.Stdin,
		SandboxProfile: c.SandboxProfile,
		Metadata:       make(map[string]string, len(c.Metadata)),
		Priority:       c.Priority,
	}

	copy(clone.Args, c.Args)

	for k, v := range c.Env {
		clone.Env[k] = v
	}

	for k, v := range c.Metadata {
		clone.Metadata[k] = v
	}

	if c.ResourceLimits != nil {
		clone.ResourceLimits = &ResourceLimits{
			MaxCPUTime:     c.ResourceLimits.MaxCPUTime,
			MaxWallTime:    c.ResourceLimits.MaxWallTime,
			MaxMemoryBytes: c.ResourceLimits.MaxMemoryBytes,
			MaxOutputBytes: c.ResourceLimits.MaxOutputBytes,
			MaxFileSize:    c.ResourceLimits.MaxFileSize,
			MaxProcesses:   c.ResourceLimits.MaxProcesses,
			MaxOpenFiles:   c.ResourceLimits.MaxOpenFiles,
		}
	}

	return clone
}

// String returns a string representation of the command.
func (c *Command) String() string {
	if len(c.Args) == 0 {
		return c.Binary
	}
	return fmt.Sprintf("%s %v", c.Binary, c.Args)
}
