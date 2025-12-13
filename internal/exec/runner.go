// Package exec provides the internal command execution wrapper.
// This is the ONLY package in the entire library that imports os/exec.
// All command execution MUST go through this package.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// Runner executes commands using os/exec.CommandContext.
// This is the sole abstraction for process invocation.
type Runner struct {
	// minimalEnv contains the minimal safe environment variables.
	minimalEnv []string
}

// NewRunner creates a new command runner.
func NewRunner() *Runner {
	return &Runner{
		minimalEnv: []string{
			"PATH=/usr/bin:/bin",
			"LANG=C.UTF-8",
			"LC_ALL=C.UTF-8",
		},
	}
}

// RunConfig contains configuration for running a command.
type RunConfig struct {
	// Binary is the absolute path to the executable.
	Binary string

	// Args are the command arguments (excluding the binary name).
	Args []string

	// Env is the environment variables. If nil, minimalEnv is used.
	Env []string

	// WorkingDir is the working directory.
	WorkingDir string

	// Stdin provides input to the command.
	Stdin io.Reader

	// Stdout receives standard output. If nil, output is captured.
	Stdout io.Writer

	// Stderr receives standard error. If nil, output is captured.
	Stderr io.Writer

	// SysProcAttr contains OS-specific process attributes.
	SysProcAttr *syscall.SysProcAttr
}

// RunResult contains the result of command execution.
type RunResult struct {
	// ExitCode is the process exit code.
	ExitCode int

	// Signal is the signal that terminated the process, if any.
	Signal syscall.Signal

	// Stdout contains captured standard output (if not streaming).
	Stdout []byte

	// Stderr contains captured standard error (if not streaming).
	Stderr []byte

	// Duration is the wall clock time of execution.
	Duration time.Duration

	// ProcessState contains the OS process state.
	ProcessState *ProcessState
}

// ProcessState contains OS-level process information.
type ProcessState struct {
	Pid        int
	UserTime   time.Duration
	SystemTime time.Duration
}

// Run executes a command with the given context and configuration.
// The context MUST have a deadline set for timeout enforcement.
func (r *Runner) Run(ctx context.Context, config *RunConfig) (*RunResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Verify context has a deadline
	if _, ok := ctx.Deadline(); !ok {
		return nil, fmt.Errorf("context must have a deadline for timeout enforcement")
	}

	// Create command with context for automatic cancellation
	// G204: Binary and Args are validated by PathValidator and ArgumentValidator
	// before reaching this point. We use CommandContext with separate binary/args
	// (not shell execution) which prevents command injection.
	// #nosec G204 -- Binary path and arguments are validated upstream
	cmd := exec.CommandContext(ctx, config.Binary, config.Args...)

	// Set environment - use minimal env if none provided
	if len(config.Env) > 0 {
		cmd.Env = config.Env
	} else {
		cmd.Env = r.minimalEnv
	}

	// Set working directory
	if config.WorkingDir != "" {
		cmd.Dir = config.WorkingDir
	}

	// Set stdin
	if config.Stdin != nil {
		cmd.Stdin = config.Stdin
	}

	// Configure stdout capture or streaming
	var stdoutBuf, stderrBuf bytes.Buffer
	if config.Stdout != nil {
		cmd.Stdout = config.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	// Configure stderr capture or streaming
	if config.Stderr != nil {
		cmd.Stderr = config.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	// Set process attributes for security
	if config.SysProcAttr != nil {
		cmd.SysProcAttr = config.SysProcAttr
	} else {
		cmd.SysProcAttr = defaultSysProcAttr()
	}

	// Execute command
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Build result
	result := &RunResult{
		Duration: duration,
	}

	// Capture output if not streaming
	if config.Stdout == nil {
		result.Stdout = stdoutBuf.Bytes()
	}
	if config.Stderr == nil {
		result.Stderr = stderrBuf.Bytes()
	}

	// Extract process state
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
		result.ProcessState = &ProcessState{
			Pid:        cmd.ProcessState.Pid(),
			UserTime:   cmd.ProcessState.UserTime(),
			SystemTime: cmd.ProcessState.SystemTime(),
		}

		// Check if killed by signal
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				result.Signal = ws.Signal()
			}
		}
	}

	return result, err
}

// defaultSysProcAttr returns secure default process attributes.
func defaultSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Create a new process group so we can kill all children
		Setpgid: true,
		Pgid:    0,
	}
}

// BuildEnv creates an environment slice from a map.
func BuildEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// MergeEnv merges base environment with overrides.
func MergeEnv(base, override []string) []string {
	envMap := make(map[string]string)

	// Parse base
	for _, e := range base {
		if idx := indexOf(e, '='); idx > 0 {
			envMap[e[:idx]] = e[idx+1:]
		}
	}

	// Parse overrides
	for _, e := range override {
		if idx := indexOf(e, '='); idx > 0 {
			envMap[e[:idx]] = e[idx+1:]
		}
	}

	// Build result
	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
