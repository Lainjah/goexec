# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

Please report security vulnerabilities by opening a GitHub issue or contacting the maintainers directly.

## Known Standard Library Vulnerabilities

The following vulnerabilities exist in the Go standard library and will be resolved when Go 1.24.8+ becomes widely available. These are **not vulnerabilities in GoExec code** but in Go's standard library.

| ID | Package | Description | Fixed In | Impact |
|----|---------|-------------|----------|--------|
| GO-2025-4011 | encoding/asn1 | DER payload memory exhaustion | Go 1.24.8 | Low - only affects ASN.1 parsing |
| GO-2025-4010 | net/url | IPv6 hostname validation | Go 1.24.8 | Low - URL parsing edge case |
| GO-2025-4009 | encoding/pem | Quadratic complexity parsing | Go 1.24.8 | Low - PEM parsing edge case |
| GO-2025-4007 | crypto/x509 | Name constraints complexity | Go 1.24.9 | Low - X.509 cert parsing |

### Mitigation

These vulnerabilities are in transitive dependencies of `os/exec.CommandContext()` and do not directly affect GoExec's core functionality:

1. **GoExec does not parse ASN.1, PEM, or X.509 data** - The vulnerable code paths are only reachable through edge cases in the Go runtime
2. **GoExec validates all inputs** - Binary paths, arguments, and environment variables are validated before execution
3. **Upgrade Go when available** - These will be automatically fixed when Go 1.24.8+ is released and adopted

### Recommendations

- Use Go 1.23.12 or later (current minimum requirement)
- Upgrade to Go 1.24.8+ when it becomes stable
- Keep GoExec updated to the latest version

## Security Features

GoExec implements multiple security layers:

- **Binary allowlisting** - Only explicitly allowed binaries can execute
- **Argument validation** - All arguments validated against configurable patterns
- **Path sanitization** - Prevents path traversal and symlink attacks
- **Environment filtering** - Controls which environment variables pass through
- **Resource limits** - CPU, memory, and process limits via cgroups/rlimits
- **Sandboxing** - seccomp-bpf and AppArmor integration (Linux)
- **Safe file I/O** - All file operations use `gowritter/safepath`
