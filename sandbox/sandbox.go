// Package sandbox provides process isolation and resource control.
package sandbox

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/victoralfred/goexec/executor"
)

// Sandbox provides process isolation and resource control.
type Sandbox interface {
	// Prepare sets up the sandbox for a command.
	Prepare(ctx context.Context, cmd *executor.Command) (*Config, error)

	// Apply applies sandbox restrictions to a process.
	Apply(ctx context.Context, config *Config) error

	// Cleanup releases sandbox resources.
	Cleanup(ctx context.Context, config *Config) error

	// Supported returns which sandbox features are available.
	Supported() Features
}

// Features indicates which sandbox features are available.
type Features struct {
	Seccomp       bool
	AppArmor      bool
	CgroupV1      bool
	CgroupV2      bool
	Namespaces    bool
	Capabilities  bool
	PrivilegeDrop bool
}

// Config contains the sandbox configuration for a process.
type Config struct {
	// SeccompProfile is the seccomp filter profile.
	SeccompProfile *SeccompProfile

	// AppArmorProfile is the AppArmor profile name.
	AppArmorProfile string

	// CgroupPath is the cgroup path.
	CgroupPath string

	// Rlimits are the resource limits.
	Rlimits []Rlimit

	// Namespaces is the namespace configuration.
	Namespaces *NamespaceConfig

	// UID is the user ID for privilege dropping.
	UID int

	// GID is the group ID for privilege dropping.
	GID int

	// DropCaps are capabilities to drop.
	DropCaps []string

	// ReadOnlyPaths are paths that should be read-only.
	ReadOnlyPaths []string

	// MaskedPaths are paths that should be hidden.
	MaskedPaths []string

	// TmpfsMounts are tmpfs mounts.
	TmpfsMounts []TmpfsMount

	// cgroup is the cgroup instance (if created).
	cgroup *Cgroup
}

// NamespaceConfig configures Linux namespaces.
type NamespaceConfig struct {
	User    bool
	Network bool
	PID     bool
	Mount   bool
	IPC     bool
	UTS     bool
}

// TmpfsMount defines a tmpfs mount.
type TmpfsMount struct {
	Path string
	Size int64
	Mode int
}

// defaultSandbox is the default sandbox implementation.
type defaultSandbox struct {
	profiles  map[string]*Profile
	cgroupMgr *CgroupManager
	mu        sync.RWMutex
}

// Profile defines a reusable sandbox profile.
type Profile struct {
	Name            string
	SeccompProfile  *SeccompProfile
	AppArmorProfile string
	Rlimits         []Rlimit
	Namespaces      *NamespaceConfig
	DropCaps        []string
	ReadOnlyPaths   []string
	MaskedPaths     []string
}

// Option configures the sandbox.
type Option func(*defaultSandbox)

// WithProfile adds a named profile.
func WithProfile(profile *Profile) Option {
	return func(s *defaultSandbox) {
		s.profiles[profile.Name] = profile
	}
}

// New creates a new sandbox.
func New(opts ...Option) (Sandbox, error) {
	if runtime.GOOS != "linux" {
		return &noopSandbox{}, nil
	}

	s := &defaultSandbox{
		profiles: make(map[string]*Profile),
	}

	// Initialize cgroup manager
	cgroupMgr, err := NewCgroupManager("goexec")
	if err != nil {
		// Continue without cgroup support
		_ = err
	} else {
		s.cgroupMgr = cgroupMgr
	}

	// Add default profiles
	s.profiles["minimal"] = minimalProfile()
	s.profiles["restricted"] = restrictedProfile()

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

// Prepare implements Sandbox.Prepare.
func (s *defaultSandbox) Prepare(ctx context.Context, cmd *executor.Command) (*Config, error) {
	config := &Config{
		Rlimits: make([]Rlimit, 0),
	}

	// Get profile if specified
	if cmd.SandboxProfile != "" {
		s.mu.RLock()
		profile, ok := s.profiles[cmd.SandboxProfile]
		s.mu.RUnlock()

		if !ok {
			return nil, fmt.Errorf("unknown sandbox profile: %s", cmd.SandboxProfile)
		}

		config.SeccompProfile = profile.SeccompProfile
		config.AppArmorProfile = profile.AppArmorProfile
		config.Namespaces = profile.Namespaces
		config.DropCaps = profile.DropCaps
		config.ReadOnlyPaths = profile.ReadOnlyPaths
		config.MaskedPaths = profile.MaskedPaths
		config.Rlimits = append(config.Rlimits, profile.Rlimits...)
	}

	// Apply resource limits from command
	if cmd.ResourceLimits != nil {
		config.Rlimits = append(config.Rlimits, resourceLimitsToRlimits(cmd.ResourceLimits)...)

		// Create cgroup if available
		if s.cgroupMgr != nil {
			cgroupLimits := &CgroupLimits{
				MemoryLimitBytes: cmd.ResourceLimits.MaxMemoryBytes,
				PidsMax:          int64(cmd.ResourceLimits.MaxProcesses),
			}

			cgroup, err := s.cgroupMgr.Create(cmd.Binary, cgroupLimits)
			if err != nil {
				return nil, fmt.Errorf("creating cgroup: %w", err)
			}
			config.cgroup = cgroup
			config.CgroupPath = cgroup.Path()
		}
	}

	return config, nil
}

// Apply implements Sandbox.Apply.
func (s *defaultSandbox) Apply(ctx context.Context, config *Config) error {
	// Apply rlimits
	for _, rl := range config.Rlimits {
		if err := SetRlimit(rl.Resource, rl.Soft, rl.Hard); err != nil {
			return fmt.Errorf("setting rlimit %d: %w", rl.Resource, err)
		}
	}

	// Apply AppArmor profile
	if config.AppArmorProfile != "" {
		if err := ApplyAppArmorProfile(config.AppArmorProfile); err != nil {
			// Log warning but continue
			_ = err
		}
	}

	// Apply seccomp filter
	if config.SeccompProfile != nil {
		filter, err := NewSeccompFilter(config.SeccompProfile)
		if err != nil {
			return fmt.Errorf("creating seccomp filter: %w", err)
		}
		if err := filter.Apply(); err != nil {
			return fmt.Errorf("applying seccomp filter: %w", err)
		}
	}

	return nil
}

// Cleanup implements Sandbox.Cleanup.
func (s *defaultSandbox) Cleanup(ctx context.Context, config *Config) error {
	if config.cgroup != nil {
		return config.cgroup.Destroy()
	}
	return nil
}

// Supported implements Sandbox.Supported.
func (s *defaultSandbox) Supported() Features {
	features := Features{}

	if runtime.GOOS == "linux" {
		features.Seccomp = isSeccompSupported()
		features.AppArmor = isAppArmorSupported()
		features.Namespaces = true
		features.Capabilities = true
		features.PrivilegeDrop = true

		if s.cgroupMgr != nil {
			if s.cgroupMgr.Version() == 2 {
				features.CgroupV2 = true
			} else {
				features.CgroupV1 = true
			}
		}
	}

	return features
}

// minimalProfile returns a minimal sandbox profile.
func minimalProfile() *Profile {
	return &Profile{
		Name: "minimal",
		SeccompProfile: &SeccompProfile{
			DefaultAction: SeccompDeny,
			Syscalls: []SyscallRule{
				{Names: []string{"read", "write", "close", "fstat", "mmap", "mprotect", "munmap", "brk", "exit_group"}, Action: SeccompAllow},
			},
		},
		DropCaps: []string{"ALL"},
		Rlimits: []Rlimit{
			{Resource: RlimitCPU, Soft: 30, Hard: 60},
			{Resource: RlimitFSize, Soft: 10 * 1024 * 1024, Hard: 10 * 1024 * 1024},
			{Resource: RlimitNOFile, Soft: 64, Hard: 64},
			{Resource: RlimitNProc, Soft: 10, Hard: 10},
		},
	}
}

// restrictedProfile returns a restricted sandbox profile.
func restrictedProfile() *Profile {
	return &Profile{
		Name: "restricted",
		SeccompProfile: &SeccompProfile{
			DefaultAction: SeccompLog,
			Syscalls: []SyscallRule{
				{Names: []string{"ptrace", "process_vm_readv", "process_vm_writev", "kexec_load", "init_module", "finit_module", "delete_module"}, Action: SeccompDeny},
			},
		},
		DropCaps: []string{"CAP_SYS_ADMIN", "CAP_NET_ADMIN", "CAP_SYS_PTRACE"},
		Rlimits: []Rlimit{
			{Resource: RlimitCPU, Soft: 120, Hard: 300},
			{Resource: RlimitFSize, Soft: 100 * 1024 * 1024, Hard: 100 * 1024 * 1024},
			{Resource: RlimitNOFile, Soft: 256, Hard: 256},
			{Resource: RlimitNProc, Soft: 32, Hard: 32},
		},
		MaskedPaths: []string{"/etc/shadow", "/etc/sudoers"},
	}
}

// resourceLimitsToRlimits converts executor resource limits to rlimits.
func resourceLimitsToRlimits(limits *executor.ResourceLimits) []Rlimit {
	var rlimits []Rlimit

	if limits.MaxCPUTime > 0 {
		seconds := uint64(limits.MaxCPUTime.Seconds())
		rlimits = append(rlimits, Rlimit{Resource: RlimitCPU, Soft: seconds, Hard: seconds * 2})
	}

	if limits.MaxMemoryBytes > 0 {
		rlimits = append(rlimits, Rlimit{Resource: RlimitAS, Soft: uint64(limits.MaxMemoryBytes), Hard: uint64(limits.MaxMemoryBytes)})
	}

	if limits.MaxFileSize > 0 {
		rlimits = append(rlimits, Rlimit{Resource: RlimitFSize, Soft: uint64(limits.MaxFileSize), Hard: uint64(limits.MaxFileSize)})
	}

	if limits.MaxOpenFiles > 0 {
		rlimits = append(rlimits, Rlimit{Resource: RlimitNOFile, Soft: uint64(limits.MaxOpenFiles), Hard: uint64(limits.MaxOpenFiles)})
	}

	if limits.MaxProcesses > 0 {
		rlimits = append(rlimits, Rlimit{Resource: RlimitNProc, Soft: uint64(limits.MaxProcesses), Hard: uint64(limits.MaxProcesses)})
	}

	return rlimits
}

// noopSandbox is a no-op sandbox for non-Linux systems.
type noopSandbox struct{}

func (s *noopSandbox) Prepare(ctx context.Context, cmd *executor.Command) (*Config, error) {
	return &Config{}, nil
}

func (s *noopSandbox) Apply(ctx context.Context, config *Config) error {
	return nil
}

func (s *noopSandbox) Cleanup(ctx context.Context, config *Config) error {
	return nil
}

func (s *noopSandbox) Supported() Features {
	return Features{}
}
