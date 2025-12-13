// Package goexec provides a secure, hardened command execution library.
//
// GoExec is a production-grade Go library that centralizes all process invocation
// behind a minimal API, banning direct os/exec usage elsewhere. It enforces strict
// security controls including binary allowlisting, argument validation, path sanitization,
// and sandbox integration.
//
// # Key Features
//
//   - Single execution abstraction with mandatory timeouts and cancellation
//   - Policy-as-code configuration via YAML for auditable security rules
//   - Linux-native sandboxing (seccomp, AppArmor, cgroups)
//   - Bounded worker pool with backpressure for scalability
//   - OpenTelemetry integration for metrics and tracing
//   - Rate limiting and circuit breaker for resilience
//
// # Basic Usage
//
//	exec, err := goexec.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer exec.Shutdown(context.Background())
//
//	cmd, _ := goexec.Cmd("/usr/bin/git", "status").Build()
//	result, err := exec.Execute(ctx, cmd)
//
// # With Security Policy
//
//	loader, _ := goexec.LoadPolicy("/etc/goexec", "policy.yaml")
//	policy, _ := loader.Load(ctx)
//
//	exec, _ := goexec.NewBuilder().
//	    WithPolicy(policy).
//	    WithDefaultTimeout(30 * time.Second).
//	    Build()
//
// # Security Model
//
// All binaries and arguments must be explicitly allowlisted in the policy
// configuration. Commands are validated against the policy before execution,
// and optionally run in a sandboxed environment with resource limits.
//
// # File I/O
//
// All file operations use github.com/victoralfred/gowritter/safepath
// for secure path handling. Direct use of os.ReadFile, os.WriteFile,
// os.Open, os.Create, or io/ioutil is prohibited within this library.
//
// # Package Structure
//
//   - goexec: Main entry point and convenience functions
//   - executor: Core Executor interface and implementation
//   - policy: YAML policy loading and validation
//   - validation: Input sanitization and validation
//   - sandbox: Linux sandboxing (seccomp, AppArmor, cgroups)
//   - pool: Bounded worker pool with backpressure
//   - resilience: Rate limiting and circuit breaker
//   - observability: OpenTelemetry metrics and audit logging
//   - hooks: Extension points for custom behavior
//   - config: Configuration management
package goexec
