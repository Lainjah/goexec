// Package validation provides input validation and sanitization.
package validation

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/victoralfred/goexec/executor"
)

// Validator validates command inputs.
type Validator interface {
	// Name returns the validator name.
	Name() string

	// Validate validates a command.
	Validate(ctx context.Context, cmd *executor.Command) error

	// Priority determines execution order (lower = earlier).
	Priority() int
}

// Registry manages custom validators.
type Registry struct {
	validators []Validator
	mu         sync.RWMutex
}

// NewRegistry creates a new validator registry.
func NewRegistry() *Registry {
	return &Registry{
		validators: make([]Validator, 0),
	}
}

// Register adds a validator to the registry.
func (r *Registry) Register(v Validator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.validators = append(r.validators, v)

	// Sort by priority
	for i := len(r.validators) - 1; i > 0; i-- {
		if r.validators[i].Priority() < r.validators[i-1].Priority() {
			r.validators[i], r.validators[i-1] = r.validators[i-1], r.validators[i]
		}
	}
}

// Unregister removes a validator by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, v := range r.validators {
		if v.Name() == name {
			r.validators = append(r.validators[:i], r.validators[i+1:]...)
			return
		}
	}
}

// ValidateAll runs all validators against a command.
func (r *Registry) ValidateAll(ctx context.Context, cmd *executor.Command) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for _, v := range r.validators {
		if err := v.Validate(ctx, cmd); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", v.Name(), err))
		}
	}

	if len(errs) > 0 {
		return &ValidationErrors{Errors: errs}
	}
	return nil
}

// ValidationErrors contains multiple validation errors.
type ValidationErrors struct {
	Errors []error
}

// Error returns the error message.
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("%d validation errors occurred", len(e.Errors))
}

// Unwrap returns the first error.
func (e *ValidationErrors) Unwrap() error {
	if len(e.Errors) > 0 {
		return e.Errors[0]
	}
	return nil
}

// Is reports whether any error matches the target.
func (e *ValidationErrors) Is(target error) bool {
	for _, err := range e.Errors {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// DefaultRegistry creates a registry with default validators.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewPathValidator(nil))
	r.Register(NewArgumentValidator(nil))
	r.Register(NewEnvironmentValidator(nil))
	return r
}
