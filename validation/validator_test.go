package validation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/victoralfred/goexec/executor"
)

// mockValidator is a mock validator for testing.
type mockValidator struct { //nolint:govet // fieldalignment: test struct field order optimized for readability not memory
	name         string
	priority     int
	validateFunc func(ctx context.Context, cmd *executor.Command) error
}

func (m *mockValidator) Name() string {
	return m.name
}

func (m *mockValidator) Priority() int {
	return m.priority
}

func (m *mockValidator) Validate(ctx context.Context, cmd *executor.Command) error {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, cmd)
	}
	return nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	v1 := &mockValidator{name: "validator1", priority: 10}
	v2 := &mockValidator{name: "validator2", priority: 5}
	v3 := &mockValidator{name: "validator3", priority: 15}

	registry.Register(v1)
	registry.Register(v2)
	registry.Register(v3)

	// Should be sorted by priority
	cmd := &executor.Command{Binary: "/bin/echo"}
	err := registry.ValidateAll(context.Background(), cmd)
	if err != nil {
		t.Errorf("Unexpected validation error: %v", err)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()

	v := &mockValidator{name: "validator1", priority: 10}
	registry.Register(v)

	registry.Unregister("validator1")

	// Should not find validator
	registry.Unregister("nonexistent") // Should not panic
}

func TestRegistry_ValidateAll_Success(t *testing.T) {
	registry := NewRegistry()

	v1 := &mockValidator{name: "v1", priority: 10}
	v2 := &mockValidator{name: "v2", priority: 20}

	registry.Register(v1)
	registry.Register(v2)

	cmd := &executor.Command{Binary: "/bin/echo", Args: []string{"test"}}
	err := registry.ValidateAll(context.Background(), cmd)
	if err != nil {
		t.Errorf("Expected no errors, got %v", err)
	}
}

func TestRegistry_ValidateAll_MultipleErrors(t *testing.T) {
	registry := NewRegistry()

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	v1 := &mockValidator{
		name:     "v1",
		priority: 10,
		validateFunc: func(ctx context.Context, cmd *executor.Command) error {
			return err1
		},
	}
	v2 := &mockValidator{
		name:     "v2",
		priority: 20,
		validateFunc: func(ctx context.Context, cmd *executor.Command) error {
			return err2
		},
	}

	registry.Register(v1)
	registry.Register(v2)

	cmd := &executor.Command{Binary: "/bin/echo"}
	err := registry.ValidateAll(context.Background(), cmd)

	var validationErrs *Errors
	if !errors.As(err, &validationErrs) {
		t.Fatalf("Expected ValidationErrors, got %T", err)
	}

	if len(validationErrs.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(validationErrs.Errors))
	}
}

func TestErrors_Error(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	validationErrs := &Errors{
		Errors: []error{err1},
	}

	// Single error
	msg := validationErrs.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}

	// Multiple errors
	validationErrs.Errors = []error{err1, err2}
	msg = validationErrs.Error()
	if !strings.Contains(msg, "2 validation errors") {
		t.Errorf("Expected message about 2 errors, got '%s'", msg)
	}
}

func TestErrors_Unwrap(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	validationErrs := &Errors{
		Errors: []error{err1, err2},
	}

	if validationErrs.Unwrap() != err1 {
		t.Error("Unwrap should return first error")
	}

	emptyErrs := &Errors{Errors: []error{}}
	if emptyErrs.Unwrap() != nil {
		t.Error("Unwrap should return nil for empty errors")
	}
}

func TestErrors_Is(t *testing.T) {
	targetErr := errors.New("target")
	err1 := errors.New("error 1")
	err2 := targetErr

	validationErrs := &Errors{
		Errors: []error{err1, err2},
	}

	if !validationErrs.Is(targetErr) {
		t.Error("Is should return true if any error matches")
	}

	if validationErrs.Is(errors.New("other")) {
		t.Error("Is should return false if no error matches")
	}
}

func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()
	if registry == nil {
		t.Fatal("DefaultRegistry returned nil")
	}

	// Should have default validators registered
	cmd := &executor.Command{
		Binary: "/bin/echo",
		Args:   []string{"test"},
	}

	err := registry.ValidateAll(context.Background(), cmd)
	// May fail due to path validation, but should not panic
	_ = err
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	var wg sync.WaitGroup
	concurrency := 20

	// Concurrent register
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			v := &mockValidator{
				name:     "validator" + string(rune('0'+id)),
				priority: id,
			}
			registry.Register(v)
		}(i)
	}

	wg.Wait()

	// Concurrent validate
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := &executor.Command{Binary: "/bin/echo"}
			validateErr := registry.ValidateAll(context.Background(), cmd)
			_ = validateErr // Ignore validation errors in concurrent test
		}()
	}

	wg.Wait()
	// Should not panic
}

func TestRegistry_PriorityOrdering(t *testing.T) {
	registry := NewRegistry()

	var callOrder []string

	v1 := &mockValidator{
		name:     "last",
		priority: 100,
		validateFunc: func(ctx context.Context, cmd *executor.Command) error {
			callOrder = append(callOrder, "last")
			return nil
		},
	}
	v2 := &mockValidator{
		name:     "first",
		priority: 10,
		validateFunc: func(ctx context.Context, cmd *executor.Command) error {
			callOrder = append(callOrder, "first")
			return nil
		},
	}

	registry.Register(v1)
	registry.Register(v2)

	cmd := &executor.Command{Binary: "/bin/echo"}
	validateErr := registry.ValidateAll(context.Background(), cmd)
	_ = validateErr // Ignore validation error for test purposes

	if len(callOrder) != 2 {
		t.Fatalf("Expected 2 validators called, got %d", len(callOrder))
	}

	// Lower priority should be called first
	if callOrder[0] != "first" {
		t.Errorf("Expected 'first' first, got '%s'", callOrder[0])
	}
}
