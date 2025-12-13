// Package config provides configuration management for goexec.
package config

import (
	"time"

	"github.com/victoralfred/goexec/observability"
	"github.com/victoralfred/goexec/pool"
	"github.com/victoralfred/goexec/resilience"
)

// Config is the main configuration for goexec.
type Config struct {
	CircuitBreaker resilience.CircuitBreakerConfig
	RateLimiter    resilience.RateLimiterConfig
	Telemetry      observability.TelemetryConfig
	PolicyPath     string
	PolicyBasePath string
	Executor       ExecutorConfig
	Audit          observability.AuditConfig
	Pool           pool.Config
}

// ExecutorConfig configures the executor.
type ExecutorConfig struct {
	DefaultSandboxProfile string
	DefaultTimeout        time.Duration
	MaxConcurrent         int
	EnableSandbox         bool
	EnableMetrics         bool
	EnableTracing         bool
	EnableAudit           bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Executor: ExecutorConfig{
			DefaultTimeout:        30 * time.Second,
			MaxConcurrent:         100,
			EnableSandbox:         true,
			DefaultSandboxProfile: "restricted",
			EnableMetrics:         true,
			EnableTracing:         true,
			EnableAudit:           true,
		},
		Pool:           pool.DefaultConfig(),
		RateLimiter:    resilience.DefaultRateLimiterConfig(),
		CircuitBreaker: resilience.DefaultCircuitBreakerConfig(),
		Telemetry:      observability.DefaultTelemetryConfig(),
		Audit:          observability.DefaultAuditConfig(),
		PolicyPath:     "policy.yaml",
		PolicyBasePath: "/etc/goexec",
	}
}

// DevelopmentConfig returns configuration suitable for development.
func DevelopmentConfig() Config {
	cfg := DefaultConfig()
	cfg.Executor.EnableSandbox = false
	cfg.Executor.DefaultTimeout = 60 * time.Second
	cfg.RateLimiter.DefaultLimit = 1000
	cfg.RateLimiter.DefaultBurst = 2000
	cfg.CircuitBreaker.FailureThreshold = 10
	cfg.Audit.LogLevel = observability.AuditLogAll
	cfg.Audit.IncludeOutput = true
	return cfg
}

// ProductionConfig returns configuration suitable for production.
func ProductionConfig() Config {
	cfg := DefaultConfig()
	cfg.Executor.EnableSandbox = true
	cfg.Executor.DefaultTimeout = 30 * time.Second
	cfg.Executor.MaxConcurrent = 50
	cfg.RateLimiter.DefaultLimit = 100
	cfg.RateLimiter.DefaultBurst = 150
	cfg.CircuitBreaker.FailureThreshold = 5
	cfg.CircuitBreaker.Timeout = 60 * time.Second
	cfg.Audit.LogLevel = observability.AuditLogAll
	cfg.Audit.IncludeOutput = false
	return cfg
}

// RestrictedConfig returns highly restrictive configuration.
func RestrictedConfig() Config {
	cfg := ProductionConfig()
	cfg.Executor.EnableSandbox = true
	cfg.Executor.DefaultSandboxProfile = "minimal"
	cfg.Executor.MaxConcurrent = 10
	cfg.RateLimiter.DefaultLimit = 10
	cfg.RateLimiter.DefaultBurst = 20
	cfg.CircuitBreaker.FailureThreshold = 3
	return cfg
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Executor.DefaultTimeout <= 0 {
		c.Executor.DefaultTimeout = 30 * time.Second
	}

	if c.Executor.MaxConcurrent <= 0 {
		c.Executor.MaxConcurrent = 100
	}

	if c.Pool.MinWorkers <= 0 {
		c.Pool.MinWorkers = 1
	}

	if c.Pool.MaxWorkers < c.Pool.MinWorkers {
		c.Pool.MaxWorkers = c.Pool.MinWorkers
	}

	return nil
}
