package policy

import (
	"fmt"
	"time"
)

// Config represents the YAML policy structure.
type Config struct {
	SandboxProfiles map[string]SandboxConfig `yaml:"sandbox_profiles"`
	Metadata        Metadata                 `yaml:"metadata"`
	Version         string                   `yaml:"version"`
	Binaries        []BinaryConfig           `yaml:"binaries"`
	Alerts          AlertConfig              `yaml:"alerts"`
	Audit           AuditConfig              `yaml:"audit"`
	Global          GlobalConfig             `yaml:"global"`
	CircuitBreaker  CircuitBreakerConfig     `yaml:"circuit_breaker"`
}

// Metadata contains policy metadata.
type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Created     string `yaml:"created"`
	Updated     string `yaml:"updated"`
}

// GlobalConfig contains global settings.
type GlobalConfig struct {
	RateLimit      *RateLimitConfig `yaml:"rate_limit"`
	DefaultSandbox string           `yaml:"default_sandbox"`
	AllowedEnv     []string         `yaml:"allowed_env"`
	DeniedEnv      []string         `yaml:"denied_env"`
	DefaultLimits  ResourceConfig   `yaml:"default_limits"`
}

// ResourceConfig contains resource limit settings.
type ResourceConfig struct {
	MaxCPUTime   Duration `yaml:"max_cpu_time"`
	MaxWallTime  Duration `yaml:"max_wall_time"`
	MaxMemory    ByteSize `yaml:"max_memory"`
	MaxOutput    ByteSize `yaml:"max_output"`
	MaxProcesses int      `yaml:"max_processes"`
	MaxOpenFiles int      `yaml:"max_open_files"`
}

// BinaryConfig defines configuration for a binary.
type BinaryConfig struct {
	Limits          *ResourceConfig  `yaml:"limits"`
	RateLimit       *RateLimitConfig `yaml:"rate_limit"`
	Path            string           `yaml:"path"`
	Sandbox         string           `yaml:"sandbox"`
	AllowedArgs     []ArgPattern     `yaml:"allowed_args"`
	DeniedArgs      []ArgPattern     `yaml:"denied_args"`
	AllowedEnv      []string         `yaml:"allowed_env"`
	DeniedEnv       []string         `yaml:"denied_env"`
	AllowedWorkdirs []string         `yaml:"allowed_workdirs"`
	MaxInstances    int              `yaml:"max_instances"`
	Enabled         bool             `yaml:"enabled"`
	RequireAudit    bool             `yaml:"require_audit"`
}

// SandboxConfig defines a sandbox profile.
type SandboxConfig struct {
	Seccomp      *SeccompConfig      `yaml:"seccomp"`
	Capabilities *CapabilitiesConfig `yaml:"capabilities"`
	Cgroup       *CgroupConfig       `yaml:"cgroup"`
	Namespaces   *NamespaceConfig    `yaml:"namespaces"`
	Filesystem   *FilesystemConfig   `yaml:"filesystem"`
	Network      *NetworkConfig      `yaml:"network"`
	Inherit      string              `yaml:"inherit"`
	AppArmor     string              `yaml:"apparmor_profile"`
}

// SeccompConfig defines seccomp settings.
type SeccompConfig struct {
	DefaultAction     string   `yaml:"default_action"`
	AllowedSyscalls   []string `yaml:"allowed_syscalls"`
	DeniedSyscalls    []string `yaml:"denied_syscalls"`
	AdditionalAllowed []string `yaml:"additional_allowed"`
}

// CapabilitiesConfig defines capability settings.
type CapabilitiesConfig struct {
	Drop []string `yaml:"drop"`
	Add  []string `yaml:"add"`
}

// CgroupConfig defines cgroup settings.
type CgroupConfig struct {
	MemoryLimit ByteSize `yaml:"memory_limit"`
	CPUQuota    int64    `yaml:"cpu_quota"`
	PidsLimit   int64    `yaml:"pids_limit"`
}

// NamespaceConfig defines namespace settings.
type NamespaceConfig struct {
	EnableUser    bool `yaml:"enable_user"`
	EnableNetwork bool `yaml:"enable_network"`
	EnablePID     bool `yaml:"enable_pid"`
	EnableMount   bool `yaml:"enable_mount"`
	EnableIPC     bool `yaml:"enable_ipc"`
	EnableUTS     bool `yaml:"enable_uts"`
}

// FilesystemConfig defines filesystem restrictions.
type FilesystemConfig struct {
	ReadOnly []string `yaml:"read_only"`
	Writable []string `yaml:"writable"`
	Masked   []string `yaml:"masked"`
}

// NetworkConfig defines network restrictions.
type NetworkConfig struct {
	AllowedHosts  []string `yaml:"allowed_hosts"`
	AllowedPorts  []int    `yaml:"allowed_ports"`
	AllowOutbound bool     `yaml:"allow_outbound"`
}

// AuditConfig defines audit settings.
type AuditConfig struct {
	LogLevel        string             `yaml:"log_level"`
	Destinations    []AuditDestination `yaml:"destinations"`
	MaxOutputLogged ByteSize           `yaml:"max_output_logged"`
	Enabled         bool               `yaml:"enabled"`
	IncludeOutput   bool               `yaml:"include_output"`
}

// AuditDestination defines an audit log destination.
type AuditDestination struct {
	Type     string          `yaml:"type"`
	Path     string          `yaml:"path,omitempty"`
	Rotation *RotationConfig `yaml:"rotation,omitempty"`
	Facility string          `yaml:"facility,omitempty"`
	Tag      string          `yaml:"tag,omitempty"`
}

// RotationConfig defines log rotation settings.
type RotationConfig struct {
	MaxSize    ByteSize `yaml:"max_size"`
	MaxAge     Duration `yaml:"max_age"`
	MaxBackups int      `yaml:"max_backups"`
}

// CircuitBreakerConfig defines circuit breaker settings.
type CircuitBreakerConfig struct {
	FailureThreshold int      `yaml:"failure_threshold"`
	SuccessThreshold int      `yaml:"success_threshold"`
	Timeout          Duration `yaml:"timeout"`
	Enabled          bool     `yaml:"enabled"`
	PerBinary        bool     `yaml:"per_binary"`
}

// AlertConfig defines alert settings.
type AlertConfig struct {
	Channels []AlertChannel `yaml:"channels"`
	Enabled  bool           `yaml:"enabled"`
}

// AlertChannel defines an alert channel.
type AlertChannel struct {
	Type   string   `yaml:"type"`
	URL    string   `yaml:"url"`
	Events []string `yaml:"events"`
}

// Duration is a time.Duration that can be unmarshaled from YAML.
type Duration struct {
	time.Duration
}

// UnmarshalYAML unmarshals a duration from YAML.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	d.Duration = duration
	return nil
}

// MarshalYAML marshals a duration to YAML.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// ByteSize represents a size in bytes that can be unmarshaled from YAML.
type ByteSize struct {
	Bytes int64
}

// UnmarshalYAML unmarshals a byte size from YAML.
func (b *ByteSize) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		// Try as int64
		var n int64
		if err := unmarshal(&n); err != nil {
			return err
		}
		b.Bytes = n
		return nil
	}

	bytes, err := parseByteSize(s)
	if err != nil {
		return err
	}

	b.Bytes = bytes
	return nil
}

// parseByteSize parses a byte size string like "10Mi", "1Gi", etc.
func parseByteSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	// Find the numeric part
	var numStr string
	var suffix string
	for i, c := range s {
		if c < '0' || c > '9' {
			numStr = s[:i]
			suffix = s[i:]
			break
		}
	}
	if numStr == "" {
		numStr = s
	}

	var num int64
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		return 0, err
	}

	// Parse suffix
	multiplier := int64(1)
	switch suffix {
	case "", "B":
		multiplier = 1
	case "K", "KB":
		multiplier = 1000
	case "Ki", "KiB":
		multiplier = 1024
	case "M", "MB":
		multiplier = 1000 * 1000
	case "Mi", "MiB":
		multiplier = 1024 * 1024
	case "G", "GB":
		multiplier = 1000 * 1000 * 1000
	case "Gi", "GiB":
		multiplier = 1024 * 1024 * 1024
	case "T", "TB":
		multiplier = 1000 * 1000 * 1000 * 1000
	case "Ti", "TiB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return num * multiplier, nil
}

// MarshalYAML marshals a byte size to YAML.
func (b ByteSize) MarshalYAML() (interface{}, error) {
	if b.Bytes == 0 {
		return "0", nil
	}

	// Use binary suffixes
	units := []struct {
		suffix string
		size   int64
	}{
		{"Ti", 1024 * 1024 * 1024 * 1024},
		{"Gi", 1024 * 1024 * 1024},
		{"Mi", 1024 * 1024},
		{"Ki", 1024},
	}

	for _, u := range units {
		if b.Bytes >= u.size && b.Bytes%u.size == 0 {
			return fmt.Sprintf("%d%s", b.Bytes/u.size, u.suffix), nil
		}
	}

	return fmt.Sprintf("%d", b.Bytes), nil
}
