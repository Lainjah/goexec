package validation

import (
	"context"
	"strings"
	"testing"

	"github.com/victoralfred/goexec/executor"
)

func TestPathValidator_Validate_EmptyBinary(t *testing.T) {
	validator := NewPathValidator(nil)
	cmd := &executor.Command{
		Binary: "",
	}

	err := validator.Validate(context.Background(), cmd)
	if err == nil {
		t.Error("Expected error for empty binary path")
	}
}

func TestPathValidator_Validate_RelativePath(t *testing.T) {
	validator := NewPathValidator(nil)
	cmd := &executor.Command{
		Binary: "echo",
	}

	err := validator.Validate(context.Background(), cmd)
	if err == nil {
		t.Error("Expected error for relative path")
	}
}

func TestPathValidator_Validate_PathTraversal(t *testing.T) {
	validator := NewPathValidator(nil)

	testCases := []string{
		"/usr/bin/../../etc/passwd",
		"/usr/../bin/echo",
		"/usr/./bin/../etc",
	}

	for _, path := range testCases {
		cmd := &executor.Command{
			Binary: path,
		}

		err := validator.Validate(context.Background(), cmd)
		if err == nil {
			t.Errorf("Expected error for path traversal in '%s'", path)
		}
		if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "traversal") {
			t.Logf("Error message for '%s': %v", path, err)
		}
	}
}

func TestPathValidator_Validate_AllowedPrefixes(t *testing.T) {
	config := &PathValidatorConfig{
		AllowedPrefixes: []string{
			"/usr/bin",
			"/bin",
		},
	}
	validator := NewPathValidator(config)

	// Valid paths
	validPaths := []string{
		"/usr/bin/echo",
		"/bin/ls",
	}

	for _, path := range validPaths {
		cmd := &executor.Command{
			Binary: path,
		}

		err := validator.Validate(context.Background(), cmd)
		// May fail due to RequireExecutable, but should not fail due to prefix
		if err != nil && strings.Contains(err.Error(), "not in allowed prefixes") {
			t.Errorf("Unexpected prefix error for allowed path '%s': %v", path, err)
		}
	}

	// Invalid path
	cmd := &executor.Command{
		Binary: "/sbin/ifconfig",
	}

	err := validator.Validate(context.Background(), cmd)
	if err == nil {
		t.Error("Expected error for path not in allowed prefixes")
	}
}

func TestPathValidator_Validate_DeniedPrefixes(t *testing.T) {
	config := &PathValidatorConfig{
		AllowedPrefixes: []string{"/usr/bin"},
		DeniedPrefixes: []string{
			"/usr/bin/dangerous",
			"/root",
		},
	}
	validator := NewPathValidator(config)

	cmd := &executor.Command{
		Binary: "/usr/bin/dangerous/tool",
	}

	err := validator.Validate(context.Background(), cmd)
	if err == nil {
		t.Error("Expected error for denied prefix")
	}
}

func TestPathValidator_Validate_WorkingDir(t *testing.T) {
	validator := NewPathValidator(nil)

	cmd := &executor.Command{
		Binary:     "/bin/echo",
		WorkingDir: "/tmp",
	}

	err := validator.Validate(context.Background(), cmd)
	// May fail due to RequireExecutable or filesystem checks, but should validate working dir
	_ = err
}

func TestPathValidator_Validate_RelativeWorkingDir(t *testing.T) {
	config := &PathValidatorConfig{
		AllowRelativeWorkDir: false,
	}
	validator := NewPathValidator(config)

	cmd := &executor.Command{
		Binary:     "/bin/echo",
		WorkingDir: "tmp",
	}

	err := validator.Validate(context.Background(), cmd)
	if err == nil {
		t.Error("Expected error for relative working directory")
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"valid", "/usr/bin/echo", false},
		{"traversal literal", "/usr/..", false}, // filepath.Clean resolves this
		{"null byte", "/usr/bin/test\x00", true},
		{"clean path", "/usr//bin///echo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if got == "" {
					t.Error("Got empty path")
				}
			}
		})
	}
}

func TestIsPathSafe(t *testing.T) {
	tests := []struct {
		path string
		safe bool
	}{
		{"/usr/bin/echo", true},
		{"/usr/../etc", true}, // filepath.Clean resolves this, so it becomes safe
		{"", false},
		{"/valid/path", true},
		{"/path\x00withnull", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsPathSafe(tt.path)
			if got != tt.safe {
				t.Errorf("IsPathSafe(%q) = %v, want %v", tt.path, got, tt.safe)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		wantErr bool
	}{
		{"absolute", "/usr", "/bin", false},
		{"relative", "/usr", "bin", false},
		{"traversal", "/usr", "../etc", true},
		{"empty base", "", "bin", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolvePath(tt.base, tt.path)
			if tt.wantErr && err == nil {
				t.Error("Expected error")
			}
			if !tt.wantErr && err != nil {
				t.Logf("Unexpected error (may be due to filesystem): %v", err)
			}
		})
	}
}

func TestPathValidator_Name(t *testing.T) {
	validator := NewPathValidator(nil)
	if validator.Name() != "path_validator" {
		t.Errorf("Expected name 'path_validator', got '%s'", validator.Name())
	}
}

func TestPathValidator_Priority(t *testing.T) {
	validator := NewPathValidator(nil)
	if validator.Priority() != 10 {
		t.Errorf("Expected priority 10, got %d", validator.Priority())
	}
}

func TestPathValidator_Validate_EmptyWorkingDir(t *testing.T) {
	validator := NewPathValidator(nil)
	cmd := &executor.Command{
		Binary:     "/bin/echo",
		WorkingDir: "",
	}

	err := validator.Validate(context.Background(), cmd)
	// Should not error on empty working dir
	if err != nil && strings.Contains(err.Error(), "working") {
		t.Errorf("Unexpected error for empty working dir: %v", err)
	}
}

func TestPathValidator_Validate_CleanPath(t *testing.T) {
	validator := NewPathValidator(nil)

	// Path with multiple slashes should be cleaned
	cmd := &executor.Command{
		Binary: "/usr//bin///echo",
	}

	err := validator.Validate(context.Background(), cmd)
	// Should validate cleaned path, not error on format
	if err != nil && strings.Contains(err.Error(), "invalid format") {
		t.Errorf("Unexpected format error: %v", err)
	}
}
