# GoExec

A secure, hardened command execution library for Go that centralizes all process invocation behind a minimal API with strict security controls.

## Installation

```bash
go get github.com/victoralfred/goexec
```

## Quick Start

### Basic Execution

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/victoralfred/goexec"
)

func main() {
    // Create an executor with default settings
    exec, err := goexec.New()
    if err != nil {
        log.Fatal(err)
    }
    defer exec.Shutdown(context.Background())

    // Build a command (binary must be absolute path)
    cmd, err := goexec.Cmd("/usr/bin/ls", "-la", "/tmp").Build()
    if err != nil {
        log.Fatal(err)
    }

    // Execute the command
    result, err := exec.Execute(context.Background(), cmd)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Exit Code: %d\n", result.ExitCode)
    fmt.Printf("Stdout: %s\n", result.StdoutString())
}
```

### One-Off Execution

For simple one-off commands, use the convenience functions:

```go
result, err := goexec.Execute(context.Background(), "/usr/bin/whoami")
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.StdoutString())
```

### With Timeout

```go
ctx := context.Background()
result, err := goexec.ExecuteWithTimeout(ctx, 30*time.Second, "/usr/bin/sleep", "10")
```

## Command Builder

The `Cmd` function returns a `CommandBuilder` with a fluent API:

```go
cmd, err := goexec.Cmd("/usr/bin/git", "clone", repoURL).
    WithWorkingDir("/tmp/repos").
    WithTimeout(5 * time.Minute).
    WithEnv("GIT_SSH_COMMAND", "ssh -o StrictHostKeyChecking=no").
    WithPriority(goexec.PriorityHigh).
    Build()
```

### Available Builder Methods

| Method | Description |
|--------|-------------|
| `WithWorkingDir(dir string)` | Set working directory (must be absolute path) |
| `WithTimeout(d time.Duration)` | Set execution timeout |
| `WithEnv(key, value string)` | Add environment variable |
| `WithEnvMap(env map[string]string)` | Add multiple environment variables |
| `WithStdin(r io.Reader)` | Set stdin source |
| `WithPriority(p Priority)` | Set execution priority |
| `WithResourceLimits(l *ResourceLimits)` | Set resource constraints |
| `WithSandboxProfile(name string)` | Specify sandbox profile |
| `WithMetadata(key, value string)` | Add metadata for tracing |
| `Build()` | Build and validate the command |
| `MustBuild()` | Build or panic on error |

## Executor Configuration

Use `NewBuilder()` to configure the executor:

```go
exec, err := goexec.NewBuilder().
    WithDefaultTimeout(30 * time.Second).
    WithPolicy(policy).
    WithRateLimiter(limiter).
    WithCircuitBreaker(cb).
    WithTelemetry(telemetry).
    Build()
```

### Builder Options

| Method | Description |
|--------|-------------|
| `WithPolicy(p Policy)` | Set security policy for validation |
| `WithDefaultTimeout(d time.Duration)` | Default timeout (30s if not set) |
| `WithRateLimiter(r RateLimiter)` | Enable rate limiting |
| `WithCircuitBreaker(cb CircuitBreaker)` | Enable circuit breaker |
| `WithSandbox(s Sandbox)` | Enable sandboxing |
| `WithPool(p WorkerPool)` | Use bounded worker pool |
| `WithHooks(h ...Hook)` | Add pre/post execution hooks |
| `WithTelemetry(t Telemetry)` | Enable OpenTelemetry metrics |

## Execution Methods

The `Executor` interface provides multiple execution modes:

```go
// Synchronous execution
result, err := exec.Execute(ctx, cmd)

// Asynchronous execution with Future
future := exec.ExecuteAsync(ctx, cmd)
result, err := future.Wait()

// Batch execution (concurrent)
results, err := exec.ExecuteBatch(ctx, []*goexec.Command{cmd1, cmd2, cmd3})

// Streaming output
err := exec.Stream(ctx, cmd, os.Stdout, os.Stderr)

// Graceful shutdown
err := exec.Shutdown(ctx)
```

## Result Inspection

The `Result` struct provides execution details:

```go
result, _ := exec.Execute(ctx, cmd)

// Basic info
fmt.Println("Exit code:", result.ExitCode)
fmt.Println("Status:", result.Status)
fmt.Println("Duration:", result.Duration)

// Output
fmt.Println("Stdout:", result.StdoutString())
fmt.Println("Stderr:", result.StderrString())

// Status checks
if result.Success() {
    fmt.Println("Command succeeded")
}
if result.Status.IsRetryable() {
    fmt.Println("Can retry this operation")
}

// Resource usage (if available)
if result.ResourceUsage != nil {
    fmt.Println("CPU Time:", result.ResourceUsage.TotalCPUTime())
}
```

### Exit Status Values

| Status | Description |
|--------|-------------|
| `StatusSuccess` | Exit code 0 |
| `StatusError` | Non-zero exit code |
| `StatusTimeout` | Execution timeout exceeded |
| `StatusCanceled` | Context was canceled |
| `StatusKilled` | Process killed by signal |
| `StatusPolicyDenied` | Denied by security policy |
| `StatusRateLimited` | Rate limit exceeded |
| `StatusCircuitOpen` | Circuit breaker is open |

## Security Policy

Define allowed binaries and arguments via YAML:

```yaml
# policy.yaml
version: "1.0"
metadata:
  name: "production-policy"
  description: "Restricted execution policy"

global:
  default_sandbox: "restricted"
  default_limits:
    max_cpu_time: "30s"
    max_wall_time: "60s"
    max_memory: "256Mi"
  allowed_env:
    - PATH
    - HOME
    - USER
  denied_env:
    - "*_SECRET*"
    - "*_PASSWORD*"
    - "AWS_*"

binaries:
  - path: /usr/bin/git
    enabled: true
    allowed_args:
      - pattern: "^(status|log|diff|branch|show)$"
        position: 0
        description: "Read-only operations"
      - pattern: "^--.*$"
        description: "Long-form flags"
    denied_args:
      - pattern: "^--exec$"
        description: "No arbitrary execution"
    sandbox: "git-profile"

  - path: /usr/bin/ls
    enabled: true
    allowed_args:
      - pattern: "^-[alh1]+$"
        description: "Common flags"
      - pattern: "^/[a-zA-Z0-9_/.-]+$"
        description: "Absolute paths only"

audit:
  enabled: true
  log_level: "all"

circuit_breaker:
  enabled: true
  failure_threshold: 5
  success_threshold: 2
  timeout: "30s"
  per_binary: true
```

### Loading a Policy

```go
// Load from directory + filename
loader, err := goexec.LoadPolicy("/etc/goexec", "policy.yaml")
if err != nil {
    log.Fatal(err)
}

// Load the policy
policy, err := loader.Load(context.Background())
if err != nil {
    log.Fatal(err)
}

// Use with executor
exec, err := goexec.NewBuilder().
    WithPolicy(policy).
    Build()
```

### Hot Reload

```go
loader, _ := goexec.LoadPolicy("/etc/goexec", "policy.yaml")
policy, _ := loader.Load(ctx)

// Watch for changes (checks every 5 seconds)
loader.Watch(ctx, 5*time.Second)

// Get current policy (non-blocking)
currentPolicy := loader.Get()

// Stop watching
loader.StopWatch()
```

## Path Validation

Validate paths before use:

```go
// Validate a path
if err := goexec.ValidatePath("/usr/bin/ls"); err != nil {
    log.Fatal("Invalid path:", err)
}

// Sanitize and clean a path
cleanPath, err := goexec.SanitizePath(userInput)
if err != nil {
    log.Fatal("Path sanitization failed:", err)
}

// Validate arguments
if err := goexec.ValidateArguments(args); err != nil {
    log.Fatal("Invalid arguments:", err)
}
```

## Error Handling

```go
result, err := exec.Execute(ctx, cmd)
if err != nil {
    switch {
    case errors.Is(err, goexec.ErrInvalidPath):
        log.Println("Invalid binary or working directory path")
    case errors.Is(err, goexec.ErrPathTraversal):
        log.Println("Path traversal attempt detected")
    case errors.Is(err, goexec.ErrArgumentNotAllowed):
        log.Println("Argument denied by validation rules")
    case errors.Is(err, goexec.ErrExecutorShutdown):
        log.Println("Executor has been shut down")
    case errors.Is(err, goexec.ErrInvalidCommand):
        log.Println("Invalid command configuration")
    default:
        log.Println("Execution error:", err)
    }
}
```

## Streaming Output

Stream stdout/stderr in real-time:

```go
err := goexec.Stream(
    context.Background(),
    os.Stdout,  // stdout writer
    os.Stderr,  // stderr writer
    "/usr/bin/tail", "-f", "/var/log/syslog",
)
```

Or with a configured executor:

```go
cmd, _ := goexec.Cmd("/usr/bin/make", "build").
    WithWorkingDir("/path/to/project").
    Build()

err := exec.Stream(ctx, cmd, os.Stdout, os.Stderr)
```

## Package Structure

| Package | Description |
|---------|-------------|
| `goexec` | Main entry point with convenience functions |
| `executor` | Core `Executor` interface and implementation |
| `policy` | YAML policy loading and validation |
| `validation` | Path, argument, and environment validation |
| `sandbox` | Linux sandboxing (seccomp, AppArmor, cgroups) |
| `pool` | Bounded worker pool with priority scheduling |
| `resilience` | Rate limiting and circuit breaker |
| `observability` | OpenTelemetry metrics and audit logging |
| `hooks` | Extension points for custom behavior |
| `config` | Configuration management |

## Requirements

- Go 1.22 or later
- Linux (for sandbox features)
- Binaries must be specified with absolute paths

## Security Features

- **Binary Allowlisting**: Only explicitly allowed binaries can execute
- **Argument Validation**: Arguments validated against regex patterns
- **Path Sanitization**: Prevents path traversal and symlink attacks
- **Environment Filtering**: Controls which environment variables pass through
- **Resource Limits**: CPU, memory, process limits via cgroups and rlimits
- **Sandboxing**: seccomp-bpf and AppArmor integration (Linux)
- **Secure File I/O**: All file operations use `gowritter/safepath`

## License

MIT License
