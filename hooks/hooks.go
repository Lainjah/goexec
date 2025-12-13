// Package hooks provides extension points for command execution lifecycle.
package hooks

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/victoralfred/goexec/executor"
)

// Hook defines extension points for command execution lifecycle.
type Hook interface {
	// Name returns a unique identifier for the hook.
	Name() string

	// Priority determines execution order (lower = earlier).
	Priority() int
}

// PreExecuteHook is called before command execution.
type PreExecuteHook interface {
	Hook
	PreExecute(ctx context.Context, cmd *executor.Command) (*executor.Command, error)
}

// PostExecuteHook is called after command execution.
type PostExecuteHook interface {
	Hook
	PostExecute(ctx context.Context, cmd *executor.Command, result *executor.Result, err error) error
}

// ValidationHook adds custom validation logic.
type ValidationHook interface {
	Hook
	Validate(ctx context.Context, cmd *executor.Command) error
}

// TransformHook can modify commands before execution.
type TransformHook interface {
	Hook
	Transform(ctx context.Context, cmd *executor.Command) (*executor.Command, error)
}

// ErrorHook is called when an error occurs.
type ErrorHook interface {
	Hook
	OnError(ctx context.Context, cmd *executor.Command, err error) error
}

// Registry manages hook registration and invocation.
type Registry struct {
	preExecute  []PreExecuteHook
	postExecute []PostExecuteHook
	validation  []ValidationHook
	transform   []TransformHook
	errorHooks  []ErrorHook
	mu          sync.RWMutex
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		preExecute:  make([]PreExecuteHook, 0),
		postExecute: make([]PostExecuteHook, 0),
		validation:  make([]ValidationHook, 0),
		transform:   make([]TransformHook, 0),
		errorHooks:  make([]ErrorHook, 0),
	}
}

// Register adds a hook to the registry.
func (r *Registry) Register(hook Hook) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Register based on hook type (can implement multiple)
	if h, ok := hook.(PreExecuteHook); ok {
		r.preExecute = append(r.preExecute, h)
		sort.Slice(r.preExecute, func(i, j int) bool {
			return r.preExecute[i].Priority() < r.preExecute[j].Priority()
		})
	}

	if h, ok := hook.(PostExecuteHook); ok {
		r.postExecute = append(r.postExecute, h)
		sort.Slice(r.postExecute, func(i, j int) bool {
			return r.postExecute[i].Priority() < r.postExecute[j].Priority()
		})
	}

	if h, ok := hook.(ValidationHook); ok {
		r.validation = append(r.validation, h)
		sort.Slice(r.validation, func(i, j int) bool {
			return r.validation[i].Priority() < r.validation[j].Priority()
		})
	}

	if h, ok := hook.(TransformHook); ok {
		r.transform = append(r.transform, h)
		sort.Slice(r.transform, func(i, j int) bool {
			return r.transform[i].Priority() < r.transform[j].Priority()
		})
	}

	if h, ok := hook.(ErrorHook); ok {
		r.errorHooks = append(r.errorHooks, h)
		sort.Slice(r.errorHooks, func(i, j int) bool {
			return r.errorHooks[i].Priority() < r.errorHooks[j].Priority()
		})
	}

	return nil
}

// Unregister removes a hook by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.preExecute = removeByName(r.preExecute, name)
	r.postExecute = removeByNamePost(r.postExecute, name)
	r.validation = removeByNameValidation(r.validation, name)
	r.transform = removeByNameTransform(r.transform, name)
	r.errorHooks = removeByNameError(r.errorHooks, name)
}

// RunPreExecute runs all pre-execute hooks.
func (r *Registry) RunPreExecute(ctx context.Context, cmd *executor.Command) (*executor.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	current := cmd
	for _, hook := range r.preExecute {
		modified, err := hook.PreExecute(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("hook %s: %w", hook.Name(), err)
		}
		current = modified
	}
	return current, nil
}

// RunPostExecute runs all post-execute hooks.
func (r *Registry) RunPostExecute(ctx context.Context, cmd *executor.Command, result *executor.Result, execErr error) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, hook := range r.postExecute {
		if err := hook.PostExecute(ctx, cmd, result, execErr); err != nil {
			return fmt.Errorf("hook %s: %w", hook.Name(), err)
		}
	}
	return nil
}

// RunValidation runs all validation hooks.
func (r *Registry) RunValidation(ctx context.Context, cmd *executor.Command) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, hook := range r.validation {
		if err := hook.Validate(ctx, cmd); err != nil {
			return fmt.Errorf("hook %s: %w", hook.Name(), err)
		}
	}
	return nil
}

// RunTransform runs all transform hooks.
func (r *Registry) RunTransform(ctx context.Context, cmd *executor.Command) (*executor.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	current := cmd
	for _, hook := range r.transform {
		modified, err := hook.Transform(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("hook %s: %w", hook.Name(), err)
		}
		current = modified
	}
	return current, nil
}

// RunError runs all error hooks.
func (r *Registry) RunError(ctx context.Context, cmd *executor.Command, execErr error) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, hook := range r.errorHooks {
		if err := hook.OnError(ctx, cmd, execErr); err != nil {
			return fmt.Errorf("hook %s: %w", hook.Name(), err)
		}
	}
	return nil
}

// Helper functions for removing hooks by name
func removeByName(hooks []PreExecuteHook, name string) []PreExecuteHook {
	result := make([]PreExecuteHook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name() != name {
			result = append(result, h)
		}
	}
	return result
}

func removeByNamePost(hooks []PostExecuteHook, name string) []PostExecuteHook {
	result := make([]PostExecuteHook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name() != name {
			result = append(result, h)
		}
	}
	return result
}

func removeByNameValidation(hooks []ValidationHook, name string) []ValidationHook {
	result := make([]ValidationHook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name() != name {
			result = append(result, h)
		}
	}
	return result
}

func removeByNameTransform(hooks []TransformHook, name string) []TransformHook {
	result := make([]TransformHook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name() != name {
			result = append(result, h)
		}
	}
	return result
}

func removeByNameError(hooks []ErrorHook, name string) []ErrorHook {
	result := make([]ErrorHook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name() != name {
			result = append(result, h)
		}
	}
	return result
}

// LoggingHook is a built-in hook that logs execution.
type LoggingHook struct {
	logger func(format string, args ...interface{})
}

// NewLoggingHook creates a new logging hook.
func NewLoggingHook(logger func(format string, args ...interface{})) *LoggingHook {
	return &LoggingHook{logger: logger}
}

func (h *LoggingHook) Name() string  { return "logging" }
func (h *LoggingHook) Priority() int { return 1000 }

func (h *LoggingHook) PreExecute(ctx context.Context, cmd *executor.Command) (*executor.Command, error) {
	h.logger("Executing: %s %v", cmd.Binary, cmd.Args)
	return cmd, nil
}

func (h *LoggingHook) PostExecute(ctx context.Context, cmd *executor.Command, result *executor.Result, err error) error {
	if err != nil {
		h.logger("Execution failed: %s - %v", cmd.Binary, err)
	} else {
		h.logger("Execution completed: %s - status=%s duration=%v", cmd.Binary, result.Status, result.Duration)
	}
	return nil
}
