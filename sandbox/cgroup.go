package sandbox

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/victoralfred/gowritter/safepath"
)

// CgroupManager manages cgroup v1/v2 resource limits.
type CgroupManager struct {
	version   int
	basePath  string
	namespace string
	fs        *safepath.SafePath
}

// CgroupLimits defines resource limits for a cgroup.
type CgroupLimits struct {
	// MemoryLimitBytes is the memory limit in bytes.
	MemoryLimitBytes int64

	// CPUQuotaUS is the CPU quota in microseconds per period.
	CPUQuotaUS int64

	// CPUPeriodUS is the CPU period in microseconds.
	CPUPeriodUS int64

	// PidsMax is the maximum number of processes.
	PidsMax int64

	// IOReadBPS is the I/O read bandwidth limit.
	IOReadBPS int64

	// IOWriteBPS is the I/O write bandwidth limit.
	IOWriteBPS int64
}

// Cgroup represents a cgroup instance.
type Cgroup struct {
	path    string
	relPath string
	version int
	fs      *safepath.SafePath
}

// NewCgroupManager creates a new cgroup manager.
func NewCgroupManager(namespace string) (*CgroupManager, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("cgroups are only supported on Linux")
	}

	version, basePath := detectCgroupVersion()
	if version == 0 {
		return nil, fmt.Errorf("no cgroup support detected")
	}

	fs, err := safepath.New(basePath)
	if err != nil {
		return nil, fmt.Errorf("creating safepath for cgroup: %w", err)
	}

	return &CgroupManager{
		version:   version,
		basePath:  basePath,
		namespace: namespace,
		fs:        fs,
	}, nil
}

// Version returns the cgroup version.
func (m *CgroupManager) Version() int {
	return m.version
}

// Create creates a new cgroup with the given limits.
func (m *CgroupManager) Create(name string, limits *CgroupLimits) (*Cgroup, error) {
	// Sanitize name
	safeName := sanitizeCgroupName(name)
	relPath := filepath.Join(m.namespace, safeName)
	fullPath := filepath.Join(m.basePath, relPath)

	if err := m.fs.Mkdir(relPath, 0755); err != nil {
		return nil, fmt.Errorf("creating cgroup directory: %w", err)
	}

	cg := &Cgroup{
		path:    fullPath,
		relPath: relPath,
		version: m.version,
		fs:      m.fs,
	}

	if limits != nil {
		if err := cg.ApplyLimits(limits); err != nil {
			if destroyErr := cg.Destroy(); destroyErr != nil {
				return nil, fmt.Errorf("warning: failed to clean up cgroup after limit application failure: %v", destroyErr)
			}
			return nil, err
		}
	}

	return cg, nil
}

// Path returns the cgroup path.
func (c *Cgroup) Path() string {
	return c.path
}

// ApplyLimits applies resource limits to the cgroup.
func (c *Cgroup) ApplyLimits(limits *CgroupLimits) error {
	if c.version == 2 {
		return c.applyLimitsV2(limits)
	}
	return c.applyLimitsV1(limits)
}

func (c *Cgroup) applyLimitsV2(limits *CgroupLimits) error {
	// Memory limit
	if limits.MemoryLimitBytes > 0 {
		if err := c.writeFile("memory.max", strconv.FormatInt(limits.MemoryLimitBytes, 10)); err != nil {
			return fmt.Errorf("setting memory limit: %w", err)
		}
	}

	// CPU quota
	if limits.CPUQuotaUS > 0 {
		period := limits.CPUPeriodUS
		if period == 0 {
			period = 100000 // 100ms default
		}
		content := fmt.Sprintf("%d %d", limits.CPUQuotaUS, period)
		if err := c.writeFile("cpu.max", content); err != nil {
			return fmt.Errorf("setting CPU limit: %w", err)
		}
	}

	// PIDs limit
	if limits.PidsMax > 0 {
		if err := c.writeFile("pids.max", strconv.FormatInt(limits.PidsMax, 10)); err != nil {
			return fmt.Errorf("setting PIDs limit: %w", err)
		}
	}

	return nil
}

func (c *Cgroup) applyLimitsV1(limits *CgroupLimits) error {
	// Memory limit (cgroup v1 uses memory.limit_in_bytes)
	if limits.MemoryLimitBytes > 0 {
		memRelPath := filepath.Join(filepath.Dir(c.relPath), "memory", filepath.Base(c.relPath))
		if err := c.fs.Mkdir(memRelPath, 0755); err == nil {
			_ = c.fs.WriteFile(filepath.Join(memRelPath, "memory.limit_in_bytes"),
				[]byte(strconv.FormatInt(limits.MemoryLimitBytes, 10)), 0644)
		}
	}

	// CPU quota (cgroup v1 uses cpu.cfs_quota_us and cpu.cfs_period_us)
	if limits.CPUQuotaUS > 0 {
		cpuRelPath := filepath.Join(filepath.Dir(c.relPath), "cpu", filepath.Base(c.relPath))
		if err := c.fs.Mkdir(cpuRelPath, 0755); err == nil {
			period := limits.CPUPeriodUS
			if period == 0 {
				period = 100000
			}
			_ = c.fs.WriteFile(filepath.Join(cpuRelPath, "cpu.cfs_period_us"),
				[]byte(strconv.FormatInt(period, 10)), 0644)
			_ = c.fs.WriteFile(filepath.Join(cpuRelPath, "cpu.cfs_quota_us"),
				[]byte(strconv.FormatInt(limits.CPUQuotaUS, 10)), 0644)
		}
	}

	return nil
}

// AddProcess adds a process to the cgroup.
func (c *Cgroup) AddProcess(pid int) error {
	return c.writeFile("cgroup.procs", strconv.Itoa(pid))
}

// GetStats returns current resource usage statistics.
func (c *Cgroup) GetStats() (*CgroupStats, error) {
	stats := &CgroupStats{}

	if c.version == 2 {
		// Read memory.current
		if data, err := c.readFile("memory.current"); err == nil {
			stats.MemoryUsageBytes, _ = strconv.ParseInt(strings.TrimSpace(data), 10, 64)
		}

		// Read cpu.stat
		if data, err := c.readFile("cpu.stat"); err == nil {
			stats.CPUUsageUS = parseCPUStat(data)
		}

		// Read pids.current
		if data, err := c.readFile("pids.current"); err == nil {
			stats.PidsCount, _ = strconv.ParseInt(strings.TrimSpace(data), 10, 64)
		}
	}

	return stats, nil
}

// Destroy removes the cgroup.
func (c *Cgroup) Destroy() error {
	return c.fs.Remove(c.relPath)
}

func (c *Cgroup) writeFile(name, content string) error {
	return c.fs.WriteFile(filepath.Join(c.relPath, name), []byte(content), 0644)
}

func (c *Cgroup) readFile(name string) (string, error) {
	data, err := c.fs.ReadFile(filepath.Join(c.relPath, name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CgroupStats contains cgroup statistics.
type CgroupStats struct {
	MemoryUsageBytes int64
	CPUUsageUS       int64
	PidsCount        int64
}

// detectCgroupVersion detects the cgroup version and base path.
func detectCgroupVersion() (int, string) {
	// Use safepath to check for cgroup filesystems
	sysFS, err := safepath.New("/sys/fs/cgroup")
	if err != nil {
		return 0, ""
	}

	// Check for cgroup v2 (unified hierarchy)
	if exists, _ := sysFS.Exists("cgroup.controllers"); exists {
		return 2, "/sys/fs/cgroup"
	}

	// Check for cgroup v1
	if exists, _ := sysFS.Exists("cpu"); exists {
		return 1, "/sys/fs/cgroup/cpu"
	}

	return 0, ""
}

// sanitizeCgroupName removes unsafe characters from cgroup name.
func sanitizeCgroupName(name string) string {
	// Replace / with _
	name = strings.ReplaceAll(name, "/", "_")
	// Remove leading dots
	name = strings.TrimLeft(name, ".")
	// Keep only alphanumeric and underscore
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// parseCPUStat parses cgroup v2 cpu.stat.
func parseCPUStat(data string) int64 {
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "usage_usec ") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				val, _ := strconv.ParseInt(parts[1], 10, 64)
				return val
			}
		}
	}
	return 0
}
