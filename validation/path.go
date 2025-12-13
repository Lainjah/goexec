package validation

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/victoralfred/goexec/executor"
	"github.com/victoralfred/gowritter/safepath"
)

// PathValidatorConfig configures the path validator.
type PathValidatorConfig struct {
	// AllowedPrefixes are path prefixes that are allowed.
	AllowedPrefixes []string

	// DeniedPrefixes are path prefixes that are denied.
	DeniedPrefixes []string

	// AllowSymlinks allows symlinks in paths.
	AllowSymlinks bool

	// RequireExecutable requires the binary to be executable.
	RequireExecutable bool

	// AllowRelativeWorkDir allows relative working directories.
	AllowRelativeWorkDir bool
}

// PathValidator validates file paths.
type PathValidator struct {
	config *PathValidatorConfig
	rootFS *safepath.SafePath
}

// NewPathValidator creates a new path validator.
func NewPathValidator(config *PathValidatorConfig) *PathValidator {
	if config == nil {
		config = &PathValidatorConfig{
			AllowedPrefixes: []string{
				"/usr/bin",
				"/usr/local/bin",
				"/bin",
				"/sbin",
			},
			DeniedPrefixes: []string{
				"/etc",
				"/root",
				"/proc",
				"/sys",
			},
			AllowSymlinks:        false,
			RequireExecutable:    true,
			AllowRelativeWorkDir: false,
		}
	}

	v := &PathValidator{config: config}

	// Initialize rootFS for file operations on Unix systems
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if fs, err := safepath.New("/"); err == nil {
			v.rootFS = fs
		}
	}

	return v
}

// Name returns the validator name.
func (v *PathValidator) Name() string {
	return "path_validator"
}

// Priority returns the execution priority.
func (v *PathValidator) Priority() int {
	return 10
}

// Validate validates a command's paths.
func (v *PathValidator) Validate(ctx context.Context, cmd *executor.Command) error {
	// Validate binary path
	if err := v.validateBinaryPath(cmd.Binary); err != nil {
		return fmt.Errorf("binary: %w", err)
	}

	// Validate working directory
	if cmd.WorkingDir != "" {
		if err := v.validateWorkingDir(cmd.WorkingDir); err != nil {
			return fmt.Errorf("working directory: %w", err)
		}
	}

	return nil
}

// validateBinaryPath validates the binary path.
func (v *PathValidator) validateBinaryPath(path string) error {
	// Must be provided
	if path == "" {
		return fmt.Errorf("%w: binary path is required", executor.ErrInvalidPath)
	}

	// Must be absolute
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: must be absolute path", executor.ErrInvalidPath)
	}

	// Clean and check for traversal
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return executor.ErrPathTraversal
	}

	// Check against allowed prefixes
	if len(v.config.AllowedPrefixes) > 0 {
		allowed := false
		for _, prefix := range v.config.AllowedPrefixes {
			if strings.HasPrefix(cleaned, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: path not in allowed prefixes", executor.ErrInvalidPath)
		}
	}

	// Check against denied prefixes
	for _, prefix := range v.config.DeniedPrefixes {
		if strings.HasPrefix(cleaned, prefix) {
			return fmt.Errorf("%w: path in denied prefix %s", executor.ErrInvalidPath, prefix)
		}
	}

	// Check symlinks if not allowed
	if !v.config.AllowSymlinks {
		realPath, err := filepath.EvalSymlinks(cleaned)
		if err == nil && realPath != cleaned {
			return fmt.Errorf("%w: symlinks not allowed", executor.ErrInvalidPath)
		}
	}

	// Check if executable exists and is executable
	if v.config.RequireExecutable {
		if v.rootFS == nil {
			return fmt.Errorf("%w: filesystem not available", executor.ErrInvalidPath)
		}

		// Use relative path from root for safepath
		relPath := strings.TrimPrefix(cleaned, "/")
		info, err := v.rootFS.Stat(relPath)
		if err != nil {
			exists, _ := v.rootFS.Exists(relPath)
			if !exists {
				return fmt.Errorf("%w: binary does not exist", executor.ErrInvalidPath)
			}
			return fmt.Errorf("%w: cannot stat binary: %v", executor.ErrInvalidPath, err)
		}

		if info.IsDir() {
			return fmt.Errorf("%w: path is a directory", executor.ErrInvalidPath)
		}

		// Check executable permission
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("%w: binary is not executable", executor.ErrInvalidPath)
		}
	}

	return nil
}

// validateWorkingDir validates the working directory.
func (v *PathValidator) validateWorkingDir(path string) error {
	// Check if absolute required
	if !v.config.AllowRelativeWorkDir && !filepath.IsAbs(path) {
		return fmt.Errorf("%w: must be absolute path", executor.ErrInvalidPath)
	}

	// Clean and check for traversal
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return executor.ErrPathTraversal
	}

	if v.rootFS == nil {
		return fmt.Errorf("%w: filesystem not available", executor.ErrInvalidPath)
	}

	// Use relative path from root for safepath
	relPath := strings.TrimPrefix(cleaned, "/")

	// Check if directory exists
	info, err := v.rootFS.Stat(relPath)
	if err != nil {
		exists, _ := v.rootFS.Exists(relPath)
		if !exists {
			return fmt.Errorf("%w: directory does not exist", executor.ErrInvalidPath)
		}
		return fmt.Errorf("%w: cannot stat directory: %v", executor.ErrInvalidPath, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: path is not a directory", executor.ErrInvalidPath)
	}

	return nil
}

// SanitizePath cleans and validates a path.
func SanitizePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%w: empty path", executor.ErrInvalidPath)
	}

	// Clean the path
	cleaned := filepath.Clean(path)

	// Check for path traversal
	if strings.Contains(cleaned, "..") {
		return "", executor.ErrPathTraversal
	}

	// Check for null bytes
	if strings.ContainsRune(cleaned, 0) {
		return "", fmt.Errorf("%w: path contains null byte", executor.ErrInvalidPath)
	}

	return cleaned, nil
}

// IsPathSafe checks if a path is safe (no traversal, etc).
func IsPathSafe(path string) bool {
	_, err := SanitizePath(path)
	return err == nil
}

// ResolvePath resolves a path relative to a base directory safely.
func ResolvePath(base, path string) (string, error) {
	// If path is absolute, validate it directly
	if filepath.IsAbs(path) {
		return SanitizePath(path)
	}

	// Join with base
	joined := filepath.Join(base, path)
	cleaned := filepath.Clean(joined)

	// Ensure result is still under base
	if !strings.HasPrefix(cleaned, filepath.Clean(base)) {
		return "", executor.ErrPathTraversal
	}

	return cleaned, nil
}
