# Changelog

All notable changes to GoExec will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2024-12-14

### Added

- **Core Executor** - Single abstraction for all process invocation
  - `Execute()` - Synchronous command execution
  - `ExecuteAsync()` - Asynchronous execution with Future pattern
  - `ExecuteBatch()` - Concurrent batch execution
  - `Stream()` - Real-time stdout/stderr streaming
  - Graceful shutdown with `Shutdown()`

- **Command Builder** - Fluent API for building commands
  - `WithWorkingDir()` - Set working directory
  - `WithTimeout()` - Set execution timeout
  - `WithEnv()` / `WithEnvMap()` - Environment variables
  - `WithStdin()` - Input stream
  - `WithPriority()` - Execution priority
  - `WithResourceLimits()` - CPU, memory, process limits
  - `WithSandboxProfile()` - Sandbox configuration
  - `WithMetadata()` - Custom metadata for tracing

- **Security Features**
  - Binary allowlisting via YAML policy
  - Argument validation with regex patterns
  - Path sanitization and traversal detection
  - Environment variable filtering
  - All file I/O uses `gowritter/safepath`

- **Resilience**
  - Rate limiting with token bucket algorithm
  - Circuit breaker with configurable thresholds
  - Exponential backoff for retries

- **Worker Pool**
  - Bounded worker pool with backpressure
  - Priority queue scheduling
  - Multiple backpressure strategies (block, reject, caller-runs, drop-oldest)
  - Auto-scaling based on queue depth

- **Observability**
  - OpenTelemetry metrics integration
  - Execution latency, exit codes, resource usage metrics
  - Audit logging support

- **Policy Engine**
  - YAML-based policy configuration
  - Hot reload support
  - Per-binary configuration
  - Global defaults with overrides

### Security

- Mandatory absolute paths for binaries
- Input validation for all command parameters
- Resource limits via cgroups and rlimits
- Seccomp and AppArmor integration (Linux)

### Requirements

- Go 1.22 or later
- Linux (for sandbox features)

[0.1.0]: https://github.com/victoralfred/goexec/releases/tag/v0.1.0
