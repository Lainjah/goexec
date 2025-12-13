package policy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/victoralfred/gowritter/safepath"
	"gopkg.in/yaml.v3"
)

// Loader loads and manages policies from YAML files.
type Loader struct {
	lastLoad   time.Time
	safePath   *safepath.SafePath
	policy     *CompiledPolicy
	watchStop  chan struct{}
	path       string
	lastHash   []byte
	validators []Validator
	onChange   []func(*CompiledPolicy)
	mu         sync.RWMutex
}

// Validator validates a policy configuration.
type Validator interface {
	Validate(config *Config) error
}

// LoaderOption configures the loader.
type LoaderOption func(*Loader)

// WithValidator adds a policy validator.
func WithValidator(v Validator) LoaderOption {
	return func(l *Loader) {
		l.validators = append(l.validators, v)
	}
}

// WithOnChange adds a callback for policy changes.
func WithOnChange(fn func(*CompiledPolicy)) LoaderOption {
	return func(l *Loader) {
		l.onChange = append(l.onChange, fn)
	}
}

// NewLoader creates a new policy loader.
func NewLoader(basePath, policyFile string, opts ...LoaderOption) (*Loader, error) {
	sp, err := safepath.New(basePath)
	if err != nil {
		return nil, fmt.Errorf("creating safe path: %w", err)
	}

	l := &Loader{
		path:       policyFile,
		safePath:   sp,
		validators: make([]Validator, 0),
		onChange:   make([]func(*CompiledPolicy), 0),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l, nil
}

// Load loads the policy from the file.
func (l *Loader) Load(ctx context.Context) (*CompiledPolicy, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Read file using gowritter
	data, err := l.safePath.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	// Check if file changed
	hash := sha256.Sum256(data)
	if l.policy != nil && len(hash) == len(l.lastHash) && bytes.Equal(hash[:], l.lastHash) {
		return l.policy, nil
	}

	// Parse YAML
	var config Config
	if parseErr := yaml.Unmarshal(data, &config); parseErr != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", parseErr)
	}

	// Validate policy
	for _, v := range l.validators {
		if validateErr := v.Validate(&config); validateErr != nil {
			return nil, fmt.Errorf("policy validation failed: %w", validateErr)
		}
	}

	// Compile policy
	compiled, err := NewCompiledPolicy(&config)
	if err != nil {
		return nil, fmt.Errorf("compiling policy: %w", err)
	}

	compiled.hash = fmt.Sprintf("%x", hash)

	l.policy = compiled
	l.lastHash = hash[:]
	l.lastLoad = time.Now()

	// Notify listeners
	for _, fn := range l.onChange {
		fn(compiled)
	}

	return compiled, nil
}

// Get returns the current policy without reloading.
func (l *Loader) Get() *CompiledPolicy {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.policy
}

// Reload reloads the policy from the file.
func (l *Loader) Reload(ctx context.Context) error {
	_, err := l.Load(ctx)
	return err
}

// Watch starts watching for policy file changes.
func (l *Loader) Watch(ctx context.Context, interval time.Duration) {
	l.watchStop = make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-l.watchStop:
				return
			case <-ticker.C:
				if _, err := l.Load(ctx); err != nil {
					// Log error but continue watching
					_ = err
				}
			}
		}
	}()
}

// StopWatch stops watching for policy changes.
func (l *Loader) StopWatch() {
	if l.watchStop != nil {
		close(l.watchStop)
	}
}

// ParseYAML parses a YAML policy configuration.
func ParseYAML(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// DefaultValidator validates policy configuration.
type DefaultValidator struct{}

// Validate validates the policy configuration.
func (v *DefaultValidator) Validate(config *Config) error {
	if config.Version == "" {
		return fmt.Errorf("policy version is required")
	}

	// Validate binaries
	for i := range config.Binaries {
		b := &config.Binaries[i]
		if b.Path == "" {
			return fmt.Errorf("binary %d: path is required", i)
		}

		// Validate arg patterns
		for j := range b.AllowedArgs {
			if b.AllowedArgs[j].Pattern == "" {
				return fmt.Errorf("binary %d, allowed_arg %d: pattern is required", i, j)
			}
		}

		for j := range b.DeniedArgs {
			if b.DeniedArgs[j].Pattern == "" {
				return fmt.Errorf("binary %d, denied_arg %d: pattern is required", i, j)
			}
		}
	}

	return nil
}

// ExamplePolicy returns an example policy configuration.
func ExamplePolicy() *Config {
	return &Config{
		Version: "1.0",
		Metadata: Metadata{
			Name:        "example-policy",
			Description: "Example security policy",
		},
		Global: GlobalConfig{
			DefaultSandbox: "restricted",
			DefaultLimits: ResourceConfig{
				MaxCPUTime:   Duration{30 * time.Second},
				MaxWallTime:  Duration{60 * time.Second},
				MaxMemory:    ByteSize{256 * 1024 * 1024},
				MaxOutput:    ByteSize{10 * 1024 * 1024},
				MaxProcesses: 10,
				MaxOpenFiles: 64,
			},
			AllowedEnv: []string{"PATH", "HOME", "USER", "LANG", "LC_ALL"},
			DeniedEnv:  []string{"*_SECRET*", "*_PASSWORD*", "AWS_*"},
		},
		Binaries: []BinaryConfig{
			{
				Path:    "/usr/bin/git",
				Enabled: true,
				AllowedArgs: []ArgPattern{
					{Pattern: "^(status|log|diff|branch|show)$", Position: 0, Description: "Read-only operations"},
					{Pattern: "^--.*$", Description: "Long-form flags"},
					{Pattern: "^-[a-zA-Z]$", Description: "Short flags"},
				},
				DeniedArgs: []ArgPattern{
					{Pattern: "^--exec$", Description: "No arbitrary execution"},
				},
				Sandbox: "git-profile",
			},
			{
				Path:    "/usr/bin/ls",
				Enabled: true,
				AllowedArgs: []ArgPattern{
					{Pattern: "^-[alh1]+$", Description: "Common flags"},
					{Pattern: "^/[a-zA-Z0-9_/.-]+$", Description: "Absolute paths"},
				},
				Sandbox: "minimal",
			},
		},
		Audit: AuditConfig{
			Enabled:  true,
			LogLevel: "all",
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          Duration{30 * time.Second},
			PerBinary:        true,
		},
	}
}
