package sandbox

import (
	"runtime"
	"syscall"
)

// Rlimit resource types.
const (
	RlimitCPU     = syscall.RLIMIT_CPU    // CPU time in seconds
	RlimitFSize   = syscall.RLIMIT_FSIZE  // Maximum file size
	RlimitData    = syscall.RLIMIT_DATA   // Maximum data segment size
	RlimitStack   = syscall.RLIMIT_STACK  // Maximum stack size
	RlimitCore    = syscall.RLIMIT_CORE   // Maximum core file size
	RlimitNOFile  = syscall.RLIMIT_NOFILE // Maximum open files
	RlimitAS      = syscall.RLIMIT_AS     // Maximum address space
	RlimitNProc   = 6                     // Maximum processes (RLIMIT_NPROC on Linux)
	RlimitMemlock = 8                     // Maximum locked memory (RLIMIT_MEMLOCK on Linux)
)

// Rlimit represents a resource limit.
type Rlimit struct {
	// Resource is the resource type.
	Resource int

	// Soft is the soft limit.
	Soft uint64

	// Hard is the hard limit.
	Hard uint64
}

// SetRlimit sets a resource limit.
func SetRlimit(resource int, soft, hard uint64) error {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return nil
	}

	rlim := syscall.Rlimit{
		Cur: soft,
		Max: hard,
	}

	return syscall.Setrlimit(resource, &rlim)
}

// GetRlimit gets a resource limit.
func GetRlimit(resource int) (soft, hard uint64, err error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return 0, 0, nil
	}

	var rlim syscall.Rlimit
	err = syscall.Getrlimit(resource, &rlim)
	return rlim.Cur, rlim.Max, err
}

// RlimitUnlimited represents unlimited resource.
const RlimitUnlimited = ^uint64(0)

// DefaultRlimits returns default resource limits.
func DefaultRlimits() []Rlimit {
	return []Rlimit{
		{Resource: RlimitCPU, Soft: 60, Hard: 120},
		{Resource: RlimitFSize, Soft: 100 * 1024 * 1024, Hard: 100 * 1024 * 1024},
		{Resource: RlimitNOFile, Soft: 256, Hard: 256},
		{Resource: RlimitNProc, Soft: 32, Hard: 32},
		{Resource: RlimitAS, Soft: 512 * 1024 * 1024, Hard: 1024 * 1024 * 1024},
	}
}

// RestrictiveRlimits returns restrictive resource limits.
func RestrictiveRlimits() []Rlimit {
	return []Rlimit{
		{Resource: RlimitCPU, Soft: 10, Hard: 30},
		{Resource: RlimitFSize, Soft: 10 * 1024 * 1024, Hard: 10 * 1024 * 1024},
		{Resource: RlimitNOFile, Soft: 64, Hard: 64},
		{Resource: RlimitNProc, Soft: 10, Hard: 10},
		{Resource: RlimitAS, Soft: 256 * 1024 * 1024, Hard: 256 * 1024 * 1024},
		{Resource: RlimitCore, Soft: 0, Hard: 0}, // Disable core dumps
	}
}

// ApplyRlimits applies multiple resource limits.
func ApplyRlimits(limits []Rlimit) error {
	for _, rl := range limits {
		if err := SetRlimit(rl.Resource, rl.Soft, rl.Hard); err != nil {
			return err
		}
	}
	return nil
}
